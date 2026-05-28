package controller

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-contrib/sessions"
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

type hermesRuntimePairingResponse struct {
	PairingURL string `json:"pairing_url"`
	ExpireIn   int64  `json:"expire_in"`
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

func StartHermesTenantPairingSelf(c *gin.Context) {
	tenant, err := model.GetHermesTenantByUserID(c.GetInt("id"))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiError(c, errors.New("hermes tenant not found"))
			return
		}
		common.ApiError(c, err)
		return
	}
	startHermesTenantPairing(c, tenant)
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
	proxyHermesTenantDashboard(c, tenant, "/api/hermes/tenant/dashboard")
}

func AdminProxyHermesTenantDashboardByUser(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	tenant, err := model.GetHermesTenantByUserID(userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiError(c, errors.New("hermes tenant not found"))
			return
		}
		common.ApiError(c, err)
		return
	}
	proxyHermesTenantDashboard(c, tenant, "/api/hermes/tenants/user/"+strconv.Itoa(userID)+"/dashboard")
}

func proxyHermesTenantDashboard(c *gin.Context, tenant *model.HermesTenant, forwardedPrefix string) {
	proxyHermesTenantDashboardPath(c, tenant, forwardedPrefix, c.Param("proxy_path"))
}

func TryProxyHermesTenantDashboardByHost(c *gin.Context) bool {
	host := strings.ToLower(strings.TrimSpace(c.GetHeader("X-Hermes-Dashboard-Host")))
	if host == "" {
		host = strings.ToLower(strings.TrimSpace(c.Request.Host))
	}
	if colon := strings.LastIndex(host, ":"); colon >= 0 {
		host = host[:colon]
	}
	suffix := service.HermesDashboardDomainSuffix()
	if suffix == "" || host == suffix || !strings.HasSuffix(host, "."+suffix) {
		return false
	}
	serviceName := strings.TrimSuffix(host, "."+suffix)
	if serviceName == "" {
		return false
	}
	if !strings.HasPrefix(serviceName, "hermes-user-") {
		return false
	}
	tenant, err := model.GetHermesTenantByServiceName(serviceName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiError(c, errors.New("hermes tenant not found for dashboard host"))
			return true
		}
		common.ApiError(c, err)
		return true
	}
	session := sessions.Default(c)
	authUserID, _ := session.Get("id").(int)
	authRole, _ := session.Get("role").(int)
	if authUserID == 0 {
		common.ApiError(c, errors.New("not logged in"))
		return true
	}
	if authUserID != tenant.UserID && authRole < common.RoleAdminUser {
		common.ApiError(c, errors.New("hermes tenant dashboard access denied"))
		return true
	}
	proxyPath := c.Request.URL.Path
	if proxyPath == "" {
		proxyPath = "/"
	}
	proxyHermesTenantDashboardPath(c, tenant, "", proxyPath)
	return true
}

