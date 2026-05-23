package model

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type HermesTenantStatus string

const (
	HermesTenantStatusCreated             HermesTenantStatus = "created"
	HermesTenantStatusDeploying           HermesTenantStatus = "deploying"
	HermesTenantStatusDeployed            HermesTenantStatus = "deployed"
	HermesTenantStatusPairingRequired     HermesTenantStatus = "pairing_required"
	HermesTenantStatusPairingURLGenerated HermesTenantStatus = "pairing_url_generated"
	HermesTenantStatusActive              HermesTenantStatus = "active"
	HermesTenantStatusDeployFailed        HermesTenantStatus = "deploy_failed"
	HermesTenantStatusPairingFailed       HermesTenantStatus = "pairing_failed"
	HermesTenantStatusRuntimeUnhealthy    HermesTenantStatus = "runtime_unhealthy"
	HermesTenantStatusSuspended           HermesTenantStatus = "suspended"
	HermesTenantStatusUpgrading           HermesTenantStatus = "upgrading"
	HermesTenantStatusDeleted             HermesTenantStatus = "deleted"
)

type HermesPairingSessionStatus string

const (
	HermesPairingSessionStatusPending      HermesPairingSessionStatus = "pending"
	HermesPairingSessionStatusURLGenerated HermesPairingSessionStatus = "url_generated"
	HermesPairingSessionStatusPaired       HermesPairingSessionStatus = "paired"
	HermesPairingSessionStatusExpired      HermesPairingSessionStatus = "expired"
	HermesPairingSessionStatusFailed       HermesPairingSessionStatus = "failed"
)

type HermesTenant struct {
	ID                  int                `json:"id" gorm:"primaryKey"`
	UserID              int                `json:"user_id" gorm:"uniqueIndex;not null"`
	TenantID            string             `json:"tenant_id" gorm:"type:varchar(64);uniqueIndex;not null"`
	ServiceName         string             `json:"service_name" gorm:"type:varchar(128);index;not null"`
	VolumeName          string             `json:"volume_name" gorm:"type:varchar(128);index;not null"`
	Status              HermesTenantStatus `json:"status" gorm:"type:varchar(32);index;not null"`
	ZeaburProjectID     string             `json:"zeabur_project_id" gorm:"type:varchar(128)"`
	ZeaburEnvironmentID string             `json:"zeabur_environment_id" gorm:"type:varchar(128)"`
	ZeaburServiceID     string             `json:"zeabur_service_id" gorm:"type:varchar(128);index"`
	ZeaburVolumeID      string             `json:"zeabur_volume_id" gorm:"type:varchar(128)"`
	PublicURL           string             `json:"public_url" gorm:"type:varchar(512)"`
	DashboardURL        string             `json:"dashboard_url" gorm:"type:varchar(512)"`
	LastHealthStatus    string             `json:"last_health_status" gorm:"type:varchar(64)"`
	LastHealthCheckedAt int64              `json:"last_health_checked_at"`
	CreatedAt           int64              `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt           int64              `json:"updated_at" gorm:"autoUpdateTime"`
}

type HermesPairingSession struct {
	ID                   int                        `json:"id" gorm:"primaryKey"`
	TenantID             int                        `json:"tenant_id" gorm:"index;not null"`
	UserID               int                        `json:"user_id" gorm:"index;not null"`
	Status               HermesPairingSessionStatus `json:"status" gorm:"type:varchar(32);index;not null"`
	PairingURL           string                     `json:"pairing_url" gorm:"type:varchar(1024)"`
	ExpiresAt            int64                      `json:"expires_at"`
	CommandExitCode      *int                       `json:"command_exit_code"`
	CommandOutputSummary string                     `json:"command_output_summary" gorm:"type:text"`
	CreatedAt            int64                      `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt            int64                      `json:"updated_at" gorm:"autoUpdateTime"`
	CompletedAt          int64                      `json:"completed_at"`
}

func BuildHermesTenantID(userID int) string {
	return fmt.Sprintf("hermes-%d", userID)
}

func BuildHermesServiceName(userID int) string {
	return fmt.Sprintf("hermes-user-%d", userID)
}

func BuildHermesVolumeName(userID int) string {
	return fmt.Sprintf("hermes-user-%d-data", userID)
}

func NewHermesTenantForUser(userID int) *HermesTenant {
	return &HermesTenant{
		UserID:      userID,
		TenantID:    BuildHermesTenantID(userID),
		ServiceName: BuildHermesServiceName(userID),
		VolumeName:  BuildHermesVolumeName(userID),
		Status:      HermesTenantStatusCreated,
	}
}

func GetHermesTenantByUserID(userID int) (*HermesTenant, error) {
	if userID <= 0 {
		return nil, errors.New("invalid user id")
	}
	var tenant HermesTenant
	err := DB.Where("user_id = ?", userID).First(&tenant).Error
	if err != nil {
		return nil, err
	}
	return &tenant, nil
}

func EnsureHermesTenantForUser(userID int) (*HermesTenant, bool, error) {
	tenant, err := GetHermesTenantByUserID(userID)
	if err == nil {
		return tenant, false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}
	tenant = NewHermesTenantForUser(userID)
	if err := DB.Create(tenant).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			tenant, retryErr := GetHermesTenantByUserID(userID)
			return tenant, false, retryErr
		}
		return nil, false, err
	}
	return tenant, true, nil
}

func UpdateHermesTenantProvisioning(tenant *HermesTenant, projectID string, environmentID string, serviceID string, volumeID string, publicURL string) error {
	if tenant == nil || tenant.ID == 0 {
		return errors.New("invalid hermes tenant")
	}
	updates := map[string]any{
		"zeabur_project_id":     projectID,
		"zeabur_environment_id": environmentID,
		"zeabur_service_id":     serviceID,
		"zeabur_volume_id":      volumeID,
		"public_url":            publicURL,
		"status":                HermesTenantStatusDeployed,
	}
	if publicURL != "" {
		updates["dashboard_url"] = publicURL
	}
	return DB.Model(tenant).Updates(updates).Error
}

func CreateHermesPairingSession(tenant *HermesTenant, expiresAt int64) (*HermesPairingSession, error) {
	if tenant == nil || tenant.ID == 0 {
		return nil, errors.New("invalid hermes tenant")
	}
	session := &HermesPairingSession{
		TenantID:  tenant.ID,
		UserID:    tenant.UserID,
		Status:    HermesPairingSessionStatusPending,
		ExpiresAt: expiresAt,
	}
	if err := DB.Create(session).Error; err != nil {
		return nil, err
	}
	return session, nil
}

func GetLatestHermesPairingSession(tenantID int) (*HermesPairingSession, error) {
	var session HermesPairingSession
	err := DB.Where("tenant_id = ?", tenantID).Order("id desc").First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (tenant *HermesTenant) MarkPairingRequired() error {
	if tenant == nil || tenant.ID == 0 {
		return errors.New("invalid hermes tenant")
	}
	return DB.Model(tenant).Update("status", HermesTenantStatusPairingRequired).Error
}

func (tenant *HermesTenant) MarkHealth(status string) error {
	if tenant == nil || tenant.ID == 0 {
		return errors.New("invalid hermes tenant")
	}
	return DB.Model(tenant).Updates(map[string]any{
		"last_health_status":     status,
		"last_health_checked_at": common.GetTimestamp(),
	}).Error
}
