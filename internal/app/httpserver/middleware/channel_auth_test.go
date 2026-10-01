package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dujiao-next/internal/app/container"
	"github.com/dujiao-next/internal/constants"
	apicredentialdomain "github.com/dujiao-next/internal/modules/apicredential/domain"
	channelhttp "github.com/dujiao-next/internal/modules/channelapi/transport/http"
	channelclientapp "github.com/dujiao-next/internal/modules/channelclient/application"
	channelclientcontract "github.com/dujiao-next/internal/modules/channelclient/contract"
	channelclientdomain "github.com/dujiao-next/internal/modules/channelclient/domain"
	userdomain "github.com/dujiao-next/internal/modules/identity/user/domain"
	orderdomain "github.com/dujiao-next/internal/modules/order/domain"
	"github.com/dujiao-next/internal/upstream"
	"github.com/gin-gonic/gin"
)

const channelVersionHeaderFixture = "Dujiao-Next-Channel-Signature-Version"

// Only the exercised methods are implemented; credentials are synthetic and
// last-used writes are a no-op so asynchronous bookkeeping cannot race fixtures.
type channelAuthFixtureStore struct {
	channelclientcontract.Store
	client *channelclientdomain.Client
}

func (s *channelAuthFixtureStore) Create(client *channelclientdomain.Client) error {
	client.ID = 1
	copy := *client
	s.client = &copy
	return nil
}

func (s *channelAuthFixtureStore) FindByChannelKey(key string) (*channelclientdomain.Client, error) {
	if s.client == nil || s.client.ChannelKey != key {
		return nil, nil
	}
	copy := *s.client
	return &copy, nil
}

func (*channelAuthFixtureStore) UpdateLastUsed(uint, time.Time) error { return nil }

type channelAuthFixture struct {
	router *gin.Engine
	client *channelclientapp.ClientDetail
	stamp  int64
	calls  int
	body   string
}

func newChannelAuthFixture(t *testing.T) *channelAuthFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	service := channelclientapp.NewService(&channelAuthFixtureStore{}, "fixture-app-key")
	client, err := service.CreateChannelClient("fixture", "telegram_bot", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	f := &channelAuthFixture{client: client, stamp: time.Now().Unix(), router: gin.New()}
	f.router.Use(ChannelAPIAuthMiddleware(&container.Container{ChannelClientService: service}))
	f.router.Any("/api/v1/channel/*rest", func(c *gin.Context) {
		f.calls++
		if c.GetUint(channelClientIDKey) != client.ID || c.GetString(channelTypeCtxKey) != "telegram_bot" {
			t.Error("authenticated channel context missing")
		}
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			t.Error(err)
		}
		f.body = string(body)
		c.Status(http.StatusNoContent)
	})
	return f
}

func (f *channelAuthFixture) request(method, target, body, version, signature string) *httptest.ResponseRecorder {
	return f.requestWithHeaders(method, target, body, version, signature, nil)
}

func (f *channelAuthFixture) requestWithHeaders(method, target, body, version, signature string, edit func(http.Header)) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set(channelHeaderKey, f.client.ChannelKey)
	req.Header.Set(channelHeaderTimestamp, strconv.FormatInt(f.stamp, 10))
	req.Header.Set(channelHeaderSignature, signature)
	if version != "" {
		req.Header.Set(channelVersionHeaderFixture, version)
	}
	if edit != nil {
		edit(req.Header)
	}
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req)
	return w
}

// Construct protocol fixtures independently of the implementation under test.
func (f *channelAuthFixture) signV2(method, canonicalTarget, body string) string {
	return upstream.Sign(f.client.ChannelSecret, method,
		"dujiao-next-channel-v2\n"+f.client.ChannelKey+"\n"+canonicalTarget,
		f.stamp, []byte(body))
}

func TestChannelAuthRejectsDuplicateAuthHeaders(t *testing.T) {
	for _, header := range []string{channelHeaderKey, channelHeaderTimestamp, channelHeaderSignature, channelVersionHeaderFixture} {
		t.Run(header, func(t *testing.T) {
			f := newChannelAuthFixture(t)
			path := "/api/v1/channel/orders"
			sig := f.signV2(http.MethodGet, path+"?channel_user_id=111", "")
			w := f.requestWithHeaders(http.MethodGet, path+"?channel_user_id=111", "", "2", sig, func(h http.Header) {
				h.Add(header, h.Get(header))
			})
			if w.Code != http.StatusUnauthorized || f.calls != 0 {
				t.Fatalf("duplicate authentication header accepted: status=%d calls=%d", w.Code, f.calls)
			}
		})
	}
}

