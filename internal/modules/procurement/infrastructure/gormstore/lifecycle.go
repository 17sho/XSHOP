package gormstore

import (
	"errors"
	"strings"
	"time"

	"github.com/dujiao-next/internal/config"
	"github.com/dujiao-next/internal/constants"
	procurementcontract "github.com/dujiao-next/internal/modules/procurement/contract"
	procurementdomain "github.com/dujiao-next/internal/modules/procurement/domain"
	settingsapp "github.com/dujiao-next/internal/modules/settings/application"
	"github.com/dujiao-next/internal/queue"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"github.com/dujiao-next/internal/telegramidentity"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Lifecycle owns the procurement-side persistence operations that span
// fulfillments and parent/child order status. It deliberately uses private
// records through the procurement-owned store.
type Lifecycle struct {
	db                 *gorm.DB
	queue              StatusEmailQueue
	settings           *settingsapp.Service
	defaultEmailConfig config.EmailConfig
}

var _ procurementcontract.OrderLifecycle = (*Lifecycle)(nil)

type StatusEmailQueue interface {
	EnqueueOrderStatusEmail(payload queue.OrderStatusEmailPayload, opts ...asynq.Option) error
}

type lifecycleOrderRecord struct {
	ID         uint                   `gorm:"primarykey"`
	ParentID   *uint                  `gorm:"index"`
	UserID     uint                   `gorm:"index;not null"`
	GuestEmail string                 `gorm:"index"`
	Status     string                 `gorm:"index;not null"`
	DeletedAt  gorm.DeletedAt         `gorm:"index"`
	Children   []lifecycleOrderRecord `gorm:"foreignKey:ParentID"`
}

func (lifecycleOrderRecord) TableName() string { return "orders" }

type lifecycleUserRecord struct {
	ID        uint `gorm:"primarykey"`
	Email     string
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

func (lifecycleUserRecord) TableName() string { return "users" }

type lifecycleFulfillmentRecord struct {
	ID            uint           `gorm:"primarykey"`
	OrderID       uint           `gorm:"uniqueIndex;not null"`
	Type          string         `gorm:"not null"`
	Status        string         `gorm:"not null"`
	Payload       string         `gorm:"type:text"`
	LogisticsJSON jsonmap.JSON   `gorm:"type:json"`
	DeliveredAt   *time.Time     `gorm:"index"`
	CreatedAt     time.Time      `gorm:"index"`
	UpdatedAt     time.Time      `gorm:"index"`
	DeletedAt     gorm.DeletedAt `gorm:"index"`
}

func (lifecycleFulfillmentRecord) TableName() string { return "fulfillments" }

func NewLifecycle(
	db *gorm.DB,
	queueClient StatusEmailQueue,
	settings *settingsapp.Service,
	defaultEmailConfig config.EmailConfig,
) *Lifecycle {
	return &Lifecycle{
		db: db, queue: queueClient, settings: settings, defaultEmailConfig: defaultEmailConfig,
	}
}

func (s *Store) NewLifecycle(
	queueClient StatusEmailQueue,
	settings *settingsapp.Service,
	defaultEmailConfig config.EmailConfig,
) *Lifecycle {
	return NewLifecycle(s.db, queueClient, settings, defaultEmailConfig)
}

func (l *Lifecycle) CreateUpstreamFulfillment(orderID uint, fulfillment *procurementcontract.Fulfillment, now time.Time) error {
	// Status-only manual delivery is valid; automatic/unknown delivery must remain retryable.
	if fulfillment == nil || (strings.TrimSpace(fulfillment.Payload) == "" && len(fulfillment.DeliveryData) == 0 && fulfillment.Type != constants.FulfillmentTypeManual) {
		var kinds []string
		if err := l.db.Table("order_items AS i").Select("m.upstream_fulfillment_type").Joins("JOIN product_mappings m ON m.local_product_id = i.product_id AND m.deleted_at IS NULL").Where("i.order_id = ? AND i.deleted_at IS NULL", orderID).Scan(&kinds).Error; err != nil {
			return err
		}
		if len(kinds) != 1 || kinds[0] != constants.FulfillmentTypeManual {
			return errors.New("delivery details missing")
		}
		fulfillment = &procurementcontract.Fulfillment{Type: constants.FulfillmentTypeManual, Status: constants.FulfillmentStatusDelivered}
	}
	deliveredAt := fulfillment.DeliveredAt
	if deliveredAt == nil {
		deliveredAt = &now
	}
	return l.db.Transaction(func(tx *gorm.DB) error {
		local, err := lockLifecycleOrder(tx, orderID)
		if err != nil {
			return err
		}
		if local.Status != "paid" && local.Status != "fulfilling" && local.Status != "delivered" && local.Status != "completed" {
			return procurementcontract.ErrStatusInvalid
		}
		var proc procurementdomain.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("local_order_id = ? AND deleted_at IS NULL", orderID).First(&proc).Error; err != nil {
			return err
		}
		if proc.Status == "canceled" || proc.Status == "refunded" {
			return procurementcontract.ErrStatusInvalid
		}
		var existing lifecycleFulfillmentRecord
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("order_id = ?", orderID).
			First(&existing).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := tx.Create(&lifecycleFulfillmentRecord{
				OrderID: orderID, Type: constants.FulfillmentTypeUpstream,
				Status: constants.FulfillmentStatusDelivered, Payload: fulfillment.Payload,
				LogisticsJSON: fulfillment.DeliveryData, DeliveredAt: deliveredAt,
				CreatedAt: now, UpdatedAt: now,
			}).Error; err != nil {
				return err
			}
		}
		if local.Status == "paid" || local.Status == "fulfilling" {
			if err := tx.Model(&local).Updates(map[string]interface{}{"status": "delivered", "updated_at": now}).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&proc).Updates(map[string]interface{}{"status": "fulfilled", "upstream_payload": fulfillment.Payload, "updated_at": now}).Error; err != nil {
			return err
		}
		if local.ParentID != nil {
			_, err := syncParentStatus(tx, *local.ParentID, now)
			return err
		}
		return nil
	})
}

// Lock parent before child so concurrent sibling finalizations serialize without
// each holding a child lock while waiting for the other child's parent repair.
func lockLifecycleOrder(tx *gorm.DB, orderID uint) (*lifecycleOrderRecord, error) {
	var snapshot lifecycleOrderRecord
	if err := tx.First(&snapshot, orderID).Error; err != nil {
		return nil, err
	}
	if snapshot.ParentID != nil {
		var parent lifecycleOrderRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&parent, *snapshot.ParentID).Error; err != nil {
			return nil, err
		}
	}
	var local lifecycleOrderRecord
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&local, orderID).Error; err != nil {
		return nil, err
	}
	return &local, nil
}

