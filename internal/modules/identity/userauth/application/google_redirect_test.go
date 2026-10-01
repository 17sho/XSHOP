package application

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"strings"

	"github.com/dujiao-next/internal/constants"
	emailverificationcontract "github.com/dujiao-next/internal/modules/identity/emailverification/contract"
	emailverificationdomain "github.com/dujiao-next/internal/modules/identity/emailverification/domain"
	externalidentitycontract "github.com/dujiao-next/internal/modules/identity/externalidentity/contract"
	externalidentitydomain "github.com/dujiao-next/internal/modules/identity/externalidentity/domain"
	usercontract "github.com/dujiao-next/internal/modules/identity/user/contract"
	userdomain "github.com/dujiao-next/internal/modules/identity/user/domain"
	"github.com/dujiao-next/internal/telegramidentity"
	"github.com/golang-jwt/jwt/v5"
	"sync"
	"testing"
	"time"

	"github.com/dujiao-next/internal/config"
	googleauthapp "github.com/dujiao-next/internal/modules/identity/googleauth/application"
	settingsapp "github.com/dujiao-next/internal/modules/settings/application"
	settingssecurity "github.com/dujiao-next/internal/modules/settings/schema/security"
	"github.com/dujiao-next/internal/shared/jsonmap"
)

type channelPolicyUsers struct {
	usercontract.Store
	target              *userdomain.User
	afterEmail, afterID func()
}

func (s *channelPolicyUsers) GetByEmail(string) (*userdomain.User, error) {
	if s.afterEmail != nil {
		s.afterEmail()
	}
	return s.target, nil
}
func (s *channelPolicyUsers) GetByID(uint) (*userdomain.User, error) {
	if s.afterID != nil {
		s.afterID()
	}
	return &userdomain.User{ID: 2, Email: telegramidentity.BuildPlaceholderEmail("919181"), Status: constants.UserStatusActive}, nil
}

type channelPolicyCodes struct {
	emailverificationcontract.Store
}

func (*channelPolicyCodes) GetLatest(string, string) (*emailverificationdomain.Code, error) {
	return &emailverificationdomain.Code{Code: "123456", ExpiresAt: time.Now().Add(time.Hour)}, nil
}
func (*channelPolicyCodes) MarkVerified(uint, time.Time) error { return nil }

type channelPolicyIdentities struct {
	externalidentitycontract.Store
	identity    *externalidentitydomain.Identity
	afterLookup func()
	writes      int
}

func (*channelPolicyIdentities) GetByUserProvider(uint, string) (*externalidentitydomain.Identity, error) {
	return nil, nil
}
func (s *channelPolicyIdentities) GetByProviderUserID(string, string) (*externalidentitydomain.Identity, error) {
	if s.afterLookup != nil {
		s.afterLookup()
	}
	if s.identity == nil {
		return nil, nil
	}
	copy := *s.identity
	return &copy, nil
}
func (s *channelPolicyIdentities) Update(identity *externalidentitydomain.Identity) error {
	s.writes++
	copy := *identity
	s.identity = &copy
	return nil
}
func (s *channelPolicyIdentities) Create(identity *externalidentitydomain.Identity) error {
	return s.Update(identity)
}

