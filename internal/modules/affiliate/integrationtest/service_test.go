package integrationtest

import (
	"fmt"
	admindomain "github.com/dujiao-next/internal/modules/identity/admin/domain"
	orderdomain "github.com/dujiao-next/internal/modules/order/domain"
	"github.com/dujiao-next/internal/shared/money"
	"github.com/shopspring/decimal"
	"testing"
	"time"

	affiliateapp "github.com/dujiao-next/internal/modules/affiliate/application"
	affiliatecontract "github.com/dujiao-next/internal/modules/affiliate/contract"
	affiliatedomain "github.com/dujiao-next/internal/modules/affiliate/domain"
	affiliategormstore "github.com/dujiao-next/internal/modules/affiliate/infrastructure/gormstore"

	userstore "github.com/dujiao-next/internal/modules/identity/user/infrastructure/gormstore"

	userdomain "github.com/dujiao-next/internal/modules/identity/user/domain"

	"github.com/dujiao-next/internal/testkit/memorysettings"

	settingsapp "github.com/dujiao-next/internal/modules/settings/application"
	settingsintegration "github.com/dujiao-next/internal/modules/settings/schema/integration"

	"github.com/dujiao-next/internal/constants"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestResolveOrderAffiliateSnapshotPreferLatestVisitorClick(t *testing.T) {
	svc, db, _ := setupAffiliateServiceTest(t)

	promoterA := createAffiliateTestUser(t, db, "affiliate-a@example.com")
	promoterB := createAffiliateTestUser(t, db, "affiliate-b@example.com")
	profileA := createAffiliateTestProfile(t, db, promoterA.ID, "AFFA0001", constants.AffiliateProfileStatusActive)
	profileB := createAffiliateTestProfile(t, db, promoterB.ID, "AFFB0002", constants.AffiliateProfileStatusActive)

	visitorKey := "visitor-key-priority"
	now := time.Now()
	createAffiliateTestClick(t, db, profileA.ID, visitorKey, now.Add(-2*time.Hour))
	createAffiliateTestClick(t, db, profileB.ID, visitorKey, now.Add(-1*time.Hour))

	profileID, code, err := svc.ResolveOrderAffiliateSnapshot(0, profileA.AffiliateCode, visitorKey)
	if err != nil {
		t.Fatalf("resolve snapshot failed: %v", err)
	}
	if profileID == nil || *profileID != profileB.ID {
		t.Fatalf("expected latest clicked profile %d, got %+v", profileB.ID, profileID)
	}
	if code != profileB.AffiliateCode {
		t.Fatalf("expected latest clicked code %s, got %s", profileB.AffiliateCode, code)
	}
}

func TestResolveOrderAffiliateSnapshotFallbackToCodeWhenNoVisitorClick(t *testing.T) {
	svc, db, _ := setupAffiliateServiceTest(t)

	promoter := createAffiliateTestUser(t, db, "affiliate-fallback@example.com")
	profile := createAffiliateTestProfile(t, db, promoter.ID, "AFFF0003", constants.AffiliateProfileStatusActive)

	profileID, code, err := svc.ResolveOrderAffiliateSnapshot(0, profile.AffiliateCode, "visitor-key-not-found")
	if err != nil {
		t.Fatalf("resolve snapshot failed: %v", err)
	}
	if profileID == nil || *profileID != profile.ID {
		t.Fatalf("expected fallback profile %d, got %+v", profile.ID, profileID)
	}
	if code != profile.AffiliateCode {
		t.Fatalf("expected fallback code %s, got %s", profile.AffiliateCode, code)
	}
}

func TestResolveOrderAffiliateSnapshotRejectSelfByVisitorClick(t *testing.T) {
	svc, db, _ := setupAffiliateServiceTest(t)

	promoter := createAffiliateTestUser(t, db, "affiliate-self@example.com")
	profile := createAffiliateTestProfile(t, db, promoter.ID, "AFFS0004", constants.AffiliateProfileStatusActive)
	createAffiliateTestClick(t, db, profile.ID, "visitor-key-self", time.Now().Add(-10*time.Minute))

	profileID, code, err := svc.ResolveOrderAffiliateSnapshot(promoter.ID, "AFFF9999", "visitor-key-self")
	if err != nil {
		t.Fatalf("resolve snapshot failed: %v", err)
	}
	if profileID != nil || code != "" {
		t.Fatalf("expected self-order attribution ignored, got profile=%+v code=%q", profileID, code)
	}
}

func TestUpdateAffiliateProfileStatus(t *testing.T) {
	svc, db, _ := setupAffiliateServiceTest(t)

	user := createAffiliateTestUser(t, db, "affiliate-status@example.com")
	profile := createAffiliateTestProfile(t, db, user.ID, "AFFST001", constants.AffiliateProfileStatusActive)

	disabled, err := svc.UpdateAffiliateProfileStatus(profile.ID, constants.AffiliateProfileStatusDisabled)
	if err != nil {
		t.Fatalf("disable profile failed: %v", err)
	}
	if disabled == nil || disabled.Status != constants.AffiliateProfileStatusDisabled {
		t.Fatalf("expected disabled status, got %+v", disabled)
	}

	enabled, err := svc.UpdateAffiliateProfileStatus(profile.ID, constants.AffiliateProfileStatusActive)
	if err != nil {
		t.Fatalf("enable profile failed: %v", err)
	}
	if enabled == nil || enabled.Status != constants.AffiliateProfileStatusActive {
		t.Fatalf("expected active status, got %+v", enabled)
	}
}

func TestBatchUpdateAffiliateProfileStatus(t *testing.T) {
	svc, db, store := setupAffiliateServiceTest(t)

	userA := createAffiliateTestUser(t, db, "affiliate-batch-a@example.com")
	userB := createAffiliateTestUser(t, db, "affiliate-batch-b@example.com")
	profileA := createAffiliateTestProfile(t, db, userA.ID, "AFFBT001", constants.AffiliateProfileStatusActive)
	profileB := createAffiliateTestProfile(t, db, userB.ID, "AFFBT002", constants.AffiliateProfileStatusActive)

	updated, err := svc.BatchUpdateAffiliateProfileStatus([]uint{profileA.ID, profileB.ID}, constants.AffiliateProfileStatusDisabled)
	if err != nil {
		t.Fatalf("batch disable failed: %v", err)
	}
	if updated != 2 {
		t.Fatalf("expected updated 2, got %d", updated)
	}

	reloadedA, err := store.GetProfileByID(profileA.ID)
	if err != nil || reloadedA == nil {
		t.Fatalf("reload profileA failed: %v", err)
	}
	reloadedB, err := store.GetProfileByID(profileB.ID)
	if err != nil || reloadedB == nil {
		t.Fatalf("reload profileB failed: %v", err)
	}
	if reloadedA.Status != constants.AffiliateProfileStatusDisabled || reloadedB.Status != constants.AffiliateProfileStatusDisabled {
		t.Fatalf("unexpected statuses after batch disable: %s, %s", reloadedA.Status, reloadedB.Status)
	}
}

func setupAffiliateServiceTest(t *testing.T) (*affiliateapp.Service, *gorm.DB, affiliatecontract.Store) {
	t.Helper()

	dsn := fmt.Sprintf("file:affiliate_service_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(&userdomain.User{}, &affiliatedomain.Profile{}, &affiliatedomain.Click{}); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}

	settingRepo := memorysettings.New()
	settingSvc := settingsapp.NewService(settingRepo)
	if _, err := settingSvc.UpdateAffiliateSetting(settingsintegration.AffiliateSetting{
		Enabled:        true,
		CommissionRate: 20,
	}); err != nil {
		t.Fatalf("init affiliate setting failed: %v", err)
	}

	affiliateRepo := affiliategormstore.New(db)
	return affiliateapp.NewService(affiliateRepo, userstore.New(db), nil, nil, settingSvc), db, affiliateRepo
}

func createAffiliateTestUser(t *testing.T, db *gorm.DB, email string) userdomain.User {
	t.Helper()

	row := userdomain.User{
		Email:        email,
		PasswordHash: "hash",
		DisplayName:  "tester",
		Status:       constants.UserStatusActive,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	return row
}

func createAffiliateTestProfile(t *testing.T, db *gorm.DB, userID uint, code, status string) affiliatedomain.Profile {
	t.Helper()

	row := affiliatedomain.Profile{
		UserID:        userID,
		AffiliateCode: code,
		Status:        status,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("create affiliate profile failed: %v", err)
	}
	return row
}

func createAffiliateTestClick(t *testing.T, db *gorm.DB, profileID uint, visitorKey string, createdAt time.Time) {
	t.Helper()

	row := affiliatedomain.Click{
		AffiliateProfileID: profileID,
		VisitorKey:         visitorKey,
		LandingPath:        "/",
		CreatedAt:          createdAt,
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("create affiliate click failed: %v", err)
	}
}

// FIN-HISTORY-02: unknown revenue is not verified retained entitlement.
func TestFinanceMissingBackingOrderFailsClosed(t *testing.T) {
	for _, history := range []int64{0, 40, 100} {
		t.Run(fmt.Sprintf("surviving_refund=%d", history), func(t *testing.T) {
			svc, db, repo := setupAffiliateServiceTest(t)
			if err := db.AutoMigrate(&admindomain.Admin{}, &orderdomain.Order{}, &orderdomain.OrderRefundRecord{}, &affiliatedomain.Commission{}, &affiliatedomain.WithdrawRequest{}); err != nil {
				t.Fatal(err)
			}
			user := createAffiliateTestUser(t, db, "missing-source@example.test")
			profile := createAffiliateTestProfile(t, db, user.ID, "MISSING1", constants.AffiliateProfileStatusActive)
			d := decimal.NewFromInt
			// Process a valid refunded commission first to exercise transaction rollback
			// when the later commission's backing order cannot be reconciled.
			root := orderdomain.Order{OrderNo: "VALID-BACKING", TotalAmount: money.FromDecimal(d(100))}
			if err := db.Create(&root).Error; err != nil {
				t.Fatal(err)
			}
			rows := []affiliatedomain.Commission{
				{AffiliateProfileID: profile.ID, OrderID: root.ID, CommissionType: constants.AffiliateCommissionTypeOrder, BaseAmount: money.FromDecimal(d(100)), CommissionAmount: money.FromDecimal(d(20)), Status: constants.AffiliateCommissionStatusAvailable},
				{AffiliateProfileID: profile.ID, OrderID: 999, CommissionType: constants.AffiliateCommissionTypeOrder, BaseAmount: money.FromDecimal(d(100)), CommissionAmount: money.FromDecimal(d(20)), Status: constants.AffiliateCommissionStatusAvailable},
			}
			for i := range rows {
				if err := repo.CreateCommission(&rows[i]); err != nil {
					t.Fatal(err)
				}
			}
			req, err := svc.ApplyWithdraw(user.ID, affiliateapp.WithdrawApplyInput{Amount: d(40), Channel: "bank", Account: "fixture"})
			if err != nil {
				t.Fatal(err)
			}
			binding := time.Now().Add(-time.Hour)
			if err := db.Model(req).Update("created_at", binding).Error; err != nil {
				t.Fatal(err)
			}
			for i, id := range []uint{root.ID, 999} {
				amount := int64(100)
				if i == 1 {
					amount = history
				}
				if amount > 0 {
					if err := db.Create(&orderdomain.OrderRefundRecord{OrderID: id, Amount: money.FromDecimal(d(amount)), CreatedAt: binding.Add(time.Minute)}).Error; err != nil {
						t.Fatal(err)
					}
				}
			}
			for range 2 {
				_, err = svc.ReviewWithdraw(1, req.ID, constants.AffiliateWithdrawActionReject, "fixture")
				if err == nil {
					t.Fatal("missing backing order released commission")
				}
				for _, row := range rows {
					var got affiliatedomain.Commission
					if err := db.First(&got, row.ID).Error; err != nil {
						t.Fatal(err)
					}
					if got.WithdrawRequestID == nil || *got.WithdrawRequestID != req.ID || !got.CommissionAmount.Decimal.Equal(d(20)) || !got.BaseAmount.Decimal.Equal(d(100)) || got.Status != constants.AffiliateCommissionStatusAvailable || got.InvalidReason != "" {
						t.Fatalf("failed rejection changed commission: %+v", got)
					}
				}
				var pending affiliatedomain.WithdrawRequest
				if err := db.First(&pending, req.ID).Error; err != nil {
					t.Fatal(err)
				}
				if pending.Status != constants.AffiliateWithdrawStatusPendingReview || pending.ProcessedAt != nil || pending.ProcessedBy != nil || pending.RejectReason != "" {
					t.Fatalf("failed rejection changed withdrawal: %+v", pending)
				}
				available, err := repo.SumCommissionByProfile(profile.ID, []string{constants.AffiliateCommissionStatusAvailable}, true)
				if err != nil {
					t.Fatal(err)
				}
				if !available.IsZero() {
					t.Fatalf("released %s without backing revenue", available)
				}
				if _, err := svc.ApplyWithdraw(user.ID, affiliateapp.WithdrawApplyInput{Amount: d(1), Channel: "bank", Account: "fixture"}); err == nil {
					t.Fatal("unreconciled commission reapplied")
				}
			}
			// Restoring the missing source permits normal reconciliation, once.
			missing := orderdomain.Order{ID: 999, OrderNo: "RESTORED-BACKING", TotalAmount: money.FromDecimal(d(100)), RefundedAmount: money.FromDecimal(d(history))}
			if err := db.Create(&missing).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := svc.ReviewWithdraw(1, req.ID, constants.AffiliateWithdrawActionReject, "reconciled"); err != nil {
				t.Fatal(err)
			}
			available, err := repo.SumCommissionByProfile(profile.ID, []string{constants.AffiliateCommissionStatusAvailable}, true)
			if err != nil {
				t.Fatal(err)
			}
			want := d(100 - history).Div(d(5))
			if !available.Equal(want) {
				t.Fatalf("retained=%s want=%s", available, want)
			}
			if want.IsPositive() {
				again, err := svc.ApplyWithdraw(user.ID, affiliateapp.WithdrawApplyInput{Amount: want, Channel: "bank", Account: "fixture"})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := svc.ReviewWithdraw(1, again.ID, constants.AffiliateWithdrawActionReject, "no new refunds"); err != nil {
					t.Fatal(err)
				}
				available, err = repo.SumCommissionByProfile(profile.ID, []string{constants.AffiliateCommissionStatusAvailable}, true)
				if err != nil || !available.Equal(want) {
					t.Fatalf("reapply double deduction: %s %v", available, err)
				}
			}
		})
	}
}

func TestRemediationHistoricalWithdrawalHistoryControls(t *testing.T) {
	for _, tc := range []struct {
		name                         string
		refund, legacy, want         int64
		marker, incomplete, archived bool
	}{
		{name: "before_binding_only", want: 16},
		{name: "legacy_partial", refund: 40, legacy: 40, want: 8},
		{name: "legacy_full", refund: 80, legacy: 80},
		{name: "marker_overlap", refund: 40, marker: true, want: 8},
		{name: "legacy_then_marker", refund: 40, legacy: 20, marker: true, want: 8},
		{name: "full_marker_overlap", refund: 80, marker: true},
		{name: "incomplete_history", refund: 40, legacy: 40, incomplete: true},
		{name: "archived_backing_and_history", refund: 40, legacy: 40, want: 8, archived: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, db, repo := setupAffiliateServiceTest(t)
			if err := db.AutoMigrate(&admindomain.Admin{}, &orderdomain.Order{}, &orderdomain.OrderRefundRecord{}, &affiliatedomain.Commission{}, &affiliatedomain.WithdrawRequest{}); err != nil {
				t.Fatal(err)
			}
			user := createAffiliateTestUser(t, db, "history-controls@example.test")
			profile := createAffiliateTestProfile(t, db, user.ID, "HISTCTRL", constants.AffiliateProfileStatusActive)
			d := decimal.NewFromInt
			root := orderdomain.Order{OrderNo: "HIST-CTRL", TotalAmount: money.FromDecimal(d(100)), RefundedAmount: money.FromDecimal(d(20))}
			if err := db.Create(&root).Error; err != nil {
				t.Fatal(err)
			}
			row := affiliatedomain.Commission{AffiliateProfileID: profile.ID, OrderID: root.ID, CommissionType: constants.AffiliateCommissionTypeOrder, BaseAmount: money.FromDecimal(d(80)), CommissionAmount: money.FromDecimal(d(16)), Status: constants.AffiliateCommissionStatusAvailable}
			if err := repo.CreateCommission(&row); err != nil {
				t.Fatal(err)
			}
			req, err := svc.ApplyWithdraw(user.ID, affiliateapp.WithdrawApplyInput{Amount: d(16), Channel: "bank", Account: "fixture"})
			if err != nil {
				t.Fatal(err)
			}
			// Deterministic ordering, with all history before any subsequent reapply.
			binding := time.Now().Add(-time.Hour)
			if err := db.Model(req).Update("created_at", binding).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&orderdomain.OrderRefundRecord{OrderID: root.ID, Amount: money.FromDecimal(d(20)), CreatedAt: binding.Add(-time.Minute)}).Error; err != nil {
				t.Fatal(err)
			}
			if tc.refund > 0 {
				if tc.marker {
					if err := repo.WithinTransaction(func(tx affiliatecontract.Store) error {
						return svc.HandleOrderRefunded(tx, &root, d(tc.refund-tc.legacy), d(20+tc.legacy), "fixture")
					}); err != nil {
						t.Fatal(err)
					}
				}
				if err := db.Model(&root).Update("refunded_amount", money.FromDecimal(d(20+tc.refund))).Error; err != nil {
					t.Fatal(err)
				}
				if !tc.incomplete {
					if err := db.Create(&orderdomain.OrderRefundRecord{OrderID: root.ID, Amount: money.FromDecimal(d(tc.refund)), CreatedAt: binding.Add(time.Minute)}).Error; err != nil {
						t.Fatal(err)
					}
				}
			}
			if tc.archived {
				if err := db.Model(&root).Update("deleted_at", binding.Add(2*time.Minute)).Error; err != nil {
					t.Fatal(err)
				}
				if err := db.Model(&orderdomain.OrderRefundRecord{}).Where("order_id = ?", root.ID).Update("deleted_at", binding.Add(2*time.Minute)).Error; err != nil {
					t.Fatal(err)
				}
			}
			_, err = svc.ReviewWithdraw(1, req.ID, constants.AffiliateWithdrawActionReject, "fixture")
			var got affiliatedomain.Commission
			if loadErr := db.First(&got, row.ID).Error; loadErr != nil {
				t.Fatal(loadErr)
			}
			if tc.incomplete {
				if err == nil || got.WithdrawRequestID == nil || !got.CommissionAmount.Decimal.Equal(d(16)) {
					t.Fatalf("incomplete history released entitlement: %v %+v", err, got)
				}
				var pending affiliatedomain.WithdrawRequest
				if err := db.First(&pending, req.ID).Error; err != nil {
					t.Fatal(err)
				}
				if pending.Status != constants.AffiliateWithdrawStatusPendingReview {
					t.Fatal("failed rejection not atomic")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !got.CommissionAmount.Decimal.Equal(d(tc.want)) || !got.BaseAmount.Decimal.Equal(d(tc.want).Mul(d(5))) || got.WithdrawRequestID != nil {
				t.Fatalf("incorrect retained entitlement: %+v", got)
			}
			if _, err := svc.ReviewWithdraw(1, req.ID, constants.AffiliateWithdrawActionReject, "repeat"); err == nil {
				t.Fatal("repeated rejection accepted")
			}
			if tc.want == 0 {
				if got.Status != constants.AffiliateCommissionStatusRejected {
					t.Fatal("full refund commission not rejected")
				}
				if _, err := svc.ApplyWithdraw(user.ID, affiliateapp.WithdrawApplyInput{Amount: d(1), Channel: "bank", Account: "fixture"}); err == nil {
					t.Fatal("full refund reopened")
				}
				return
			}
			if got.Status != constants.AffiliateCommissionStatusAvailable {
				t.Fatal("legitimate retained entitlement unavailable")
			}
			again, err := svc.ApplyWithdraw(user.ID, affiliateapp.WithdrawApplyInput{Amount: d(tc.want), Channel: "bank", Account: "fixture"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := svc.ReviewWithdraw(1, again.ID, constants.AffiliateWithdrawActionReject, "no new refund"); err != nil {
				t.Fatal(err)
			}
			if err := db.First(&got, row.ID).Error; err != nil {
				t.Fatal(err)
			}
			if !got.CommissionAmount.Decimal.Equal(d(tc.want)) || !got.BaseAmount.Decimal.Equal(d(tc.want).Mul(d(5))) {
				t.Fatalf("reapply deducted history twice: %+v", got)
			}
		})
	}
}

func TestRemediationHistoricalBoundRefundRejection(t *testing.T) {
	for _, childRefund := range []bool{false, true} {
		t.Run(fmt.Sprint(childRefund), func(t *testing.T) {
			svc, db, repo := setupAffiliateServiceTest(t)
			if err := db.AutoMigrate(&admindomain.Admin{}, &orderdomain.Order{}, &orderdomain.OrderRefundRecord{}, &affiliatedomain.Commission{}, &affiliatedomain.WithdrawRequest{}); err != nil {
				t.Fatal(err)
			}
			user := createAffiliateTestUser(t, db, "historical-bound@example.test")
			profile := createAffiliateTestProfile(t, db, user.ID, "HISTORY1", constants.AffiliateProfileStatusActive)
			d := decimal.NewFromInt
			root := orderdomain.Order{OrderNo: "HISTORY-ROOT", TotalAmount: money.FromDecimal(d(100)), RefundedAmount: money.FromDecimal(d(20))}
			if err := db.Create(&root).Error; err != nil {
				t.Fatal(err)
			}
			// Twenty was already reversed before binding; eighty of eligible base remains.
			row := affiliatedomain.Commission{AffiliateProfileID: profile.ID, OrderID: root.ID, CommissionType: constants.AffiliateCommissionTypeOrder, BaseAmount: money.FromDecimal(d(80)), CommissionAmount: money.FromDecimal(d(16)), Status: constants.AffiliateCommissionStatusAvailable}
			if err := repo.CreateCommission(&row); err != nil {
				t.Fatal(err)
			}
			req, err := svc.ApplyWithdraw(user.ID, affiliateapp.WithdrawApplyInput{Amount: d(16), Channel: "bank", Account: "fixture"})
			if err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&orderdomain.OrderRefundRecord{OrderID: root.ID, Amount: money.FromDecimal(d(20)), CreatedAt: req.CreatedAt.Add(-time.Hour)}).Error; err != nil {
				t.Fatal(err)
			}
			refundID := root.ID
			if childRefund {
				child := orderdomain.Order{OrderNo: "HISTORY-CHILD", ParentID: &root.ID, TotalAmount: money.FromDecimal(d(100)), RefundedAmount: money.FromDecimal(d(40))}
				if err := db.Create(&child).Error; err != nil {
					t.Fatal(err)
				}
				refundID = child.ID
			} else {
				if err := db.Model(&root).Update("refunded_amount", money.FromDecimal(d(60))).Error; err != nil {
					t.Fatal(err)
				}
			}
			// Legacy code skipped this while bound and wrote no deferred_refund marker.
			if err := db.Create(&orderdomain.OrderRefundRecord{OrderID: refundID, Amount: money.FromDecimal(d(40)), CreatedAt: req.CreatedAt.Add(time.Second)}).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := svc.ReviewWithdraw(1, req.ID, constants.AffiliateWithdrawActionReject, "fixture"); err != nil {
				t.Fatal(err)
			}
			var got affiliatedomain.Commission
			if err := db.First(&got, row.ID).Error; err != nil {
				t.Fatal(err)
			}
			if !got.CommissionAmount.Decimal.Equal(d(8)) || !got.BaseAmount.Decimal.Equal(d(40)) || got.WithdrawRequestID != nil {
				t.Fatalf("historical refund restored stale entitlement: %+v", got)
			}
		})
	}
}

// Hook-focused fixtures still require persisted, reconciliable backing revenue.
func persistAffiliateRefundFixture(t *testing.T, db *gorm.DB, order *orderdomain.Order, amount decimal.Decimal) {
	t.Helper()
	order.OrderNo = fmt.Sprintf("DEFERRED-%d", order.ID)
	order.RefundedAmount = money.FromDecimal(order.RefundedAmount.Decimal.Add(amount))
	if err := db.Save(order).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&orderdomain.OrderRefundRecord{OrderID: order.ID, Amount: money.FromDecimal(amount), CreatedAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
}

// Regression: rejected withdrawals cannot restore refunded entitlement.
func TestAuditRefundThenRejectedWithdrawRestoresCommission(t *testing.T) {
	svc, db, repo := setupAffiliateServiceTest(t)
	if err := db.AutoMigrate(&admindomain.Admin{}, &orderdomain.Order{}, &orderdomain.OrderRefundRecord{}, &affiliatedomain.Commission{}, &affiliatedomain.WithdrawRequest{}); err != nil {
		t.Fatal(err)
	}
	user := createAffiliateTestUser(t, db, "synthetic-audit@example.test")
	profile := createAffiliateTestProfile(t, db, user.ID, "AUDIT001", constants.AffiliateProfileStatusActive)
	d := decimal.NewFromInt
	row := affiliatedomain.Commission{AffiliateProfileID: profile.ID, OrderID: 999, CommissionType: constants.AffiliateCommissionTypeOrder, BaseAmount: money.FromDecimal(d(100)), RatePercent: money.FromDecimal(d(20)), CommissionAmount: money.FromDecimal(d(20)), Status: constants.AffiliateCommissionStatusAvailable}
	if err := repo.CreateCommission(&row); err != nil {
		t.Fatal(err)
	}
	req, err := svc.ApplyWithdraw(user.ID, affiliateapp.WithdrawApplyInput{Amount: d(20), Channel: "bank", Account: "synthetic-only"})
	if err != nil {
		t.Fatal(err)
	}
	order := &orderdomain.Order{ID: 999, TotalAmount: money.FromDecimal(d(100))}
	if err := repo.WithinTransaction(func(tx affiliatecontract.Store) error {
		return svc.HandleOrderRefunded(tx, order, d(100), decimal.Zero, "synthetic full refund")
	}); err != nil {
		t.Fatal(err)
	}
	persistAffiliateRefundFixture(t, db, order, d(100))
	if _, err := svc.ReviewWithdraw(1, req.ID, constants.AffiliateWithdrawActionReject, "synthetic rejection"); err != nil {
		t.Fatal(err)
	}
	available, err := repo.SumCommissionByProfile(profile.ID, []string{constants.AffiliateCommissionStatusAvailable}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !available.IsZero() {
		t.Fatalf("expected reproduction of stale 20.00, got %s", available)
	}
	_, err = svc.ApplyWithdraw(user.ID, affiliateapp.WithdrawApplyInput{Amount: d(20), Channel: "bank", Account: "synthetic-only"})
	if err == nil {
		t.Fatal("refunded commission became withdrawable again")
	}
}

func TestRemediationDeferredRefundCentExhaustion(t *testing.T) {
	svc, db, repo := setupAffiliateServiceTest(t)
	if err := db.AutoMigrate(&admindomain.Admin{}, &orderdomain.Order{}, &orderdomain.OrderRefundRecord{}, &affiliatedomain.Commission{}, &affiliatedomain.WithdrawRequest{}); err != nil {
		t.Fatal(err)
	}
	user := createAffiliateTestUser(t, db, "cent-exhaustion@example.test")
	profile := createAffiliateTestProfile(t, db, user.ID, "CENT001", constants.AffiliateProfileStatusActive)
	d := decimal.NewFromInt
	row := affiliatedomain.Commission{AffiliateProfileID: profile.ID, OrderID: 999, CommissionType: constants.AffiliateCommissionTypeOrder, BaseAmount: money.FromDecimal(d(100)), CommissionAmount: money.FromDecimal(d(20)), Status: constants.AffiliateCommissionStatusAvailable}
	if err := repo.CreateCommission(&row); err != nil {
		t.Fatal(err)
	}
	req, err := svc.ApplyWithdraw(user.ID, affiliateapp.WithdrawApplyInput{Amount: d(20), Channel: "bank", Account: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	order := &orderdomain.Order{ID: 999, TotalAmount: money.FromDecimal(d(100))}
	before := decimal.Zero
	for _, text := range []string{"99.99", "0.01"} {
		delta := decimal.RequireFromString(text)
		if err := repo.WithinTransaction(func(tx affiliatecontract.Store) error {
			return svc.HandleOrderRefunded(tx, order, delta, before, "refund")
		}); err != nil {
			t.Fatal(err)
		}
		persistAffiliateRefundFixture(t, db, order, delta)
		before = before.Add(delta)
	}
	var held affiliatedomain.Commission
	if err := db.First(&held, row.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !held.CommissionAmount.Decimal.Equal(d(20)) || held.Status != constants.AffiliateCommissionStatusAvailable {
		t.Fatal("in-flight payout changed")
	}
	if _, err := svc.ReviewWithdraw(1, req.ID, constants.AffiliateWithdrawActionReject, "fixture"); err != nil {
		t.Fatal(err)
	}
	available, err := repo.SumCommissionByProfile(profile.ID, []string{constants.AffiliateCommissionStatusAvailable}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !available.IsZero() {
		t.Fatalf("resurrected rounded commission: %s", available)
	}
}

func TestRemediationDeferredPartialRefundWithdrawalTransitions(t *testing.T) {
	for _, action := range []string{constants.AffiliateWithdrawActionReject, constants.AffiliateWithdrawActionPay} {
		t.Run(action, func(t *testing.T) {
			svc, db, repo := setupAffiliateServiceTest(t)
			if err := db.AutoMigrate(&admindomain.Admin{}, &orderdomain.Order{}, &orderdomain.OrderRefundRecord{}, &affiliatedomain.Commission{}, &affiliatedomain.WithdrawRequest{}); err != nil {
				t.Fatal(err)
			}
			user := createAffiliateTestUser(t, db, "partial-withdraw@example.test")
			profile := createAffiliateTestProfile(t, db, user.ID, "PARTW001", constants.AffiliateProfileStatusActive)
			d := decimal.NewFromInt
			row := affiliatedomain.Commission{AffiliateProfileID: profile.ID, OrderID: 999, CommissionType: constants.AffiliateCommissionTypeOrder, BaseAmount: money.FromDecimal(d(100)), CommissionAmount: money.FromDecimal(d(20)), Status: constants.AffiliateCommissionStatusAvailable}
			if err := repo.CreateCommission(&row); err != nil {
				t.Fatal(err)
			}
			req, err := svc.ApplyWithdraw(user.ID, affiliateapp.WithdrawApplyInput{Amount: d(20), Channel: "bank", Account: "fixture"})
			if err != nil {
				t.Fatal(err)
			}
			order := &orderdomain.Order{ID: 999, TotalAmount: money.FromDecimal(d(100))}
			for _, before := range []int64{0, 25} {
				if err := repo.WithinTransaction(func(tx affiliatecontract.Store) error {
					return svc.HandleOrderRefunded(tx, order, d(25), d(before), "refund")
				}); err != nil {
					t.Fatal(err)
				}
				persistAffiliateRefundFixture(t, db, order, d(25))
			}
			var held affiliatedomain.Commission
			if err := db.First(&held, row.ID).Error; err != nil {
				t.Fatal(err)
			}
			if !held.CommissionAmount.Decimal.Equal(d(20)) {
				t.Fatal("in-flight amount changed")
			}
			if _, err := svc.ReviewWithdraw(1, req.ID, action, "fixture"); err != nil {
				t.Fatal(err)
			}
			if err := db.First(&held, row.ID).Error; err != nil {
				t.Fatal(err)
			}
			if action == constants.AffiliateWithdrawActionReject {
				if held.WithdrawRequestID != nil || held.Status != constants.AffiliateCommissionStatusAvailable || !held.CommissionAmount.Decimal.Equal(d(10)) || !held.BaseAmount.Decimal.Equal(d(50)) {
					t.Fatalf("incorrect release: %+v", held)
				}
				again, err := svc.ApplyWithdraw(user.ID, affiliateapp.WithdrawApplyInput{Amount: d(10), Channel: "bank", Account: "fixture"})
				if err != nil {
					t.Fatal(err)
				}
				if err := repo.WithinTransaction(func(tx affiliatecontract.Store) error {
					return svc.HandleOrderRefunded(tx, order, d(50), d(50), "refund")
				}); err != nil {
					t.Fatal(err)
				}
				persistAffiliateRefundFixture(t, db, order, d(50))
				if _, err := svc.ReviewWithdraw(1, again.ID, action, "fixture"); err != nil {
					t.Fatal(err)
				}
				available, err := repo.SumCommissionByProfile(profile.ID, []string{constants.AffiliateCommissionStatusAvailable}, true)
				if err != nil {
					t.Fatal(err)
				}
				if !available.IsZero() {
					t.Fatal("second rejection restored credit")
				}
			} else {
				if held.Status != constants.AffiliateCommissionStatusWithdrawn || !held.CommissionAmount.Decimal.Equal(d(20)) {
					t.Fatal("approved payout policy changed")
				}
			}
		})
	}
}