func proxyHermesTenantDashboardPath(c *gin.Context, tenant *model.HermesTenant, forwardedPrefix string, proxyPath string) {
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

	if proxyPath == "" {
		proxyPath = "/"
	}
	if isHermesRuntimeAdminPath(proxyPath) {
		common.ApiError(c, errors.New("hermes runtime admin path is not available through dashboard proxy"))
		return
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
	req.Header.Set("New-Api-User", strconv.Itoa(tenant.UserID))
	req.Header.Set("X-Hermes-Admin-Token", tenant.HermesAdminToken)
	if forwardedPrefix != "" {
		req.Header.Set("X-Forwarded-Prefix", forwardedPrefix)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	body = rewriteHermesDashboardProxyBody(body, resp.Header.Get("Content-Type"), forwardedPrefix)

	for key, values := range resp.Header {
		if strings.EqualFold(key, "Content-Length") {
			continue
		}
		for _, value := range values {
			c.Writer.Header().Add(key, value)
		}
	}
	c.Writer.Header().Set("Content-Length", strconv.Itoa(len(body)))
	c.Status(resp.StatusCode)
	if _, err := c.Writer.Write(body); err != nil {
		common.SysLog("failed to proxy hermes dashboard response: " + err.Error())
	}
}

func rewriteHermesDashboardProxyBody(body []byte, contentType string, forwardedPrefix string) []byte {
	prefix := strings.TrimRight(forwardedPrefix, "/")
	if prefix == "" {
		return body
	}
	lowerContentType := strings.ToLower(contentType)
	text := string(body)
	switch {
	case strings.Contains(lowerContentType, "text/html"):
		text = rewriteHermesHTMLAbsolutePaths(text, prefix)
		injectedBase := `<script>window.__HERMES_BASE_PATH__=` + strconv.Quote(prefix) + `;</script>`
		if strings.Contains(text, "</head>") && !strings.Contains(text, "__HERMES_BASE_PATH__") {
			text = strings.Replace(text, "</head>", injectedBase+"</head>", 1)
		}
	case strings.Contains(lowerContentType, "text/css") || strings.Contains(lowerContentType, "javascript"):
		text = rewriteHermesAbsolutePaths(text, prefix)
	default:
		return body
	}
	return []byte(text)
}

func rewriteHermesHTMLAbsolutePaths(text string, prefix string) string {
	replacements := []struct {
		old string
		new string
	}{
		{`href="/`, `href="` + prefix + `/`},
		{`src="/`, `src="` + prefix + `/`},
		{`action="/`, `action="` + prefix + `/`},
		{`content="/`, `content="` + prefix + `/`},
	}
	for _, replacement := range replacements {
		text = strings.ReplaceAll(text, replacement.old, replacement.new)
	}
	return text
}

func rewriteHermesAbsolutePaths(text string, prefix string) string {
	replacements := []struct {
		old string
		new string
	}{
		{`url(/`, `url(` + prefix + `/`},
		{`url('/`, `url('` + prefix + `/`},
		{`url("/`, `url("` + prefix + `/`},
		{`"/assets/`, `"` + prefix + `/assets/`},
		{`'/assets/`, `'` + prefix + `/assets/`},
		{`"/ds-assets/`, `"` + prefix + `/ds-assets/`},
		{`'/ds-assets/`, `'` + prefix + `/ds-assets/`},
		{`"/fonts/`, `"` + prefix + `/fonts/`},
		{`'/fonts/`, `'` + prefix + `/fonts/`},
		{`"/fonts-terminal/`, `"` + prefix + `/fonts-terminal/`},
		{`'/fonts-terminal/`, `'` + prefix + `/fonts-terminal/`},
		{`"/favicon.ico`, `"` + prefix + `/favicon.ico`},
		{`'/favicon.ico`, `'` + prefix + `/favicon.ico`},
		{`"/api/`, `"` + prefix + `/api/`},
		{`'/api/`, `'` + prefix + `/api/`},
	}
	for _, replacement := range replacements {
		text = strings.ReplaceAll(text, replacement.old, replacement.new)
	}
	return text
}

func isHermesRuntimeAdminPath(proxyPath string) bool {
	if !strings.HasPrefix(proxyPath, "/") {
		proxyPath = "/" + proxyPath
	}
	return proxyPath == "/admin" || strings.HasPrefix(proxyPath, "/admin/")
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
	if tenant.ZeaburServiceID != "" && tenant.Status != model.HermesTenantStatusDeployFailed && tenant.Status != model.HermesTenantStatusDeleted {
		if tenant.DashboardURL == "" {
			if err := model.UpdateHermesTenantDashboardURL(tenant, service.HermesTenantInternalDashboardURL(tenant.ServiceName)); err != nil {
				common.ApiError(c, err)
				return
			}
		}
		if tenant.PublicURL == "" {
			if err := model.UpdateHermesTenantPublicURL(tenant, service.HermesTenantPublicDashboardURL(tenant)); err != nil {
				common.ApiError(c, err)
				return
			}
		}
		common.ApiSuccess(c, gin.H{
			"tenant":  tenant,
			"created": created,
			"reused":  true,
			"zeabur": gin.H{
				"project_id":        tenant.ZeaburProjectID,
				"environment_id":    tenant.ZeaburEnvironmentID,
				"deployment_id":     tenant.ZeaburDeploymentID,
				"service_name":      tenant.ServiceName,
				"volume_name":       tenant.VolumeName,
				"zeabur_service_id": tenant.ZeaburServiceID,
			},
		})
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
	if err := model.MarkHermesTenantDeploying(tenant, result.ProjectID, result.EnvironmentID, result.ServiceID, result.DeploymentID, result.DashboardURL); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.UpdateHermesTenantPublicURL(tenant, service.HermesTenantPublicDashboardURL(tenant)); err != nil {
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

func AdminStartHermesPairingByUser(c *gin.Context) {
	userID, err := strconv.Atoi(c.Param("user_id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	tenant, err := model.GetHermesTenantByUserID(userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			common.ApiError(c, errors.New("hermes tenant not found"))
			return
		}
		common.ApiError(c, err)
		return
	}
	startHermesTenantPairing(c, tenant)
}

func startHermesTenantPairing(c *gin.Context, tenant *model.HermesTenant) {
	adminToken, _, err := model.EnsureHermesTenantAdminToken(tenant)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	resp, err := callHermesRuntimePairing(c.Request.Context(), tenant, adminToken)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	expiresAt := common.GetTimestamp() + resp.ExpireIn
	if resp.ExpireIn <= 0 {
		expiresAt = common.GetTimestamp() + 600
	}
	session, err := model.CreateHermesPairingSession(tenant, expiresAt)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.RecordHermesPairingURL(tenant, session, resp.PairingURL, 0, "pairing url generated by hermes runtime"); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"session": session,
		"tenant":  tenant,
	})
}

func callHermesRuntimePairing(ctx context.Context, tenant *model.HermesTenant, adminToken string) (*hermesRuntimePairingResponse, error) {
	rawBaseURL := strings.TrimSpace(tenant.DashboardURL)
	if rawBaseURL == "" {
		rawBaseURL = strings.TrimSpace(tenant.PublicURL)
	}
	if rawBaseURL == "" {
		return nil, errors.New("hermes tenant dashboard url is not configured")
	}
	target, err := url.Parse(rawBaseURL)
	if err != nil || target.Scheme == "" || target.Host == "" {
		return nil, errors.New("invalid hermes tenant dashboard url")
	}
	target.Path = joinURLPath(target.Path, "/admin/feishu/pair")
	target.RawQuery = ""
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("New-Api-User", strconv.Itoa(tenant.UserID))
	req.Header.Set("X-Hermes-Admin-Token", adminToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errors.New("hermes runtime pairing failed: " + string(body))
	}
	var pairingResp hermesRuntimePairingResponse
	if err := json.Unmarshal(body, &pairingResp); err != nil {
		return nil, err
	}
	if !isValidHTTPURL(pairingResp.PairingURL) {
		return nil, errors.New("hermes runtime returned invalid pairing url")
	}
	return &pairingResp, nil
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
