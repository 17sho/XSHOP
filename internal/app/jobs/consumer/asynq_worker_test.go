package consumer

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/mail"
	"net/textproto"
	"reflect"
	"strings"
	"testing"
	"time"

	fulfillmentdomain "github.com/dujiao-next/internal/modules/fulfillment/domain"
	orderdomain "github.com/dujiao-next/internal/modules/order/domain"

	"github.com/dujiao-next/internal/app/container"
	"github.com/dujiao-next/internal/config"
	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/i18n"
	notificationcontract "github.com/dujiao-next/internal/modules/notification/contract"
	notificationsmtp "github.com/dujiao-next/internal/modules/notification/infrastructure/smtp"
	"github.com/dujiao-next/internal/queue"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"github.com/dujiao-next/internal/shared/mailbrand"
)

// captureOrderEmail uses only a loopback SMTP sink, never a real recipient.
func captureOrderEmail(t *testing.T, order *orderdomain.Order, queuedStatus string) (*mail.Message, string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	type captured struct{ recipient, raw string }
	messages := make(chan captured, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		reader := textproto.NewReader(bufio.NewReader(conn))
		fmt.Fprint(conn, "220 loopback ESMTP\r\n")
		recipient := ""
		for {
			line, err := reader.ReadLine()
			if err != nil {
				return
			}
			switch {
			case strings.HasPrefix(line, "RCPT TO"):
				recipient = line
				fmt.Fprint(conn, "250 OK\r\n")
			case strings.HasPrefix(line, "EHLO"), strings.HasPrefix(line, "HELO"), strings.HasPrefix(line, "MAIL FROM"):
				fmt.Fprint(conn, "250 OK\r\n")
			case line == "DATA":
				fmt.Fprint(conn, "354 continue\r\n")
				data, err := reader.ReadDotBytes()
				if err != nil {
					return
				}
				messages <- captured{recipient, string(data)}
				fmt.Fprint(conn, "250 accepted\r\n")
			case line == "QUIT":
				fmt.Fprint(conn, "221 bye\r\n")
				return
			default:
				fmt.Fprint(conn, "500 unsupported\r\n")
			}
		}
	}()
	cfg := &config.EmailConfig{Enabled: true, Host: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port, From: "notify@example.test"}
	consumer := &Consumer{Container: &container.Container{EmailSender: notificationsmtp.New(cfg)}, orderReader: orderStatusEmailWorkerOrderRepoStub{order: order}}
	task, err := queue.NewOrderStatusEmailTask(queue.OrderStatusEmailPayload{OrderID: order.ID, Status: queuedStatus})
	if err != nil {
		t.Fatal(err)
	}
	if err := consumer.handleOrderStatusEmail(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-messages:
		msg, err := mail.ReadMessage(strings.NewReader(result.raw))
		if err != nil {
			t.Fatal(err)
		}
		return msg, result.recipient
	case <-time.After(5 * time.Second):
		t.Fatal("no loopback message")
		return nil, ""
	}
}

