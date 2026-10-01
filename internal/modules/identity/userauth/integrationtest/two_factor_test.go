package integrationtest

import (
	"fmt"
	"testing"
	"time"

	usercontract "github.com/dujiao-next/internal/modules/identity/user/contract"
	userstore "github.com/dujiao-next/internal/modules/identity/user/infrastructure/gormstore"

	userdomain "github.com/dujiao-next/internal/modules/identity/user/domain"

	settingsstore "github.com/dujiao-next/internal/modules/settings/infrastructure/gormstore"

	"github.com/dujiao-next/internal/config"
	"github.com/dujiao-next/internal/constants"
	emailverificationdomain "github.com/dujiao-next/internal/modules/identity/emailverification/domain"
	emailverificationstore "github.com/dujiao-next/internal/modules/identity/emailverification/infrastructure/gormstore"
	externalidentitydomain "github.com/dujiao-next/internal/modules/identity/externalidentity/domain"
	externalidentitystore "github.com/dujiao-next/internal/modules/identity/externalidentity/infrastructure/gormstore"
	"github.com/dujiao-next/internal/modules/identity/jwttoken"
	userauthapp "github.com/dujiao-next/internal/modules/identity/userauth/application"
	"github.com/dujiao-next/internal/modules/identity/userauth/challenge"
	usertotpapp "github.com/dujiao-next/internal/modules/identity/userauth/totp/application"

	"bytes"
	"context"
	"encoding/json"
	"github.com/dujiao-next/internal/app/container"
	"github.com/dujiao-next/internal/app/httpserver/middleware"
	userauthwiring "github.com/dujiao-next/internal/bootstrap/userauth"
	"github.com/dujiao-next/internal/cache"
	captchacontract "github.com/dujiao-next/internal/modules/captcha/contract"
	captchahttp "github.com/dujiao-next/internal/modules/captcha/transport/http"
	githubauthapp "github.com/dujiao-next/internal/modules/identity/githubauth/application"
	userauthhttp "github.com/dujiao-next/internal/modules/identity/userauth/transport/http"
	"github.com/dujiao-next/internal/shared/mailbrand"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/pquerna/otp/totp"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
)

func newUser2FATestServices(t *testing.T) (*userauthapp.Service, *usertotpapp.Service, usercontract.Store, *gorm.DB) {
	t.Helper()
	dsn := fmt.Sprintf("file:user_auth_2fa_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&userdomain.User{}, &externalidentitydomain.Identity{}, &emailverificationdomain.Code{}, &settingsstore.SettingRecord{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cfg := &config.Config{
		App: config.AppConfig{SecretKey: "test-app-secret-2fa"},
		UserJWT: config.JWTConfig{
			SecretKey:   "user-jwt-test-secret-2fa",
			ExpireHours: 24,
		},
	}
	userRepo := userstore.New(db)
	authSvc := userauthapp.NewService(
		cfg,
		userRepo,
		externalidentitystore.New(db),
		emailverificationstore.New(db),
		nil,
		nil,
		nil,
	)
	totpSvc := usertotpapp.NewService(cfg, userRepo, nil)
	return authSvc, totpSvc, userRepo, db
}

