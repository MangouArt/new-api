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
	TenantTokenID       int                `json:"tenant_token_id" gorm:"index"`
	TenantID            string             `json:"tenant_id" gorm:"type:varchar(64);uniqueIndex;not null"`
	ServiceName         string             `json:"service_name" gorm:"type:varchar(128);index;not null"`
	VolumeName          string             `json:"volume_name" gorm:"type:varchar(128);index;not null"`
	HermesAdminToken    string             `json:"-" gorm:"type:varchar(256)"`
	Status              HermesTenantStatus `json:"status" gorm:"type:varchar(32);index;not null"`
	ZeaburProjectID     string             `json:"zeabur_project_id" gorm:"type:varchar(128)"`
	ZeaburEnvironmentID string             `json:"zeabur_environment_id" gorm:"type:varchar(128)"`
	ZeaburServiceID     string             `json:"zeabur_service_id" gorm:"type:varchar(128);index"`
	ZeaburDeploymentID  string             `json:"zeabur_deployment_id" gorm:"type:varchar(128);index"`
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

type HermesTenantUser struct {
	UserID        int                   `json:"user_id"`
	Username      string                `json:"username"`
	DisplayName   string                `json:"display_name"`
	Email         string                `json:"email"`
	Role          int                   `json:"role"`
	Status        int                   `json:"status"`
	Group         string                `json:"group"`
	CreatedAt     int64                 `json:"created_at"`
	Tenant        *HermesTenant         `json:"tenant"`
	LatestPairing *HermesPairingSession `json:"latest_pairing"`
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

const (
	HermesTenantTokenName  = "hermes-tenant-runtime"
	hermesTenantTokenGroup = "auto"
)

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

func ListHermesTenantUsers(pageInfo *common.PageInfo) ([]*HermesTenantUser, int64, error) {
	var total int64
	if err := DB.Unscoped().Model(&User{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var users []*User
	if err := DB.Unscoped().
		Order("id desc").
		Limit(pageInfo.GetPageSize()).
		Offset(pageInfo.GetStartIdx()).
		Omit("password", "original_password", "access_token", "verification_code").
		Find(&users).Error; err != nil {
		return nil, 0, err
	}

	userIDs := make([]int, 0, len(users))
	for _, user := range users {
		userIDs = append(userIDs, user.Id)
	}

	tenantsByUserID := map[int]*HermesTenant{}
	pairingsByTenantID := map[int]*HermesPairingSession{}
	if len(userIDs) > 0 {
		var tenants []*HermesTenant
		if err := DB.Where("user_id IN ?", userIDs).Find(&tenants).Error; err != nil {
			return nil, 0, err
		}
		tenantIDs := make([]int, 0, len(tenants))
		for _, tenant := range tenants {
			tenantsByUserID[tenant.UserID] = tenant
			tenantIDs = append(tenantIDs, tenant.ID)
		}
		if len(tenantIDs) > 0 {
			var sessions []*HermesPairingSession
			if err := DB.
				Where("tenant_id IN ?", tenantIDs).
				Order("tenant_id asc, id desc").
				Find(&sessions).Error; err != nil {
				return nil, 0, err
			}
			for _, session := range sessions {
				if _, ok := pairingsByTenantID[session.TenantID]; !ok {
					pairingsByTenantID[session.TenantID] = session
				}
			}
		}
	}

	items := make([]*HermesTenantUser, 0, len(users))
	for _, user := range users {
		item := &HermesTenantUser{
			UserID:      user.Id,
			Username:    user.Username,
			DisplayName: user.DisplayName,
			Email:       user.Email,
			Role:        user.Role,
			Status:      user.Status,
			Group:       user.Group,
			CreatedAt:   user.CreatedAt,
			Tenant:      tenantsByUserID[user.Id],
		}
		if item.Tenant != nil {
			item.LatestPairing = pairingsByTenantID[item.Tenant.ID]
		}
		items = append(items, item)
	}
	return items, total, nil
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

func EnsureHermesTenantRuntimeToken(tenant *HermesTenant) (*Token, bool, error) {
	if tenant == nil || tenant.ID == 0 || tenant.UserID <= 0 {
		return nil, false, errors.New("invalid hermes tenant")
	}

	var token Token
	if tenant.TenantTokenID > 0 {
		err := DB.Where("id = ? AND user_id = ?", tenant.TenantTokenID, tenant.UserID).First(&token).Error
		if err == nil {
			updated, err := ensureHermesTenantTokenShape(&token)
			if err != nil {
				return nil, false, err
			}
			return updated, false, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, err
		}
	}

	err := DB.Where("user_id = ? AND name = ?", tenant.UserID, HermesTenantTokenName).First(&token).Error
	if err == nil {
		updated, err := ensureHermesTenantTokenShape(&token)
		if err != nil {
			return nil, false, err
		}
		if tenant.TenantTokenID != updated.Id {
			if err := DB.Model(tenant).Update("tenant_token_id", updated.Id).Error; err != nil {
				return nil, false, err
			}
			tenant.TenantTokenID = updated.Id
		}
		return updated, false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}

	key, err := common.GenerateKey()
	if err != nil {
		return nil, false, err
	}
	token = Token{
		UserId:             tenant.UserID,
		Name:               HermesTenantTokenName,
		Key:                key,
		Status:             common.TokenStatusEnabled,
		CreatedTime:        common.GetTimestamp(),
		AccessedTime:       common.GetTimestamp(),
		ExpiredTime:        -1,
		RemainQuota:        0,
		UnlimitedQuota:     true,
		ModelLimitsEnabled: false,
		Group:              hermesTenantTokenGroup,
		CrossGroupRetry:    true,
	}
	if err := token.Insert(); err != nil {
		return nil, false, err
	}
	if err := DB.Model(tenant).Update("tenant_token_id", token.Id).Error; err != nil {
		return nil, false, err
	}
	tenant.TenantTokenID = token.Id
	return &token, true, nil
}

func EnsureHermesTenantAdminToken(tenant *HermesTenant) (string, bool, error) {
	if tenant == nil || tenant.ID == 0 {
		return "", false, errors.New("invalid hermes tenant")
	}
	if tenant.HermesAdminToken != "" {
		return tenant.HermesAdminToken, false, nil
	}
	adminToken, err := common.GenerateRandomCharsKey(48)
	if err != nil {
		return "", false, err
	}
	if err := DB.Model(tenant).Update("hermes_admin_token", adminToken).Error; err != nil {
		return "", false, err
	}
	tenant.HermesAdminToken = adminToken
	return adminToken, true, nil
}

func ensureHermesTenantTokenShape(token *Token) (*Token, error) {
	needsUpdate := false
	if token.Name != HermesTenantTokenName {
		token.Name = HermesTenantTokenName
		needsUpdate = true
	}
	if token.Status != common.TokenStatusEnabled {
		token.Status = common.TokenStatusEnabled
		needsUpdate = true
	}
	if token.ExpiredTime != -1 {
		token.ExpiredTime = -1
		needsUpdate = true
	}
	if !token.UnlimitedQuota {
		token.UnlimitedQuota = true
		needsUpdate = true
	}
	if token.Group != hermesTenantTokenGroup {
		token.Group = hermesTenantTokenGroup
		needsUpdate = true
	}
	if !token.CrossGroupRetry {
		token.CrossGroupRetry = true
		needsUpdate = true
	}
	if needsUpdate {
		if err := token.Update(); err != nil {
			return nil, err
		}
	}
	return token, nil
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

func MarkHermesTenantDeploying(tenant *HermesTenant, projectID string, environmentID string, deploymentID string) error {
	if tenant == nil || tenant.ID == 0 {
		return errors.New("invalid hermes tenant")
	}
	updates := map[string]any{
		"zeabur_project_id":     projectID,
		"zeabur_environment_id": environmentID,
		"zeabur_deployment_id":  deploymentID,
		"status":                HermesTenantStatusDeploying,
	}
	if err := DB.Model(tenant).Updates(updates).Error; err != nil {
		return err
	}
	tenant.ZeaburProjectID = projectID
	tenant.ZeaburEnvironmentID = environmentID
	tenant.ZeaburDeploymentID = deploymentID
	tenant.Status = HermesTenantStatusDeploying
	return nil
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

func GetHermesPairingSession(tenantID int, sessionID int) (*HermesPairingSession, error) {
	var session HermesPairingSession
	err := DB.Where("tenant_id = ? AND id = ?", tenantID, sessionID).First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func RecordHermesPairingURL(tenant *HermesTenant, session *HermesPairingSession, pairingURL string, exitCode int, outputSummary string) error {
	if tenant == nil || tenant.ID == 0 {
		return errors.New("invalid hermes tenant")
	}
	if session == nil || session.ID == 0 || session.TenantID != tenant.ID {
		return errors.New("invalid hermes pairing session")
	}
	now := common.GetTimestamp()
	return DB.Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{
			"status":                 HermesPairingSessionStatusURLGenerated,
			"pairing_url":            pairingURL,
			"command_exit_code":      exitCode,
			"command_output_summary": outputSummary,
			"completed_at":           now,
		}
		if err := tx.Model(session).Updates(updates).Error; err != nil {
			return err
		}
		if err := tx.Model(tenant).Update("status", HermesTenantStatusPairingURLGenerated).Error; err != nil {
			return err
		}
		session.Status = HermesPairingSessionStatusURLGenerated
		session.PairingURL = pairingURL
		session.CommandExitCode = &exitCode
		session.CommandOutputSummary = outputSummary
		session.CompletedAt = now
		tenant.Status = HermesTenantStatusPairingURLGenerated
		return nil
	})
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
