package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/logger"
	downstreamcontract "github.com/dujiao-next/internal/modules/downstreamcallback/contract"
	downstreamdomain "github.com/dujiao-next/internal/modules/downstreamcallback/domain"
	orderdomain "github.com/dujiao-next/internal/modules/order/domain"
)

const maxCallbackRetries = 5

var callbackRetryDelays = []time.Duration{
	30 * time.Second,
	60 * time.Second,
	120 * time.Second,
	300 * time.Second,
}

// Options 声明下游回调应用服务的全部端口。
type Options struct {
	References  downstreamcontract.Repository
	Orders      downstreamcontract.OrderReader
	Credentials downstreamcontract.CredentialReader
	Queue       downstreamcontract.CallbackQueue
	Deliverer   downstreamcontract.Deliverer
	Now         func() time.Time
}

// Service 编排 B 侧下游回调引用、投递和重试状态。
type Service struct {
	references  downstreamcontract.Repository
	orders      downstreamcontract.OrderReader
	credentials downstreamcontract.CredentialReader
	queue       downstreamcontract.CallbackQueue
	deliverer   downstreamcontract.Deliverer
	now         func() time.Time
}

// NewService 创建下游回调应用服务。
func NewService(options Options) *Service {
	if options.References == nil {
		panic("downstream callback service: references are nil")
	}
	if options.Orders == nil {
		panic("downstream callback service: orders are nil")
	}
	if options.Credentials == nil {
		panic("downstream callback service: credentials are nil")
	}
	if options.Deliverer == nil {
		panic("downstream callback service: deliverer is nil")
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	return &Service{
		references:  options.References,
		orders:      options.Orders,
		credentials: options.Credentials,
		queue:       options.Queue,
		deliverer:   options.Deliverer,
		now:         options.Now,
	}
}

// CreateRef 创建下游订单引用并强制初始化为待回调状态。
func (s *Service) CreateRef(ref *downstreamdomain.OrderRef) error {
	if ref == nil || ref.OrderID == 0 {
		return downstreamcontract.ErrInvalidRef
	}
	ref.CallbackStatus = downstreamdomain.StatusPending
	return s.references.Create(ref)
}

// GetByOrderID 根据订单 ID 查询下游引用。
func (s *Service) GetByOrderID(orderID uint) (*downstreamdomain.OrderRef, error) {
	return s.references.GetByOrderID(orderID)
}

// EnqueueCallback 在 B 侧订单状态变更后解析引用并投递回调任务。
func (s *Service) EnqueueCallback(orderID uint) {
	if s.queue == nil {
		logger.Debugw("downstream_callback_skip_no_queue", "order_id", orderID)
		return
	}

	ref, err := s.references.GetByOrderID(orderID)
	if err != nil || ref == nil {
		order, orderErr := s.orders.GetByID(orderID)
		if orderErr != nil || order == nil || order.ParentID == nil {
			logger.Debugw("downstream_callback_skip_no_ref", "order_id", orderID, "error", err)
			return
		}
		ref, err = s.references.GetByOrderID(*order.ParentID)
		if err != nil || ref == nil {
			logger.Debugw("downstream_callback_skip_no_ref", "order_id", orderID, "parent_id", *order.ParentID, "error", err)
			return
		}
		logger.Debugw("downstream_callback_resolved_parent_ref", "order_id", orderID, "parent_id", *order.ParentID, "ref_id", ref.ID)
	}

	if strings.TrimSpace(ref.CallbackURL) == "" {
		logger.Warnw("downstream_callback_skip_empty_url", "order_id", orderID, "ref_id", ref.ID)
		return
	}
	logger.Infow("downstream_callback_enqueue",
		"order_id", orderID,
		"ref_id", ref.ID,
		"callback_status", ref.CallbackStatus,
	)
	if ref.CallbackStatus != downstreamdomain.StatusPending {
		ref.CallbackStatus = downstreamdomain.StatusPending
		ref.CallbackRetryCount = 0
		_ = s.references.Update(ref)
	}
	if err := s.queue.EnqueueCallback(ref.ID, 0); err != nil {
		logger.Warnw("downstream_enqueue_callback_failed", "order_id", orderID, "ref_id", ref.ID, "error", err)
	}
}

// SendCallback 读取最新订单状态并执行一次下游回调。
func (s *Service) SendCallback(ctx context.Context, refID uint) error {
	ref, err := s.references.GetByID(refID)
	if err != nil {
		return err
	}
	if ref == nil {
		return downstreamcontract.ErrRefNotFound
	}
	if strings.TrimSpace(ref.CallbackURL) == "" {
		return nil
	}

	order, err := s.orders.GetByID(ref.OrderID)
	if err != nil || order == nil {
		logger.Warnw("downstream_callback_order_not_found", "ref_id", ref.ID, "order_id", ref.OrderID)
		return err
	}
	credential, err := s.credentials.GetByID(ref.ApiCredentialID)
	if err != nil {
		// A read failure is not evidence of revocation. Fail closed and allow the
		// worker's normal bounded error handling without disclosing repository data.
		return fmt.Errorf("credential lookup failed for ref %d", ref.ID)
	}
	if credential == nil || !credential.DeliveryAllowed {
		// Acknowledge the job without retrying. Keep paid order, destination, retry
		// history and last actual attempt intact for explicit retry/status events.
		ref.CallbackStatus = downstreamdomain.StatusPaused
		logger.Infow("downstream_callback_paused", "ref_id", ref.ID, "credential_id", ref.ApiCredentialID)
		return s.references.Update(ref)
	}

	event := "order.status_changed"
	if order.Status == constants.OrderStatusDelivered || order.Status == constants.OrderStatusCompleted {
		event = "order.fulfilled"
	}
	fulfillment := deliveredFulfillment(order)
	now := s.now()
	payload := downstreamcontract.CallbackPayload{
		Event:             event,
		OrderID:           order.ID,
		OrderNo:           order.OrderNo,
		DownstreamOrderNo: ref.DownstreamOrderNo,
		Status:            order.Status,
		Fulfillment:       fulfillment,
		Timestamp:         now.Unix(),
	}

	logger.Infow("downstream_callback_sending",
		"ref_id", ref.ID,
		"order_id", order.ID,
		"event", event,
		"status", order.Status,
		"has_fulfillment", fulfillment != nil,
	)
	if err := s.deliverer.Send(ctx, downstreamcontract.DeliveryRequest{
		URL:       ref.CallbackURL,
		APIKey:    credential.APIKey,
		APISecret: credential.APISecret,
		Payload:   payload,
	}); err != nil {
		logger.Warnw("downstream_callback_http_error", "ref_id", ref.ID)
		return s.handleCallbackFailure(ref, now)
	}

	ref.CallbackStatus = downstreamdomain.StatusSent
	ref.LastCallbackAt = &now
	return s.references.Update(ref)
}

func deliveredFulfillment(order *downstreamcontract.OrderSnapshot) *downstreamcontract.Fulfillment {
	if order == nil {
		return nil
	}
	// Preserve the stable singular fulfillment shape: first releasable parent,
	// otherwise first releasable child. Never borrow the parent's paid state.
	candidates := []*downstreamcontract.OrderSnapshot{order}
	for i := range order.Children {
		candidates = append(candidates, &order.Children[i])
	}
	for _, candidate := range candidates {
		source := candidate.Fulfillment
		if source != nil && orderdomain.FulfillmentReleaseAllowed(candidate.Status, candidate.PaidAt, source.Status, source.DeliveredAt) {
			copy := *source
			return &copy
		}
	}
	return nil
}

func (s *Service) handleCallbackFailure(ref *downstreamdomain.OrderRef, now time.Time) error {
	ref.CallbackRetryCount++
	ref.LastCallbackAt = &now

	if ref.CallbackRetryCount >= maxCallbackRetries {
		ref.CallbackStatus = downstreamdomain.StatusFailed
		logger.Warnw("downstream_callback_max_retries",
			"ref_id", ref.ID,
			"order_id", ref.OrderID,
			"retry_count", ref.CallbackRetryCount,
			"delivery_failed", true,
		)
	} else if s.queue != nil {
		index := ref.CallbackRetryCount - 1
		if index >= len(callbackRetryDelays) {
			index = len(callbackRetryDelays) - 1
		}
		if err := s.queue.EnqueueCallback(ref.ID, callbackRetryDelays[index]); err != nil {
			logger.Warnw("downstream_callback_requeue_failed", "ref_id", ref.ID, "error", err)
		}
	}

	return s.references.Update(ref)
}
