package integrationtest

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/dujiao-next/internal/app/httpserver/middleware"
	"github.com/dujiao-next/internal/constants"
	apicredentialapp "github.com/dujiao-next/internal/modules/apicredential/application"
	apicredentialcontract "github.com/dujiao-next/internal/modules/apicredential/contract"
	apicredentialdomain "github.com/dujiao-next/internal/modules/apicredential/domain"
	apicredentialgormstore "github.com/dujiao-next/internal/modules/apicredential/infrastructure/gormstore"
	userdomain "github.com/dujiao-next/internal/modules/identity/user/domain"
	"github.com/dujiao-next/internal/upstream"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"net/http/httptest"
	"strconv"
)

func setupApiCredentialServiceTest(t *testing.T) (*apicredentialapp.Service, apicredentialcontract.Repository, *gorm.DB) {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(&apicredentialdomain.ApiCredential{}); err != nil {
		t.Fatalf("auto migrate api credential failed: %v", err)
	}

	repo := apicredentialgormstore.New(db)
	return apicredentialapp.NewService(repo), repo, db
}

func TestApiCredentialServiceApplyCreatesPendingRecordWhenMissing(t *testing.T) {
	svc, repo, _ := setupApiCredentialServiceTest(t)

	cred, err := svc.Apply(1001)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if cred == nil {
		t.Fatal("expected credential, got nil")
	}
	if cred.Status != constants.ApiCredentialStatusPendingReview {
		t.Fatalf("expected status %s, got %s", constants.ApiCredentialStatusPendingReview, cred.Status)
	}
	if cred.UserID != 1001 {
		t.Fatalf("expected user id 1001, got %d", cred.UserID)
	}

	stored, err := repo.GetByUserID(1001)
	if err != nil {
		t.Fatalf("get by user id failed: %v", err)
	}
	if stored == nil {
		t.Fatal("expected stored credential, got nil")
	}
}

func TestApiCredentialServiceApplyRestoresDeletedCredential(t *testing.T) {
	svc, repo, _ := setupApiCredentialServiceTest(t)
	now := time.Now()

	cred := &apicredentialdomain.ApiCredential{
		UserID:       1002,
		ApiKey:       "legacy-key",
		ApiSecret:    "legacy-secret",
		Status:       constants.ApiCredentialStatusApproved,
		RejectReason: "legacy reason",
		ApprovedAt:   &now,
		LastUsedAt:   &now,
		IsActive:     true,
	}
	if err := repo.Create(cred); err != nil {
		t.Fatalf("create credential failed: %v", err)
	}
	if err := repo.Delete(cred.ID); err != nil {
		t.Fatalf("delete credential failed: %v", err)
	}

	reapplied, err := svc.Apply(1002)
	if err != nil {
		t.Fatalf("reapply failed: %v", err)
	}
	if reapplied.ID != cred.ID {
		t.Fatalf("expected to reuse credential id %d, got %d", cred.ID, reapplied.ID)
	}
	if reapplied.Status != constants.ApiCredentialStatusPendingReview {
		t.Fatalf("expected status %s, got %s", constants.ApiCredentialStatusPendingReview, reapplied.Status)
	}
	if reapplied.ApiKey == "" || reapplied.ApiKey == "legacy-key" {
		t.Fatalf("expected new api key, got %q", reapplied.ApiKey)
	}
	if reapplied.ApiSecret != "" {
		t.Fatalf("expected api secret to be cleared, got %q", reapplied.ApiSecret)
	}
	if reapplied.RejectReason != "" {
		t.Fatalf("expected reject reason cleared, got %q", reapplied.RejectReason)
	}
	if reapplied.ApprovedAt != nil || reapplied.LastUsedAt != nil {
		t.Fatal("expected approved_at and last_used_at cleared")
	}
	if reapplied.IsActive {
		t.Fatal("expected inactive credential after reapply")
	}
	if reapplied.DeletedAt != nil {
		t.Fatal("expected deleted_at to be cleared")
	}

	stored, err := repo.GetByUserID(1002)
	if err != nil {
		t.Fatalf("get by user id failed: %v", err)
	}
	if stored == nil {
		t.Fatal("expected restored credential to be queryable")
	}
	if stored.ID != cred.ID {
		t.Fatalf("expected stored credential id %d, got %d", cred.ID, stored.ID)
	}
}