func TestOrderEmailReleaseThroughLoopbackSMTP(t *testing.T) {
	stamp := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	for _, tc := range []struct {
		name, currentStatus, queuedStatus string
		long, valid, child                bool
	}{
		{name: "delivered inline", currentStatus: "delivered", queuedStatus: "delivered", valid: true},
		{name: "paid instructions without delivery", currentStatus: "paid", queuedStatus: "delivered"},
		{name: "stale delivered job pending snapshot", currentStatus: "pending_payment", queuedStatus: "delivered", long: true},
		{name: "paid state email", currentStatus: "paid", queuedStatus: "paid", valid: true},
		{name: "historical attachment", currentStatus: "completed", queuedStatus: "completed", long: true, valid: true},
		{name: "refunded state attachment", currentStatus: "refunded", queuedStatus: "refunded", long: true, valid: true},
		{name: "independent child attachment", currentStatus: "pending_payment", queuedStatus: "delivered", long: true, valid: true, child: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := "SYNTHETIC-VALID"
			if tc.long {
				payload = strings.TrimSpace(strings.Repeat(payload+"\n", fulfillmentdomain.PayloadMaxEmailLines+1))
			}
			order := &orderdomain.Order{ID: 7, OrderNo: "SYNTHETIC-ORDER", Status: tc.currentStatus, PaidAt: &stamp, GuestEmail: "buyer@example.test", GuestLocale: "en-US", Currency: "CNY",
				Items:       []orderdomain.OrderItem{{TitleJSON: jsonmap.JSON{"en-US": "SYNTHETIC-ITEM"}, InstructionsJSON: jsonmap.JSON{"en-US": "SYNTHETIC-INSTRUCTIONS"}, Quantity: 1}},
				Fulfillment: &fulfillmentdomain.Fulfillment{Status: "delivered", Payload: payload}}
			if !tc.valid {
				order.Fulfillment.Payload = strings.ReplaceAll(payload, "SYNTHETIC-VALID", "SYNTHETIC-HIDDEN")
			}
			if tc.name == "paid instructions without delivery" {
				order.Fulfillment.Status = "pending"
			}
			if tc.child {
				order.Fulfillment.Payload = "SYNTHETIC-HIDDEN-PARENT"
				order.Children = []orderdomain.Order{
					{OrderNo: "BLOCKED", Status: "pending_payment", Fulfillment: &fulfillmentdomain.Fulfillment{Status: "delivered", Payload: "SYNTHETIC-HIDDEN-CHILD"}},
					{OrderNo: "HISTORY", Status: "completed", Fulfillment: &fulfillmentdomain.Fulfillment{Status: "delivered", Payload: payload}},
				}
			}
			msg, recipient := captureOrderEmail(t, order, tc.queuedStatus)
			if msg.Header.Get("To") != order.GuestEmail || recipient != "RCPT TO:<buyer@example.test>" {
				t.Fatalf("recipient changed: %v / %s", msg.Header, recipient)
			}
			label := i18n.T("en-US", "order.status."+tc.queuedStatus)
			if label == "order.status."+tc.queuedStatus {
				label = tc.queuedStatus
			}
			wantSubject := i18n.Sprintf("en-US", "email.order_status.subject", label)
			subject, err := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
			if err != nil || subject != wantSubject {
				t.Fatalf("state-specific subject changed: %q / %v", subject, err)
			}
			var text, attachment string
			var decode func(string, string, string, io.Reader)
			decode = func(contentType, encoding, disposition string, body io.Reader) {
				mediaType, params, err := mime.ParseMediaType(contentType)
				if err != nil {
					t.Fatal(err)
				}
				if strings.HasPrefix(mediaType, "multipart/") {
					reader := multipart.NewReader(body, params["boundary"])
					for {
						part, err := reader.NextPart()
						if err == io.EOF {
							break
						}
						if err != nil {
							t.Fatal(err)
						}
						decode(part.Header.Get("Content-Type"), part.Header.Get("Content-Transfer-Encoding"), part.Header.Get("Content-Disposition"), part)
					}
					return
				}
				if encoding == "base64" {
					body = base64.NewDecoder(base64.StdEncoding, body)
				}
				data, err := io.ReadAll(body)
				if err != nil {
					t.Fatal(err)
				}
				if strings.HasPrefix(disposition, "attachment;") {
					_, fields, err := mime.ParseMediaType(disposition)
					if err != nil || fields["filename"] != "order_SYNTHETIC-ORDER_delivery.txt" {
						t.Fatalf("attachment filename changed: %s", disposition)
					}
					attachment += string(data)
				}
				text += string(data)
			}
			decode(msg.Header.Get("Content-Type"), msg.Header.Get("Content-Transfer-Encoding"), msg.Header.Get("Content-Disposition"), msg.Body)
			if strings.Contains(text, "SYNTHETIC-HIDDEN") {
				t.Fatal("blocked snapshot reached email MIME")
			}
			if !strings.Contains(text, "SYNTHETIC-ITEM") {
				t.Fatal("item content disappeared")
			}
			wantInstructions := tc.queuedStatus == "delivered" || tc.queuedStatus == "completed"
			if strings.Contains(text, "SYNTHETIC-INSTRUCTIONS") != wantInstructions {
				t.Fatal("instruction scene semantics changed")
			}
			if tc.long && tc.valid {
				want := payload
				if tc.child {
					want = "[HISTORY]\n" + payload
				}
				if attachment != want {
					t.Fatalf("attachment = %q, want %q", attachment, want)
				}
			} else if attachment != "" {
				t.Fatal("unexpected attachment")
			}
			if tc.valid && !strings.Contains(text, "SYNTHETIC-VALID") {
				t.Fatal("legitimate delivery disappeared")
			}
		})
	}
}

