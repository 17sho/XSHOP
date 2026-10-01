package gormstore

import (
	"errors"

	downstreamcontract "github.com/dujiao-next/internal/modules/downstreamcallback/contract"
	downstreamdomain "github.com/dujiao-next/internal/modules/downstreamcallback/domain"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Store 是下游回调引用的 GORM 仓储。
type Store struct {
	db *gorm.DB
}

var _ downstreamcontract.Repository = (*Store)(nil)

// New 创建下游回调引用仓储。
func New(db *gorm.DB) *Store {
	if db == nil {
		panic("downstream callback store: db is nil")
	}
	return &Store{db: db}
}

func (s *Store) GetByID(id uint) (*downstreamdomain.OrderRef, error) {
	var ref downstreamdomain.OrderRef
	if err := s.db.First(&ref, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &ref, nil
}

func (s *Store) GetByOrderID(orderID uint) (*downstreamdomain.OrderRef, error) {
	var ref downstreamdomain.OrderRef
	if err := s.db.Where("order_id = ?", orderID).First(&ref).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &ref, nil
}

func (s *Store) GetByCredentialAndDownstreamNo(credentialID uint, downstreamOrderNo string) (*downstreamdomain.OrderRef, error) {
	if credentialID == 0 || downstreamOrderNo == "" {
		return nil, nil
	}
	var refs []downstreamdomain.OrderRef
	if err := s.db.Where("api_credential_id = ? AND downstream_order_no = ?", credentialID, downstreamOrderNo).Limit(2).Find(&refs).Error; err != nil {
		return nil, err
	}
	if len(refs) > 1 {
		return nil, errors.New("ambiguous historical downstream order reference")
	}
	if len(refs) == 0 {
		return nil, nil
	}
	return &refs[0], nil
}

// PrepareCreateClaims must run as a reviewed additive migration before enabling
// creates. It fences distinct historical keys without altering/deleting refs or
// trying to add a uniqueness constraint over potentially duplicate legacy data.
func (s *Store) PrepareCreateClaims() error {
	if err := s.db.AutoMigrate(&downstreamdomain.CreateClaim{}); err != nil {
		return err
	}
	var refs []struct {
		ApiCredentialID   uint
		DownstreamOrderNo string
	}
	if err := s.db.Model(&downstreamdomain.OrderRef{}).Select("api_credential_id, downstream_order_no").Where("downstream_order_no <> ''").Group("api_credential_id, downstream_order_no").Scan(&refs).Error; err != nil {
		return err
	}
	for _, ref := range refs {
		if _, err := s.ReserveCreate(ref.ApiCredentialID, ref.DownstreamOrderNo); err != nil {
			return err
		}
	}
	return nil
}
func (s *Store) ReserveCreate(credentialID uint, no string) (bool, error) {
	if credentialID == 0 || no == "" {
		return false, errors.New("invalid idempotency identity")
	}
	result := s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&downstreamdomain.CreateClaim{CredentialID: credentialID, DownstreamOrderNo: no})
	return result.RowsAffected == 1, result.Error
}

func (s *Store) Create(ref *downstreamdomain.OrderRef) error {
	return s.db.Create(ref).Error
}

func (s *Store) Update(ref *downstreamdomain.OrderRef) error {
	return s.db.Save(ref).Error
}

func (s *Store) ListPendingCallbacks(limit int) ([]downstreamdomain.OrderRef, error) {
	var refs []downstreamdomain.OrderRef
	query := s.db.Where("callback_status = ? AND callback_url != ''", downstreamdomain.StatusPending).
		Order("created_at ASC").
		Limit(limit)
	if err := query.Find(&refs).Error; err != nil {
		return nil, err
	}
	return refs, nil
}

func (s *Store) ListByCredentialID(credentialID uint, filter downstreamcontract.RefListFilter) ([]downstreamdomain.OrderRef, int64, error) {
	var refs []downstreamdomain.OrderRef
	var total int64

	query := s.db.Model(&downstreamdomain.OrderRef{}).Where("api_credential_id = ?", credentialID)
	if filter.CallbackStatus != "" {
		query = query.Where("callback_status = ?", filter.CallbackStatus)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	query = query.Order("created_at DESC")
	if filter.Page > 0 && filter.PageSize > 0 {
		query = query.Offset((filter.Page - 1) * filter.PageSize).Limit(filter.PageSize)
	}
	if err := query.Find(&refs).Error; err != nil {
		return nil, 0, err
	}
	return refs, total, nil
}
