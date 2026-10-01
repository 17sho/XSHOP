package application

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/dujiao-next/internal/constants"
	orderdomain "github.com/dujiao-next/internal/modules/order/domain"
	paymentdomain "github.com/dujiao-next/internal/modules/payment/domain"
	"github.com/dujiao-next/internal/modules/payment/infrastructure/gateway/bepusdt"
	walletdomain "github.com/dujiao-next/internal/modules/wallet/domain"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"github.com/dujiao-next/internal/shared/money"

	"github.com/shopspring/decimal"
)

func TestSupportsBlindWebhookCandidateMatching(t *testing.T) {
	cases := []struct {
		channelType string
		want        bool
	}{
		{constants.PaymentChannelTypeWechat, true},
		{constants.PaymentChannelTypeStripe, true},
		{constants.PaymentChannelTypePaypal, false},
		{constants.PaymentChannelTypeAlipay, false},
		{"", false},
		{"  WECHAT  ", true},
	}
	for _, tc := range cases {
		if got := supportsBlindWebhookCandidateMatching(tc.channelType); got != tc.want {
			t.Fatalf("channelType=%q: got %v, want %v", tc.channelType, got, tc.want)
		}
	}
}

// TestHandleWechatWebhookFallbackNoCandidate 验证 channel_id 为 0 且没有任何 active wechat
// channel 时,wechat webhook 返回 ErrPaymentChannelNotFound(而非 ErrPaymentInvalid);
// 这是相对 67bbc47 重构后回归行为(硬性拒绝 channel_id==0)的关键修复。
func TestHandleWechatWebhookFallbackNoCandidate(t *testing.T) {
	svc, _ := setupPaymentServiceWalletTest(t)

	_, _, err := svc.HandleWechatWebhook(WebhookCallbackInput{
		ChannelID: 0,
		Body:      []byte(`{"resource":{}}`),
		Headers:   map[string]string{},
	})
	if !errors.Is(err, ErrPaymentChannelNotFound) {
		t.Fatalf("expected ErrPaymentChannelNotFound for empty wechat candidate list, got: %v", err)
	}
}

// TestHandlePaypalWebhookRequiresChannelID 验证 paypal webhook 必须带 channel_id,
// 不具备盲匹配能力;channel_id==0 直接走 ErrPaymentInvalid。
func TestHandlePaypalWebhookRequiresChannelID(t *testing.T) {
	svc, _ := setupPaymentServiceWalletTest(t)

	_, _, err := svc.HandlePaypalWebhook(WebhookCallbackInput{
		ChannelID: 0,
		Body:      []byte(`{}`),
		Headers:   map[string]string{},
	})
	if !errors.Is(err, ErrPaymentInvalid) {
		t.Fatalf("expected ErrPaymentInvalid for paypal without channel_id, got: %v", err)
	}
}