func TestChannelAuthCanonicalQuerySignature(t *testing.T) {
	f := newChannelAuthFixture(t)
	path := "/api/v1/channel/orders/900"
	sig := f.signV2(http.MethodGet, path+"?channel_user_id=111&locale=en+US", "")
	w := f.request(http.MethodGet, path+"?locale=en%20US&channel_user_id=111", "", "2", sig)
	if w.Code != http.StatusNoContent || f.calls != 1 {
		t.Fatalf("canonical signed query rejected: status=%d handler calls=%d", w.Code, f.calls)
	}
	w = f.request(http.MethodGet, path+"?locale=en%20US&channel_user_id=222", "", "2", sig)
	if w.Code != http.StatusUnauthorized || f.calls != 1 {
		t.Fatalf("changed identity reached handler: status=%d handler calls=%d", w.Code, f.calls)
	}
}

func TestChannelAuthBindsEveryQueryParameter(t *testing.T) {
	for _, identity := range []string{"channel_user_id", "telegram_user_id"} {
		t.Run(identity, func(t *testing.T) {
			f := newChannelAuthFixture(t)
			path := "/api/v1/channel/orders"
			query := url.Values{identity: {"111"}, "page": {"1"}, "page_size": {"5"}, "status": {"completed"}, "locale": {"en"}, "username": {"fixture"}, "avatar_url": {"https://example.test/avatar"}, "future_parameter": {"value"}}
			sig := f.signV2(http.MethodGet, path+"?"+query.Encode(), "")
			if w := f.request(http.MethodGet, path+"?"+query.Encode(), "", "2", sig); w.Code != http.StatusNoContent {
				t.Fatalf("legitimate %s query rejected: %d", identity, w.Code)
			}
			for key := range query {
				original := query.Get(key)
				query.Set(key, "changed")
				if w := f.request(http.MethodGet, path+"?"+query.Encode(), "", "2", sig); w.Code != http.StatusUnauthorized {
					t.Errorf("changed %s accepted: %d", key, w.Code)
				}
				query.Del(key)
				if w := f.request(http.MethodGet, path+"?"+query.Encode(), "", "2", sig); w.Code != http.StatusUnauthorized {
					t.Errorf("removed %s accepted: %d", key, w.Code)
				}
				query.Set(key, original)
			}
			query.Set("extra", "unsigned")
			if w := f.request(http.MethodGet, path+"?"+query.Encode(), "", "2", sig); w.Code != http.StatusUnauthorized {
				t.Errorf("added query parameter accepted: %d", w.Code)
			}
			if f.calls != 1 {
				t.Fatalf("tampered request reached handler: %d calls", f.calls)
			}
		})
	}
}

func TestChannelAuthLegacyOnlyWithoutQuery(t *testing.T) {
	f := newChannelAuthFixture(t)
	path := "/api/v1/channel/identities/telegram/resolve"
	body := `{"channel_user_id":"111"}`
	sig := upstream.Sign(f.client.ChannelSecret, http.MethodPost, path, f.stamp, []byte(body))
	for _, version := range []string{"", "1"} {
		if w := f.request(http.MethodPost, path, body, version, sig); w.Code != http.StatusNoContent || f.body != body {
			t.Fatalf("legacy signed JSON body was not preserved: %d body=%q", w.Code, f.body)
		}
		for _, query := range []string{"?channel_user_id=111", "?telegram_user_id=111", "?locale=en", "?", "?bad=%GG"} {
			if w := f.request(http.MethodPost, path+query, body, version, sig); w.Code != http.StatusUnauthorized {
				t.Errorf("legacy query signature accepted for %q: %d", query, w.Code)
			}
		}
		if w := f.request(http.MethodPost, path, `{"channel_user_id":"222"}`, version, sig); w.Code != http.StatusUnauthorized {
			t.Errorf("modified legacy body accepted: %d", w.Code)
		}
	}
	if f.calls != 2 {
		t.Fatalf("unexpected handler calls: %d", f.calls)
	}
}

