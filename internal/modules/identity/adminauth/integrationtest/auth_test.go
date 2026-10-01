package integrationtest

import (
	"bytes"
	"encoding/json"
	"github.com/dujiao-next/internal/app/container"
	adminauthwiring "github.com/dujiao-next/internal/bootstrap/adminauth"
	"net/http"
	"testing"
	"time"

	"github.com/dujiao-next/internal/config"
	admincontract "github.com/dujiao-next/internal/modules/identity/admin/contract"
	admindomain "github.com/dujiao-next/internal/modules/identity/admin/domain"
	adminstore "github.com/dujiao-next/internal/modules/identity/admin/infrastructure/gormstore"
	adminauthapp "github.com/dujiao-next/internal/modules/identity/adminauth/application"
	adminchallenge "github.com/dujiao-next/internal/modules/identity/adminauth/challenge"
	admintotpapp "github.com/dujiao-next/internal/modules/identity/adminauth/totp/application"

	"context"
	"fmt"
	"github.com/dujiao-next/internal/admincmd"
	"github.com/dujiao-next/internal/app/httpserver/middleware"
	"github.com/dujiao-next/internal/cache"
	auditlogdomain "github.com/dujiao-next/internal/modules/auditlog/domain"
	"github.com/dujiao-next/internal/platform/database/gormdb"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/pquerna/otp/totp"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"net/http/httptest"
)

func newAuthTestService(t *testing.T) (*adminauthapp.Service, *admintotpapp.Service, admincontract.Store) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&admindomain.Admin{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cfg := &config.Config{
		App: config.AppConfig{SecretKey: "auth-test-key"},
		JWT: config.JWTConfig{SecretKey: "jwt-secret-for-test", ExpireHours: 24},
	}
	adminRepo := adminstore.New(db)
	return adminauthapp.NewService(cfg, adminRepo), admintotpapp.NewService(cfg, adminRepo, nil), adminRepo
}

func createAuthTestAdmin(t *testing.T, repo admincontract.Store, username, password string) *admindomain.Admin {
	t.Helper()
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	admin := &admindomain.Admin{Username: username, PasswordHash: string(hash)}
	if err := repo.Create(admin); err != nil {
		t.Fatalf("create: %v", err)
	}
	return admin
}

func TestLoginWithoutTOTP(t *testing.T) {
	auth, _, repo := newAuthTestService(t)
	createAuthTestAdmin(t, repo, "noma", "secret123")

	res, err := auth.Login("noma", "secret123")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if res.RequiresTOTP {
		t.Fatalf("expected RequiresTOTP=false")
	}
	if res.Token == "" {
		t.Fatalf("expected jwt token")
	}
}

func TestLoginWithTOTPReturnsChallenge(t *testing.T) {
	auth, totpSvc, repo := newAuthTestService(t)
	admin := createAuthTestAdmin(t, repo, "alice", "secret123")
	setupRes, _ := totpSvc.Setup(admin.ID)
	code, _ := totp.GenerateCode(setupRes.Secret, time.Now())
	if _, err := totpSvc.Enable(admin.ID, code); err != nil {
		t.Fatalf("enable: %v", err)
	}

	res, err := auth.Login("alice", "secret123")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if !res.RequiresTOTP {
		t.Fatalf("expected RequiresTOTP=true")
	}
	if res.ChallengeToken == "" || res.ChallengeJTI == "" {
		t.Fatalf("expected challenge token + jti")
	}
	claims, err := auth.ParseChallengeToken(res.ChallengeToken)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if claims.AdminID != admin.ID {
		t.Fatalf("admin id mismatch: %d vs %d", claims.AdminID, admin.ID)
	}
	if claims.Purpose != adminchallenge.PurposeTwoFactor {
		t.Fatalf("purpose mismatch")
	}
}

func TestLoginInvalidCredentials(t *testing.T) {
	auth, _, repo := newAuthTestService(t)
	createAuthTestAdmin(t, repo, "bob", "secret")
	if _, err := auth.Login("bob", "wrong"); err != adminauthapp.ErrInvalidCredentials {
		t.Fatalf("expected invalid creds, got %v", err)
	}
	if _, err := auth.Login("nosuch", "x"); err != adminauthapp.ErrInvalidCredentials {
		t.Fatalf("expected invalid creds for missing user, got %v", err)
	}
}