func TestEmailFulfillmentReleaseEvidence(t *testing.T) {
	stamp := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	zero := time.Time{}
	cases := []struct {
		name, status, deliveryStatus                         string
		paidAt, deliveredAt, deletedAt, fulfillmentDeletedAt *time.Time
		want                                                 bool
	}{
		{name: "paid delivered", status: "paid", deliveryStatus: "delivered", paidAt: &stamp, want: true},
		{name: "fulfilling delivered", status: "fulfilling", deliveryStatus: "delivered", paidAt: &stamp, want: true},
		{name: "partial delivery", status: "partially_delivered", deliveryStatus: "delivered", paidAt: &stamp, want: true},
		{name: "historical delivered", status: "delivered", deliveryStatus: "delivered", want: true},
		{name: "historical completed", status: "completed", deliveryStatus: "delivered", want: true},
		{name: "refunded paid", status: "refunded", deliveryStatus: "delivered", paidAt: &stamp, want: true},
		{name: "partial refund historical", status: "partially_refunded", deliveryStatus: "delivered", deliveredAt: &stamp, want: true},
		{name: "canceled historical", status: "canceled", deliveryStatus: "delivered", paidAt: &stamp, deliveredAt: &stamp, want: true},
		{name: "pending inconsistent", status: "pending_payment", deliveryStatus: "delivered", paidAt: &stamp, deliveredAt: &stamp},
		{name: "unknown", status: "unknown", deliveryStatus: "delivered", paidAt: &stamp},
		{name: "paid missing proof", status: "paid", deliveryStatus: "delivered"},
		{name: "paid zero proof", status: "paid", deliveryStatus: "delivered", paidAt: &zero},
		{name: "paid reserved", status: "paid", deliveryStatus: "pending", paidAt: &stamp},
		{name: "refunded unproven", status: "refunded", deliveryStatus: "delivered"},
		{name: "refund before delivery", status: "refunded", deliveryStatus: "pending", paidAt: &stamp},
		{name: "canceled undated", status: "canceled", deliveryStatus: "delivered", paidAt: &stamp},
		{name: "deleted order", status: "delivered", deliveryStatus: "delivered", deletedAt: &stamp},
		{name: "deleted fulfillment", status: "delivered", deliveryStatus: "delivered", fulfillmentDeletedAt: &stamp},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			order := orderdomain.Order{OrderNo: "CHILD", Status: tc.status, PaidAt: tc.paidAt, DeletedAt: tc.deletedAt,
				Items:       []orderdomain.OrderItem{{InstructionsJSON: jsonmap.JSON{"en-US": "Keep these instructions"}}},
				Fulfillment: &fulfillmentdomain.Fulfillment{Status: tc.deliveryStatus, DeliveredAt: tc.deliveredAt, DeletedAt: tc.fulfillmentDeletedAt, Payload: " SYNTHETIC-CARD "}}
			want := ""
			if tc.want {
				want = "SYNTHETIC-CARD"
			}
			if got := buildOrderFulfillmentEmailPayload(&order); got != want {
				t.Errorf("parent payload = %q, want %q", got, want)
			}
			// Parent payment must never authorize a child.
			parent := &orderdomain.Order{Status: "paid", PaidAt: &stamp, Children: []orderdomain.Order{order}}
			childWant := ""
			var deliveriesWant []notificationcontract.OrderEmailDelivery
			if tc.want {
				childWant = "[CHILD]\nSYNTHETIC-CARD"
				deliveriesWant = []notificationcontract.OrderEmailDelivery{{OrderNo: "CHILD", Content: "SYNTHETIC-CARD"}}
			}
			if got := buildOrderFulfillmentEmailPayload(parent); got != childWant {
				t.Errorf("child payload = %q, want %q", got, childWant)
			}
			if got := orderEmailDeliveries(parent); !reflect.DeepEqual(got, deliveriesWant) {
				t.Errorf("child deliveries = %+v, want %+v", got, deliveriesWant)
			}
			if got := buildOrderInstructionsEmailText(&order, "en-US"); got != "Keep these instructions" {
				t.Errorf("instructions lost: %q", got)
			}
			if order.Fulfillment.Payload != " SYNTHETIC-CARD " {
				t.Fatal("stored snapshot mutated")
			}
		})
	}
}

