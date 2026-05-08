package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	PaymentProductStatusEnabled  = 1
	PaymentProductStatusDisabled = 2
)

type PaymentProduct struct {
	Id          int            `json:"id"`
	Provider    string         `json:"provider" gorm:"type:varchar(50);index"`
	ProductId   string         `json:"product_id" gorm:"type:varchar(128);index"`
	Name        string         `json:"name" gorm:"type:varchar(128)"`
	Currency    string         `json:"currency" gorm:"type:varchar(16);default:'USD'"`
	Price       float64        `json:"price"`
	Quota       int64          `json:"quota" gorm:"index"`
	Tier        string         `json:"tier" gorm:"type:varchar(64);index"`
	Status      int            `json:"status" gorm:"default:1;index"`
	SortOrder   int            `json:"sort_order" gorm:"default:0"`
	Metadata    string         `json:"metadata" gorm:"type:text"`
	CreatedTime int64          `json:"created_time" gorm:"bigint"`
	UpdatedTime int64          `json:"updated_time" gorm:"bigint"`
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

func (p *PaymentProduct) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	if p.CreatedTime == 0 {
		p.CreatedTime = now
	}
	p.UpdatedTime = now
	if p.Status == 0 {
		p.Status = PaymentProductStatusEnabled
	}
	p.Provider = strings.TrimSpace(strings.ToLower(p.Provider))
	p.ProductId = strings.TrimSpace(p.ProductId)
	p.Tier = strings.TrimSpace(p.Tier)
	if p.Currency == "" {
		p.Currency = "USD"
	}
	return nil
}

func (p *PaymentProduct) BeforeUpdate(tx *gorm.DB) error {
	p.UpdatedTime = common.GetTimestamp()
	p.Provider = strings.TrimSpace(strings.ToLower(p.Provider))
	p.ProductId = strings.TrimSpace(p.ProductId)
	p.Tier = strings.TrimSpace(p.Tier)
	return nil
}

func HasActivePaymentProducts(provider string) bool {
	if DB == nil {
		return false
	}
	var count int64
	err := DB.Model(&PaymentProduct{}).
		Where("provider = ? AND status = ?", strings.TrimSpace(strings.ToLower(provider)), PaymentProductStatusEnabled).
		Count(&count).Error
	return err == nil && count > 0
}

func GetActivePaymentProduct(provider string, amount int64, tier string) (*PaymentProduct, error) {
	if DB == nil {
		return nil, errors.New("database is not initialized")
	}
	provider = strings.TrimSpace(strings.ToLower(provider))
	tier = strings.TrimSpace(tier)
	query := DB.Where("provider = ? AND status = ?", provider, PaymentProductStatusEnabled)
	if tier != "" {
		query = query.Where("(tier = ? OR quota = ?)", tier, amount)
	} else {
		query = query.Where("quota = ?", amount)
	}
	var product PaymentProduct
	err := query.Order("sort_order asc, id asc").First(&product).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errors.New("payment product is not configured")
	}
	if err != nil {
		return nil, err
	}
	if product.ProductId == "" || product.Quota <= 0 {
		return nil, errors.New("payment product is incomplete")
	}
	return &product, nil
}