func TestParseChallengeRejectsWrongPurpose(t *testing.T) {
	auth, _, repo := newAuthTestService(t)
	createAuthTestAdmin(t, repo, "carol", "secret")
	admin, _ := repo.GetByUsername("carol")
	regular, _, err := auth.GenerateJWT(admin)
	if err != nil {
		t.Fatalf("gen jwt: %v", err)
	}
	if _, err := auth.ParseChallengeToken(regular); err == nil {
		t.Fatalf("expected error for non-challenge token")
	}
}

func TestSecurityAdminChallengeRevokedByPasswordChange(t *testing.T) {
	auth, otp, repo := newAuthTestService(t)
	u := createAuthTestAdmin(t, repo, "challenge-admin", "old-password")
	setup, e := otp.Setup(u.ID)
	if e != nil {
		t.Fatal(e)
	}
	code, _ := totp.GenerateCode(setup.Secret, time.Now())
	if _, e = otp.Enable(u.ID, code); e != nil {
		t.Fatal(e)
	}
	first, e := auth.Login(u.Username, "old-password")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = auth.ParseChallengeToken(first.ChallengeToken); e != nil {
		t.Fatal(e)
	}
	if e = auth.ChangePassword(u.ID, "old-password", "new-password"); e != nil {
		t.Fatal(e)
	}
	if _, e = auth.ParseChallengeToken(first.ChallengeToken); e == nil {
		t.Fatal("pre-password-change challenge accepted")
	}
	fresh, e := auth.Login(u.Username, "new-password")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = auth.ParseChallengeToken(fresh.ChallengeToken); e != nil {
		t.Fatal(e)
	}
}

// An in-memory Redis command hook: no sockets, servers, or live data are used.
type auditCacheHook struct{ values map[string]string }

