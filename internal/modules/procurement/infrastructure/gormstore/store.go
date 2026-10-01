package gormstore

import (
	"errors"
	"time"

	orderdomain "github.com/dujiao-next/internal/modules/order/domain"

	procurementcontract "github.com/dujiao-next/internal/modules/procurement/contract"
	procurementdomain "github.com/dujiao-next/internal/modules/procurement/domain"
	"github.com/dujiao-next/internal/modules/procurement/infrastructure/orderreader"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Store struct {
	db *gorm.DB
}

var _ procurementcontract.Repository = (*Store)(nil)

func New(db *gorm.DB) *Store { return &Store{db: db} }

func (s *Store) GetByID(id uint) (*procurementdomain.Order, error) {
	var order procurementdomain.Order
	if err := preloadProcurement(s.active()).First(&order, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if err := s.attachLocalOrder(&order); err != nil {
		return nil, err
	}
	return &order, nil
}

func (s *Store) GetByLocalOrderID(localOrderID uint) (*procurementdomain.Order, error) {
	return s.first("local_order_id = ?", localOrderID)
}

func (s *Store) GetByLocalOrderNo(localOrderNo string) (*procurementdomain.Order, error) {
	return s.first("local_order_no = ?", localOrderNo)
}

func (s *Store) first(query string, argument any) (*procurementdomain.Order, error) {
	var order procurementdomain.Order
	if err := preloadProcurement(s.active().Where(query, argument)).First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if err := s.attachLocalOrder(&order); err != nil {
		return nil, err
	}
	return &order, nil
}

// ListUnprocuredOrderIDs recovers paid leaf orders even if initial dispatch died.
func (s *Store) ListUnprocuredOrderIDs(afterID uint, limit int) ([]uint, error) {
	var ids []uint
	err := s.db.Model(&lifecycleOrderRecord{}).Where("orders.id > ? AND orders.status IN ?", afterID, []string{"paid", "fulfilling"}).
		Where("NOT EXISTS (SELECT 1 FROM orders child WHERE child.parent_id = orders.id AND child.deleted_at IS NULL)").
		Where("EXISTS (SELECT 1 FROM order_items item WHERE item.order_id = orders.id AND item.deleted_at IS NULL AND item.fulfillment_type = ?)", "upstream").
		Where("NOT EXISTS (SELECT 1 FROM procurement_orders p WHERE p.local_order_id = orders.id AND p.deleted_at IS NULL)").
		Order("orders.id ASC").Limit(limit).Pluck("orders.id", &ids).Error
	return ids, err
}

func (s *Store) Create(order *procurementdomain.Order) error {
	return s.db.Omit(clause.Associations).Create(order).Error
}

// BeginSubmission is the durable spend fence. A crash/unknown transport result
// leaves submitting in place indefinitely, recoverable by an authenticated
// supplier callback, never by blindly repeating CreateOrder.
func (s *Store) BeginSubmission(id uint, now time.Time) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var snapshot procurementdomain.Order
		if err := tx.Where("deleted_at IS NULL").First(&snapshot, id).Error; err != nil {
			return err
		}
		local, err := lockLifecycleOrder(tx, snapshot.LocalOrderID)
		if err != nil {
			return err
		}
		if local.Status != "paid" && local.Status != "fulfilling" {
			return procurementcontract.ErrStatusInvalid
		}
		result := tx.Model(&procurementdomain.Order{}).Where("id = ? AND deleted_at IS NULL AND status IN ? AND upstream_order_id = 0 AND (next_retry_at IS NULL OR next_retry_at <= ?)", id, []string{"pending", "failed"}, now).
			Updates(map[string]interface{}{"status": "submitting", "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return procurementcontract.ErrStatusInvalid
		}
		return nil
	})
}

