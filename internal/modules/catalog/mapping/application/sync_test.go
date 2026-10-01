package application

import (
	"context"
	"errors"
	mappingcontract "github.com/dujiao-next/internal/modules/catalog/mapping/contract"
	mappingdomain "github.com/dujiao-next/internal/modules/catalog/mapping/domain"
	productdomain "github.com/dujiao-next/internal/modules/catalog/product/domain"
	siteconnectiondomain "github.com/dujiao-next/internal/modules/siteconnection/domain"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"github.com/dujiao-next/internal/upstream"
	"testing"
	"time"

	settingsintegration "github.com/dujiao-next/internal/modules/settings/schema/integration"
)

type truncatedSyncAdapter struct{ upstream.Adapter }

func (truncatedSyncAdapter) ListProducts(context.Context, upstream.ListProductsOpts) (*upstream.ProductListResult, error) {
	return &upstream.ProductListResult{Total: 2, IncludesInactive: true}, nil
}

type syncConnections struct{ adapter upstream.Adapter }

func (s syncConnections) GetByID(uint) (*siteconnectiondomain.Connection, error) {
	return &siteconnectiondomain.Connection{ID: 1}, nil
}
func (s syncConnections) GetAdapter(*siteconnectiondomain.Connection) (upstream.Adapter, error) {
	return s.adapter, nil
}
func TestTruncatedSyncDoesNotReportSuccess(t *testing.T) {
	svc := &Service{connections: syncConnections{adapter: truncatedSyncAdapter{}}}
	if err := svc.SyncConnectionStock(1, nil, 100, 2); err == nil {
		t.Fatal("truncated fetch advanced success checkpoint")
	}
}

type failingSyncProducts struct {
	mappingcontract.ProductRepository
}

func (failingSyncProducts) GetByID(string) (*productdomain.Product, error) {
	return &productdomain.Product{ID: 1}, nil
}
func (failingSyncProducts) Update(*productdomain.Product) error {
	return errors.New("injected product write")
}

type syncMaps struct {
	mappingcontract.MappingRepository
	saved bool
}

func (m *syncMaps) GetByID(uint) (*mappingdomain.Mapping, error) {
	return &mappingdomain.Mapping{ID: 1, LocalProductID: 1, ConnectionID: 1, UpstreamProductID: 1}, nil
}
func (m *syncMaps) Update(*mappingdomain.Mapping) error { m.saved = true; return nil }

type emptySyncSKUs struct {
	mappingcontract.SKUMappingRepository
}

func (emptySyncSKUs) ListByProductMapping(uint) ([]mappingdomain.SKUMapping, error) { return nil, nil }

type activeSyncAdapter struct{ upstream.Adapter }

func (activeSyncAdapter) GetProduct(context.Context, uint) (*upstream.UpstreamProduct, error) {
	return &upstream.UpstreamProduct{ID: 1, IsActive: true, ManualFormSchema: jsonmap.JSON{"fields": []interface{}{}}}, nil
}
func (activeSyncAdapter) ListProducts(context.Context, upstream.ListProductsOpts) (*upstream.ProductListResult, error) {
	p, _ := activeSyncAdapter{}.GetProduct(context.Background(), 1)
	return &upstream.ProductListResult{Total: 1, Items: []upstream.UpstreamProduct{*p}}, nil
}
func TestSyncWriteFailureCannotAdvanceMapping(t *testing.T) {
	for _, batch := range []bool{false, true} {
		t.Run(map[bool]string{false: "single", true: "batch"}[batch], func(t *testing.T) {
			m := &syncMaps{}
			svc := &Service{connections: syncConnections{adapter: activeSyncAdapter{}}, products: failingSyncProducts{}, mappings: m, skuMappings: emptySyncSKUs{}}
			var err error
			if batch {
				err = svc.SyncConnectionStock(1, []mappingdomain.Mapping{{ID: 1, LocalProductID: 1, UpstreamProductID: 1}}, 100, 2)
			} else {
				err = svc.SyncProduct(1)
			}
			if err == nil || m.saved {
				t.Fatal("failed product write reported successful mapping sync")
			}
		})
	}
}

// fakeSettingsProvider 以固定同步间隔实现 SettingsProvider。
type fakeSettingsProvider struct {
	interval time.Duration
}

func (f fakeSettingsProvider) GetUpstreamSyncConfig(fallbackInterval string) (settingsintegration.UpstreamSyncConfig, error) {
	cfg := settingsintegration.DefaultUpstreamSyncConfig()
	cfg.IntervalMinutes = int(f.interval / time.Minute)
	return cfg, nil
}

func (f fakeSettingsProvider) GetUpstreamSyncInterval(fallbackInterval string) (time.Duration, error) {
	return f.interval, nil
}

func TestComputeFullSyncIntervalFloorsAt24h(t *testing.T) {
	svc := &Service{settings: fakeSettingsProvider{interval: 5 * time.Minute}}

	// 5m 同步间隔 × 3 = 15m < 24h，期望落到 24h floor
	got := svc.computeFullSyncInterval()
	if got != fullSyncIntervalFloor {
		t.Fatalf("expected floor=24h, got %v", got)
	}
}

func TestComputeFullSyncIntervalScalesWithLongInterval(t *testing.T) {
	svc := &Service{settings: fakeSettingsProvider{interval: 12 * time.Hour}}

	// 12h * 3 = 36h，应使用 scaled 值
	got := svc.computeFullSyncInterval()
	want := 36 * time.Hour
	if got != want {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestComputeFullSyncIntervalWithoutSettings(t *testing.T) {
	svc := &Service{}
	got := svc.computeFullSyncInterval()
	if got != fullSyncIntervalFloor {
		t.Fatalf("expected floor when settings=nil, got %v", got)
	}
}