func TestEmailFulfillmentFallbackFiltersAttachmentSource(t *testing.T) {
	valid := strings.Repeat("SYNTHETIC-VALID\n", fulfillmentdomain.PayloadMaxEmailLines+1)
	order := &orderdomain.Order{Status: "pending_payment", Fulfillment: &fulfillmentdomain.Fulfillment{Status: "delivered", Payload: "SYNTHETIC-HIDDEN-PARENT"}, Children: []orderdomain.Order{
		{OrderNo: "UNPAID", Status: "pending_payment", Fulfillment: &fulfillmentdomain.Fulfillment{Status: "delivered", Payload: "SYNTHETIC-HIDDEN-CHILD"}},
		{OrderNo: "HISTORY", Status: "completed", Fulfillment: &fulfillmentdomain.Fulfillment{Status: "delivered", Payload: valid}},
	}}
	want := "[HISTORY]\n" + strings.TrimSpace(valid)
	if got := buildOrderFulfillmentEmailPayload(order); got != want || !fulfillmentdomain.ShouldAttachFulfillmentPayload(got) {
		t.Errorf("attachment source not independently filtered: %q", got)
	}
	wantDeliveries := []notificationcontract.OrderEmailDelivery{{OrderNo: "HISTORY", Content: strings.TrimSpace(valid)}}
	if got := orderEmailDeliveries(order); !reflect.DeepEqual(got, wantDeliveries) {
		t.Errorf("blocked parent suppressed legitimate child: %+v", got)
	}
	order.Status = "completed"
	if got := buildOrderFulfillmentEmailPayload(order); got != "SYNTHETIC-HIDDEN-PARENT" {
		t.Errorf("legitimate parent priority changed: %q", got)
	}
	if got := orderEmailDeliveries(order); got != nil {
		t.Errorf("legitimate parent duplicated child output: %+v", got)
	}
}

func TestBuildOrderFulfillmentEmailPayloadNilOrder(t *testing.T) {
	if got := buildOrderFulfillmentEmailPayload(nil); got != "" {
		t.Fatalf("expected empty payload for nil order, got %q", got)
	}
}

