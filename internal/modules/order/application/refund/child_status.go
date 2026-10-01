package refund

import (
	"errors"
	orderdomain "github.com/dujiao-next/internal/modules/order/domain"
	walletcontract "github.com/dujiao-next/internal/modules/wallet/contract"
	"github.com/dujiao-next/internal/shared/money"
	"github.com/shopspring/decimal"
	"strings"
	"time"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/logger"
	ordercontract "github.com/dujiao-next/internal/modules/order/contract"
)

// applyParentRefundChildStatusUpdates 根据父订单退款结果统一更新子订单状态。
// 已全额分摊退款的子订单保持 refunded，不因父订单部分退款重新打开。
//
// orderStore 必须来自订单工作单元，确保父子状态在同一事务中更新。
func applyParentRefundChildStatusUpdates(orderStore ordercontract.Store, parentOrderID uint, parentTargetStatus string, now time.Time) error {
	if orderStore == nil || parentOrderID == 0 {
		return nil
	}

	target := strings.ToLower(strings.TrimSpace(parentTargetStatus))
	if target != constants.OrderStatusPartiallyRefunded && target != constants.OrderStatusRefunded {
		return nil
	}

	children, err := orderStore.ListChildren(parentOrderID)
	if err != nil {
		return err
	}
	for _, child := range children {
		status := target
		if child.TotalAmount.Decimal.IsPositive() && child.RefundedAmount.Decimal.GreaterThanOrEqual(child.TotalAmount.Decimal) {
			status = constants.OrderStatusRefunded
		}
		if err := orderStore.UpdateFields(child.ID, map[string]interface{}{"status": status, "updated_at": now}); err != nil {
			return err
		}
	}
	affected := len(children)
	if affected > 0 {
		logger.Debugw("refund_child_status_propagated",
			"parent_order_id", parentOrderID,
			"target_status", target,
			"rows_affected", affected,
		)
	}
	return nil
}

// Lock the financial root first for every refund mode; sibling refunds share this lock.
func lockRefundOrder(store ordercontract.Store, id uint) (*orderdomain.Order, *orderdomain.Order, error) {
	selected, err := store.GetByID(id)
	if err != nil {
		return nil, nil, err
	}
	if selected == nil {
		return nil, nil, ErrOrderNotFound
	}
	rootID := selected.ID
	if selected.ParentID != nil {
		rootID = *selected.ParentID
	}
	root, err := store.GetByIDForUpdate(rootID)
	if err != nil {
		return nil, nil, err
	}
	if root == nil {
		return nil, nil, ErrOrderNotFound
	}
	if rootID == id {
		return root, root, nil
	}
	selected, err = store.GetByIDForUpdate(id)
	if err != nil {
		return nil, nil, err
	}
	if selected == nil || selected.ParentID == nil || *selected.ParentID != rootID {
		return nil, nil, ErrOrderNotFound
	}
	return selected, root, nil
}

// Root RefundedAmount is the family aggregate; child amounts are allocations, not extra credit.
func reserveFamilyRefund(store ordercontract.Store, order, root *orderdomain.Order, amount decimal.Decimal, now time.Time) (decimal.Decimal, error) {
	children, err := store.ListChildren(root.ID)
	if err != nil {
		return decimal.Zero, err
	}
	childRefunded := decimal.Zero
	for _, child := range children {
		childRefunded = childRefunded.Add(child.RefundedAmount.Decimal)
	}
	before := decimal.Max(root.RefundedAmount.Decimal, childRefunded).Round(2)
	ids := []uint{root.ID}
	for _, child := range children {
		ids = append(ids, child.ID)
	}
	records, err := store.ListFinancialRefundRecordsByOrderIDs(ids)
	if err != nil {
		return decimal.Zero, err
	}
	recorded := decimal.Zero
	for _, record := range records {
		recorded = recorded.Add(record.Amount.Decimal)
	}
	// Counters cannot reconstruct missing cash history or distinguish legacy
	// independent refunds from modern allocations. Require reconciliation when
	// even the known aggregate is not backed by immutable records.
	if before.GreaterThan(recorded.Round(2)) {
		return decimal.Zero, errors.New("incomplete refund history: reconcile before refunding")
	}
	// Legacy rows tracked root and child refunds independently; records disambiguate them.
	before = decimal.Max(before, recorded).Round(2)
	if amount.GreaterThan(root.TotalAmount.Decimal.Sub(before).Round(2)) {
		return decimal.Zero, walletcontract.ErrRefundExceeded
	}
	// A legacy parent refund was not allocated to its children. Only fill the
	// aggregate gap: modern root totals/records already include child allocations.
	// Replaying all parent records here would deduct modern refunds twice.
	if len(children) > 0 {
		allocateChildRefund(children, decimal.Max(decimal.Zero, before.Sub(childRefunded)))
		if order.ID == root.ID {
			allocateChildRefund(children, amount)
		}
		for i := range children {
			child := &children[i]
			updates := map[string]interface{}{"refunded_amount": child.RefundedAmount, "updated_at": now}
			if child.RefundedAmount.Decimal.IsPositive() {
				status := constants.OrderStatusPartiallyRefunded
				if child.RefundedAmount.Decimal.GreaterThanOrEqual(child.TotalAmount.Decimal) {
					status = constants.OrderStatusRefunded
				}
				updates["status"] = status
			}
			if err := store.UpdateFields(child.ID, updates); err != nil {
				return decimal.Zero, err
			}
			if child.ID == order.ID {
				order.RefundedAmount = child.RefundedAmount
			}
		}
	}
	if order.ID != root.ID {
		next := before.Add(amount)
		status := constants.OrderStatusPartiallyRefunded
		if next.GreaterThanOrEqual(root.TotalAmount.Decimal) {
			status = constants.OrderStatusRefunded
		}
		if err := store.UpdateFields(root.ID, map[string]interface{}{"refunded_amount": money.FromDecimal(next), "status": status, "updated_at": now}); err != nil {
			return decimal.Zero, err
		}
	}
	return before, nil
}

// Allocate only unassigned money, preserving every existing child allocation.
func allocateChildRefund(children []orderdomain.Order, amount decimal.Decimal) {
	capacity := decimal.Zero
	for _, child := range children {
		capacity = capacity.Add(decimal.Max(decimal.Zero, child.TotalAmount.Decimal.Sub(child.RefundedAmount.Decimal)))
	}
	if !capacity.IsPositive() || !amount.IsPositive() {
		return
	}
	allocate := decimal.Min(amount, capacity)
	left := allocate
	shares := make([]decimal.Decimal, len(children))
	for i, child := range children {
		room := decimal.Max(decimal.Zero, child.TotalAmount.Decimal.Sub(child.RefundedAmount.Decimal))
		shares[i] = allocate.Mul(room).Div(capacity).Truncate(2)
		left = left.Sub(shares[i])
	}
	for i, child := range children {
		room := decimal.Max(decimal.Zero, child.TotalAmount.Decimal.Sub(child.RefundedAmount.Decimal).Sub(shares[i]))
		extra := decimal.Min(left, room)
		shares[i] = shares[i].Add(extra)
		left = left.Sub(extra)
		children[i].RefundedAmount = money.FromDecimal(child.RefundedAmount.Decimal.Add(shares[i]))
	}
}