func (l *Lifecycle) SyncParentStatus(parentID uint, now time.Time) (string, error) {
	var status string
	err := l.db.Transaction(func(tx *gorm.DB) error {
		var err error
		status, err = syncParentStatus(tx, parentID, now)
		return err
	})
	return status, err
}

func syncParentStatus(tx *gorm.DB, parentID uint, now time.Time) (string, error) {
	if parentID == 0 {
		return "", nil
	}
	var parent lifecycleOrderRecord
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Preload("Children").First(&parent, parentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", err
	}
	if parent.ParentID != nil {
		return "", nil
	}
	if parent.Status == constants.OrderStatusCanceled {
		return parent.Status, nil
	}
	newStatus := calculateParentStatus(parent.Children, parent.Status)
	if newStatus == "" || newStatus == parent.Status {
		return parent.Status, nil
	}
	if err := tx.Table(parent.TableName()).Where("id = ? AND deleted_at IS NULL", parent.ID).Updates(map[string]interface{}{
		"status": newStatus, "updated_at": now,
	}).Error; err != nil {
		return "", err
	}
	return newStatus, nil
}

func (l *Lifecycle) EnqueueStatusEmail(orderID uint, status string) (bool, error) {
	if l.queue == nil || orderID == 0 {
		return true, nil
	}
	if l.settings != nil {
		smtpSetting, err := l.settings.GetSMTPSetting(l.defaultEmailConfig)
		if err != nil {
			return false, err
		}
		if !smtpSetting.Enabled || !smtpSetting.OrderNotificationEnabled {
			return true, nil
		}
	}
	receiverEmail, err := l.resolveReceiverEmail(orderID)
	if err == nil {
		receiverEmail = strings.TrimSpace(receiverEmail)
		if receiverEmail == "" || telegramidentity.IsPlaceholderEmail(receiverEmail) {
			return true, nil
		}
	}
	if err := l.queue.EnqueueOrderStatusEmail(queue.OrderStatusEmailPayload{
		OrderID: orderID, Status: strings.TrimSpace(status),
	}); err != nil {
		return false, err
	}
	return false, nil
}

func (l *Lifecycle) resolveReceiverEmail(orderID uint) (string, error) {
	var order lifecycleOrderRecord
	if err := l.db.Select("user_id", "guest_email").First(&order, orderID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", err
	}
	if order.UserID == 0 {
		return strings.TrimSpace(order.GuestEmail), nil
	}
	var user lifecycleUserRecord
	if err := l.db.Select("email").First(&user, order.UserID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(user.Email), nil
}

func calculateParentStatus(children []lifecycleOrderRecord, currentStatus string) string {
	if len(children) == 0 {
		return currentStatus
	}
	counts := make(map[string]int, 8)
	for _, child := range children {
		counts[strings.ToLower(strings.TrimSpace(child.Status))]++
	}
	size := len(children)
	if counts[constants.OrderStatusCanceled] == size {
		return constants.OrderStatusCanceled
	}
	if counts[constants.OrderStatusRefunded] == size {
		return constants.OrderStatusRefunded
	}
	if counts[constants.OrderStatusRefunded] > 0 || counts[constants.OrderStatusPartiallyRefunded] > 0 {
		return constants.OrderStatusPartiallyRefunded
	}
	if counts[constants.OrderStatusCompleted] == size {
		return constants.OrderStatusCompleted
	}
	delivered := counts[constants.OrderStatusDelivered] + counts[constants.OrderStatusCompleted]
	if delivered == size {
		return constants.OrderStatusDelivered
	}
	if delivered > 0 {
		return constants.OrderStatusPartiallyDelivered
	}
	if counts[constants.OrderStatusFulfilling] > 0 {
		return constants.OrderStatusFulfilling
	}
	if counts[constants.OrderStatusPaid] > 0 {
		return constants.OrderStatusPaid
	}
	if counts[constants.OrderStatusPendingPayment] > 0 {
		return constants.OrderStatusPendingPayment
	}
	return currentStatus
}