// AcceptSubmission attaches supplier identity without regressing an early callback.
func (s *Store) AcceptSubmission(id uint, updates map[string]interface{}, now time.Time) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var snapshot procurementdomain.Order
		if err := tx.First(&snapshot, id).Error; err != nil {
			return err
		}
		local, err := lockLifecycleOrder(tx, snapshot.LocalOrderID)
		if err != nil {
			return err
		}
		var current procurementdomain.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, id).Error; err != nil {
			return err
		}
		if current.Status == "submitting" || current.Status == "pending" || current.Status == "failed" {
			updates["status"] = "accepted"
		}
		if err := tx.Model(&current).Updates(updates).Error; err != nil {
			return err
		}
		if local.Status == "paid" && (current.Status == "submitting" || current.Status == "pending" || current.Status == "failed" || current.Status == "accepted") {
			return tx.Model(&local).Where("status = ?", "paid").Updates(map[string]interface{}{"status": "fulfilling", "updated_at": now}).Error
		}
		return nil
	})
}

// Manual cancellation must validate the observed supplier identity under the
// same lock as BeginSubmission. Zero means certainly unsent, not unknown.
func (s *Store) CancelSubmission(id, confirmedUpstreamID uint, now time.Time) error {
	return s.updateStatus(id, "canceled", map[string]interface{}{"error_message": "manually canceled", "updated_at": now}, &confirmedUpstreamID, false)
}

func (s *Store) UpdateStatus(id uint, status string, updates map[string]interface{}) error {
	return s.updateStatus(id, status, updates, nil, false)
}

// Only a definitive supplier rejection may release an in-flight spend fence.
func (s *Store) FailSubmission(id uint, status string, updates map[string]interface{}) error {
	if status != "failed" && status != "rejected" {
		return procurementcontract.ErrStatusInvalid
	}
	return s.updateStatus(id, status, updates, nil, true)
}

func (s *Store) updateStatus(id uint, status string, updates map[string]interface{}, canceledIdentity *uint, submissionFailure bool) error {
	if updates == nil {
		updates = map[string]interface{}{}
	}
	updates["status"] = status
	return s.db.Transaction(func(tx *gorm.DB) error {
		var snapshot procurementdomain.Order
		if err := tx.Where("deleted_at IS NULL").First(&snapshot, id).Error; err != nil {
			return err
		}
		local, err := lockLifecycleOrder(tx, snapshot.LocalOrderID)
		if err != nil {
			return err
		}
		var current procurementdomain.Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("deleted_at IS NULL").First(&current, id).Error; err != nil {
			return err
		}
		if canceledIdentity != nil {
			if current.UpstreamOrderID != *canceledIdentity || (*canceledIdentity == 0 && (current.Status == "submitting" || current.Status == "accepted")) {
				return procurementcontract.ErrStatusInvalid
			}
		}
		if submissionFailure && current.Status != "submitting" {
			return procurementcontract.ErrStatusInvalid
		}
		allowed := true
		switch status {
		case "pending":
			allowed = current.Status == "pending" || current.Status == "failed" || current.Status == "rejected"
		case "failed", "rejected":
			allowed = current.Status == "pending" || current.Status == "failed" || (submissionFailure && current.Status == "submitting")
		case "accepted":
			allowed = current.Status == "accepted" || current.Status == "pending" || current.Status == "failed"
		case "canceled":
			allowed = current.Status == "submitting" || current.Status == "pending" || current.Status == "failed" || current.Status == "rejected" || current.Status == "accepted"
		case "partially_refunded":
			allowed = current.Status != "refunded" && current.Status != "canceled"
		}
		if !allowed {
			return procurementcontract.ErrStatusInvalid
		}
		if err := tx.Model(&current).Where("status = ?", current.Status).Updates(updates).Error; err != nil {
			return err
		}
		if status == "canceled" || status == "rejected" {
			now := time.Now()
			if err := tx.Model(local).Where("status = ?", "fulfilling").Updates(map[string]interface{}{"status": "paid", "updated_at": now}).Error; err != nil {
				return err
			}
			if local.ParentID != nil {
				_, err := syncParentStatus(tx, *local.ParentID, now)
				return err
			}
		}
		return nil
	})
}

