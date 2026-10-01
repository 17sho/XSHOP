package queueadapter

import (
	"time"

	procurementcontract "github.com/dujiao-next/internal/modules/procurement/contract"
	"github.com/dujiao-next/internal/queue"

	"github.com/hibiken/asynq"
)

type Client interface {
	EnqueueProcurementSubmit(payload queue.ProcurementSubmitPayload, opts ...asynq.Option) error
	EnqueueProcurementPollStatus(payload queue.ProcurementPollStatusPayload, delay time.Duration) error
}

type Enqueuer struct{ client Client }

var _ procurementcontract.Enqueuer = (*Enqueuer)(nil)

func New(client Client) procurementcontract.Enqueuer {
	if client == nil {
		return nil
	}
	return &Enqueuer{client: client}
}

func (e *Enqueuer) EnqueueSubmit(orderID uint, delay ...time.Duration) error {
	var opts []asynq.Option
	if len(delay) > 0 && delay[0] > 0 {
		opts = append(opts, asynq.ProcessIn(delay[0]))
	}
	return e.client.EnqueueProcurementSubmit(queue.ProcurementSubmitPayload{ProcurementOrderID: orderID}, opts...)
}

func (e *Enqueuer) EnqueuePoll(orderID uint, delay time.Duration) error {
	return e.client.EnqueueProcurementPollStatus(queue.ProcurementPollStatusPayload{ProcurementOrderID: orderID}, delay)
}
