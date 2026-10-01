package transport_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	fulfillmentdomain "github.com/dujiao-next/internal/modules/fulfillment/domain"
	orderdomain "github.com/dujiao-next/internal/modules/order/domain"
	orderhttp "github.com/dujiao-next/internal/modules/order/transport/http"
	reseller "github.com/dujiao-next/internal/modules/reseller/contract"
	"github.com/dujiao-next/internal/platform/http/response"
	"github.com/gin-gonic/gin"
)

// Embedded ports panic if the download handler calls an unexpected query method.
type fulfillmentDownloadQueryStub struct {
	orderhttp.UserOrderQuery
	orderhttp.GuestOrderQuery
	t     *testing.T
	order *orderdomain.Order
	calls int
}

func (s *fulfillmentDownloadQueryStub) GetAnyOrderByUserOrderNoForTenant(_ reseller.TenantContext, orderNo string, userID uint) (*orderdomain.Order, error) {
	s.calls++
	if orderNo != s.order.OrderNo || userID != s.order.UserID {
		s.t.Fatalf("unexpected member download query: order=%q user=%d", orderNo, userID)
	}
	return s.order, nil
}

func (s *fulfillmentDownloadQueryStub) GetAnyOrderByGuestOrderNoForTenant(_ reseller.TenantContext, orderNo, email, password string) (*orderdomain.Order, error) {
	s.calls++
	if orderNo != s.order.OrderNo || email != "fixture@example.test" || password != "fixture-password" {
		s.t.Fatal("unexpected guest download query")
	}
	return s.order, nil
}

func TestDownloadReleaseGatesEachOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Unix(1700000000, 0)
	for _, userID := range []uint{0, 7} {
		pending := orderdomain.Order{OrderNo: "DJ-FIXTURE", UserID: userID, Status: "pending_payment", Fulfillment: &fulfillmentdomain.Fulfillment{Status: "pending", Payload: "FIXTURE-BLOCKED"}}
		delivered := orderdomain.Order{OrderNo: "DJ-FIXTURE", UserID: userID, Status: "refunded", PaidAt: &now, Fulfillment: &fulfillmentdomain.Fulfillment{Status: "delivered", DeliveredAt: &now, Payload: "FIXTURE-RELEASED"}}
		parent := pending
		parent.Children = []orderdomain.Order{pending, delivered}
		delivered.Children = []orderdomain.Order{pending}
		for _, tc := range []struct {
			name  string
			order orderdomain.Order
			want  string
		}{
			{"unpaid", pending, ""},
			{"per-order gate", parent, "FIXTURE-RELEASED"},
			{"historical delivery", delivered, "FIXTURE-RELEASED"},
		} {
			t.Run(fmt.Sprintf("user-%d/%s", userID, tc.name), func(t *testing.T) {
				stub := &fulfillmentDownloadQueryStub{t: t, order: &tc.order}
				router := gin.New()
				request := httptest.NewRequest(http.MethodGet, "/orders/DJ-FIXTURE/fulfillment/download", nil)
				if userID == 0 {
					orderhttp.RegisterGuestReadRoutes(router, orderhttp.NewGuestHandler(stub, nil, nil))
					request.Header.Set("Authorization", "Guest "+base64.RawURLEncoding.EncodeToString([]byte("fixture@example.test\nfixture-password")))
				} else {
					router.Use(func(c *gin.Context) { c.Set("user_id", userID) })
					orderhttp.RegisterUserReadRoutes(router, orderhttp.NewUserHandler(stub, nil, nil, nil))
				}
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, request)
				if stub.calls != 1 {
					t.Fatalf("download query calls = %d, want 1", stub.calls)
				}
				if recorder.Code != http.StatusOK || strings.Contains(recorder.Body.String(), "FIXTURE-BLOCKED") {
					t.Fatalf("unexpected download response: %d %s", recorder.Code, recorder.Body.String())
				}
				if tc.want == "" {
					var result response.Response
					if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
						t.Fatal(err)
					}
					if result.StatusCode != response.CodeNotFound || recorder.Header().Get("Content-Disposition") != "" {
						t.Fatalf("unpaid download not withheld: %s", recorder.Body.String())
					}
				} else {
					if got := recorder.Body.String(); got != tc.want {
						t.Errorf("download got %q, want %q", got, tc.want)
					}
					if recorder.Header().Get("Content-Disposition") != `attachment; filename="fulfillment-DJ-FIXTURE.txt"` {
						t.Error("released download missing attachment header")
					}
				}
			})
		}
	}
}