func (s *Store) List(filter procurementcontract.ListFilter) ([]procurementdomain.Order, int64, error) {
	query := applyFilter(s.active().Model(&procurementdomain.Order{}), filter)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if filter.Ascending {
		query = query.Order("id ASC")
	} else {
		query = query.Order("created_at DESC")
	}
	query = preloadProcurement(query)
	if filter.Page > 0 && filter.PageSize > 0 {
		query = query.Offset((filter.Page - 1) * filter.PageSize).Limit(filter.PageSize)
	}
	var orders []procurementdomain.Order
	if err := query.Find(&orders).Error; err != nil {
		return nil, 0, err
	}
	if err := s.attachLocalOrders(orders); err != nil {
		return nil, 0, err
	}
	return orders, total, nil
}

func (s *Store) StatsByStatus(filter procurementcontract.ListFilter) (map[string]int64, error) {
	filter.Status = ""
	type row struct {
		Status string
		Count  int64
	}
	var rows []row
	query := applyFilter(s.active().Model(&procurementdomain.Order{}), filter)
	if err := query.Select("status, COUNT(*) as count").Group("status").Scan(&rows).Error; err != nil {
		return nil, err
	}
	result := make(map[string]int64, len(rows))
	for _, item := range rows {
		result[item.Status] = item.Count
	}
	return result, nil
}

func (s *Store) ListByConnectionAndTimeRange(connectionID uint, start, end time.Time) ([]procurementdomain.Order, error) {
	var orders []procurementdomain.Order
	query := preloadProcurement(s.active().Where(
		"connection_id = ? AND created_at >= ? AND created_at <= ?",
		connectionID, start, end,
	))
	if err := query.Find(&orders).Error; err != nil {
		return nil, err
	}
	if err := s.attachLocalOrders(orders); err != nil {
		return nil, err
	}
	return orders, nil
}

func (s *Store) active() *gorm.DB {
	return s.db.Where("procurement_orders.deleted_at IS NULL")
}

func preloadProcurement(query *gorm.DB) *gorm.DB {
	return query.Preload("Connection", "deleted_at IS NULL")
}

func applyFilter(query *gorm.DB, filter procurementcontract.ListFilter) *gorm.DB {
	if filter.AfterID > 0 {
		query = query.Where("id > ?", filter.AfterID)
	}
	if len(filter.Statuses) > 0 {
		query = query.Where("status IN ?", filter.Statuses)
	}
	if filter.ConnectionID > 0 {
		query = query.Where("connection_id = ?", filter.ConnectionID)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.LocalOrderNo != "" {
		query = query.Where("local_order_no = ?", filter.LocalOrderNo)
	}
	if filter.UpstreamOrderNo != "" {
		query = query.Where("upstream_order_no = ?", filter.UpstreamOrderNo)
	}
	if filter.CreatedFrom != nil {
		query = query.Where("created_at >= ?", *filter.CreatedFrom)
	}
	if filter.CreatedTo != nil {
		query = query.Where("created_at <= ?", *filter.CreatedTo)
	}
	return query
}

func (s *Store) attachLocalOrder(order *procurementdomain.Order) error {
	if order == nil || order.LocalOrderID == 0 {
		return nil
	}
	var local orderdomain.Order
	if err := s.db.
		Where("orders.deleted_at IS NULL").
		Preload("Items", "deleted_at IS NULL").
		First(&local, order.LocalOrderID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	snapshot := orderreader.MapOrder(local)
	order.LocalOrder = &snapshot
	return nil
}

func (s *Store) attachLocalOrders(orders []procurementdomain.Order) error {
	if len(orders) == 0 {
		return nil
	}
	ids := make([]uint, 0, len(orders))
	for i := range orders {
		if orders[i].LocalOrderID > 0 {
			ids = append(ids, orders[i].LocalOrderID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	var locals []orderdomain.Order
	if err := s.db.
		Where("orders.deleted_at IS NULL AND id IN ?", ids).
		Preload("Items", "deleted_at IS NULL").
		Find(&locals).Error; err != nil {
		return err
	}
	byID := make(map[uint]procurementdomain.LocalOrder, len(locals))
	for i := range locals {
		byID[locals[i].ID] = orderreader.MapOrder(locals[i])
	}
	for i := range orders {
		if snapshot, ok := byID[orders[i].LocalOrderID]; ok {
			snapshot := snapshot
			orders[i].LocalOrder = &snapshot
		}
	}
	return nil
}