func createActiveUser(t *testing.T, repo usercontract.Store, email, password string) *userdomain.User {
	t.Helper()
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	now := time.Now()
	user := &userdomain.User{
		Email:           email,
		PasswordHash:    string(hash),
		DisplayName:     email,
		Status:          constants.UserStatusActive,
		EmailVerifiedAt: &now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := repo.Create(user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	return user
}

func TestUserLoginStep1WithoutTOTPReturnsToken(t *testing.T) {
	authSvc, _, repo, _ := newUser2FATestServices(t)
	createActiveUser(t, repo, "no2fa@example.com", "secret123")

	res, err := authSvc.LoginStep1("no2fa@example.com", "secret123", false)
	if err != nil {
		t.Fatalf("login step1: %v", err)
	}
	if res.RequiresTOTP {
		t.Fatalf("expected RequiresTOTP=false")
	}
	if res.Token == "" {
		t.Fatalf("expected jwt token")
	}
	if res.ChallengeToken != "" {
		t.Fatalf("did not expect challenge token, got %q", res.ChallengeToken)
	}
}

func TestUserLoginStep1WithTOTPReturnsChallenge(t *testing.T) {
	authSvc, totpSvc, repo, _ := newUser2FATestServices(t)
	user := createActiveUser(t, repo, "twofa@example.com", "secret123")
	setupRes, _ := totpSvc.Setup(user.ID)
	code, _ := totp.GenerateCode(setupRes.Secret, time.Now())
	if _, err := totpSvc.Enable(user.ID, code); err != nil {
		t.Fatalf("enable: %v", err)
	}

	res, err := authSvc.LoginStep1("twofa@example.com", "secret123", true)
	if err != nil {
		t.Fatalf("login step1: %v", err)
	}
	if !res.RequiresTOTP {
		t.Fatalf("expected RequiresTOTP=true")
	}
	if res.Token != "" {
		t.Fatalf("did not expect access token in challenge phase, got %q", res.Token)
	}
	if res.ChallengeToken == "" || res.ChallengeJTI == "" {
		t.Fatalf("expected challenge token + jti")
	}

	claims, err := authSvc.ParseUserChallengeToken(res.ChallengeToken)
	if err != nil {
		t.Fatalf("parse challenge: %v", err)
	}
	if claims.UserID != user.ID {
		t.Fatalf("user id mismatch: %d vs %d", claims.UserID, user.ID)
	}
	if claims.Purpose != challenge.PurposeTwoFactor {
		t.Fatalf("purpose mismatch: %s", claims.Purpose)
	}
	if claims.Typ != jwttoken.TypeTwoFactorChallenge {
		t.Fatalf("typ mismatch: %s", claims.Typ)
	}
	if !claims.RememberMe {
		t.Fatalf("expected remember_me=true to flow into challenge claims")
	}
}

func TestUserCompleteLoginAfter2FAIssuesAccessToken(t *testing.T) {
	authSvc, totpSvc, repo, _ := newUser2FATestServices(t)
	user := createActiveUser(t, repo, "complete@example.com", "secret123")
	setupRes, _ := totpSvc.Setup(user.ID)
	code, _ := totp.GenerateCode(setupRes.Secret, time.Now())
	if _, err := totpSvc.Enable(user.ID, code); err != nil {
		t.Fatalf("enable: %v", err)
	}

	res, err := authSvc.CompleteLoginAfter2FA(user.ID, false)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if res.Token == "" {
		t.Fatalf("expected access token")
	}
	parsed, err := authSvc.ParseUserJWT(res.Token)
	if err != nil {
		t.Fatalf("parse jwt: %v", err)
	}
	if parsed.UserID != user.ID {
		t.Fatalf("user id mismatch")
	}
	if parsed.Typ != jwttoken.TypeAccess {
		t.Fatalf("expected typ=access, got %q", parsed.Typ)
	}
}

func TestUserParseChallengeRejectsAccessToken(t *testing.T) {
	authSvc, _, repo, _ := newUser2FATestServices(t)
	user := createActiveUser(t, repo, "swap@example.com", "secret123")

	access, _, err := authSvc.GenerateUserJWT(user, 1)
	if err != nil {
		t.Fatalf("gen jwt: %v", err)
	}
	if _, err := authSvc.ParseUserChallengeToken(access); err == nil {
		t.Fatalf("expected error parsing access token as challenge token")
	}
}

func TestUserParseUserJWTRejectsChallengeToken(t *testing.T) {
	// 验证挑战 token 即便签名通过、字段名重叠，typ 也已显式标记为 2fa_challenge，
	// 不能再被当作访问 token；中间件会进一步基于 typ 拒绝。
	authSvc, _, repo, _ := newUser2FATestServices(t)
	user := createActiveUser(t, repo, "sniff@example.com", "secret123")
	challenge, _, _, err := authSvc.IssueUserChallengeToken(user.ID, false)
	if err != nil {
		t.Fatalf("issue challenge: %v", err)
	}
	parsed, err := authSvc.ParseUserJWT(challenge)
	if err != nil {
		// 部分 jwt 库会因签名通过 + claims 兼容而成功解析；这里允许两种结果
		// 但若解析成功，typ 必须是 2fa_challenge 而非 access
		return
	}
	if parsed.Typ == jwttoken.TypeAccess {
		t.Fatalf("challenge token must not present as access token; typ=%q", parsed.Typ)
	}
	if parsed.Typ != jwttoken.TypeTwoFactorChallenge {
		t.Fatalf("expected typ=2fa_challenge after parsing challenge token, got %q", parsed.Typ)
	}
}

func TestUserLoginStep1RejectsInvalidCredentials(t *testing.T) {
	authSvc, _, repo, _ := newUser2FATestServices(t)
	createActiveUser(t, repo, "wrong@example.com", "secret123")
	if _, err := authSvc.LoginStep1("wrong@example.com", "bad", false); err != userauthapp.ErrInvalidCredentials {
		t.Fatalf("expected invalid creds, got %v", err)
	}
	if _, err := authSvc.LoginStep1("none@example.com", "x", false); err != userauthapp.ErrInvalidCredentials {
		t.Fatalf("expected invalid creds for missing user, got %v", err)
	}
}

type auditReadInterleave struct {
	usercontract.Store
	afterRead func()
}

func (r *auditReadInterleave) GetByID(id uint) (*userdomain.User, error) {
	u, e := r.Store.GetByID(id)
	if r.afterRead != nil {
		f := r.afterRead
		r.afterRead = nil
		f()
	}
	return u, e
}
func TestSecurityProfilePreservesSecurityUpdates(t *testing.T) {
	for _, kind := range []string{"password", "disable", "totp"} {
		t.Run(kind, func(t *testing.T) {
			auth, otp, repo, _ := newUser2FATestServices(t)
			u := createActiveUser(t, repo, "race@example.test", "old-password")
			oldToken, _, e := auth.GenerateUserJWT(u, 1)
			if e != nil {
				t.Fatal(e)
			}
			interleave := &auditReadInterleave{Store: repo}
			interleave.afterRead = func() {
				switch kind {
				case "password":
					if e := auth.ChangePassword(u.ID, "old-password", "new-password"); e != nil {
						t.Fatal(e)
					}
				case "disable":
					if e := repo.BatchUpdateStatus([]uint{u.ID}, constants.UserStatusDisabled); e != nil {
						t.Fatal(e)
					}
				case "totp":
					setup, e := otp.Setup(u.ID)
					if e != nil {
						t.Fatal(e)
					}
					code, _ := totp.GenerateCode(setup.Secret, time.Now())
					if _, e = otp.Enable(u.ID, code); e != nil {
						t.Fatal(e)
					}
				}
				changed, _ := repo.GetByID(u.ID)
				if changed.TokenVersion == u.TokenVersion {
					t.Fatal("security change did not execute")
				}
			}
			profile := userauthapp.NewService(&config.Config{}, interleave, nil, nil, nil, nil, nil)
			name := "benign profile edit"
			if _, e := profile.UpdateProfile(u.ID, &name, nil); e != nil {
				t.Fatal(e)
			}
			after, _ := repo.GetByID(u.ID)
			if after.TokenVersion <= u.TokenVersion {
				t.Fatal("security epoch rolled back")
			}
			if kind == "disable" && after.Status != constants.UserStatusDisabled {
				t.Fatal("disable rolled back")
			}
			if kind == "totp" && after.TOTPEnabledAt == nil {
				t.Fatal("2FA rolled back")
			}
			if kind == "password" && bcrypt.CompareHashAndPassword([]byte(after.PasswordHash), []byte("new-password")) != nil {
				t.Fatal("password rolled back")
			}
			router := gin.New()
			router.GET("/private", middleware.UserJWTAuthMiddleware("user-jwt-test-secret-2fa", repo), func(c *gin.Context) { c.Status(204) })
			req := httptest.NewRequest(http.MethodGet, "/private", nil)
			req.Header.Set("Authorization", "Bearer "+oldToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code == 204 {
				t.Fatal("old token accepted after security change")
			}
			t.Logf("%s security change preserved", kind)
		})
	}
}

type auditSettings struct{}

func (auditSettings) GetEmailVerificationEnabled(bool) (bool, error) { return true, nil }
func (auditSettings) GetRegistrationEnabled(bool) (bool, error)      { return false, nil }

type auditLoginRecorder struct {
	userauthhttp.UserLoginAuth
	emails []string
}

func (s *auditLoginRecorder) LoginStep1(email, password string, remember bool) (*userauthhttp.AuthLoginResult, error) {
	s.emails = append(s.emails, email)
	return nil, userauthhttp.ErrInvalidCredentials
}
func TestSecurityLoginRateLimitCaseAliases(t *testing.T) {
	target := "victim@example.test"
	fake := &auditLoginRecorder{}
	handler := userauthhttp.NewUserLoginHandler(auditSettings{}, fake, nil, nil)
	r := gin.New()
	lim := middleware.RateLimitMiddleware(nil, middleware.RateLimitRule{Prefix: "audit", WindowSeconds: 60, MaxRequests: 2}, middleware.KeyByIPAndJSONField("email"))
	userauthhttp.RegisterUserLoginAuthRoutes(r.Group("/auth"), handler, lim)
	send := func(body string) int {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/auth/login", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w.Code
	}
	plain := `{"email":"victim@example.test","password":"invalid"}`
	send(plain)
	send(plain)
	if status := send(plain); status != 429 {
		t.Fatalf("normal limit did not trigger: %d", status)
	}
	for i := 0; i < 10; i++ {
		body := fmt.Sprintf(`{"email":"bucket-%d@example.test","Email":"victim@example.test","password":"invalid"}`, i)
		if status := send(body); status != 429 {
			t.Fatalf("alias request bypassed limit: %d status=%d", i, status)
		}
	}
	if len(fake.emails) != 2 {
		t.Fatalf("expected 2 backend attempts, got %d", len(fake.emails))
	}
	for _, email := range fake.emails {
		if email != target {
			t.Fatalf("unexpected target %q", email)
		}
	}
	t.Log("normal third request and ten case-aliased requests blocked in the same identity bucket")
}

type auditSender struct{ calls int }

func (s *auditSender) SendVerifyCode(_, _, _, _ string, _ mailbrand.Brand) error {
	s.calls++
	return nil
}

type auditCaptcha struct{ calls int }

func (s *auditCaptcha) Verify(_ string, _ captchahttp.CaptchaPayloadRequest, _ string) error {
	s.calls++
	return captchacontract.ErrRequired
}
func TestSecurityPublicEmailPurposeRejected(t *testing.T) {
	sender := &auditSender{}
	auth, _, _ := newRegistrationDomainPolicyAuthServiceWithSender(t, sender)
	cap := &auditCaptcha{}
	handler := userauthhttp.NewUserVerifyHandler(auditSettings{}, cap, auth)
	r := gin.New()
	userauthhttp.RegisterUserVerifyAuthRoutes(r.Group("/auth"), handler)
	payload, _ := json.Marshal(map[string]string{"email": "arbitrary@example.test", "purpose": constants.VerifyPurposeChangeEmailNew})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/auth/send-verify-code", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if sender.calls != 0 || cap.calls != 0 {
		t.Fatalf("sender=%d captcha=%d response=%s", sender.calls, cap.calls, w.Body.String())
	}
	for _, purpose := range []string{constants.VerifyPurposeChangeEmailOld, constants.VerifyPurposeChangeEmailNew} {
		if err := auth.SendVerifyCode(context.Background(), "another@example.test", purpose, "en"); err != userauthapp.ErrInvalidVerifyPurpose {
			t.Fatalf("public service accepts private purpose: %v", err)
		}
	}
	if sender.calls != 0 {
		t.Fatal("mail sent for private purpose")
	}
	_ = context.Background()
}

func TestSecurityAuthenticatedEmailChangeStillWorks(t *testing.T) {
	sender := &auditSender{}
	auth, _, db := newRegistrationDomainPolicyAuthServiceWithSender(t, sender)
	users := userstore.New(db)
	u := createActiveUser(t, users, "old-email@example.test", "fixture-password")
	const newEmail = "new-email@example.test"
	if err := auth.SendChangeEmailCode(context.Background(), u.ID, "old", "", "en"); err != nil {
		t.Fatal(err)
	}
	if err := auth.SendChangeEmailCode(context.Background(), u.ID, "new", newEmail, "en"); err != nil {
		t.Fatal(err)
	}
	codes := emailverificationstore.New(db)
	oldCode, err := codes.GetLatest(u.Email, constants.VerifyPurposeChangeEmailOld)
	if err != nil || oldCode == nil {
		t.Fatalf("old code missing: %v", err)
	}
	newCode, err := codes.GetLatest(newEmail, constants.VerifyPurposeChangeEmailNew)
	if err != nil || newCode == nil {
		t.Fatalf("new code missing: %v", err)
	}
	if _, err = auth.ChangeEmail(u.ID, newEmail, oldCode.Code, newCode.Code); err != nil {
		t.Fatal(err)
	}
	current, err := users.GetByID(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Email != newEmail || sender.calls != 2 {
		t.Fatal("authenticated email change failed")
	}
}

func TestSecurityUserChallengeRevokedByPasswordChange(t *testing.T) {
	auth, otp, repo, _ := newUser2FATestServices(t)
	u := createActiveUser(t, repo, "challenge@example.test", "old-password")
	setup, e := otp.Setup(u.ID)
	if e != nil {
		t.Fatal(e)
	}
	code, _ := totp.GenerateCode(setup.Secret, time.Now())
	if _, e = otp.Enable(u.ID, code); e != nil {
		t.Fatal(e)
	}
	first, e := auth.LoginStep1(u.Email, "old-password", false)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = auth.ParseUserChallengeToken(first.ChallengeToken); e != nil {
		t.Fatal(e)
	}
	if e = auth.ChangePassword(u.ID, "old-password", "new-password"); e != nil {
		t.Fatal(e)
	}
	if _, e = auth.ParseUserChallengeToken(first.ChallengeToken); e == nil {
		t.Fatal("pre-password-change challenge accepted")
	}
	fresh, e := auth.LoginStep1(u.Email, "new-password", false)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = auth.ParseUserChallengeToken(fresh.ChallengeToken); e != nil {
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
func TestSecurityStaleUserCacheCannotAuthorizeRevokedSession(t *testing.T) {
	auth, _, repo, _ := newUser2FATestServices(t)
	u := createActiveUser(t, repo, "cached@example.test", "old-password")
	token, _, err := auth.GenerateUserJWT(u, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err = cache.InitRedis(&config.RedisConfig{Enabled: true, Host: "127.0.0.1", Port: 1, Prefix: "isolated-user"}); err != nil {
		t.Fatal(err)
	}
	client := cache.Client()
	client.AddHook(&auditCacheHook{values: map[string]string{}})
	t.Cleanup(func() { _ = client.Close(); _ = cache.InitRedis(&config.RedisConfig{Enabled: false}) })
	if err = cache.SetUserAuthState(context.Background(), cache.BuildUserAuthState(u)); err != nil {
		t.Fatal(err)
	}
	if err = repo.BatchUpdateStatus([]uint{u.ID}, constants.UserStatusDisabled); err != nil {
		t.Fatal(err)
	}
	// Also models an old in-flight login repopulating Redis after invalidation.
	if err = cache.SetUserAuthState(context.Background(), cache.BuildUserAuthState(u)); err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.GET("/private", middleware.UserJWTAuthMiddleware("user-jwt-test-secret-2fa", repo), func(c *gin.Context) { c.Status(204) })
	req := httptest.NewRequest("GET", "/private", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code == 204 {
		t.Fatal("stale cache authorized revoked user")
	}
}

func TestSecurityCompletionRechecksFirstFactorEpoch(t *testing.T) {
	auth, otp, repo, _ := newUser2FATestServices(t)
	u := createActiveUser(t, repo, "finish@example.test", "old-password")
	setup, e := otp.Setup(u.ID)
	if e != nil {
		t.Fatal(e)
	}
	code, _ := totp.GenerateCode(setup.Secret, time.Now())
	if _, e = otp.Enable(u.ID, code); e != nil {
		t.Fatal(e)
	}
	first, e := auth.LoginStep1(u.Email, "old-password", false)
	if e != nil {
		t.Fatal(e)
	}
	claims, e := auth.ParseUserChallengeToken(first.ChallengeToken)
	if e != nil {
		t.Fatal(e)
	}
	if e = auth.ChangePassword(u.ID, "old-password", "new-password"); e != nil {
		t.Fatal(e)
	}
	if _, e = auth.CompleteLoginAfter2FA(u.ID, false, *claims.TokenVersion); e == nil {
		t.Fatal("completion upgraded stale first factor to new epoch")
	}
}

type securityContextSender struct {
	auditSender
	got context.Context
}

func (s *securityContextSender) SendVerifyCodeContext(ctx context.Context, _, _, _, _ string, _ mailbrand.Brand) error {
	s.got = ctx
	return ctx.Err()
}
func TestSecurityVerifyCodePreservesRequestCancellation(t *testing.T) {
	sender := &securityContextSender{}
	auth, _, _ := newRegistrationDomainPolicyAuthServiceWithSender(t, sender)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := auth.SendVerifyCode(ctx, "cancelled@example.test", constants.VerifyPurposeRegister, "en")
	if err != context.Canceled || sender.got != ctx || sender.calls != 0 {
		t.Fatalf("context lost: err=%v got=%v legacyCalls=%d", err, sender.got, sender.calls)
	}
}
func TestSecurityVirtualRepairCannotUndoPasswordSetup(t *testing.T) {
	auth, _, repo, _ := newUser2FATestServices(t)
	u := createActiveUser(t, repo, "telegram_123@login.local", "old-password")
	wrapped := &auditReadInterleave{Store: repo, afterRead: func() {
		if e := auth.ChangePassword(u.ID, "old-password", "new-password"); e != nil {
			t.Fatal(e)
		}
	}}
	service := userauthapp.NewService(&config.Config{}, wrapped, nil, nil, nil, nil, nil)
	_, _ = service.GetUserByID(u.ID)
	current, e := repo.GetByID(u.ID)
	if e != nil {
		t.Fatal(e)
	}
	if current.PasswordSetupRequired {
		t.Fatal("stale placeholder repair restored password-setup bypass")
	}
}

func TestSecurityWiredUser2FARejectsEpochChangeDuringCompletion(t *testing.T) {
	for _, recovery := range []bool{false, true} {
		t.Run(fmt.Sprintf("recovery=%v", recovery), func(t *testing.T) {
			auth, otp, repo, _ := newUser2FATestServices(t)
			u := createActiveUser(t, repo, "wired@example.test", "old-password")
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
			first, err := auth.LoginStep1(u.Email, "old-password", false)
			if err != nil {
				t.Fatal(err)
			}
			wrapped := &auditReadInterleave{Store: repo, afterRead: func() {
				if err := auth.ChangePassword(u.ID, "old-password", "new-password"); err != nil {
					t.Fatal(err)
				}
			}}
			cfg := &config.Config{UserJWT: config.JWTConfig{SecretKey: "user-jwt-test-secret-2fa", ExpireHours: 24}}
			wiredAuth := userauthapp.NewService(cfg, wrapped, nil, nil, nil, nil, nil)
			h := userauthwiring.New(&container.Container{UserAuthService: wiredAuth, UserTOTPService: otp, UserStore: repo, GitHubAuthService: &githubauthapp.Service{}}).TwoFA
			r := gin.New()
			r.POST("/verify", h.VerifyUser2FA)
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
			fresh, err := auth.LoginStep1(u.Email, "new-password", false)
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

type securityRuntimeEmailSender struct {
	auditSender
	cfg config.EmailConfig
}

func (s *securityRuntimeEmailSender) ConfigSnapshot() config.EmailConfig { return s.cfg }

func TestSecurityVerificationUsesRuntimeEmailPolicy(t *testing.T) {
	sender := &securityRuntimeEmailSender{}
	auth, _, db := newRegistrationDomainPolicyAuthServiceWithSender(t, sender)
	// Simulate the runtime SMTP update after auth was constructed. Startup config
	// remains unchanged; the SMTP sender now owns the synchronized runtime copy.
	sender.cfg.VerifyCode = config.VerifyCodeConfig{Length: 8, ExpireMinutes: 3, SendIntervalSeconds: 300, MaxAttempts: 1}
	const email = "runtime-policy@example.test"
	if err := auth.SendVerifyCode(context.Background(), email, constants.VerifyPurposeRegister, "en"); err != nil {
		t.Fatal(err)
	}
	repo := emailverificationstore.New(db)
	record, err := repo.GetLatest(email, constants.VerifyPurposeRegister)
	if err != nil || record == nil {
		t.Fatalf("record: %v", err)
	}
	if len(record.Code) != 8 || record.ExpiresAt.Sub(record.SentAt) != 3*time.Minute {
		t.Fatalf("runtime generation policy ignored: length=%d expiry=%s", len(record.Code), record.ExpiresAt.Sub(record.SentAt))
	}
	if err = db.Model(record).Update("sent_at", time.Now().Add(-2*time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	if err = auth.SendVerifyCode(context.Background(), email, constants.VerifyPurposeRegister, "en"); err != userauthapp.ErrVerifyCodeTooFrequent {
		t.Fatalf("runtime cooldown ignored: %v", err)
	}
	if _, _, _, err = auth.Register(email, "fixture-password", "not-the-code", true, true); err != userauthapp.ErrVerifyCodeInvalid {
		t.Fatalf("first wrong attempt: %v", err)
	}
	if _, _, _, err = auth.Register(email, "fixture-password", record.Code, true, true); err != userauthapp.ErrVerifyCodeAttemptsExceeded {
		t.Fatalf("runtime attempt limit ignored: %v", err)
	}
}

func TestSecurityVirtualAccountKeepsConfiguredPassword(t *testing.T) {
	auth, _, repo, _ := newUser2FATestServices(t)
	u := createActiveUser(t, repo, "telegram_456@login.local", "old-placeholder")
	if e := auth.ChangePassword(u.ID, "", "new-password"); e != nil {
		t.Fatal(e)
	}
	current, e := auth.GetUserByID(u.ID)
	if e != nil {
		t.Fatal(e)
	}
	if current.PasswordSetupRequired {
		t.Fatal("configured password lost its old-password requirement")
	}
	if e = auth.ChangePassword(u.ID, "wrong-password", "attacker-password"); e != userauthapp.ErrInvalidPassword {
		t.Fatalf("configured password change bypassed old factor: %v", e)
	}
}