func TestSyncCallbackImmutableReferenceBinding(t *testing.T) {
	for _, provider := range []string{constants.PaymentProviderEpay, constants.PaymentProviderBepusdt} {
		for _, tc := range []struct {
			name, stored, incoming string
			accepted               bool
		}{
			{"matching", "trade-created", "trade-created", true},
			{"conflicting", "trade-created", "trade-other", false},
			{"missing", "trade-created", "", false},
			{"empty creation reference", "", "trade-paid", true},
			{"creation order placeholder", "gateway-order", "trade-paid", true},
		} {
			t.Run(provider+"/"+tc.name, func(t *testing.T) {
				svc, db := setupPaymentServiceWalletTest(t)
				amount := money.FromDecimal(decimal.NewFromInt(10))
				order := &orderdomain.Order{OrderNo: "business-order", Status: constants.OrderStatusPendingPayment, Currency: "CNY", TotalAmount: amount}
				if err := db.Create(order).Error; err != nil {
					t.Fatal(err)
				}
				channel := &paymentdomain.PaymentChannel{ProviderType: provider, ChannelType: constants.PaymentChannelTypeAlipay, ConfigJSON: jsonmap.JSON{"merchant_id": "fixture-merchant", "merchant_key": "fixture-secret", "epay_version": "v1", "auth_token": "fixture-secret", "fiat": "CNY"}}
				if err := db.Create(channel).Error; err != nil {
					t.Fatal(err)
				}
				payment := &paymentdomain.Payment{OrderID: order.ID, ChannelID: channel.ID, ProviderType: provider, ChannelType: channel.ChannelType, GatewayOrderNo: "gateway-order", ProviderRef: tc.stored, Status: constants.PaymentStatusPending, Amount: amount, Currency: "CNY", ProviderPayload: jsonmap.JSON{"creation_snapshot": "preserved"}}
				if err := db.Create(payment).Error; err != nil {
					t.Fatal(err)
				}
				form, body := signedReferenceCallback(t, provider, payment.GatewayOrderNo, tc.incoming)
				_, err := svc.HandleSyncCallback(channel, form, body)
				if (err == nil) != tc.accepted {
					t.Fatalf("accepted=%v got error %v", tc.accepted, err)
				}
				if tc.accepted {
					if _, err := svc.HandleSyncCallback(channel, form, body); err != nil {
						t.Fatalf("retry failed: %v", err)
					}
					// Once bound by settlement, neither missing nor conflicting references
					// may mutate the payment on retries.
					for _, bad := range []string{"", "different-on-retry"} {
						badForm, badBody := signedReferenceCallback(t, provider, payment.GatewayOrderNo, bad)
						if _, err := svc.HandleSyncCallback(channel, badForm, badBody); err == nil {
							t.Errorf("accepted retry reference %q", bad)
						}
					}
				}
				got, err := svc.paymentRepo.GetByID(payment.ID)
				if err != nil {
					t.Fatal(err)
				}
				if err := db.First(order, order.ID).Error; err != nil {
					t.Fatal(err)
				}
				if tc.accepted {
					if got.Status != constants.PaymentStatusSuccess || got.PaidAt == nil || got.ProviderRef != tc.incoming || order.PaidAt == nil {
						t.Fatalf("inconsistent settlement: payment=%+v order=%+v", got, order)
					}
				} else if got.Status != constants.PaymentStatusPending || got.PaidAt != nil || got.ProviderRef != tc.stored || order.PaidAt != nil {
					t.Fatalf("rejected callback mutated settlement: payment=%+v order=%+v", got, order)
				}
				if got.ProviderPayload["creation_snapshot"] != "preserved" {
					t.Fatal("creation metadata lost")
				}
			})
		}
	}
}