func TestSecurityCenterChannelFinalPolicyBeforeWrites(t *testing.T) {
	for _, stage := range []string{"users", "create", "update", "reassign"} {
		t.Run(stage, func(t *testing.T) {
			policy := &securityCenterTestStore{}
			disable := func() { policy.value = jsonmap.JSON{"telegram_binding": false} }
			users := &channelPolicyUsers{target: &userdomain.User{ID: 1, Email: "buyer@example.com", Status: constants.UserStatusActive}}
			identities := &channelPolicyIdentities{}
			switch stage {
			case "users":
				users.afterEmail = disable
			case "create":
				identities.afterLookup = disable
			case "update":
				identities.identity = &externalidentitydomain.Identity{UserID: 1, ProviderUserID: "919181", Username: "old"}
				identities.afterLookup = disable
			case "reassign":
				identities.identity = &externalidentitydomain.Identity{UserID: 2, ProviderUserID: "919181"}
				users.afterID = disable
			}
			service := &Service{cfg: &config.Config{}, settingService: settingsapp.NewService(policy), userRepo: users, userOAuthIdentityRepo: identities, codeRepo: &channelPolicyCodes{}}
			_, binding, _, err := service.BindTelegramChannelByEmailCode(BindTelegramChannelByEmailCodeInput{Email: "buyer@example.com", Code: "123456", Identity: TelegramChannelIdentityInput{ChannelUserID: "919181", Username: "new"}})
			if !errors.Is(err, settingssecurity.ErrSecurityCenterDisabled) || binding != nil || identities.writes != 0 {
				t.Fatalf("stage=%s err=%v binding=%+v writes=%d", stage, err, binding, identities.writes)
			}
			if identities.identity != nil && (identities.identity.Username == "new" || stage == "reassign" && identities.identity.UserID != 2) {
				t.Fatal("persisted identity changed")
			}
			users.afterEmail, users.afterID, identities.afterLookup = nil, nil, nil
			policy.value = nil
			if _, _, _, err := service.BindTelegramChannelByEmailCode(BindTelegramChannelByEmailCodeInput{Email: "buyer@example.com", Code: "123456", Identity: TelegramChannelIdentityInput{ChannelUserID: "919181", Username: "new"}}); err != nil || identities.writes != 1 {
				t.Fatalf("enabled bind err=%v writes=%d", err, identities.writes)
			}
		})
	}
}

func TestSecurityCenterChannelProvisionAndResolveRemainAvailable(t *testing.T) {
	for _, provision := range []bool{false, true} {
		policy := &securityCenterTestStore{value: jsonmap.JSON{"telegram_binding": false}}
		identities := &channelPolicyIdentities{identity: &externalidentitydomain.Identity{UserID: 2, ProviderUserID: "919181", Username: "old"}}
		service := &Service{settingService: settingsapp.NewService(policy), userRepo: &channelPolicyUsers{}, userOAuthIdentityRepo: identities}
		input := TelegramChannelIdentityInput{ChannelUserID: "919181", Username: "new"}
		if provision {
			user, identity, created, err := service.ProvisionTelegramChannelIdentity(input)
			if err != nil || user == nil || identity == nil || created {
				t.Fatalf("provision user=%+v identity=%+v created=%t err=%v", user, identity, created, err)
			}
		} else {
			user, identity, err := service.ResolveTelegramChannelIdentity(input)
			if err != nil || user == nil || identity == nil {
				t.Fatalf("resolve user=%+v identity=%+v err=%v", user, identity, err)
			}
		}
		if identities.writes != 1 || identities.identity.Username != "new" {
			t.Fatal("channel profile refresh gated by binding flag")
		}
	}
}

type redirectPolicyTransport func(*http.Request) (*http.Response, error)