func TestChannelAuthRejectsWrongSignatureContext(t *testing.T) {
	f := newChannelAuthFixture(t)
	path := "/api/v1/channel/orders"
	target := path + "?channel_user_id=111"
	body := `{"reason":"fixture"}`
	sig := f.signV2(http.MethodPost, target, body)
	for _, tc := range []struct{ name, method, target, body, version, signature string }{
		{"method", http.MethodGet, target, body, "2", sig},
		{"path", http.MethodPost, "/api/v1/channel/wallet?channel_user_id=111", body, "2", sig},
		{"body", http.MethodPost, target, `{"reason":"changed"}`, "2", sig},
		{"version removed", http.MethodPost, target, body, "", sig},
		{"version downgraded", http.MethodPost, target, body, "1", sig},
		{"unknown version", http.MethodPost, target, body, "3", sig},
		{"path-only", http.MethodPost, target, body, "2", upstream.Sign(f.client.ChannelSecret, http.MethodPost, path, f.stamp, []byte(body))},
		{"ordinary protocol", http.MethodPost, target, body, "2", upstream.Sign(f.client.ChannelSecret, http.MethodPost, target, f.stamp, []byte(body))},
		{"other channel context", http.MethodPost, target, body, "2", upstream.Sign(f.client.ChannelSecret, http.MethodPost, "dujiao-next-channel-v2\nother-fixture-channel\n"+target, f.stamp, []byte(body))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if w := f.request(tc.method, tc.target, tc.body, tc.version, tc.signature); w.Code != http.StatusUnauthorized {
				t.Fatalf("wrong signature context accepted: %d", w.Code)
			}
		})
	}
	if f.calls != 0 {
		t.Fatalf("wrong context reached handler: %d calls", f.calls)
	}
}

func TestChannelAuthV2NoQueryAndRetrySemantics(t *testing.T) {
	f := newChannelAuthFixture(t)
	path := "/api/v1/channel/identities/telegram/resolve"
	body := `{"channel_user_id":"111"}`
	sig := f.signV2(http.MethodPost, path, body)
	for attempt := 0; attempt < 2; attempt++ {
		if w := f.request(http.MethodPost, path, body, "2", sig); w.Code != http.StatusNoContent || f.body != body {
			t.Fatalf("valid retry/body rejected: %d", w.Code)
		}
	}
	for _, version := range []string{"", "1"} {
		if w := f.request(http.MethodPost, path, body, version, sig); w.Code != http.StatusUnauthorized {
			t.Errorf("v2 no-query signature accepted as legacy: %d", w.Code)
		}
	}
	w := f.requestWithHeaders(http.MethodPost, path, body, "2", sig, func(h http.Header) {
		h.Set(channelHeaderTimestamp, strconv.FormatInt(f.stamp+1, 10))
	})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("modified timestamp accepted: %d", w.Code)
	}
	f.stamp -= upstream.MaxTimestampSkew + 120
	expired := f.signV2(http.MethodPost, path, body)
	if w := f.request(http.MethodPost, path, body, "2", expired); w.Code != http.StatusUnauthorized {
		t.Fatalf("expired signature accepted: %d", w.Code)
	}
}

func TestChannelAuthRejectsMalformedOrAmbiguousSignedQuery(t *testing.T) {
	for _, raw := range []string{
		"channel_user_id=111&channel_user_id=111", "channel_user_id=111&%63hannel_user_id=222",
		"channel_user_id=111&telegram_user_id=222", "page=1&page=2", "=111",
		"channel_user_id=111&broken=%GG", "channel_user_id=111;telegram_user_id=222",
	} {
		t.Run(raw, func(t *testing.T) {
			f := newChannelAuthFixture(t)
			path := "/api/v1/channel/orders"
			query, _ := url.ParseQuery(raw)
			// Even a credential holder cannot authorize ambiguous query input.
			sig := f.signV2(http.MethodGet, path+"?"+query.Encode(), "")
			w := f.request(http.MethodGet, path+"?"+raw, "", "2", sig)
			if w.Code != http.StatusUnauthorized || f.calls != 0 {
				t.Fatalf("invalid query reached handler: %d calls=%d", w.Code, f.calls)
			}
		})
	}
}

type signedChannelIdentityFixture struct {
	channelhttp.IdentityService
	calls int
}

func (s *signedChannelIdentityFixture) ProvisionTelegramChannelUserID(input channelhttp.TelegramIdentityInput) (uint, error) {
	s.calls++
	if input.ChannelUserID == "222" {
		return 22, nil
	}
	return 11, nil
}

type signedChannelOrderFixture struct{ channelhttp.Orders }