// Synthetic signatures are only used against the in-memory application fixture.
func signedReferenceCallback(t *testing.T, provider, orderNo, reference string) (map[string][]string, []byte) {
	t.Helper()
	if provider == constants.PaymentProviderEpay {
		params := map[string]string{"pid": "fixture-merchant", "out_trade_no": orderNo, "trade_no": reference, "trade_status": "TRADE_SUCCESS", "money": "10.00", "type": "alipay"}
		var keys []string
		for key, value := range params {
			if value != "" {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		var pairs []string
		form := map[string][]string{}
		for _, key := range keys {
			pairs = append(pairs, key+"="+params[key])
			form[key] = []string{params[key]}
		}
		digest := md5.Sum([]byte(strings.Join(pairs, "&") + "fixture-secret"))
		form["sign"] = []string{hex.EncodeToString(digest[:])}
		form["sign_type"] = []string{"MD5"}
		return form, nil
	}
	data := &bepusdt.CallbackData{TradeID: reference, OrderID: orderNo, Amount: 10.0, ActualAmount: 1.0, Status: bepusdt.StatusSuccess}
	data.Signature = bepusdt.Sign(map[string]interface{}{"trade_id": data.TradeID, "order_id": data.OrderID, "amount": data.GetAmount(), "actual_amount": data.GetActualAmount(), "token": data.Token, "block_transaction_id": data.BlockTransactionID, "status": data.Status}, "fixture-secret")
	body, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	return nil, body
}

func TestWalletCallbackReferenceBindingAndLateSettlement(t *testing.T) {
	for _, provider := range []string{constants.PaymentProviderEpay, constants.PaymentProviderBepusdt} {
		for _, initial := range []string{constants.PaymentStatusPending, constants.PaymentStatusFailed, constants.PaymentStatusExpired} {
			t.Run(provider+"/"+initial, func(t *testing.T) {
				svc, db := setupPaymentServiceWalletTest(t)
				payment, recharge := createWalletRechargeFixture(t, db, initial, initial)
				payment.ProviderType = provider
				payment.ProviderRef = "immutable-trade"
				if err := db.Save(payment).Error; err != nil {
					t.Fatal(err)
				}
				input := buildWalletRechargeCallbackInput(payment, recharge, constants.PaymentStatusSuccess, "other-trade")
				if _, err := svc.HandleCallback(input); err == nil {
					t.Fatal("conflicting wallet reference accepted")
				}
				got, err := svc.paymentRepo.GetByID(payment.ID)
				if err != nil {
					t.Fatal(err)
				}
				if got.Status != initial || got.PaidAt != nil {
					t.Fatal("rejected wallet callback changed state")
				}
				input.ProviderRef = "immutable-trade"
				for retry := 0; retry < 2; retry++ {
					if _, err := svc.HandleCallback(input); err != nil {
						t.Fatalf("settlement/retry failed: %v", err)
					}
				}
				input.Status = constants.PaymentStatusPending
				if _, err := svc.HandleCallback(input); err != nil {
					t.Fatal(err)
				}
				assertWalletRechargeSuccessState(t, db, payment.ID, recharge.ID, recharge.UserID, payment.Amount.Decimal)
				var count int64
				if err := db.Model(&walletdomain.Transaction{}).Count(&count).Error; err != nil {
					t.Fatal(err)
				}
				if count != 1 {
					t.Fatalf("wallet ledger entries=%d, want 1", count)
				}
			})
		}
	}
}

func TestCallbackReferencePolicyPreservesObjectTransitions(t *testing.T) {
	amount := money.FromDecimal(decimal.NewFromInt(10))
	for _, tc := range []struct {
		name, provider, channel, stored, incoming string
		payload                                   jsonmap.JSON
		accepted                                  bool
	}{
		{"stripe session to intent", constants.PaymentProviderOfficial, constants.PaymentChannelTypeStripe, "cs_fixture", "pi_fixture", nil, true},
		{"wechat placeholder to transaction", constants.PaymentProviderOfficial, constants.PaymentChannelTypeWechat, "gateway-order", "wx-transaction", nil, true},
		{"epay immutable snapshot", constants.PaymentProviderEpay, "", "gateway-order", "other", jsonmap.JSON{"trade_no": "gateway-order"}, false},
		{"bepusdt immutable transaction snapshot", constants.PaymentProviderBepusdt, "", "gateway-order", "other", jsonmap.JSON{"data": map[string]interface{}{"trade_id": "gateway-order", "order_mode": "transaction"}}, false},
		// The cashier order may be distinct from a selected on-chain transaction.
		// Do not enforce creation-reference equality without a documented invariant.
		{"bepusdt cashier transition", constants.PaymentProviderBepusdt, "", "cashier-order", "selected-transaction", jsonmap.JSON{"data": map[string]interface{}{"trade_id": "cashier-order", "order_mode": "cashier"}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payment := &paymentdomain.Payment{ProviderType: tc.provider, ChannelType: tc.channel, GatewayOrderNo: "gateway-order", ProviderRef: tc.stored, Status: constants.PaymentStatusPending, Amount: amount, Currency: "CNY", ProviderPayload: tc.payload}
			input := PaymentCallbackInput{ProviderRef: tc.incoming, Amount: amount, Currency: "CNY"}
			err := validateCallbackPaymentFacts(payment, "business-order", constants.PaymentStatusSuccess, input)
			if (err == nil) != tc.accepted {
				t.Fatalf("accepted=%v error=%v", tc.accepted, err)
			}
		})
	}
}
