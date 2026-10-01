package paymentcallbackhttp

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dujiao-next/internal/shared/jsonmap"
	"github.com/gin-gonic/gin"
)

type callbackAlertRecorder struct {
	calls int
	data  jsonmap.JSON
}

func (a *callbackAlertRecorder) EnqueuePaymentExceptionAlert(_, _, _ string, data jsonmap.JSON) error {
	a.calls++
	a.data = data
	return nil
}

func TestPaymentCallbackProbeAlertPolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, method, query, body string
		headers                   map[string]string
		unknownBody               bool
		wantAlert                 bool
	}{
		{name: "bare GET is not a payment incident", method: "GET"},
		{name: "browser GET is not a payment incident", method: "GET", headers: map[string]string{"User-Agent": "synthetic-browser", "Accept": "text/html"}},
		{name: "empty POST remains suspicious", method: "POST", wantAlert: true},
		{name: "unsupported JSON stays visible", method: "POST", body: `{"diagnostic":"synthetic"}`, headers: map[string]string{"Content-Type": "application/json"}, wantAlert: true},
		{name: "GET payload stays visible", method: "GET", body: `{"diagnostic":"synthetic"}`, wantAlert: true},
		{name: "partial form callback stays visible", method: "GET", query: "?out_trade_no=synthetic-missing-fields", wantAlert: true},
		{name: "malformed query stays visible", method: "GET", query: "?sign=%zz", wantAlert: true},
		{name: "channel hint stays visible", method: "GET", query: "?channel_id=1", wantAlert: true},
		{name: "partial WeChat headers stay visible", method: "GET", headers: map[string]string{"Wechatpay-Nonce": "synthetic"}, wantAlert: true},
		{name: "wrong-endpoint Stripe stays visible", method: "GET", headers: map[string]string{"Stripe-Signature": "synthetic"}, wantAlert: true},
		{name: "wrong-endpoint PayPal stays visible", method: "GET", headers: map[string]string{"Paypal-Transmission-Id": "synthetic"}, wantAlert: true},
		{name: "authorization stays visible", method: "GET", headers: map[string]string{"Authorization": "synthetic-not-a-credential"}, wantAlert: true},
		{name: "custom signature stays visible", method: "GET", headers: map[string]string{"X-Signature": "synthetic"}, wantAlert: true},
		{name: "unknown-length empty body is conservative", method: "GET", unknownBody: true, wantAlert: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			var body io.Reader
			if tc.body != "" {
				body = strings.NewReader(tc.body)
			}
			c.Request = httptest.NewRequest(tc.method, "/api/v1/payments/callback"+tc.query, body)
			for name, value := range tc.headers {
				c.Request.Header.Set(name, value)
			}
			if tc.unknownBody {
				c.Request.Body = io.NopCloser(strings.NewReader(""))
				c.Request.ContentLength = -1
				c.Request.TransferEncoding = []string{"chunked"}
			}
			alerts := &callbackAlertRecorder{}
			// Nil business ports ensure these unmatched cases cannot call payment services.
			(&Handler{alerts: alerts}).PaymentCallback(c)
			if w.Code != http.StatusNotFound {
				t.Fatalf("status=%d, want 404", w.Code)
			}
			wantCalls := 0
			if tc.wantAlert {
				wantCalls = 1
			}
			if alerts.calls != wantCalls {
				t.Fatalf("alerts=%d, want %d", alerts.calls, wantCalls)
			}
			if tc.wantAlert && (alerts.data["alert_type"] != "callback_unrecognized" || alerts.data["alert_level"] != "warning") {
				t.Fatalf("unexpected alert classification: %v", alerts.data)
			}
		})
	}
}