func (signedChannelOrderFixture) GetOrderByUser(id, userID uint) (*orderdomain.Order, error) {
	if id != 900 || userID != 22 {
		return nil, channelhttp.ErrOrderNotFound
	}
	return &orderdomain.Order{ID: 900, UserID: 22, OrderNo: "FIXTURE-900"}, nil
}

func TestChannelAuthIdentityBindingBeforeOrderAuthorization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := channelclientapp.NewService(&channelAuthFixtureStore{}, "fixture-app-key")
	client, err := service.CreateChannelClient("fixture", "telegram_bot", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	identities := &signedChannelIdentityFixture{}
	handler := &channelhttp.Handler{Dependencies: channelhttp.Dependencies{
		UserAuthService: identities, OrderService: signedChannelOrderFixture{},
	}}
	f := &channelAuthFixture{client: client, stamp: time.Now().Unix(), router: gin.New()}
	f.router.Use(ChannelAPIAuthMiddleware(&container.Container{ChannelClientService: service}))
	f.router.GET("/api/v1/channel/orders/:id", handler.GetOrderStatus)
	path := "/api/v1/channel/orders/900"
	for _, identityParam := range []string{"channel_user_id", "telegram_user_id"} {
		t.Run(identityParam, func(t *testing.T) {
			before := identities.calls
			ownerTarget := path + "?" + identityParam + "=222"
			sig := f.signV2(http.MethodGet, ownerTarget, "")
			if w := f.request(http.MethodGet, ownerTarget, "", "2", sig); w.Code != http.StatusOK {
				t.Fatalf("signed owner request rejected: %d", w.Code)
			}
			nonOwnerTarget := path + "?" + identityParam + "=111"
			nonOwnerSig := f.signV2(http.MethodGet, nonOwnerTarget, "")
			if w := f.request(http.MethodGet, nonOwnerTarget, "", "2", nonOwnerSig); w.Code != http.StatusNotFound {
				t.Fatalf("signed non-owner request bypassed ownership check: %d", w.Code)
			}
			if w := f.request(http.MethodGet, ownerTarget, "", "2", nonOwnerSig); w.Code != http.StatusUnauthorized {
				t.Fatalf("substituted identity reached order lookup: %d", w.Code)
			}
			if identities.calls != before+2 {
				t.Fatalf("invalid signature reached identity provisioning: %d calls", identities.calls-before)
			}
		})
	}
}

type channelCompatibilityCredentialFixture struct{}

func (channelCompatibilityCredentialFixture) GetByApiKey(key string) (*apicredentialdomain.ApiCredential, error) {
	if key != "fixture-upstream-key" {
		return nil, nil
	}
	return &apicredentialdomain.ApiCredential{
		ID: 7, UserID: 11, ApiKey: key, ApiSecret: "fixture-upstream-secret",
		Status: constants.ApiCredentialStatusApproved, IsActive: true,
		User: &userdomain.User{ID: 11, Status: constants.UserStatusActive},
	}, nil
}
func (channelCompatibilityCredentialFixture) TouchLastUsedAt(uint, time.Time) error { return nil }

func TestChannelProtocolDoesNotChangeOrdinaryUpstreamAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(UpstreamAPIAuthMiddleware(channelCompatibilityCredentialFixture{}))
	router.Any("/api/v1/upstream/orders", func(c *gin.Context) {
		if c.GetUint(upstreamUserIDKey) != 11 {
			t.Error("ordinary upstream identity changed")
		}
		c.Status(http.StatusNoContent)
	})
	stamp := time.Now().Unix()
	path := "/api/v1/upstream/orders"
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		body := ""
		if method == http.MethodPost {
			body = `{"sku_id":1,"quantity":1}`
		}
		for _, query := range []string{"", "?page=1&locale=en"} {
			req := httptest.NewRequest(method, path+query, strings.NewReader(body))
			req.Header.Set(upstream.HeaderApiKey, "fixture-upstream-key")
			req.Header.Set(upstream.HeaderTimestamp, strconv.FormatInt(stamp, 10))
			req.Header.Set(upstream.HeaderSignature, upstream.Sign("fixture-upstream-secret", method, path, stamp, []byte(body)))
			// A channel-only header cannot opt ordinary APIs into a new protocol.
			req.Header.Set(channelVersionHeaderFixture, "2")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != http.StatusNoContent {
				t.Fatalf("ordinary %s signature changed: %d", method, w.Code)
			}
		}
	}
}