func TestApiCredentialServiceApplyResetsRejectedCredential(t *testing.T) {
	svc, repo, _ := setupApiCredentialServiceTest(t)
	now := time.Now()

	cred := &apicredentialdomain.ApiCredential{
		UserID:       1003,
		ApiKey:       "old-key",
		ApiSecret:    "old-secret",
		Status:       constants.ApiCredentialStatusRejected,
		RejectReason: "missing docs",
		ApprovedAt:   &now,
		LastUsedAt:   &now,
		IsActive:     true,
	}
	if err := repo.Create(cred); err != nil {
		t.Fatalf("create rejected credential failed: %v", err)
	}

	reapplied, err := svc.Apply(1003)
	if err != nil {
		t.Fatalf("reapply failed: %v", err)
	}
	if reapplied.ID != cred.ID {
		t.Fatalf("expected to reuse credential id %d, got %d", cred.ID, reapplied.ID)
	}
	if reapplied.Status != constants.ApiCredentialStatusPendingReview {
		t.Fatalf("expected status %s, got %s", constants.ApiCredentialStatusPendingReview, reapplied.Status)
	}
	if reapplied.ApiKey == "" || reapplied.ApiKey == "old-key" {
		t.Fatalf("expected new api key, got %q", reapplied.ApiKey)
	}
	if reapplied.ApiSecret != "" {
		t.Fatalf("expected api secret cleared, got %q", reapplied.ApiSecret)
	}
	if reapplied.RejectReason != "" {
		t.Fatalf("expected reject reason cleared, got %q", reapplied.RejectReason)
	}
	if reapplied.ApprovedAt != nil || reapplied.LastUsedAt != nil {
		t.Fatal("expected approved_at and last_used_at cleared")
	}
	if reapplied.IsActive {
		t.Fatal("expected inactive credential after reapply")
	}
}

func TestApiCredentialServiceApplyBlocksPendingReview(t *testing.T) {
	svc, repo, _ := setupApiCredentialServiceTest(t)

	cred := &apicredentialdomain.ApiCredential{
		UserID: 1004,
		Status: constants.ApiCredentialStatusPendingReview,
	}
	if err := repo.Create(cred); err != nil {
		t.Fatalf("create pending credential failed: %v", err)
	}

	_, err := svc.Apply(1004)
	if !errors.Is(err, apicredentialcontract.ErrPendingExist) {
		t.Fatalf("expected ErrPendingExist, got %v", err)
	}
}

func TestApiCredentialServiceApplyBlocksApprovedAndDisabled(t *testing.T) {
	cases := []struct {
		name   string
		userID uint
		status string
	}{
		{name: "approved", userID: 1005, status: constants.ApiCredentialStatusApproved},
		{name: "disabled", userID: 1006, status: constants.ApiCredentialStatusDisabled},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, _ := setupApiCredentialServiceTest(t)
			cred := &apicredentialdomain.ApiCredential{
				UserID: tc.userID,
				Status: tc.status,
			}
			if err := repo.Create(cred); err != nil {
				t.Fatalf("create credential failed: %v", err)
			}

			_, err := svc.Apply(tc.userID)
			if !errors.Is(err, apicredentialcontract.ErrExists) {
				t.Fatalf("expected ErrExists, got %v", err)
			}
		})
	}
}

func TestSecurityUserCannotReenableAdminDisabledCredential(t *testing.T) {
	svc, repo, db := setupApiCredentialServiceTest(t)
	if e := db.AutoMigrate(&userdomain.User{}); e != nil {
		t.Fatal(e)
	}
	u := &userdomain.User{Email: "fixture@example.test", Status: constants.UserStatusActive}
	if e := db.Create(u).Error; e != nil {
		t.Fatal(e)
	}
	c, e := svc.Apply(u.ID)
	if e != nil {
		t.Fatal(e)
	}
	approved, secret, e := svc.Approve(c.ID)
	if e != nil {
		t.Fatal(e)
	}
	if e = svc.SetActive(c.ID, false); e != nil {
		t.Fatal(e)
	}
	disabled, _ := repo.GetByID(c.ID)
	if disabled.IsActive {
		t.Fatal("disable failed")
	}
	if e = svc.SetActiveByUserID(u.ID, true); e == nil {
		t.Fatal("user re-enabled admin suspension")
	}
	r := gin.New()
	r.GET("/api", middleware.UpstreamAPIAuthMiddleware(repo), func(c *gin.Context) { c.Status(204) })
	now := time.Now().Unix()
	req := httptest.NewRequest("GET", "/api", nil)
	req.Header.Set(upstream.HeaderApiKey, approved.ApiKey)
	req.Header.Set(upstream.HeaderTimestamp, strconv.FormatInt(now, 10))
	req.Header.Set(upstream.HeaderSignature, upstream.Sign(secret, "GET", "/api", now, nil))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code == 204 {
		t.Fatal("suspended credential authenticated")
	}
	if e = svc.SetActive(c.ID, true); e != nil {
		t.Fatal(e)
	}
	if e = svc.SetActiveByUserID(u.ID, false); e != nil {
		t.Fatal(e)
	}
	if e = svc.SetActiveByUserID(u.ID, true); e != nil {
		t.Fatal(e)
	}
	req = httptest.NewRequest("GET", "/api", nil)
	req.Header.Set(upstream.HeaderApiKey, approved.ApiKey)
	req.Header.Set(upstream.HeaderTimestamp, strconv.FormatInt(now, 10))
	req.Header.Set(upstream.HeaderSignature, upstream.Sign(secret, "GET", "/api", now, nil))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 204 {
		t.Fatalf("ordinary signed integration rejected after admin restore: %d", w.Code)
	}
}