func TestPaymentCallbackContentTypeHintsCannotBeHidden(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, key := range []string{"Content-Type", "content-type"} {
		t.Run(key, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/payments/callback", nil)
			c.Request.Header[key] = []string{"", "application/json"}
			alerts := &callbackAlertRecorder{}
			(&Handler{alerts: alerts}).PaymentCallback(c)
			if w.Code != 404 || alerts.calls != 1 {
				t.Fatalf("status=%d alerts=%d, want 404 and 1", w.Code, alerts.calls)
			}
		})
	}
	results := make(chan int, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		alerts := &callbackAlertRecorder{}
		c, _ := gin.CreateTestContext(w)
		c.Request = r
		(&Handler{alerts: alerts}).PaymentCallback(c)
		results <- alerts.calls
	}))
	defer server.Close()
	request, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/payments/callback", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header["Content-Type"] = []string{"", "application/json"}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 404 {
		t.Fatalf("status=%d", response.StatusCode)
	}
	if calls := <-results; calls != 1 {
		t.Fatalf("duplicate Content-Type wire GET queued %d alerts, want 1", calls)
	}
}

func TestPaymentCallbackBareGETOverHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	results := make(chan int, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		alerts := &callbackAlertRecorder{}
		c, _ := gin.CreateTestContext(w)
		c.Request = r
		(&Handler{alerts: alerts}).PaymentCallback(c)
		results <- alerts.calls
	}))
	defer server.Close()
	response, err := server.Client().Get(server.URL + "/api/v1/payments/callback")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("status=%d", response.StatusCode)
	}
	if calls := <-results; calls != 0 {
		t.Fatalf("bare wire GET queued %d payment alerts", calls)
	}
}

func TestPaymentCallbackRejectsUnknownPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/payments/callback", strings.NewReader(`{"payment_id":1,"status":"success"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	(&Handler{}).PaymentCallback(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

func TestParseCallbackFormNormalizesNonStandardQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for name, target := range map[string]string{
		"semicolon separator":    "/api/v1/payments/callback?pid=2026;out_trade_no=ORDER-1;trade_status=TRADE_SUCCESS;sign=abc",
		"html escaped ampersand": "/api/v1/payments/callback?pid=2026&amp;out_trade_no=ORDER-1&amp;trade_status=TRADE_SUCCESS&amp;sign=abc",
	} {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, target, nil)
			form, err := parseCallbackForm(c)
			if err != nil {
				t.Fatalf("parse callback form failed: %v", err)
			}
			for key, want := range map[string]string{"out_trade_no": "ORDER-1", "trade_status": "TRADE_SUCCESS", "sign": "abc"} {
				if got := getFirstValue(form, key); got != want {
					t.Fatalf("unexpected %s: %q", key, got)
				}
			}
		})
	}
}

func TestParseCallbackFormPrefersSignedPostForm(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/payments/callback?channel_id=999",
		strings.NewReader("out_trade_no=ORDER-POST&trade_status=TRADE_SUCCESS&sign=abc&notify_id=n1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	c.Request = req

	form, err := parseCallbackForm(c)
	if err != nil {
		t.Fatalf("parse callback form failed: %v", err)
	}
	if got := getFirstValue(form, "out_trade_no"); got != "ORDER-POST" {
		t.Fatalf("unexpected out_trade_no: %s", got)
	}
	if got := getFirstValue(form, "channel_id"); got != "" {
		t.Fatalf("expected query param excluded from signed form, got %s", got)
	}
}

func TestWechatCallbackFeatureGuard(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{"id":"EV-1","resource":{"algorithm":"AEAD_AES_256_GCM"}}`
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/payments/callback", strings.NewReader(body))
	for key, value := range map[string]string{
		"Wechatpay-Signature": "mock-sign", "Wechatpay-Timestamp": "1760000000",
		"Wechatpay-Nonce": "mock-nonce", "Wechatpay-Serial": "mock-serial",
	} {
		c.Request.Header.Set(key, value)
	}
	if !isWechatCallbackRequest(c, []byte(body)) {
		t.Fatal("expected wechat callback request")
	}
	c.Request.Header.Del("Wechatpay-Signature")
	if isWechatCallbackRequest(c, []byte(body)) {
		t.Fatal("expected missing header to reject wechat callback")
	}
}

func TestEpusdtFeatureGuardFallsThroughWithoutPID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for name, body := range map[string]string{
		"bepusdt payload": `{"trade_id":"t1","order_id":"o1","status":2,"signature":"x"}`,
		"empty payload":   "",
	} {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/payments/callback", bytes.NewBufferString(body))
			if handled := (&Handler{}).handleEpusdtCallback(c); handled {
				t.Fatal("expected epusdt feature guard to fall through")
			}
		})
	}
}
