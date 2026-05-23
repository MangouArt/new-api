package controller

import (
	"errors"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type hermesTenantProvisionRequest struct {
	ZeaburProjectID     string `json:"zeabur_project_id"`
	ZeaburEnvironmentID string `json:"zeabur_environment_id"`
	ZeaburServiceID     string `json:"zeabur_service_id"`
	ZeaburVolumeID      string `json:"zeabur_volume_id"`
	PublicURL           string `json:"public_url"`
}

func GetHermesTenantSelf(c *gin.Context) {
	tenant, err := model.GetHermesTenantByUserID(c.GetInt("id"))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiSuccess(c, gin.H{
				"tenant": nil,
				"exists": false,
			})
			return
		}
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"tenant": tenant,
		"exists": true,
	})
}

func EnsureHermesTenantSelf(c *gin.Context) {
	tenant, created, err := model.EnsureHermesTenantForUser(c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"tenant":  tenant,
		"created": created,
	})
}

func AdminGetHermesTenantByUser(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	tenant, err := model.GetHermesTenantByUserID(userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiSuccess(c, gin.H{
				"tenant": nil,
				"exists": false,
			})
			return
		}
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"tenant": tenant,
		"exists": true,
	})
}

func AdminEnsureHermesTenantByUser(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	tenant, created, err := model.EnsureHermesTenantForUser(userID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"tenant":  tenant,
		"created": created,
	})
}

func AdminUpdateHermesTenantProvisioning(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req hermesTenantProvisionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	tenant, _, err := model.EnsureHermesTenantForUser(userID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.UpdateHermesTenantProvisioning(
		tenant,
		req.ZeaburProjectID,
		req.ZeaburEnvironmentID,
		req.ZeaburServiceID,
		req.ZeaburVolumeID,
		req.PublicURL,
	); err != nil {
		common.ApiError(c, err)
		return
	}
	tenant, err = model.GetHermesTenantByUserID(userID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"tenant": tenant,
	})
}
