package epusdt

import "testing"

func TestVerifyCallbackRequiresExactNonemptyPID(t *testing.T) {
	for _, tc := range []struct {
		name, configured, supplied string
		valid                      bool
	}{
		{"matching", "merchant-1", "merchant-1", true},
		{"different", "merchant-1", "merchant-2", false},
		{"missing", "merchant-1", "", false},
		{"both empty", "", "", false},
		{"both whitespace", " ", " ", false},
		{"whitespace not normalized", "merchant-1", " merchant-1 ", false},
		{"case sensitive", "Merchant-1", "merchant-1", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{PID: tc.configured, SecretKey: "synthetic-fixture-secret"}
			data := &CallbackData{PID: tc.supplied, TradeID: "fixture-trade", OrderID: "fixture-order", Status: StatusSuccess}
			data.Signature = Sign(map[string]interface{}{
				"pid": data.PID, "trade_id": data.TradeID, "order_id": data.OrderID,
				"amount": data.GetAmount(), "actual_amount": data.GetActualAmount(),
				"receive_address": data.ReceiveAddress, "token": data.Token,
				"block_transaction_id": data.BlockTransactionID, "status": data.Status,
			}, cfg.SecretKey)
			if err := VerifyCallback(cfg, data); (err == nil) != tc.valid {
				t.Fatalf("valid=%v, got error %v", tc.valid, err)
			}
		})
	}
}