func TestSecurityCredentialLegacyInactiveMigrationFailsClosed(t *testing.T) {
	svc, repo, db := setupApiCredentialServiceTest(t)
	active := &apicredentialdomain.ApiCredential{UserID: 71, ApiKey: "fixture-active", ApiSecret: "fixture-secret-a", Status: constants.ApiCredentialStatusApproved, IsActive: true}
	paused := &apicredentialdomain.ApiCredential{UserID: 72, ApiKey: "fixture-paused", ApiSecret: "fixture-secret-b", Status: constants.ApiCredentialStatusApproved, IsActive: false}
	if e := repo.Create(active); e != nil {
		t.Fatal(e)
	}
	if e := repo.Create(paused); e != nil {
		t.Fatal(e)
	}
	// Rehearse the pre-change schema without the provenance marker.
	if e := db.Migrator().DropColumn(&apicredentialdomain.ApiCredential{}, "UserDisabled"); e != nil {
		t.Fatal(e)
	}
	if e := db.AutoMigrate(&apicredentialdomain.ApiCredential{}); e != nil {
		t.Fatal(e)
	}
	migrated, e := repo.GetByID(active.ID)
	if e != nil {
		t.Fatal(e)
	}
	if !migrated.IsActive || migrated.ApiSecret != active.ApiSecret || migrated.UserDisabled {
		t.Fatal("active legacy credential changed")
	}
	if e = svc.SetActiveByUserID(paused.UserID, true); e == nil {
		t.Fatal("ambiguous legacy inactive credential enabled by owner")
	}
	if e = svc.SetActive(paused.ID, true); e != nil {
		t.Fatal(e)
	}
	if e = svc.SetActiveByUserID(paused.UserID, false); e != nil {
		t.Fatal(e)
	}
	if e = svc.SetActiveByUserID(paused.UserID, true); e != nil {
		t.Fatal(e)
	}
}

type securityCredentialInterleave struct {
	apicredentialcontract.Repository
	afterRead func()
}

func (r *securityCredentialInterleave) GetByUserID(id uint) (*apicredentialdomain.ApiCredential, error) {
	c, e := r.Repository.GetByUserID(id)
	if r.afterRead != nil {
		f := r.afterRead
		r.afterRead = nil
		f()
	}
	return c, e
}
func (r *securityCredentialInterleave) GetByID(id uint) (*apicredentialdomain.ApiCredential, error) {
	c, e := r.Repository.GetByID(id)
	if r.afterRead != nil {
		f := r.afterRead
		r.afterRead = nil
		f()
	}
	return c, e
}
func TestSecurityCredentialOwnerMutationsCannotUndoSuspension(t *testing.T) {
	for _, action := range []string{"enable", "rotate"} {
		t.Run(action, func(t *testing.T) {
			svc, repo, _ := setupApiCredentialServiceTest(t)
			c, e := svc.Apply(901)
			if e != nil {
				t.Fatal(e)
			}
			_, secret, e := svc.Approve(c.ID)
			if e != nil {
				t.Fatal(e)
			}
			wrapped := &securityCredentialInterleave{Repository: repo, afterRead: func() {
				if e := svc.SetActive(c.ID, false); e != nil {
					t.Fatal(e)
				}
			}}
			owner := apicredentialapp.NewService(wrapped)
			if action == "enable" {
				e = owner.SetActiveByUserID(c.UserID, true)
			} else {
				_, e = owner.Regenerate(c.ID)
			}
			if e == nil {
				t.Fatal("owner write crossed admin suspension")
			}
			current, e := repo.GetByID(c.ID)
			if e != nil {
				t.Fatal(e)
			}
			if current.IsActive || current.Status != constants.ApiCredentialStatusDisabled || current.ApiSecret != secret {
				t.Fatal("suspension or key changed")
			}
		})
	}
}
