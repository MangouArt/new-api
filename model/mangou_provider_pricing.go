package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type MangouProviderPricing struct {
	Id              int    `json:"id"`
	Provider        string `json:"provider" gorm:"type:varchar(64);uniqueIndex:idx_mangou_provider_type"`
	TaskType        string `json:"task_type" gorm:"type:varchar(32);uniqueIndex:idx_mangou_provider_type"`
	BaseQuota       int    `json:"base_quota" gorm:"type:int;default:0"`
	MultipliersJSON string `json:"multipliers_json" gorm:"type:text"`
	Enabled         bool   `json:"enabled" gorm:"default:true;index"`
	CreatedAt       int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt       int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

func GetMangouProviderPricing(provider string, taskType string) (*MangouProviderPricing, bool, error) {
	var pricing MangouProviderPricing
	err := DB.Where("provider = ? AND task_type = ? AND enabled = ?", provider, taskType, true).First(&pricing).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return &pricing, true, nil
}

func (p *MangouProviderPricing) GetMultipliers() (map[string]map[string]float64, error) {
	if p.MultipliersJSON == "" {
		return map[string]map[string]float64{}, nil
	}
	var multipliers map[string]map[string]float64
	if err := common.Unmarshal([]byte(p.MultipliersJSON), &multipliers); err != nil {
		return nil, err
	}
	return multipliers, nil
}
