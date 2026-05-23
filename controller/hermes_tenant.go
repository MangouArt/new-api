package controller

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

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

func ProxyHermesTenantDashboard(c *gin.Context) {
	tenant, err := model.GetHermesTenantByUserID(c.GetInt("id"))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiError(c, errors.New("hermes tenant not found"))
			return
		}
		common.ApiError(c, err)
		return
	}
	rawBaseURL := strings.TrimSpace(tenant.DashboardURL)
	if rawBaseURL == "" {
		rawBaseURL = strings.TrimSpace(tenant.PublicURL)
	}
	if rawBaseURL == "" {
		common.ApiError(c, errors.New("hermes tenant dashboard url is not configured"))
		return
	}
	target, err := url.Parse(rawBaseURL)
	if err != nil || target.Scheme == "" || target.Host == "" {
		common.ApiError(c, errors.New("invalid hermes tenant dashboard url"))
		return
	}

	proxyPath := c.Param("proxy_path")
	if proxyPath == "" {
		proxyPath = "/"
	}
	outboundURL := *target
	outboundURL.Path = joinURLPath(target.Path, proxyPath)
	outboundURL.RawQuery = c.Request.URL.RawQuery
	req, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, outboundURL.String(), c.Request.Body)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	req.Header = c.Request.Header.Clone()
	req.Host = target.Host
	req.Header.Set("X-Hermes-Admin-Token", tenant.HermesAdminToken)
	req.Header.Set("X-Forwarded-Prefix", "/api/hermes/tenant/dashboard")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	defer resp.Body.Close()

	for key, values := range resp.Header {
		for _, value := range values {
			c.Writer.Header().Add(key, value)
		}
	}
	c.Status(resp.StatusCode)
	if _, err := io.Copy(c.Writer, resp.Body); err != nil {
		common.SysLog("failed to proxy hermes dashboard response: " + err.Error())
	}
}

func joinURLPath(basePath string, proxyPath string) string {
	basePath = strings.TrimRight(basePath, "/")
	if proxyPath == "" || proxyPath == "/" {
		if basePath == "" {
			return "/"
		}
		return basePath + "/"
	}
	if !strings.HasPrefix(proxyPath, "/") {
		proxyPath = "/" + proxyPath
	}
	return basePath + proxyPath
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
	token, tokenCreated, err := model.EnsureHermesTenantRuntimeToken(tenant)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	adminToken, adminTokenCreated, err := model.EnsureHermesTenantAdminToken(tenant)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	tenant, err = model.GetHermesTenantByUserID(userID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"tenant":                     tenant,
		"created":                    created,
		"tenant_token":               token.GetFullKey(),
		"tenant_token_id":            token.Id,
		"tenant_token_created":       tokenCreated,
		"hermes_admin_token":         adminToken,
		"hermes_admin_token_created": adminTokenCreated,
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