func (f redirectPolicyTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSecurityCenterGoogleRedirectToggleDuringVerification(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{"iss": "https://accounts.google.com", "sub": "subject", "aud": "client-id", "exp": now.Add(time.Hour).Unix(), "iat": now.Add(-time.Minute).Unix(), "email": "buyer@gmail.com", "email_verified": true})
	token.Header["kid"] = "test"
	credential, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	jwks, err := json.Marshal(map[string]interface{}{"keys": []map[string]string{{"kid": "test", "kty": "RSA", "alg": "RS256", "use": "sig", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, flow := range []string{GoogleRedirectFlowBind, GoogleRedirectFlowLogin} {
		t.Run(flow, func(t *testing.T) {
			policy := &securityCenterTestStore{}
			verified := false
			client := &http.Client{Transport: redirectPolicyTransport(func(*http.Request) (*http.Response, error) {
				verified = true
				policy.value = jsonmap.JSON{"google_binding": false}
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(jwks)))}, nil
			})}
			store := newFakeGoogleRedirectStore()
			service := &Service{settingService: settingsapp.NewService(policy), googleRedirectStore: store, googleAuthService: googleauthapp.NewService(config.GoogleAuthConfig{Enabled: true, ClientID: "client-id"}, googleauthapp.WithHTTPClient(client), googleauthapp.WithJWKSURL("https://fixture.invalid/jwks"))}
			tenant := GoogleRedirectTenant{Host: "shop.example.com", IsMain: true}
			state, err := service.CreateGoogleRedirectIntent(context.Background(), flow, 1, tenant)
			if err != nil {
				t.Fatal(err)
			}
			completion, err := service.CompleteGoogleRedirect(context.Background(), state, credential, tenant)
			if !verified || completion == nil || completion.Flow != flow || len(store.intents) != 0 {
				t.Fatalf("verification/intent completion=%+v err=%v", completion, err)
			}
			if flow == GoogleRedirectFlowBind {
				if !errors.Is(err, settingssecurity.ErrSecurityCenterDisabled) || completion.HandoffHandle != "" || len(store.handoffs) != 0 {
					t.Fatalf("revoked bind completion=%+v err=%v handoffs=%d", completion, err, len(store.handoffs))
				}
			} else if err != nil || completion.HandoffHandle == "" || len(store.handoffs) != 1 {
				t.Fatalf("login changed completion=%+v err=%v", completion, err)
			}
		})
	}
}

type securityCenterTestStore struct {
	value jsonmap.JSON
	err   error
}

func (s *securityCenterTestStore) GetByKey(key string) (jsonmap.JSON, bool, error) {
	return s.value, s.value != nil, s.err
}
func (s *securityCenterTestStore) Upsert(key string, value jsonmap.JSON) (jsonmap.JSON, error) {
	s.value = value
	return value, nil
}

func TestSecurityCenterBlocksRawApplicationEntryPoints(t *testing.T) {
	store := &securityCenterTestStore{value: jsonmap.JSON{"email_change": false, "password_change": false, "telegram_binding": false, "google_binding": false}}
	service := &Service{settingService: settingsapp.NewService(store)}
	calls := map[string]func() error{
		"email_send": func() error {
			return service.SendChangeEmailCode(context.Background(), 1, "new", "new@example.com", "en-US")
		},
		"email_change":     func() error { _, err := service.ChangeEmail(1, "new@example.com", "1", "2"); return err },
		"password":         func() error { return service.ChangePassword(1, "old", "new") },
		"telegram_widget":  func() error { _, err := service.BindTelegram(BindTelegramInput{UserID: 1}); return err },
		"telegram_miniapp": func() error { _, err := service.BindTelegramMiniApp(BindTelegramMiniAppInput{UserID: 1}); return err },
		"telegram_oidc_start": func() error {
			_, err := service.StartTelegramOIDC(StartTelegramOIDCInput{Intent: "bind", UserID: 1})
			return err
		},
		"telegram_oidc_complete": func() error { _, err := service.BindTelegramOIDC(BindTelegramOIDCInput{UserID: 1}); return err },
		"telegram_channel": func() error {
			_, _, _, err := service.BindTelegramChannelByEmailCode(BindTelegramChannelByEmailCodeInput{})
			return err
		},
		"telegram_bind_code": func() error {
			return service.SendVerifyCode(context.Background(), "test@example.com", "telegram_bind", "en-US")
		},
		"telegram_unbind":   func() error { return service.UnbindTelegram(1) },
		"google_credential": func() error { _, err := service.BindGoogle(BindGoogleInput{UserID: 1}); return err },
		"google_start": func() error {
			_, err := service.CreateGoogleRedirectIntent(context.Background(), "bind", 1, GoogleRedirectTenant{})
			return err
		},
		"google_exchange": func() error {
			_, err := service.ExchangeGoogleRedirectBind(context.Background(), "", 1, GoogleRedirectTenant{})
			return err
		},
		"google_unbind": func() error { return service.UnbindGoogle(1) },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, settingssecurity.ErrSecurityCenterDisabled) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestSecurityCenterIndependentFlagsFallbackAndReadFailure(t *testing.T) {
	store := &securityCenterTestStore{}
	settings := settingsapp.NewService(store)
	for _, key := range settingssecurity.SecurityCenterKeys() {
		if err := settings.CheckSecurityCenterFeature(key); err != nil {
			t.Fatalf("missing default %s: %v", key, err)
		}
	}
	for _, enabledKey := range settingssecurity.SecurityCenterKeys() {
		raw := jsonmap.JSON{}
		for _, key := range settingssecurity.SecurityCenterKeys() {
			raw[key] = key == enabledKey
		}
		store.value = raw
		for _, key := range settingssecurity.SecurityCenterKeys() {
			err := settings.CheckSecurityCenterFeature(key)
			if (err == nil) != (key == enabledKey) {
				t.Fatalf("flag coupling enabled=%s checked=%s err=%v", enabledKey, key, err)
			}
		}
	}
	store.value = jsonmap.JSON{"email_change": "true"}
	if err := settings.CheckSecurityCenterFeature("email_change"); err == nil {
		t.Fatal("malformed persisted flag enabled")
	}
	store.err = errors.New("settings read failed")
	for _, key := range settingssecurity.SecurityCenterKeys() {
		if err := settings.CheckSecurityCenterFeature(key); !errors.Is(err, settingssecurity.ErrSecurityCenterDisabled) || !errors.Is(err, store.err) {
			t.Fatalf("read failure allowed %s: %v", key, err)
		}
	}
}

func TestSecurityCenterGoogleRedirectToggleBeforeCompletionAndExchange(t *testing.T) {
	policy := &securityCenterTestStore{}
	store := newFakeGoogleRedirectStore()
	service := &Service{settingService: settingsapp.NewService(policy), googleRedirectStore: store, googleAuthService: googleauthapp.NewService(config.GoogleAuthConfig{Enabled: true, ClientID: "client-id"})}
	tenant := GoogleRedirectTenant{Host: "shop.example.com", IsMain: true}
	state, err := service.CreateGoogleRedirectIntent(context.Background(), "bind", 1, tenant)
	if err != nil {
		t.Fatal(err)
	}
	policy.value = jsonmap.JSON{"google_binding": false}
	completion, err := service.CompleteGoogleRedirect(context.Background(), state, "not-a-real-credential", tenant)
	if !errors.Is(err, settingssecurity.ErrSecurityCenterDisabled) || completion == nil || completion.HandoffHandle != "" || len(store.handoffs) != 0 {
		t.Fatalf("midflow completion=%+v err=%v", completion, err)
	}
	handle, _ := newGoogleRedirectHandle()
	store.handoffs[handle] = GoogleRedirectHandoff{Flow: "bind", UserID: 1, Tenant: tenant}
	if _, err := service.ExchangeGoogleRedirectBind(context.Background(), handle, 1, tenant); !errors.Is(err, settingssecurity.ErrSecurityCenterDisabled) || store.takeHandoff != 0 {
		t.Fatalf("exchange bypass %v takes=%d", err, store.takeHandoff)
	}
	if _, err := service.CreateGoogleRedirectIntent(context.Background(), "login", 0, tenant); err != nil {
		t.Fatalf("binding flag disabled login: %v", err)
	}
	policy.value = jsonmap.JSON{"google_binding": true, "telegram_binding": false, "email_change": false, "password_change": false, "two_factor": false, "login_history": false}
	if _, err := service.CreateGoogleRedirectIntent(context.Background(), "bind", 1, tenant); err != nil {
		t.Fatalf("positive isolated flag: %v", err)
	}
}

type fakeGoogleRedirectStore struct {
	mu           sync.Mutex
	intents      map[string]GoogleRedirectIntent
	handoffs     map[string]GoogleRedirectHandoff
	takeIntent   int
	takeHandoff  int
	intentTTL    time.Duration
	handoffTTL   time.Duration
	putIntentErr error
}

func newFakeGoogleRedirectStore() *fakeGoogleRedirectStore {
	return &fakeGoogleRedirectStore{
		intents:  make(map[string]GoogleRedirectIntent),
		handoffs: make(map[string]GoogleRedirectHandoff),
	}
}

func (s *fakeGoogleRedirectStore) PutIntent(
	_ context.Context,
	state string,
	intent GoogleRedirectIntent,
	ttl time.Duration,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.putIntentErr != nil {
		return s.putIntentErr
	}
	s.intents[state] = intent
	s.intentTTL = ttl
	return nil
}

func (s *fakeGoogleRedirectStore) TakeIntent(
	_ context.Context,
	state string,
) (*GoogleRedirectIntent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.takeIntent++
	intent, ok := s.intents[state]
	if !ok {
		return nil, nil
	}
	delete(s.intents, state)
	copyValue := intent
	return &copyValue, nil
}

func (s *fakeGoogleRedirectStore) PutHandoff(
	_ context.Context,
	handle string,
	handoff GoogleRedirectHandoff,
	ttl time.Duration,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handoffs[handle] = handoff
	s.handoffTTL = ttl
	return nil
}

func (s *fakeGoogleRedirectStore) TakeHandoff(
	_ context.Context,
	handle string,
) (*GoogleRedirectHandoff, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.takeHandoff++
	handoff, ok := s.handoffs[handle]
	if !ok {
		return nil, nil
	}
	delete(s.handoffs, handle)
	copyValue := handoff
	return &copyValue, nil
}

func TestCreateGoogleRedirectIntentUsesCanonicalStateAndTenMinuteTTL(t *testing.T) {
	store := newFakeGoogleRedirectStore()
	service := &Service{
		googleAuthService: googleauthapp.NewService(config.GoogleAuthConfig{
			Enabled:  true,
			ClientID: "client-id",
		}),
		googleRedirectStore: store,
	}
	tenant := GoogleRedirectTenant{Host: "shop.example.com", IsMain: true}

	state, err := service.CreateGoogleRedirectIntent(
		context.Background(),
		GoogleRedirectFlowLogin,
		0,
		tenant,
	)
	if err != nil {
		t.Fatalf("CreateGoogleRedirectIntent() error = %v", err)
	}
	if !ValidGoogleRedirectHandle(state) {
		t.Fatalf("state %q is not a canonical 32-byte base64url handle", state)
	}
	if store.intentTTL != GoogleRedirectIntentTTL {
		t.Fatalf("intent TTL = %s, want %s", store.intentTTL, GoogleRedirectIntentTTL)
	}
	if stored := store.intents[state]; stored.Flow != GoogleRedirectFlowLogin || stored.Tenant != tenant {
		t.Fatalf("stored intent = %#v", stored)
	}
}

func TestCreateGoogleRedirectIntentFailsClosedWhenStoreFails(t *testing.T) {
	store := newFakeGoogleRedirectStore()
	store.putIntentErr = errors.New("redis down")
	service := &Service{
		googleAuthService: googleauthapp.NewService(config.GoogleAuthConfig{
			Enabled:  true,
			ClientID: "client-id",
		}),
		googleRedirectStore: store,
	}

	_, err := service.CreateGoogleRedirectIntent(
		context.Background(),
		GoogleRedirectFlowLogin,
		0,
		GoogleRedirectTenant{Host: "shop.example.com", IsMain: true},
	)
	if !errors.Is(err, ErrGoogleRedirectUnavailable) {
		t.Fatalf("error = %v, want ErrGoogleRedirectUnavailable", err)
	}
}

func TestGoogleRedirectMalformedHandleDoesNotReachStore(t *testing.T) {
	store := newFakeGoogleRedirectStore()
	service := &Service{googleRedirectStore: store}
	tenant := GoogleRedirectTenant{Host: "shop.example.com", IsMain: true}

	if _, err := service.takeGoogleRedirectHandoff(
		context.Background(),
		"not-a-valid-handle",
		GoogleRedirectFlowLogin,
		0,
		tenant,
	); !errors.Is(err, ErrGoogleRedirectSessionExpired) {
		t.Fatalf("error = %v, want ErrGoogleRedirectSessionExpired", err)
	}
	if store.takeHandoff != 0 {
		t.Fatalf("TakeHandoff calls = %d, want 0", store.takeHandoff)
	}
	if _, err := service.CompleteGoogleRedirect(
		context.Background(),
		string(make([]byte, 4096)),
		"credential",
		tenant,
	); !errors.Is(err, ErrGoogleRedirectSessionExpired) {
		t.Fatalf("CompleteGoogleRedirect error = %v, want ErrGoogleRedirectSessionExpired", err)
	}
	if store.takeIntent != 0 {
		t.Fatalf("TakeIntent calls = %d, want 0", store.takeIntent)
	}
}

func TestCompleteGoogleRedirectReturnsConsumedTrustedFlowOnCredentialFailure(t *testing.T) {
	store := newFakeGoogleRedirectStore()
	service := &Service{
		googleAuthService: googleauthapp.NewService(config.GoogleAuthConfig{
			Enabled:  true,
			ClientID: "client-id",
		}),
		googleRedirectStore: store,
	}
	state, err := newGoogleRedirectHandle()
	if err != nil {
		t.Fatalf("newGoogleRedirectHandle() error = %v", err)
	}
	tenant := GoogleRedirectTenant{Host: "shop.example.com", IsMain: true}
	store.intents[state] = GoogleRedirectIntent{
		Flow:   GoogleRedirectFlowBind,
		UserID: 42,
		Tenant: tenant,
	}

	completion, err := service.CompleteGoogleRedirect(
		context.Background(),
		state,
		"",
		tenant,
	)
	if !errors.Is(err, googleauthapp.ErrGoogleCredentialInvalid) {
		t.Fatalf("error = %v, want invalid credential", err)
	}
	if completion == nil || completion.Flow != GoogleRedirectFlowBind || completion.HandoffHandle != "" {
		t.Fatalf("completion = %#v, want trusted bind flow without handoff", completion)
	}
	if _, exists := store.intents[state]; exists {
		t.Fatalf("credential failure restored consumed intent")
	}
}

func TestCompleteGoogleRedirectReturnsConsumedTrustedFlowOnTenantFailure(t *testing.T) {
	store := newFakeGoogleRedirectStore()
	service := &Service{googleRedirectStore: store}
	state, err := newGoogleRedirectHandle()
	if err != nil {
		t.Fatalf("newGoogleRedirectHandle() error = %v", err)
	}
	store.intents[state] = GoogleRedirectIntent{
		Flow:   GoogleRedirectFlowBind,
		UserID: 42,
		Tenant: GoogleRedirectTenant{Host: "shop.example.com", IsMain: true},
	}

	completion, err := service.CompleteGoogleRedirect(
		context.Background(),
		state,
		"credential-must-not-be-verified",
		GoogleRedirectTenant{Host: "other.example.com", IsMain: true},
	)
	if !errors.Is(err, ErrGoogleRedirectTenantMismatch) {
		t.Fatalf("error = %v, want tenant mismatch", err)
	}
	if completion == nil || completion.Flow != GoogleRedirectFlowBind {
		t.Fatalf("completion = %#v, want trusted bind flow", completion)
	}
	if _, exists := store.intents[state]; exists {
		t.Fatalf("tenant failure restored consumed intent")
	}
}

func TestGoogleRedirectTenantMismatchConsumesHandoff(t *testing.T) {
	service, store, handle := redirectExchangeFixture(t, GoogleRedirectFlowLogin, 0)

	_, err := service.ExchangeGoogleRedirectLogin(
		context.Background(),
		handle,
		GoogleRedirectTenant{Host: "other.example.com", IsMain: true},
	)
	if !errors.Is(err, ErrGoogleRedirectTenantMismatch) {
		t.Fatalf("first error = %v, want tenant mismatch", err)
	}
	_, err = service.ExchangeGoogleRedirectLogin(
		context.Background(),
		handle,
		GoogleRedirectTenant{Host: "shop.example.com", IsMain: true},
	)
	if !errors.Is(err, ErrGoogleRedirectSessionExpired) {
		t.Fatalf("second error = %v, want expired", err)
	}
	if store.takeHandoff != 2 {
		t.Fatalf("TakeHandoff calls = %d, want 2", store.takeHandoff)
	}
}

func TestGoogleRedirectBindUserMismatchConsumesHandoff(t *testing.T) {
	service, _, handle := redirectExchangeFixture(t, GoogleRedirectFlowBind, 42)

	_, err := service.ExchangeGoogleRedirectBind(
		context.Background(),
		handle,
		43,
		GoogleRedirectTenant{Host: "shop.example.com", IsMain: true},
	)
	if !errors.Is(err, ErrGoogleRedirectUserMismatch) {
		t.Fatalf("first error = %v, want user mismatch", err)
	}
	_, err = service.ExchangeGoogleRedirectBind(
		context.Background(),
		handle,
		42,
		GoogleRedirectTenant{Host: "shop.example.com", IsMain: true},
	)
	if !errors.Is(err, ErrGoogleRedirectSessionExpired) {
		t.Fatalf("second error = %v, want expired", err)
	}
}

func TestGoogleRedirectExchangeConsumesHandoffWhenProviderWasDisabled(t *testing.T) {
	service, _, handle := redirectExchangeFixture(t, GoogleRedirectFlowLogin, 0)
	service.googleAuthService.SetConfig(config.GoogleAuthConfig{
		Enabled:  false,
		ClientID: "client-id",
	})

	_, err := service.ExchangeGoogleRedirectLogin(
		context.Background(),
		handle,
		GoogleRedirectTenant{Host: "shop.example.com", IsMain: true},
	)
	if !errors.Is(err, googleauthapp.ErrGoogleAuthDisabled) {
		t.Fatalf("first error = %v, want Google auth disabled", err)
	}
	_, err = service.ExchangeGoogleRedirectLogin(
		context.Background(),
		handle,
		GoogleRedirectTenant{Host: "shop.example.com", IsMain: true},
	)
	if !errors.Is(err, ErrGoogleRedirectSessionExpired) {
		t.Fatalf("second error = %v, want expired", err)
	}
}

func TestGoogleRedirectExchangeRejectsHandoffFromPreviousClientID(t *testing.T) {
	service, _, handle := redirectExchangeFixture(t, GoogleRedirectFlowLogin, 0)
	service.googleAuthService.SetConfig(config.GoogleAuthConfig{
		Enabled:  true,
		ClientID: "rotated-client-id",
	})

	_, err := service.ExchangeGoogleRedirectLogin(
		context.Background(),
		handle,
		GoogleRedirectTenant{Host: "shop.example.com", IsMain: true},
	)
	if !errors.Is(err, ErrGoogleRedirectSessionExpired) {
		t.Fatalf("error = %v, want expired after client ID rotation", err)
	}
}

func redirectExchangeFixture(
	t *testing.T,
	flow string,
	userID uint,
) (*Service, *fakeGoogleRedirectStore, string) {
	t.Helper()
	handle, err := newGoogleRedirectHandle()
	if err != nil {
		t.Fatalf("newGoogleRedirectHandle() error = %v", err)
	}
	store := newFakeGoogleRedirectStore()
	store.handoffs[handle] = GoogleRedirectHandoff{
		Flow:   flow,
		UserID: userID,
		Tenant: GoogleRedirectTenant{Host: "shop.example.com", IsMain: true},
		Identity: googleauthapp.VerifiedIdentity{
			Sub:                "google-sub",
			Email:              "person@gmail.com",
			ClientID:           "client-id",
			EmailAuthoritative: true,
			AuthAt:             time.Now(),
		},
		CreatedAt: time.Now(),
	}
	service := &Service{
		googleAuthService: googleauthapp.NewService(config.GoogleAuthConfig{
			Enabled:  true,
			ClientID: "client-id",
		}),
		googleRedirectStore: store,
	}
	return service, store, handle
}