func TestBuildOrderInstructionsEmailText(t *testing.T) {
	t.Run("nil order returns empty", func(t *testing.T) {
		if got := buildOrderInstructionsEmailText(nil, "zh-CN"); got != "" {
			t.Fatalf("expected empty, got %q", got)
		}
	})

	t.Run("locale preferred over fallback", func(t *testing.T) {
		order := &orderdomain.Order{
			Items: []orderdomain.OrderItem{
				{InstructionsJSON: jsonmap.JSON{
					"zh-CN": "<p>中文说明</p>",
					"en-US": "<p>English</p>",
				}},
			},
		}
		if got := buildOrderInstructionsEmailText(order, "en-US"); got != "English" {
			t.Fatalf("want 'English', got %q", got)
		}
	})

	t.Run("falls back to zh-CN when locale missing", func(t *testing.T) {
		order := &orderdomain.Order{
			Items: []orderdomain.OrderItem{
				{InstructionsJSON: jsonmap.JSON{"zh-CN": "fallback"}},
			},
		}
		if got := buildOrderInstructionsEmailText(order, "ja-JP"); got != "fallback" {
			t.Fatalf("want 'fallback', got %q", got)
		}
	})

	t.Run("dedupes identical items and joins distinct", func(t *testing.T) {
		order := &orderdomain.Order{
			Items: []orderdomain.OrderItem{
				{InstructionsJSON: jsonmap.JSON{"zh-CN": "<p>A</p>"}},
				{InstructionsJSON: jsonmap.JSON{"zh-CN": "<p>A</p>"}}, // 重复，应去重
				{InstructionsJSON: jsonmap.JSON{"zh-CN": "<p>B</p>"}},
			},
		}
		got := buildOrderInstructionsEmailText(order, "zh-CN")
		if got != "A\n\nB" {
			t.Fatalf("want 'A\\n\\nB', got %q", got)
		}
	})

	t.Run("collects from children items", func(t *testing.T) {
		order := &orderdomain.Order{
			Children: []orderdomain.Order{
				{Items: []orderdomain.OrderItem{{InstructionsJSON: jsonmap.JSON{"zh-CN": "child1"}}}},
				{Items: []orderdomain.OrderItem{{InstructionsJSON: jsonmap.JSON{"zh-CN": "child2"}}}},
			},
		}
		got := buildOrderInstructionsEmailText(order, "zh-CN")
		if got != "child1\n\nchild2" {
			t.Fatalf("unexpected: %q", got)
		}
	})

	t.Run("empty instructions yield empty result", func(t *testing.T) {
		order := &orderdomain.Order{
			Items: []orderdomain.OrderItem{{InstructionsJSON: nil}},
		}
		if got := buildOrderInstructionsEmailText(order, "zh-CN"); got != "" {
			t.Fatalf("expected empty, got %q", got)
		}
	})

	t.Run("strips HTML from instructions", func(t *testing.T) {
		order := &orderdomain.Order{
			Items: []orderdomain.OrderItem{
				{InstructionsJSON: jsonmap.JSON{"zh-CN": "<p>步骤一</p><ul><li>登录</li><li>激活</li></ul>"}},
			},
		}
		got := buildOrderInstructionsEmailText(order, "zh-CN")
		want := "步骤一\n\n• 登录\n• 激活"
		if got != want {
			t.Fatalf("want %q, got %q", want, got)
		}
	})
}

func TestBuildOrderFulfillmentEmailPayloadPreferOrderFulfillment(t *testing.T) {
	order := &orderdomain.Order{
		Status:      constants.OrderStatusDelivered,
		Fulfillment: &fulfillmentdomain.Fulfillment{Status: constants.FulfillmentStatusDelivered, Payload: "  MAIN-LINE-1\nMAIN-LINE-2  "},
		Children: []orderdomain.Order{
			{
				OrderNo:     "CHILD-1",
				Status:      constants.OrderStatusDelivered,
				Fulfillment: &fulfillmentdomain.Fulfillment{Status: constants.FulfillmentStatusDelivered, Payload: "SECRET-1"},
			},
		},
	}

	got := buildOrderFulfillmentEmailPayload(order)
	want := "MAIN-LINE-1\nMAIN-LINE-2"
	if got != want {
		t.Fatalf("unexpected payload, want %q, got %q", want, got)
	}
}

type orderStatusEmailWorkerOrderRepoStub struct {
	order *orderdomain.Order
	err   error
}

func (s orderStatusEmailWorkerOrderRepoStub) GetByID(_ uint) (*orderdomain.Order, error) {
	return s.order, s.err
}

type orderEmailBrandResolverStub struct {
	brand mailbrand.Brand
	scope mailbrand.Scope
}

func (s *orderEmailBrandResolverStub) ResolveEmailBrand(_ context.Context, scope mailbrand.Scope) (mailbrand.Brand, error) {
	s.scope = scope
	return s.brand, nil
}

func TestResolveOrderEmailBrandUsesResellerScope(t *testing.T) {
	resellerID := uint(17)
	resolver := &orderEmailBrandResolverStub{brand: mailbrand.Brand{
		SiteName: "White Label Store",
		SiteURL:  "https://shop.example.test",
		FromName: "White Label Store",
		ReplyTo:  "support@example.test",
	}}
	consumer := &Consumer{
		Container: &container.Container{EmailBrandResolver: resolver},
	}
	order := &orderdomain.Order{
		ResellerID:     &resellerID,
		ResellerDomain: "shop.example.test",
	}

	got, err := consumer.resolveOrderEmailBrand(context.Background(), order)
	if err != nil {
		t.Fatalf("resolve order email brand failed: %v", err)
	}
	if resolver.scope.ResellerID == nil || *resolver.scope.ResellerID != resellerID || resolver.scope.Host != "shop.example.test" {
		t.Fatalf("unexpected resolver scope: %+v", resolver.scope)
	}
	if got.SiteName != "White Label Store" || got.SiteURL != "https://shop.example.test" || got.FromName != "White Label Store" {
		t.Fatalf("unexpected reseller brand: %+v", got)
	}
}