func (h *auditCacheHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h *auditCacheHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (h *auditCacheHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		args := cmd.Args()
		key := fmt.Sprint(args[1])
		switch cmd.Name() {
		case "set":
			if b, ok := args[2].([]byte); ok {
				h.values[key] = string(b)
			} else {
				h.values[key] = fmt.Sprint(args[2])
			}
			cmd.(*redis.StatusCmd).SetVal("OK")
			return nil
		case "get":
			v, ok := h.values[key]
			if !ok {
				cmd.SetErr(redis.Nil)
				return redis.Nil
			}
			cmd.(*redis.StringCmd).SetVal(v)
			return nil
		case "del":
			delete(h.values, key)
			cmd.(*redis.IntCmd).SetVal(1)
			return nil
		default:
			return fmt.Errorf("unexpected fixture redis command %s", cmd.Name())
		}
	}
}
func TestSecurityCLIRecoveryRejectsCachedAdminSession(t *testing.T) {
	auth, _, repo := newAuthTestService(t)
	u := createAuthTestAdmin(t, repo, "fixture-admin", "old-password")
	token, _, err := auth.GenerateJWT(u)
	if err != nil {
		t.Fatal(err)
	}
	if err = cache.InitRedis(&config.RedisConfig{Enabled: true, Host: "127.0.0.1", Port: 1, Prefix: "isolated-audit"}); err != nil {
		t.Fatal(err)
	}
	client := cache.Client()
	client.AddHook(&auditCacheHook{values: map[string]string{}})
	t.Cleanup(func() { _ = client.Close(); _ = cache.InitRedis(&config.RedisConfig{Enabled: false}) })
	if err = cache.SetAdminAuthState(context.Background(), cache.BuildAdminAuthState(u)); err != nil {
		t.Fatal(err)
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte("new-password"), bcrypt.DefaultCost)
	// The exact DB-only call made by the operational CLI reset-password path.
	if err = repo.UpdatePassword(u.ID, string(hash)); err != nil {
		t.Fatal(err)
	}
	fresh, _ := repo.GetByID(u.ID)
	if fresh.TokenVersion == u.TokenVersion {
		t.Fatal("reset failed")
	}
	r := gin.New()
	r.GET("/private", middleware.JWTAuthMiddleware("jwt-secret-for-test", repo), func(c *gin.Context) { c.Status(204) })
	request := func() int {
		req := httptest.NewRequest("GET", "/private", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}
	if code := request(); code == 204 {
		t.Fatal("DB-only recovery left cached session authorized")
	}
	if err = cache.DelAdminAuthState(context.Background(), u.ID); err != nil {
		t.Fatal(err)
	}
	if code := request(); code == 204 {
		t.Fatal("expected old token rejection after clearing stale cache")
	}
	t.Log("DB-only recovery immediately revokes cached session")
}

func TestSecurityCompletionRechecksFirstFactorEpoch(t *testing.T) {
	auth, otp, repo := newAuthTestService(t)
	u := createAuthTestAdmin(t, repo, "finish@example.test", "old-password")
	setup, e := otp.Setup(u.ID)
	if e != nil {
		t.Fatal(e)
	}
	code, _ := totp.GenerateCode(setup.Secret, time.Now())
	if _, e = otp.Enable(u.ID, code); e != nil {
		t.Fatal(e)
	}
	first, e := auth.Login(u.Username, "old-password")
	if e != nil {
		t.Fatal(e)
	}
	claims, e := auth.ParseChallengeToken(first.ChallengeToken)
	if e != nil {
		t.Fatal(e)
	}
	if e = auth.ChangePassword(u.ID, "old-password", "new-password"); e != nil {
		t.Fatal(e)
	}
	if _, e = auth.CompleteLoginAfter2FA(u.ID, *claims.TokenVersion); e == nil {
		t.Fatal("completion upgraded stale first factor to new epoch")
	}
}

type securityAdminLoginInterleave struct {
	admincontract.Store
	afterRead func()
}

func (s *securityAdminLoginInterleave) GetByUsername(name string) (*admindomain.Admin, error) {
	a, e := s.Store.GetByUsername(name)
	if s.afterRead != nil {
		f := s.afterRead
		s.afterRead = nil
		f()
	}
	return a, e
}
func TestSecurityAdminLoginCannotUndoCLIRecovery(t *testing.T) {
	_, _, repo := newAuthTestService(t)
	u := createAuthTestAdmin(t, repo, "stale-admin", "old-password")
	wrapped := &securityAdminLoginInterleave{Store: repo, afterRead: func() {
		hash, _ := bcrypt.GenerateFromPassword([]byte("new-password"), bcrypt.DefaultCost)
		if e := repo.UpdatePassword(u.ID, string(hash)); e != nil {
			t.Fatal(e)
		}
	}}
	auth := adminauthapp.NewService(&config.Config{JWT: config.JWTConfig{SecretKey: "fixture-only", ExpireHours: 1}}, wrapped)
	_, _ = auth.Login(u.Username, "old-password")
	fresh, e := repo.GetByID(u.ID)
	if e != nil {
		t.Fatal(e)
	}
	if fresh.TokenVersion <= u.TokenVersion || bcrypt.CompareHashAndPassword([]byte(fresh.PasswordHash), []byte("new-password")) != nil {
		t.Fatal("in-flight login undid CLI recovery")
	}
}

func (s *securityAdminLoginInterleave) GetByID(id uint) (*admindomain.Admin, error) {
	a, err := s.Store.GetByID(id)
	if s.afterRead != nil {
		f := s.afterRead
		s.afterRead = nil
		f()
	}
	return a, err
}

func TestSecurityWiredAdmin2FARejectsEpochChangeDuringCompletion(t *testing.T) {
	for _, recovery := range []bool{false, true} {
		t.Run(fmt.Sprintf("recovery=%v", recovery), func(t *testing.T) {
			auth, otp, repo := newAuthTestService(t)
			u := createAuthTestAdmin(t, repo, "wired-admin", "old-password")
			setup, err := otp.Setup(u.ID)
			if err != nil {
				t.Fatal(err)
			}
			code, err := totp.GenerateCode(setup.Secret, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			enabled, err := otp.Enable(u.ID, code)
			if err != nil {
				t.Fatal(err)
			}
			first, err := auth.Login(u.Username, "old-password")
			if err != nil {
				t.Fatal(err)
			}
			wrapped := &securityAdminLoginInterleave{Store: repo, afterRead: func() {
				if err := auth.ChangePassword(u.ID, "old-password", "new-password"); err != nil {
					t.Fatal(err)
				}
			}}
			cfg := &config.Config{JWT: config.JWTConfig{SecretKey: "jwt-secret-for-test", ExpireHours: 24}}
			wiredAuth := adminauthapp.NewService(cfg, wrapped)
			h := adminauthwiring.New(&container.Container{AuthService: wiredAuth, TOTPService: otp}).TwoFA
			r := gin.New()
			r.POST("/verify", h.Verify2FA)
			send := func(challenge, field, value string) *httptest.ResponseRecorder {
				body, err := json.Marshal(map[string]string{"challenge_token": challenge, field: value})
				if err != nil {
					t.Fatal(err)
				}
				req := httptest.NewRequest("POST", "/verify", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				return w
			}
			field, value := "code", code
			if recovery {
				field, value = "recovery_code", enabled.RecoveryCodes[0]
			}
			w := send(first.ChallengeToken, field, value)
			if bytes.Contains(w.Body.Bytes(), []byte(`"status_code":0`)) || bytes.Contains(w.Body.Bytes(), []byte(`"token":`)) {
				t.Fatalf("stale wired completion authorized: %d %s", w.Code, w.Body.String())
			}
			fresh, err := auth.Login(u.Username, "new-password")
			if err != nil {
				t.Fatal(err)
			}
			w = send(fresh.ChallengeToken, "code", code)
			if w.Code != http.StatusOK || !bytes.Contains(w.Body.Bytes(), []byte(`"token":`)) {
				t.Fatalf("fresh wired completion rejected: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestSecurityActualRecoveryCLIRevokesWarmCache(t *testing.T) {
	for _, command := range []string{"reset-password", "reset-2fa"} {
		t.Run(command, func(t *testing.T) {
			db, e := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			if e != nil {
				t.Fatal(e)
			}
			if e = db.AutoMigrate(&admindomain.Admin{}, &auditlogdomain.AdminLoginLog{}); e != nil {
				t.Fatal(e)
			}
			previous := gormdb.DB
			gormdb.DB = db
			t.Cleanup(func() { gormdb.DB = previous })
			repo := adminstore.New(db)
			u := createAuthTestAdmin(t, repo, "cli-fixture", "old-password")
			if command == "reset-2fa" {
				if e = repo.UpdateTOTPEnabled(u.ID, "fixture-encrypted-secret", time.Now(), "[]"); e != nil {
					t.Fatal(e)
				}
				u, e = repo.GetByID(u.ID)
				if e != nil {
					t.Fatal(e)
				}
			}
			auth := adminauthapp.NewService(&config.Config{JWT: config.JWTConfig{SecretKey: "cli-fixture-signing", ExpireHours: 1}}, repo)
			token, _, e := auth.GenerateJWT(u)
			if e != nil {
				t.Fatal(e)
			}
			if e = cache.InitRedis(&config.RedisConfig{Enabled: true, Host: "127.0.0.1", Port: 1, Prefix: "cli-fixture-cache"}); e != nil {
				t.Fatal(e)
			}
			client := cache.Client()
			client.AddHook(&auditCacheHook{values: map[string]string{}})
			t.Cleanup(func() { _ = client.Close(); _ = cache.InitRedis(&config.RedisConfig{Enabled: false}) })
			if e = cache.SetAdminAuthState(context.Background(), cache.BuildAdminAuthState(u)); e != nil {
				t.Fatal(e)
			}
			args := []string{command, "--username", u.Username}
			if command == "reset-password" {
				args = append(args, "--password", "new-fixture-password")
			}
			admincmd.Run(args) // Real command dispatcher; fixture DB and fake Redis only.
			r := gin.New()
			r.GET("/private", middleware.JWTAuthMiddleware("cli-fixture-signing", repo), func(c *gin.Context) { c.Status(204) })
			req := httptest.NewRequest("GET", "/private", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code == 204 {
				t.Fatal("recovery command left a cached stolen token authorized")
			}
			current, e := repo.GetByID(u.ID)
			if e != nil {
				t.Fatal(e)
			}
			if current.TokenVersion <= u.TokenVersion {
				t.Fatal("recovery epoch unchanged")
			}
			password := "old-password"
			if command == "reset-password" {
				password = "new-fixture-password"
			} else if current.TOTPEnabledAt != nil {
				t.Fatal("TOTP not cleared")
			}
			if _, e = auth.Login(u.Username, password); e != nil {
				t.Fatalf("legitimate recovery login failed: %v", e)
			}
		})
	}
}
