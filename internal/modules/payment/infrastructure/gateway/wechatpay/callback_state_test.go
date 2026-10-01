package wechatpay

import (
	"context"
	"encoding/json"
	"testing"
)

func TestSuccessfulWebhookRequiresTransactionEvent(t *testing.T) {
	key := newTestWechatPayKey(t)
	raw := baseWechatPayConfig(t)
	addPublicKeyMode(raw, key)
	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range []string{"TRANSACTION.SUCCESS", "REFUND.SUCCESS", "UNKNOWN", ""} {
		t.Run(event, func(t *testing.T) {
			headers, body := buildSignedWechatPayNotification(t, key, cfg.APIV3Key, `{"appid":"wx1234567890","mchid":"1900000109","out_trade_no":"fixture","transaction_id":"fixture-tx","trade_state":"SUCCESS","amount":{"total":100,"currency":"CNY"}}`)
			var payload map[string]interface{}
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatal(err)
			}
			payload["event_type"] = event
			body, err = json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			headers["Wechatpay-Signature"] = signWechatPayMessage(t, key.privateKey, headers["Wechatpay-Timestamp"]+"\n"+headers["Wechatpay-Nonce"]+"\n"+string(body)+"\n")
			_, err := VerifyAndDecodeWebhook(context.Background(), cfg, headers, body)
			if (err == nil) != (event == "TRANSACTION.SUCCESS") {
				t.Fatalf("event=%q error=%v", event, err)
			}
		})
	}
}

func TestRefundCannotInitiatePaymentSuccess(t *testing.T) {
	if status, ok := ToPaymentStatus("REFUND"); ok {
		t.Fatalf("refund mapped to payment state %q", status)
	}
}
