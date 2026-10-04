package transport_test

import (
	"encoding/base64"
	"encoding/json"
	"github.com/dujiao-next/internal/config"
	captchaapp "github.com/dujiao-next/internal/modules/captcha/application"
	captchahttp "github.com/dujiao-next/internal/modules/captcha/transport/http"
	orderdomain "github.com/dujiao-next/internal/modules/order/domain"
	orderhttp "github.com/dujiao-next/internal/modules/order/transport/http"
	reseller "github.com/dujiao-next/internal/modules/reseller/contract"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"net/url"
	"testing"
)

type filterQuery struct {
	browserOrderQueryStub
	no    string
	calls int
}

func (s *filterQuery) GetOrderByGuestOrderNoForTenant(_ reseller.TenantContext, no, email, password string) (*orderdomain.Order, error) {
	s.calls++
	s.no = no
	if no != "MATCH" || email != "guest@example.com" || password != "password" {
		return nil, orderhttp.ErrGuestOrderNotFound
	}
	return &orderdomain.Order{OrderNo: "MATCH"}, nil
}
func TestGuestOrderFilterPaginationAndIndistinguishableMisses(t *testing.T) {
	for _, tc := range []struct {
		no, email, password string
		page, total, length int
	}{
		{" MATCH ", "guest@example.com", "password", 1, 1, 1},
		{"MATCH", "guest@example.com", "password", 2, 1, 0},
		{"UNKNOWN", "guest@example.com", "password", 1, 0, 0},
		{"MATCH", "other@example.com", "password", 1, 0, 0},
		{"MATCH", "guest@example.com", "wrong", 1, 0, 0},
	} {
		gin.SetMode(gin.TestMode)
		spy := &filterQuery{}
		svc := captchaapp.NewService(lookupSettings{"turnstile", true}, config.CaptchaConfig{}, lookupTurnstile{})
		r := gin.New()
		orderhttp.RegisterGuestReadRoutes(r, orderhttp.NewGuestHandler(spy, nil, nil, captchahttp.NewVerifier(svc)))
		req := httptest.NewRequest("GET", "/orders?order_no="+stringsQuery(tc.no)+"&page="+string(rune('0'+tc.page))+"&page_size=20", nil)
		req.Header.Set("Authorization", "Guest "+base64.RawURLEncoding.EncodeToString([]byte(tc.email+"\n"+tc.password)))
		req.Header.Set("X-Turnstile-Token", "valid-fixture-token")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var body struct {
			Status     int                 `json:"status_code"`
			Data       []orderdomain.Order `json:"data"`
			Pagination struct {
				Page, Total int
				PageSize    int `json:"page_size"`
			} `json:"pagination"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Status != 0 || len(body.Data) != tc.length || body.Pagination.Total != tc.total || body.Pagination.Page != tc.page || body.Pagination.PageSize != 20 {
			t.Fatalf("filter %q: %s", tc.no, w.Body.String())
		}
	}
}
func stringsQuery(s string) string { return url.QueryEscape(s) }
