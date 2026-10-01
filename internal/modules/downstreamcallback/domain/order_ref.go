package domain

import (
	"time"
)

const (
	StatusPending = "pending"
	StatusSent    = "sent"
	StatusFailed  = "failed"
	// StatusPaused retains the reference but is excluded from pending sweeps.
	StatusPaused = "paused"
)

// CreateClaim fences a remote request before any order/stock/wallet side effect.
// Claims never expire automatically: a crashed creator is uncertain, not safe
// to charge again. Existing references remain untouched during migration.
type CreateClaim struct {
	CredentialID      uint   `gorm:"primaryKey;autoIncrement:false"`
	DownstreamOrderNo string `gorm:"primaryKey;type:varchar(64)"`
	CreatedAt         time.Time
}

func (CreateClaim) TableName() string { return "downstream_create_claims" }

// OrderRef 是 B 侧保存的下游订单回调引用。
type OrderRef struct {
	ID                 uint       `gorm:"primarykey" json:"id"`
	OrderID            uint       `gorm:"uniqueIndex;not null" json:"order_id"`
	ApiCredentialID    uint       `gorm:"index;not null" json:"api_credential_id"`
	DownstreamOrderNo  string     `gorm:"type:varchar(64);index" json:"downstream_order_no"`
	CallbackURL        string     `gorm:"type:varchar(500)" json:"callback_url"`
	TraceID            string     `gorm:"type:varchar(64);index" json:"trace_id"`
	CallbackStatus     string     `gorm:"type:varchar(20);not null;default:'pending'" json:"callback_status"`
	CallbackRetryCount int        `gorm:"not null;default:0" json:"callback_retry_count"`
	LastCallbackAt     *time.Time `json:"last_callback_at,omitempty"`
	CreatedAt          time.Time  `gorm:"index" json:"created_at"`
	UpdatedAt          time.Time  `gorm:"index" json:"updated_at"`
}

// TableName 指定表名
func (OrderRef) TableName() string {
	return "downstream_order_refs"
}