func TestResolveOrderEmailBrandNeverFallsBackToMainBrandForReseller(t *testing.T) {
	resellerID := uint(23)
	consumer := &Consumer{
		Container: &container.Container{},
	}

	got, err := consumer.resolveOrderEmailBrand(context.Background(), &orderdomain.Order{
		ResellerID:     &resellerID,
		ResellerDomain: "fallback.example.test",
	})
	if err != nil {
		t.Fatalf("resolve safe fallback failed: %v", err)
	}
	if got.SiteName != "fallback.example.test" || got.SiteURL != "https://fallback.example.test" || got.FromName != "fallback.example.test" {
		t.Fatalf("unexpected safe reseller fallback: %+v", got)
	}
}

func TestHandleOrderStatusEmailSkipsNonRetryableEmailErrors(t *testing.T) {
	testCases := []struct {
		name         string
		order        *orderdomain.Order
		emailConfig  config.EmailConfig
		expectNilErr bool
	}{
		{
			name: "smtp_disabled",
			order: &orderdomain.Order{
				ID:          1,
				OrderNo:     "DJ-ORDER-001",
				GuestEmail:  "buyer@example.com",
				GuestLocale: "zh-CN",
				Currency:    "CNY",
			},
			emailConfig:  config.EmailConfig{Enabled: false},
			expectNilErr: true,
		},
		{
			name: "smtp_not_configured",
			order: &orderdomain.Order{
				ID:          2,
				OrderNo:     "DJ-ORDER-002",
				GuestEmail:  "buyer@example.com",
				GuestLocale: "zh-CN",
				Currency:    "CNY",
			},
			emailConfig:  config.EmailConfig{Enabled: true},
			expectNilErr: true,
		},
		{
			name: "invalid_receiver_email",
			order: &orderdomain.Order{
				ID:          3,
				OrderNo:     "DJ-ORDER-003",
				GuestEmail:  "invalid-email",
				GuestLocale: "zh-CN",
				Currency:    "CNY",
			},
			emailConfig: config.EmailConfig{
				Enabled: true,
				Host:    "127.0.0.1",
				Port:    1,
				From:    "sender@example.com",
			},
			expectNilErr: true,
		},
		{
			name: "generic_send_failure_keeps_retryable_error",
			order: &orderdomain.Order{
				ID:          4,
				OrderNo:     "DJ-ORDER-004",
				GuestEmail:  "buyer@example.com",
				GuestLocale: "zh-CN",
				Currency:    "CNY",
			},
			emailConfig: config.EmailConfig{
				Enabled: true,
				Host:    "127.0.0.1",
				Port:    1,
				From:    "sender@example.com",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			task, err := queue.NewOrderStatusEmailTask(queue.OrderStatusEmailPayload{
				OrderID: tc.order.ID,
				Status:  "paid",
			})
			if err != nil {
				t.Fatalf("new order status email task failed: %v", err)
			}

			consumer := &Consumer{
				Container: &container.Container{
					EmailSender: notificationsmtp.New(&tc.emailConfig),
				},
				orderReader: orderStatusEmailWorkerOrderRepoStub{order: tc.order},
			}

			err = consumer.handleOrderStatusEmail(context.Background(), task)
			if tc.expectNilErr {
				if err != nil {
					t.Fatalf("expected nil error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected retryable send error, got nil")
			}
			if errors.Is(err, notificationcontract.ErrEmailServiceDisabled) || errors.Is(err, notificationcontract.ErrEmailNotConfigured) || errors.Is(err, notificationcontract.ErrInvalidEmail) {
				t.Fatalf("expected generic retryable error, got %v", err)
			}
		})
	}
}

func TestHandleOrderStatusEmailSkipsCanceledWithoutDependencies(t *testing.T) {
	task, err := queue.NewOrderStatusEmailTask(queue.OrderStatusEmailPayload{
		OrderID: 106,
		Status:  constants.OrderStatusCanceled,
	})
	if err != nil {
		t.Fatalf("new order status email task failed: %v", err)
	}

	consumer := &Consumer{}
	if err := consumer.handleOrderStatusEmail(context.Background(), task); err != nil {
		t.Fatalf("expected canceled email task to be dropped, got %v", err)
	}
}

func TestHandleOrderStatusEmailSkipsCanceledRegisteredOrderWhenPayloadStatusEmpty(t *testing.T) {
	task, err := queue.NewOrderStatusEmailTask(queue.OrderStatusEmailPayload{OrderID: 107})
	if err != nil {
		t.Fatalf("new order status email task failed: %v", err)
	}

	consumer := &Consumer{
		Container: &container.Container{},
		orderReader: orderStatusEmailWorkerOrderRepoStub{order: &orderdomain.Order{
			ID:      107,
			OrderNo: "DJ-ORDER-107",
			UserID:  42,
			Status:  constants.OrderStatusCanceled,
		}},
	}
	if err := consumer.handleOrderStatusEmail(context.Background(), task); err != nil {
		t.Fatalf("expected canceled registered-order task to be dropped, got %v", err)
	}
}

func TestOrderEmailItemsRetainChildOrderNumbers(t *testing.T) {
	order := &orderdomain.Order{Children: []orderdomain.Order{{OrderNo: "DJ-CHILD-01", Items: []orderdomain.OrderItem{{Quantity: 1}}}, {OrderNo: "DJ-CHILD-02", Items: []orderdomain.OrderItem{{Quantity: 2}}}}}
	items := orderEmailItems(order, "zh-CN")
	if len(items) != 2 || items[0].ChildOrderNo != "DJ-CHILD-01" || items[1].ChildOrderNo != "DJ-CHILD-02" {
		t.Fatalf("lost child association: %+v", items)
	}
}

func TestOrderEmailDeliveriesRetainOnlyFulfilledChildren(t *testing.T) {
	order := &orderdomain.Order{Children: []orderdomain.Order{{OrderNo: "DJ-CHILD-01", Status: constants.OrderStatusDelivered, Fulfillment: &fulfillmentdomain.Fulfillment{Status: constants.FulfillmentStatusDelivered, Payload: " CODE-01 "}}, {OrderNo: "DJ-CHILD-02"}}}
	got := orderEmailDeliveries(order)
	if len(got) != 1 || got[0].OrderNo != "DJ-CHILD-01" || got[0].Content != "CODE-01" {
		t.Fatalf("unexpected child delivery: %+v", got)
	}
}

func TestBuildOrderFulfillmentEmailPayloadFromChildren(t *testing.T) {
	order := &orderdomain.Order{
		Children: []orderdomain.Order{
			{
				OrderNo:     "DJ-CHILD-01",
				Status:      constants.OrderStatusDelivered,
				Fulfillment: &fulfillmentdomain.Fulfillment{Status: constants.FulfillmentStatusDelivered, Payload: "  SECRET-01  "},
			},
			{
				OrderNo:     "DJ-CHILD-02",
				Fulfillment: nil,
			},
			{
				OrderNo:     "DJ-CHILD-03",
				Status:      constants.OrderStatusDelivered,
				Fulfillment: &fulfillmentdomain.Fulfillment{Status: constants.FulfillmentStatusDelivered, Payload: "    "},
			},
			{
				OrderNo:     "DJ-CHILD-04",
				Status:      constants.OrderStatusDelivered,
				Fulfillment: &fulfillmentdomain.Fulfillment{Status: constants.FulfillmentStatusDelivered, Payload: "SECRET-04-L1\nSECRET-04-L2"},
			},
		},
	}

	got := buildOrderFulfillmentEmailPayload(order)
	want := "[DJ-CHILD-01]\nSECRET-01\n\n[DJ-CHILD-04]\nSECRET-04-L1\nSECRET-04-L2"
	if got != want {
		t.Fatalf("unexpected payload, want %q, got %q", want, got)
	}
}
