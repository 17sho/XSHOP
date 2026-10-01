package refund_test

import (
	"errors"
	"fmt"
	refundapp "github.com/dujiao-next/internal/modules/order/application/refund"
	resellerapp "github.com/dujiao-next/internal/modules/reseller/application"
	resellerdomain "github.com/dujiao-next/internal/modules/reseller/domain"
	resellerstore "github.com/dujiao-next/internal/modules/reseller/infrastructure/gormstore"
	"testing"
	"time"

	fulfillmentdomain "github.com/dujiao-next/internal/modules/fulfillment/domain"
	. "github.com/dujiao-next/internal/modules/order/application/refund"
	orderdomain "github.com/dujiao-next/internal/modules/order/domain"

	walletdomain "github.com/dujiao-next/internal/modules/wallet/domain"

	siteconnectiondomain "github.com/dujiao-next/internal/modules/siteconnection/domain"

	affiliateapp "github.com/dujiao-next/internal/modules/affiliate/application"
	affiliatedomain "github.com/dujiao-next/internal/modules/affiliate/domain"
	affiliategormstore "github.com/dujiao-next/internal/modules/affiliate/infrastructure/gormstore"
	ordergormstore "github.com/dujiao-next/internal/modules/order/infrastructure/gormstore"
	paymentdomain "github.com/dujiao-next/internal/modules/payment/domain"
	paymentgormstore "github.com/dujiao-next/internal/modules/payment/infrastructure/gormstore"

	userstore "github.com/dujiao-next/internal/modules/identity/user/infrastructure/gormstore"

	userdomain "github.com/dujiao-next/internal/modules/identity/user/domain"
	procurementdomain "github.com/dujiao-next/internal/modules/procurement/domain"

	settingsapp "github.com/dujiao-next/internal/modules/settings/application"
	settingsstore "github.com/dujiao-next/internal/modules/settings/infrastructure/gormstore"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/platform/database/gormdb"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"github.com/dujiao-next/internal/shared/money"

	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func setupOrderRefundServiceTest(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()
	dsn := fmt.Sprintf("file:order_service_refund_test_%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(
		&userdomain.User{},
		&orderdomain.Order{},
		&orderdomain.OrderItem{},
		&fulfillmentdomain.Fulfillment{},
		&siteconnectiondomain.Connection{},
		&procurementdomain.Order{},
		&affiliatedomain.Profile{},
		&affiliatedomain.Commission{},
		&affiliatedomain.WithdrawRequest{},
		&walletdomain.Account{},
		&walletdomain.Transaction{},
		&orderdomain.OrderRefundRecord{},
		&paymentdomain.Payment{},
		&settingsstore.SettingRecord{},
	); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}
	gormdb.DB = db

	orderStore := ordergormstore.New(db, "test-guest-credential-secret-with-32-bytes")
	affiliateSvc := affiliateapp.NewService(affiliategormstore.New(db), nil, nil, nil, nil)
	userRepo := userstore.New(db)
	settingSvc := settingsapp.NewService(settingsstore.New(db))
	paymentStore := paymentgormstore.New(db, "test-guest-credential-secret-with-32-bytes")
	return New(orderStore, userRepo, affiliateSvc, settingSvc, nil, paymentStore), db
}

