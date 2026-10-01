package contract

import (
	"time"

	ordercontract "github.com/dujiao-next/internal/modules/order/contract"
)

// Queue 聚合支付流程需要的异步任务能力，并复用订单状态邮件端口。
type Queue interface {
	ordercontract.Queue
	EnqueueOrderAutoFulfill(orderID uint) error

	EnqueueWalletRechargeExpire(paymentID uint, delay time.Duration) error
}
