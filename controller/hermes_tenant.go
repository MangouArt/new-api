package controller

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
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

type hermesPairingSessionRequest struct {
	ExpiresAt int64 `json:"expires_at"`
}

type hermesPairingURLRequest struct {
	PairingURL           string `json:"pairing_url"`
	CommandExitCode      int    `json:"command_exit_code"`
	CommandOutputSummary string `json:"command_output_summary"`
}

var deployHermesTenantOnZeabur = service.DeployHermesTenantOnZeabur

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

func GetHermesTenantPairingSelf(c *gin.Context) {
	tenant, err := model.GetHermesTenantByUserID(c.GetInt("id"))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiSuccess(c, gin.H{
				"session": nil,
				"exists":  false,
			})
			return
		}
		common.ApiError(c, err)
		return
	}
	session, err := model.GetLatestHermesPairingSession(tenant.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiSuccess(c, gin.H{
				"session": nil,
				"exists":  false,
			})
			return
		}
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"session": session,
		"exists":  true,
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

func AdminListHermesTenantUsers(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	items, total, err := model.ListHermesTenantUsers(pageInfo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

func AdminGetHermesProvisioningConfig(c *gin.Context) {
	common.ApiSuccess(c, service.HermesZeaburConfig())
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

func AdminDeployHermesTenantByUser(c *gin.Context) {
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
	result, err := deployHermesTenantOnZeabur(context.WithoutCancel(c.Request.Context()), service.HermesTenantZeaburDeployRequest{
		Tenant:        tenant,
		TenantToken:   token.GetFullKey(),
		AdminToken:    adminToken,
		NewAPIBaseURL: hermesNewAPIBaseURL(c),
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.MarkHermesTenantDeploying(tenant, result.ProjectID, result.EnvironmentID, result.DeploymentID); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"tenant":                     tenant,
		"created":                    created,
		"tenant_token_id":            token.Id,
		"tenant_token_created":       tokenCreated,
		"hermes_admin_token_created": adminTokenCreated,
		"zeabur":                     result,
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

func hermesNewAPIBaseURL(c *gin.Context) string {
	if configured := strings.TrimSpace(service.HermesNewAPIBaseURL()); configured != "" {
		return configured
	}
	scheme := "https"
	if c.Request.TLS != nil {
		scheme = "https"
	} else if forwardedProto := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")); forwardedProto != "" {
		scheme = strings.Split(forwardedProto, ",")[0]
	} else if c.Request.URL.Scheme != "" {
		scheme = c.Request.URL.Scheme
	}
	return scheme + "://" + c.Request.Host
}

func AdminCreateHermesPairingSessionByUser(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req hermesPairingSessionRequest
	if c.Request.Body != nil {
		_ = c.ShouldBindJSON(&req)
	}
	tenant, _, err := model.EnsureHermesTenantForUser(userID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	expiresAt := req.ExpiresAt
	if expiresAt <= 0 {
		expiresAt = common.GetTimestamp() + 600
	}
	session, err := model.CreateHermesPairingSession(tenant, expiresAt)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := tenant.MarkPairingRequired(); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"session": session,
		"tenant":  tenant,
	})
}

func AdminRecordHermesPairingURLByUser(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	sessionID, err := strconv.Atoi(c.Param("session_id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var req hermesPairingURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if !isValidHTTPURL(req.PairingURL) {
		common.ApiError(c, errors.New("invalid pairing url"))
		return
	}
	tenant, err := model.GetHermesTenantByUserID(userID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	session, err := model.GetHermesPairingSession(tenant.ID, sessionID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.RecordHermesPairingURL(tenant, session, req.PairingURL, req.CommandExitCode, req.CommandOutputSummary); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"session": session,
		"tenant":  tenant,
	})
}

func isValidHTTPURL(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	return (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}