func createOrderRefundTestSiteConnection(t *testing.T, db *gorm.DB, id uint) *siteconnectiondomain.Connection {
	t.Helper()
	conn := &siteconnectiondomain.Connection{
		ID:        id,
		Name:      fmt.Sprintf("refund-conn-%d", id),
		BaseURL:   "https://upstream.example.com",
		ApiKey:    fmt.Sprintf("refund-key-%d", id),
		ApiSecret: "secret",
		Protocol:  constants.ConnectionProtocolDujiaoNext,
		Status:    constants.ConnectionStatusActive,
		RetryMax:  3,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := db.Create(conn).Error; err != nil {
		t.Fatalf("create site connection failed: %v", err)
	}
	return conn
}

func TestOrderRefundServiceAdminManualRefundGuestCreatesRecord(t *testing.T) {
	svc, db := setupOrderRefundServiceTest(t)
	now := time.Now()
	order := &orderdomain.Order{
		OrderNo:          "REFUND-MANUAL-GUEST-001",
		UserID:           0,
		GuestEmail:       "guest-refund-record@example.com",
		Status:           constants.OrderStatusCompleted,
		Currency:         "CNY",
		OriginalAmount:   money.FromDecimal(decimal.NewFromInt(88)),
		DiscountAmount:   money.FromDecimal(decimal.Zero),
		TotalAmount:      money.FromDecimal(decimal.NewFromInt(88)),
		WalletPaidAmount: money.FromDecimal(decimal.Zero),
		OnlinePaidAmount: money.FromDecimal(decimal.NewFromInt(88)),
		RefundedAmount:   money.FromDecimal(decimal.Zero),
		PaidAt:           &now,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := db.Create(order).Error; err != nil {
		t.Fatalf("create guest order failed: %v", err)
	}
	conn := createOrderRefundTestSiteConnection(t, db, 1)
	proc := &procurementdomain.Order{
		ConnectionID:    conn.ID,
		LocalOrderID:    order.ID,
		LocalOrderNo:    order.OrderNo,
		Status:          constants.ProcurementStatusFulfilled,
		LocalSellAmount: money.FromDecimal(order.TotalAmount.Decimal),
		Currency:        order.Currency,
		TraceID:         "manual-refund-proc-sync",
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := db.Create(proc).Error; err != nil {
		t.Fatalf("create procurement order failed: %v", err)
	}

	updatedOrder, createdRecord, err := svc.AdminManualRefund(AdminManualRefundInput{
		OrderID: order.ID,
		Amount:  money.FromDecimal(decimal.NewFromInt(20)),
		Remark:  "manual partial refund",
	})
	if err != nil {
		t.Fatalf("admin manual refund failed: %v", err)
	}
	if updatedOrder == nil || updatedOrder.Status != constants.OrderStatusPartiallyRefunded {
		t.Fatalf("expected partially_refunded order, got %+v", updatedOrder)
	}
	if createdRecord == nil || createdRecord.ID == 0 {
		t.Fatalf("expected created refund record, got %+v", createdRecord)
	}

	var record orderdomain.OrderRefundRecord
	if err := db.Order("id DESC").First(&record).Error; err != nil {
		t.Fatalf("query refund record failed: %v", err)
	}
	if record.OrderID != order.ID {
		t.Fatalf("unexpected refund record order id: %d", record.OrderID)
	}
	if record.UserID != 0 {
		t.Fatalf("unexpected refund record user id: %d", record.UserID)
	}
	if record.GuestEmail != "guest-refund-record@example.com" {
		t.Fatalf("unexpected refund record guest email: %s", record.GuestEmail)
	}
	if record.Type != constants.OrderRefundTypeManual {
		t.Fatalf("unexpected refund record type: %s", record.Type)
	}
	if !record.Amount.Decimal.Equal(decimal.NewFromInt(20)) {
		t.Fatalf("unexpected refund record amount: %s", record.Amount.String())
	}
	if record.Remark != "manual partial refund" {
		t.Fatalf("unexpected refund record remark: %s", record.Remark)
	}
	var refreshedProc procurementdomain.Order
	if err := db.First(&refreshedProc, proc.ID).Error; err != nil {
		t.Fatalf("reload procurement order failed: %v", err)
	}
	if refreshedProc.Status != constants.ProcurementStatusFulfilled {
		t.Fatalf("expected procurement status fulfilled, got: %s", refreshedProc.Status)
	}
}

func TestOrderRefundServiceTracksAndCorrectsReturnedPaymentFee(t *testing.T) {
	svc, db := setupOrderRefundServiceTest(t)
	now := time.Now().UTC().Truncate(time.Second)
	order := &orderdomain.Order{
		OrderNo:          "REFUND-PAYMENT-FEE-001",
		UserID:           0,
		GuestEmail:       "fee-refund@example.com",
		Status:           constants.OrderStatusCompleted,
		Currency:         "CNY",
		OriginalAmount:   money.FromDecimal(decimal.NewFromInt(100)),
		TotalAmount:      money.FromDecimal(decimal.NewFromInt(100)),
		OnlinePaidAmount: money.FromDecimal(decimal.NewFromInt(100)),
		PaidAt:           &now,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := db.Create(order).Error; err != nil {
		t.Fatalf("create order failed: %v", err)
	}
	payment := &paymentdomain.Payment{
		OrderID:         order.ID,
		ProviderType:    constants.PaymentProviderOfficial,
		ChannelType:     constants.PaymentChannelTypeAlipay,
		InteractionMode: constants.PaymentInteractionRedirect,
		Amount:          money.FromDecimal(decimal.NewFromInt(100)),
		FeeAmount:       money.FromDecimal(decimal.RequireFromString("3.00")),
		FeePolicy:       constants.PaymentFeePolicyMerchantAbsorbed,
		Currency:        "CNY",
		Status:          constants.PaymentStatusSuccess,
		PaidAt:          &now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := db.Create(payment).Error; err != nil {
		t.Fatalf("create payment failed: %v", err)
	}

	_, first, err := svc.AdminManualRefund(AdminManualRefundInput{
		OrderID:            order.ID,
		Amount:             money.FromDecimal(decimal.NewFromInt(40)),
		PaymentFeeRefunded: true,
	})
	if err != nil {
		t.Fatalf("first manual refund failed: %v", err)
	}
	if !first.PaymentFeeRefunded || !first.PaymentFeeRefundedAmount.Decimal.Equal(decimal.RequireFromString("1.20")) {
		t.Fatalf("unexpected first payment fee refund: %+v", first)
	}

	_, second, err := svc.AdminManualRefund(AdminManualRefundInput{
		OrderID:            order.ID,
		Amount:             money.FromDecimal(decimal.NewFromInt(60)),
		PaymentFeeRefunded: true,
	})
	if err != nil {
		t.Fatalf("second manual refund failed: %v", err)
	}
	if !second.PaymentFeeRefunded || !second.PaymentFeeRefundedAmount.Decimal.Equal(decimal.RequireFromString("1.80")) {
		t.Fatalf("unexpected second payment fee refund: %+v", second)
	}

	updated, err := svc.UpdatePaymentFeeRefunded(UpdatePaymentFeeRefundedInput{
		RefundRecordID:     first.ID,
		PaymentFeeRefunded: false,
	})
	if err != nil {
		t.Fatalf("disable payment fee refund failed: %v", err)
	}
	if updated.PaymentFeeRefunded || !updated.PaymentFeeRefundedAmount.Decimal.IsZero() {
		t.Fatalf("disabled payment fee refund not cleared: %+v", updated)
	}

	updated, err = svc.UpdatePaymentFeeRefunded(UpdatePaymentFeeRefundedInput{
		RefundRecordID:     first.ID,
		PaymentFeeRefunded: true,
	})
	if err != nil {
		t.Fatalf("restore payment fee refund failed: %v", err)
	}
	if !updated.PaymentFeeRefundedAmount.Decimal.Equal(decimal.RequireFromString("1.20")) {
		t.Fatalf("restored payment fee refund = %s, want 1.20", updated.PaymentFeeRefundedAmount.StringFixed(2))
	}
}

func TestOrderRefundServicePaymentFeeRefundDoesNotRequestSecondConnection(t *testing.T) {
	svc, db := setupOrderRefundServiceTest(t)
	now := time.Now().UTC().Truncate(time.Second)
	order := &orderdomain.Order{
		OrderNo:          "REFUND-PAYMENT-FEE-SINGLE-CONNECTION-001",
		GuestEmail:       "fee-refund-single-connection@example.com",
		Status:           constants.OrderStatusCompleted,
		Currency:         "CNY",
		OriginalAmount:   money.FromDecimal(decimal.NewFromInt(100)),
		TotalAmount:      money.FromDecimal(decimal.NewFromInt(100)),
		OnlinePaidAmount: money.FromDecimal(decimal.NewFromInt(100)),
		PaidAt:           &now,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := db.Create(order).Error; err != nil {
		t.Fatalf("create order failed: %v", err)
	}
	if err := db.Create(&paymentdomain.Payment{
		OrderID:         order.ID,
		ProviderType:    constants.PaymentProviderOfficial,
		ChannelType:     constants.PaymentChannelTypeAlipay,
		InteractionMode: constants.PaymentInteractionRedirect,
		Amount:          money.FromDecimal(decimal.NewFromInt(100)),
		FeeAmount:       money.FromDecimal(decimal.RequireFromString("3.00")),
		FeePolicy:       constants.PaymentFeePolicyMerchantAbsorbed,
		Currency:        "CNY",
		Status:          constants.PaymentStatusSuccess,
		PaidAt:          &now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}).Error; err != nil {
		t.Fatalf("create payment failed: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get raw db failed: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	done := make(chan error, 1)
	go func() {
		_, record, refundErr := svc.AdminManualRefund(AdminManualRefundInput{
			OrderID:            order.ID,
			Amount:             money.FromDecimal(decimal.NewFromInt(40)),
			PaymentFeeRefunded: true,
		})
		if refundErr != nil {
			done <- fmt.Errorf("manual refund failed: %w", refundErr)
			return
		}
		if record == nil || !record.PaymentFeeRefundedAmount.Decimal.Equal(decimal.RequireFromString("1.20")) {
			done <- fmt.Errorf("unexpected manual refund fee: %+v", record)
			return
		}
		if _, updateErr := svc.UpdatePaymentFeeRefunded(UpdatePaymentFeeRefundedInput{
			RefundRecordID:     record.ID,
			PaymentFeeRefunded: false,
		}); updateErr != nil {
			done <- fmt.Errorf("disable payment fee refund failed: %w", updateErr)
			return
		}
		updated, updateErr := svc.UpdatePaymentFeeRefunded(UpdatePaymentFeeRefundedInput{
			RefundRecordID:     record.ID,
			PaymentFeeRefunded: true,
		})
		if updateErr != nil {
			done <- fmt.Errorf("restore payment fee refund failed: %w", updateErr)
			return
		}
		if updated == nil || !updated.PaymentFeeRefundedAmount.Decimal.Equal(decimal.RequireFromString("1.20")) {
			done <- fmt.Errorf("unexpected restored payment fee: %+v", updated)
			return
		}
		done <- nil
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("payment fee refund deadlocked with MaxOpenConns=1")
	}
}

func TestOrderRefundServiceAdminManualRefundExpiredWindow(t *testing.T) {
	svc, db := setupOrderRefundServiceTest(t)
	paidAt := time.Now().AddDate(0, 0, -31)
	order := &orderdomain.Order{
		OrderNo:          "REFUND-MANUAL-EXPIRED-001",
		UserID:           0,
		GuestEmail:       "guest-refund-expired@example.com",
		Status:           constants.OrderStatusCompleted,
		Currency:         "CNY",
		OriginalAmount:   money.FromDecimal(decimal.NewFromInt(88)),
		DiscountAmount:   money.FromDecimal(decimal.Zero),
		TotalAmount:      money.FromDecimal(decimal.NewFromInt(88)),
		WalletPaidAmount: money.FromDecimal(decimal.Zero),
		OnlinePaidAmount: money.FromDecimal(decimal.NewFromInt(88)),
		RefundedAmount:   money.FromDecimal(decimal.Zero),
		PaidAt:           &paidAt,
		CreatedAt:        paidAt,
		UpdatedAt:        paidAt,
	}
	if err := db.Create(order).Error; err != nil {
		t.Fatalf("create guest order failed: %v", err)
	}

	_, _, err := svc.AdminManualRefund(AdminManualRefundInput{
		OrderID: order.ID,
		Amount:  money.FromDecimal(decimal.NewFromInt(20)),
		Remark:  "manual refund expired",
	})
	if !errors.Is(err, ErrOrderRefundExpired) {
		t.Fatalf("expected refund expired, got: %v", err)
	}
}

func TestOrderRefundServiceAdminManualRefundNoLimitWhenZero(t *testing.T) {
	svc, db := setupOrderRefundServiceTest(t)
	paidAt := time.Now().AddDate(0, 0, -90)
	order := &orderdomain.Order{
		OrderNo:          "REFUND-MANUAL-NOLIMIT-001",
		UserID:           0,
		GuestEmail:       "guest-refund-nolimit@example.com",
		Status:           constants.OrderStatusCompleted,
		Currency:         "CNY",
		OriginalAmount:   money.FromDecimal(decimal.NewFromInt(88)),
		DiscountAmount:   money.FromDecimal(decimal.Zero),
		TotalAmount:      money.FromDecimal(decimal.NewFromInt(88)),
		WalletPaidAmount: money.FromDecimal(decimal.Zero),
		OnlinePaidAmount: money.FromDecimal(decimal.NewFromInt(88)),
		RefundedAmount:   money.FromDecimal(decimal.Zero),
		PaidAt:           &paidAt,
		CreatedAt:        paidAt,
		UpdatedAt:        paidAt,
	}
	if err := db.Create(order).Error; err != nil {
		t.Fatalf("create guest order failed: %v", err)
	}

	if _, err := settingsapp.NewService(settingsstore.New(db)).Update(constants.SettingKeyOrderConfig, map[string]interface{}{
		settingsapp.OrderConfigFieldMaxRefundDays: 0,
	}); err != nil {
		t.Fatalf("update order refund config failed: %v", err)
	}

	updatedOrder, _, err := svc.AdminManualRefund(AdminManualRefundInput{
		OrderID: order.ID,
		Amount:  money.FromDecimal(decimal.NewFromInt(20)),
		Remark:  "manual refund no limit",
	})
	if err != nil {
		t.Fatalf("expected no-limit refund success, got: %v", err)
	}
	if updatedOrder == nil || updatedOrder.Status != constants.OrderStatusPartiallyRefunded {
		t.Fatalf("expected partially_refunded order, got %+v", updatedOrder)
	}
}

func createOrderRefundTestChildWithFulfillmentType(
	t *testing.T,
	db *gorm.DB,
	parent *orderdomain.Order,
	orderNo string,
	status string,
	total decimal.Decimal,
	fulfillmentType string,
	withFulfillment bool,
) *orderdomain.Order {
	t.Helper()
	now := time.Now()
	child := &orderdomain.Order{
		OrderNo:          orderNo,
		ParentID:         &parent.ID,
		UserID:           parent.UserID,
		GuestEmail:       parent.GuestEmail,
		GuestPassword:    parent.GuestPassword,
		GuestLocale:      parent.GuestLocale,
		Status:           status,
		Currency:         parent.Currency,
		OriginalAmount:   money.FromDecimal(total),
		DiscountAmount:   money.FromDecimal(decimal.Zero),
		TotalAmount:      money.FromDecimal(total),
		WalletPaidAmount: money.FromDecimal(decimal.Zero),
		OnlinePaidAmount: money.FromDecimal(total),
		RefundedAmount:   money.FromDecimal(decimal.Zero),
		PaidAt:           parent.PaidAt,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := db.Create(child).Error; err != nil {
		t.Fatalf("create child order failed: %v", err)
	}
	item := &orderdomain.OrderItem{
		OrderID:         child.ID,
		ProductID:       child.ID + 2000,
		SKUID:           1,
		TitleJSON:       jsonmap.JSON{"zh-CN": orderNo},
		UnitPrice:       money.FromDecimal(total),
		CostPrice:       money.FromDecimal(decimal.Zero),
		Quantity:        1,
		TotalPrice:      money.FromDecimal(total),
		FulfillmentType: fulfillmentType,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := db.Create(item).Error; err != nil {
		t.Fatalf("create child order item failed: %v", err)
	}
	if withFulfillment {
		fulfillment := &fulfillmentdomain.Fulfillment{
			OrderID:     child.ID,
			Type:        fulfillmentType,
			Status:      constants.FulfillmentStatusDelivered,
			Payload:     "DELIVERED-CONTENT",
			DeliveredAt: &now,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := db.Create(fulfillment).Error; err != nil {
			t.Fatalf("create child fulfillment failed: %v", err)
		}
	}
	return child
}

type orderManualRefundMixedChildrenFixture struct {
	parentOrderNo        string
	guestEmail           string
	refundAmount         decimal.Decimal
	remark               string
	expectedParentStatus string
	expectedChildStatus  string
}

func assertOrderManualRefundMixedChildrenStatus(t *testing.T, fixture orderManualRefundMixedChildrenFixture) {
	t.Helper()
	svc, db := setupOrderRefundServiceTest(t)
	now := time.Now()
	parent := &orderdomain.Order{
		OrderNo:          fixture.parentOrderNo,
		UserID:           0,
		GuestEmail:       fixture.guestEmail,
		Status:           constants.OrderStatusCompleted,
		Currency:         "CNY",
		OriginalAmount:   money.FromDecimal(decimal.NewFromInt(100)),
		DiscountAmount:   money.FromDecimal(decimal.Zero),
		TotalAmount:      money.FromDecimal(decimal.NewFromInt(100)),
		WalletPaidAmount: money.FromDecimal(decimal.Zero),
		OnlinePaidAmount: money.FromDecimal(decimal.NewFromInt(100)),
		RefundedAmount:   money.FromDecimal(decimal.Zero),
		PaidAt:           &now,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := db.Create(parent).Error; err != nil {
		t.Fatalf("create parent order failed: %v", err)
	}

	manualChild := createOrderRefundTestChildWithFulfillmentType(
		t, db, parent, fixture.parentOrderNo+"-1", constants.OrderStatusPaid,
		decimal.NewFromInt(60), constants.FulfillmentTypeManual, false,
	)
	autoChild := createOrderRefundTestChildWithFulfillmentType(
		t, db, parent, fixture.parentOrderNo+"-2", constants.OrderStatusCompleted,
		decimal.NewFromInt(40), constants.FulfillmentTypeAuto, true,
	)

	updatedOrder, _, err := svc.AdminManualRefund(AdminManualRefundInput{
		OrderID: parent.ID,
		Amount:  money.FromDecimal(fixture.refundAmount),
		Remark:  fixture.remark,
	})
	if err != nil {
		t.Fatalf("admin manual refund failed: %v", err)
	}
	if updatedOrder == nil || updatedOrder.Status != fixture.expectedParentStatus {
		t.Fatalf("expected parent %s, got %+v", fixture.expectedParentStatus, updatedOrder)
	}
	assertOrderRefundOrderStatus(t, db, manualChild.ID, "manual child", fixture.expectedChildStatus)
	assertOrderRefundOrderStatus(t, db, autoChild.ID, "auto child", fixture.expectedChildStatus)
}

func assertOrderRefundOrderStatus(t *testing.T, db *gorm.DB, orderID uint, label, expected string) {
	t.Helper()
	var refreshed orderdomain.Order
	if err := db.First(&refreshed, orderID).Error; err != nil {
		t.Fatalf("reload %s failed: %v", label, err)
	}
	if refreshed.Status != expected {
		t.Fatalf("expected %s %s, got: %s", label, expected, refreshed.Status)
	}
}

func TestOrderRefundServiceAdminManualRefundParentPartialMixedChildrenStatus(t *testing.T) {
	assertOrderManualRefundMixedChildrenStatus(t, orderManualRefundMixedChildrenFixture{
		parentOrderNo:        "REFUND-MANUAL-MIXED-PARTIAL",
		guestEmail:           "guest-mixed-partial@example.com",
		refundAmount:         decimal.NewFromInt(20),
		remark:               "manual mixed partial refund",
		expectedParentStatus: constants.OrderStatusPartiallyRefunded,
		expectedChildStatus:  constants.OrderStatusPartiallyRefunded,
	})
}

func TestOrderRefundServiceAdminManualRefundParentFullMixedChildrenStatus(t *testing.T) {
	assertOrderManualRefundMixedChildrenStatus(t, orderManualRefundMixedChildrenFixture{
		parentOrderNo:        "REFUND-MANUAL-MIXED-FULL",
		guestEmail:           "guest-mixed-full@example.com",
		refundAmount:         decimal.NewFromInt(100),
		remark:               "manual mixed full refund",
		expectedParentStatus: constants.OrderStatusRefunded,
		expectedChildStatus:  constants.OrderStatusRefunded,
	})
}

func TestOrderRefundServiceResolveStatusEmailRefundDetails(t *testing.T) {
	svc, db := setupOrderRefundServiceTest(t)
	now := time.Now()

	parent := &orderdomain.Order{
		OrderNo:          "REFUND-DETAILS-PARENT",
		UserID:           0,
		GuestEmail:       "guest-details-parent@example.com",
		Status:           constants.OrderStatusPartiallyRefunded,
		Currency:         "CNY",
		OriginalAmount:   money.FromDecimal(decimal.NewFromInt(100)),
		DiscountAmount:   money.FromDecimal(decimal.Zero),
		TotalAmount:      money.FromDecimal(decimal.NewFromInt(100)),
		WalletPaidAmount: money.FromDecimal(decimal.Zero),
		OnlinePaidAmount: money.FromDecimal(decimal.NewFromInt(100)),
		RefundedAmount:   money.FromDecimal(decimal.NewFromInt(20)),
		PaidAt:           &now,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := db.Create(parent).Error; err != nil {
		t.Fatalf("create parent order failed: %v", err)
	}

	child := &orderdomain.Order{
		OrderNo:          "REFUND-DETAILS-PARENT-1",
		ParentID:         &parent.ID,
		UserID:           0,
		GuestEmail:       parent.GuestEmail,
		Status:           constants.OrderStatusPartiallyRefunded,
		Currency:         parent.Currency,
		OriginalAmount:   money.FromDecimal(decimal.NewFromInt(60)),
		DiscountAmount:   money.FromDecimal(decimal.Zero),
		TotalAmount:      money.FromDecimal(decimal.NewFromInt(60)),
		WalletPaidAmount: money.FromDecimal(decimal.Zero),
		OnlinePaidAmount: money.FromDecimal(decimal.NewFromInt(60)),
		RefundedAmount:   money.FromDecimal(decimal.NewFromInt(20)),
		PaidAt:           &now,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := db.Create(child).Error; err != nil {
		t.Fatalf("create child order failed: %v", err)
	}

	unrelated := &orderdomain.Order{
		OrderNo:          "REFUND-DETAILS-UNRELATED",
		UserID:           0,
		GuestEmail:       "guest-unrelated@example.com",
		Status:           constants.OrderStatusPaid,
		Currency:         "CNY",
		OriginalAmount:   money.FromDecimal(decimal.NewFromInt(50)),
		DiscountAmount:   money.FromDecimal(decimal.Zero),
		TotalAmount:      money.FromDecimal(decimal.NewFromInt(50)),
		WalletPaidAmount: money.FromDecimal(decimal.Zero),
		OnlinePaidAmount: money.FromDecimal(decimal.NewFromInt(50)),
		RefundedAmount:   money.FromDecimal(decimal.Zero),
		PaidAt:           &now,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := db.Create(unrelated).Error; err != nil {
		t.Fatalf("create unrelated order failed: %v", err)
	}

	parentRecord := &orderdomain.OrderRefundRecord{
		OrderID:    parent.ID,
		Type:       constants.OrderRefundTypeManual,
		Amount:     money.FromDecimal(decimal.NewFromInt(12)),
		Currency:   "CNY",
		Remark:     "parent refund record",
		CreatedAt:  now,
		UpdatedAt:  now,
		GuestEmail: parent.GuestEmail,
	}
	if err := db.Create(parentRecord).Error; err != nil {
		t.Fatalf("create parent refund record failed: %v", err)
	}
	childRecord := &orderdomain.OrderRefundRecord{
		OrderID:    child.ID,
		Type:       constants.OrderRefundTypeManual,
		Amount:     money.FromDecimal(decimal.NewFromInt(8)),
		Currency:   "CNY",
		Remark:     "child refund record",
		CreatedAt:  now,
		UpdatedAt:  now,
		GuestEmail: child.GuestEmail,
	}
	if err := db.Create(childRecord).Error; err != nil {
		t.Fatalf("create child refund record failed: %v", err)
	}

	details, ok, err := svc.ResolveStatusEmailRefundDetails(parent.ID, parentRecord.ID)
	if err != nil {
		t.Fatalf("resolve same order refund details failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected same order refund record to match")
	}
	if !details.Amount.Decimal.Equal(decimal.NewFromInt(12)) || details.Reason != "parent refund record" {
		t.Fatalf("unexpected same order details: %+v", details)
	}

	details, ok, err = svc.ResolveStatusEmailRefundDetails(parent.ID, childRecord.ID)
	if err != nil {
		t.Fatalf("resolve child refund details failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected child order refund record to match parent order")
	}
	if !details.Amount.Decimal.Equal(decimal.NewFromInt(8)) || details.Reason != "child refund record" {
		t.Fatalf("unexpected child order details: %+v", details)
	}

	_, ok, err = svc.ResolveStatusEmailRefundDetails(unrelated.ID, childRecord.ID)
	if err != nil {
		t.Fatalf("resolve unrelated order refund details failed: %v", err)
	}
	if ok {
		t.Fatalf("expected unrelated order to not match refund record")
	}

	parsed, err := svc.ResolveOrderStatusEmailRefundDetails(parent, parentRecord.ID)
	if err != nil {
		t.Fatalf("resolve order status email refund details by record failed: %v", err)
	}
	if !parsed.Amount.Decimal.Equal(decimal.NewFromInt(12)) || parsed.Reason != "parent refund record" {
		t.Fatalf("unexpected parsed details by record: %+v", parsed)
	}

	fallback, err := svc.ResolveOrderStatusEmailRefundDetails(parent, 999999)
	if err != nil {
		t.Fatalf("resolve order status email refund details fallback failed: %v", err)
	}
	if !fallback.Amount.Decimal.Equal(decimal.NewFromInt(20)) || fallback.Reason != "" {
		t.Fatalf("unexpected fallback details: %+v", fallback)
	}

	var nilSvc *Service
	fallbackFromNilSvc, err := nilSvc.ResolveOrderStatusEmailRefundDetails(parent, parentRecord.ID)
	if err != nil {
		t.Fatalf("resolve order status email refund details with nil service failed: %v", err)
	}
	if !fallbackFromNilSvc.Amount.Decimal.Equal(decimal.NewFromInt(20)) || fallbackFromNilSvc.Reason != "" {
		t.Fatalf("unexpected fallback from nil service: %+v", fallbackFromNilSvc)
	}
}

func TestAuditManualRefundCommissionBoundary(t *testing.T) {
	for _, mode := range []string{"member-control", "guest", "member-child"} {
		t.Run(mode, func(t *testing.T) {
			svc, db := setupOrderRefundServiceTest(t)
			createTestUser(t, db, 1)
			createTestUser(t, db, 2)
			now := time.Now()
			userID := uint(1)
			if mode == "guest" {
				userID = 0
			}
			profile := affiliatedomain.Profile{UserID: 2, AffiliateCode: "REFTEST", Status: constants.AffiliateProfileStatusActive}
			if err := db.Create(&profile).Error; err != nil {
				t.Fatal(err)
			}
			d := decimal.NewFromInt
			parent := orderdomain.Order{OrderNo: "SYNTHETIC-PARENT", UserID: userID, GuestEmail: "synthetic@example.test", Status: constants.OrderStatusCompleted, Currency: "CNY", TotalAmount: money.FromDecimal(d(100)), OriginalAmount: money.FromDecimal(d(100)), PaidAt: &now, CreatedAt: now, UpdatedAt: now}
			if err := db.Create(&parent).Error; err != nil {
				t.Fatal(err)
			}
			commission := affiliatedomain.Commission{AffiliateProfileID: profile.ID, OrderID: parent.ID, CommissionType: constants.AffiliateCommissionTypeOrder, BaseAmount: money.FromDecimal(d(100)), RatePercent: money.FromDecimal(d(20)), CommissionAmount: money.FromDecimal(d(20)), Status: constants.AffiliateCommissionStatusAvailable}
			if err := db.Create(&commission).Error; err != nil {
				t.Fatal(err)
			}
			targetID := parent.ID
			if mode == "member-child" {
				child := orderdomain.Order{OrderNo: "SYNTHETIC-CHILD", ParentID: &parent.ID, UserID: userID, Status: constants.OrderStatusCompleted, Currency: "CNY", TotalAmount: money.FromDecimal(d(100)), OriginalAmount: money.FromDecimal(d(100)), PaidAt: &now, CreatedAt: now, UpdatedAt: now}
				if err := db.Create(&child).Error; err != nil {
					t.Fatal(err)
				}
				targetID = child.ID
			}
			order, record, err := svc.AdminManualRefund(refundapp.AdminManualRefundInput{OrderID: targetID, Amount: money.FromDecimal(d(100)), Remark: "synthetic local audit"})
			if err != nil {
				t.Fatal(err)
			}
			if order.Status != constants.OrderStatusRefunded || record == nil {
				t.Fatalf("refund did not complete")
			}
			var got affiliatedomain.Commission
			if err := db.First(&got, commission.ID).Error; err != nil {
				t.Fatal(err)
			}
			if got.Status != constants.AffiliateCommissionStatusRejected || !got.CommissionAmount.Decimal.IsZero() {
				t.Fatalf("control reversal failed: %+v", got)
			}
			t.Logf("mode=%s refunded=%s commission=%s status=%s", mode, record.Amount.StringFixed(2), got.CommissionAmount.StringFixed(2), got.Status)
		})
	}
}

func TestAuditChildRefundLeavesResellerProfit(t *testing.T) {
	svc, db := setupOrderRefundServiceTest(t)
	if err := db.AutoMigrate(&resellerdomain.Profile{}, &resellerdomain.OrderSnapshot{}, &resellerdomain.LedgerEntry{}, &resellerdomain.BalanceAccount{}, &resellerdomain.WithdrawRequest{}); err != nil {
		t.Fatal(err)
	}
	createTestUser(t, db, 1)
	createTestUser(t, db, 2)
	profile := resellerdomain.Profile{UserID: 2, Status: resellerdomain.ProfileStatusActive, SettlementStatus: resellerdomain.SettlementStatusNormal}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	d := decimal.NewFromInt
	now := time.Now()
	parent := orderdomain.Order{OrderNo: "RESELLER-PARENT", UserID: 1, ResellerID: &profile.ID, Status: constants.OrderStatusCompleted, Currency: "CNY", TotalAmount: money.FromDecimal(d(100)), PaidAt: &now, CreatedAt: now, UpdatedAt: now}
	if err := db.Create(&parent).Error; err != nil {
		t.Fatal(err)
	}
	child := orderdomain.Order{OrderNo: "RESELLER-CHILD", ParentID: &parent.ID, UserID: 1, ResellerID: &profile.ID, Status: constants.OrderStatusCompleted, Currency: "CNY", TotalAmount: money.FromDecimal(d(100)), PaidAt: &now, CreatedAt: now, UpdatedAt: now}
	if err := db.Create(&child).Error; err != nil {
		t.Fatal(err)
	}
	snapshot := resellerdomain.OrderSnapshot{OrderID: parent.ID, ResellerID: profile.ID, Domain: "synthetic.example.test", Currency: "CNY", ResellerUserID: 2, BuyerUserID: 1, BaseAmount: money.FromDecimal(d(80)), ResellerAmount: money.FromDecimal(d(100)), ProfitAmount: money.FromDecimal(d(20)), ProfitEligible: true}
	if err := db.Create(&snapshot).Error; err != nil {
		t.Fatal(err)
	}
	store := resellerstore.New(db)
	ledger := resellerapp.NewAccountingLedgerService(store, 0)
	if err := ledger.PostOrderProfitForOrder(store, &parent); err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.ConfirmDueLedgerEntries(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	svc.SetResellerAccounting(ledger)
	order, _, err := svc.AdminManualRefund(refundapp.AdminManualRefundInput{OrderID: child.ID, Amount: money.FromDecimal(d(100)), Remark: "synthetic local audit"})
	if err != nil {
		t.Fatal(err)
	}
	if order.Status != constants.OrderStatusRefunded {
		t.Fatal("not refunded")
	}
	var deductions int64
	if err := db.Model(&resellerdomain.LedgerEntry{}).Where("type = ?", resellerdomain.LedgerTypeRefundDeduct).Count(&deductions).Error; err != nil {
		t.Fatal(err)
	}
	sums, err := store.SumLedgerAmountGroupedByStatus(profile.ID, "CNY", []string{resellerdomain.LedgerStatusAvailable})
	if err != nil {
		t.Fatal(err)
	}
	if deductions != 1 || !sums[resellerdomain.LedgerStatusAvailable].IsZero() {
		t.Fatalf("unexpected: deductions=%d sums=%v", deductions, sums)
	}
	t.Logf("CONFIRMED: child refunded 100.00; reseller deduction rows=%d; parent profit available=%s", deductions, sums[resellerdomain.LedgerStatusAvailable].StringFixed(2))
}

func TestRemediationPartialFamilyRefundConservesAllLedgers(t *testing.T) {
	svc, db := setupOrderRefundWalletTest(t)
	createTestUser(t, db, 81)
	createTestUser(t, db, 82)
	if err := db.AutoMigrate(&resellerdomain.Profile{}, &resellerdomain.OrderSnapshot{}, &resellerdomain.LedgerEntry{}, &resellerdomain.BalanceAccount{}, &resellerdomain.WithdrawRequest{}); err != nil {
		t.Fatal(err)
	}
	d := decimal.NewFromInt
	now := time.Now()
	reseller := resellerdomain.Profile{UserID: 82, Status: resellerdomain.ProfileStatusActive, SettlementStatus: resellerdomain.SettlementStatusNormal}
	if err := db.Create(&reseller).Error; err != nil {
		t.Fatal(err)
	}
	root := createTestOrder(t, db, 81, "PARTIAL-ROOT", d(100))
	a := createTestOrder(t, db, 81, "PARTIAL-A", d(60))
	b := createTestOrder(t, db, 81, "PARTIAL-B", d(40))
	a.ParentID = &root.ID
	b.ParentID = &root.ID
	for _, o := range []*orderdomain.Order{root, a, b} {
		o.PaidAt = &now
		o.Status = constants.OrderStatusPaid
		o.ResellerID = &reseller.ID
		if err := db.Save(o).Error; err != nil {
			t.Fatal(err)
		}
	}
	profile := affiliatedomain.Profile{UserID: 82, AffiliateCode: "PARTIAL", Status: constants.AffiliateProfileStatusActive}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatal(err)
	}
	commission := affiliatedomain.Commission{AffiliateProfileID: profile.ID, OrderID: root.ID, CommissionType: constants.AffiliateCommissionTypeOrder, BaseAmount: money.FromDecimal(d(100)), CommissionAmount: money.FromDecimal(d(20)), Status: constants.AffiliateCommissionStatusAvailable}
	if err := db.Create(&commission).Error; err != nil {
		t.Fatal(err)
	}
	snapshot := resellerdomain.OrderSnapshot{OrderID: root.ID, ResellerID: reseller.ID, Domain: "fixture.invalid", Currency: "CNY", ResellerUserID: 82, BuyerUserID: 81, BaseAmount: money.FromDecimal(d(80)), ResellerAmount: money.FromDecimal(d(100)), ProfitAmount: money.FromDecimal(d(20)), ProfitEligible: true}
	if err := db.Create(&snapshot).Error; err != nil {
		t.Fatal(err)
	}
	store := resellerstore.New(db)
	ledger := resellerapp.NewAccountingLedgerService(store, 0)
	svc.SetResellerAccounting(ledger)
	if err := ledger.PostOrderProfitForOrder(store, root); err != nil {
		t.Fatal(err)
	}
	before := decimal.Zero
	for i, step := range []struct {
		id     uint
		amount int64
		wallet bool
	}{{a.ID, 20, true}, {root.ID, 30, false}, {b.ID, 25, true}, {root.ID, 25, false}} {
		if step.wallet {
			if _, _, _, err := svc.AdminRefundToWallet(refundapp.AdminRefundToWalletInput{OrderID: step.id, Amount: money.FromDecimal(d(step.amount))}); err != nil {
				t.Fatal(err)
			}
		} else {
			if _, _, err := svc.AdminManualRefund(refundapp.AdminManualRefundInput{OrderID: step.id, Amount: money.FromDecimal(d(step.amount))}); err != nil {
				t.Fatal(err)
			}
		}
		before = before.Add(d(step.amount))
		var got orderdomain.Order
		if err := db.First(&got, root.ID).Error; err != nil {
			t.Fatal(err)
		}
		if !got.RefundedAmount.Decimal.Equal(before) {
			t.Fatalf("step %d family refunded=%s want %s", i, got.RefundedAmount.String(), before)
		}
		var earned affiliatedomain.Commission
		if err := db.First(&earned, commission.ID).Error; err != nil {
			t.Fatal(err)
		}
		want := d(100).Sub(before).Div(d(5))
		if !earned.CommissionAmount.Decimal.Equal(want) {
			t.Fatalf("step %d commission=%s want=%s", i, earned.CommissionAmount.String(), want)
		}
		deducted, err := store.SumLedgerAmountByOrderAndType(root.ID, resellerdomain.LedgerTypeRefundDeduct)
		if err != nil {
			t.Fatal(err)
		}
		if !deducted.Abs().Equal(before.Div(d(5))) {
			t.Fatalf("deducted=%s", deducted)
		}
	}
	for _, id := range []uint{root.ID, a.ID, b.ID} {
		if _, _, _, err := svc.AdminRefundToWallet(refundapp.AdminRefundToWalletInput{OrderID: id, Amount: money.FromDecimal(d(1))}); err == nil {
			t.Fatalf("retry exceeded exhausted family %d", id)
		}
	}
	var balance walletdomain.Account
	if err := db.Where("user_id = ?", 81).First(&balance).Error; err != nil {
		t.Fatal(err)
	}
	if !balance.Balance.Decimal.Equal(d(45)) {
		t.Fatalf("wallet=%s", balance.Balance.String())
	}
	var records []orderdomain.OrderRefundRecord
	if err := db.Find(&records).Error; err != nil {
		t.Fatal(err)
	}
	total := decimal.Zero
	for _, record := range records {
		total = total.Add(record.Amount.Decimal)
	}
	if len(records) != 4 || !total.Equal(d(100)) {
		t.Fatalf("refund records=%d amount=%s", len(records), total)
	}
	for _, child := range []*orderdomain.Order{a, b} {
		var got orderdomain.Order
		if err := db.First(&got, child.ID).Error; err != nil {
			t.Fatal(err)
		}
		if !got.RefundedAmount.Decimal.Equal(child.TotalAmount.Decimal) {
			t.Fatalf("child allocation=%s", got.RefundedAmount.String())
		}
	}
}
