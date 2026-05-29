package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupHermesTenantControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	model.InitSQLColumnNames()

	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.HermesTenant{}, &model.HermesPairingSession{}))

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestAdminEnsureHermesTenantByUserReturnsProvisioningSecrets(t *testing.T) {
	setupHermesTenantControllerTestDB(t)

	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/hermes/tenants/user/42", nil, 1)
	ctx.Params = gin.Params{{Key: "user_id", Value: "42"}}

	AdminEnsureHermesTenantByUser(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	resp := decodeAPIResponse(t, recorder)
	require.True(t, resp.Success)

	var data map[string]any
	require.NoError(t, common.Unmarshal(resp.Data, &data))
	require.NotEmpty(t, data["tenant_token"])
	require.NotEmpty(t, data["hermes_admin_token"])
	require.Equal(t, true, data["created"])
	require.Equal(t, true, data["tenant_token_created"])
	require.Equal(t, true, data["hermes_admin_token_created"])

	tenant := data["tenant"].(map[string]any)
	require.Equal(t, float64(42), tenant["user_id"])
	require.Equal(t, "hermes-42", tenant["tenant_id"])
	require.Equal(t, "hermes-user-42", tenant["service_name"])
	require.Equal(t, "hermes-user-42-data", tenant["volume_name"])
	require.Equal(t, "created", tenant["status"])
	require.NotContains(t, tenant, "hermes_admin_token")
}

func TestAdminEnsureHermesTenantByUserReusesProvisioningSecrets(t *testing.T) {
	setupHermesTenantControllerTestDB(t)

	firstCtx, firstRecorder := newAuthenticatedContext(t, http.MethodPost, "/api/hermes/tenants/user/42", nil, 1)
	firstCtx.Params = gin.Params{{Key: "user_id", Value: "42"}}
	AdminEnsureHermesTenantByUser(firstCtx)

	firstResp := decodeAPIResponse(t, firstRecorder)
	require.True(t, firstResp.Success)
	var firstData map[string]any
	require.NoError(t, common.Unmarshal(firstResp.Data, &firstData))

	secondCtx, secondRecorder := newAuthenticatedContext(t, http.MethodPost, "/api/hermes/tenants/user/42", nil, 1)
	secondCtx.Params = gin.Params{{Key: "user_id", Value: "42"}}
	AdminEnsureHermesTenantByUser(secondCtx)

	secondResp := decodeAPIResponse(t, secondRecorder)
	require.True(t, secondResp.Success)
	var secondData map[string]any
	require.NoError(t, common.Unmarshal(secondResp.Data, &secondData))

	require.Equal(t, false, secondData["created"])
	require.Equal(t, false, secondData["tenant_token_created"])
	require.Equal(t, false, secondData["hermes_admin_token_created"])
	require.Equal(t, firstData["tenant_token"], secondData["tenant_token"])
	require.Equal(t, firstData["hermes_admin_token"], secondData["hermes_admin_token"])
}

func TestAdminDeployHermesTenantByUserCallsZeaburProvisioner(t *testing.T) {
	setupHermesTenantControllerTestDB(t)

	original := deployHermesTenantOnZeabur
	t.Cleanup(func() {
		deployHermesTenantOnZeabur = original
	})
	var sawRequest service.HermesTenantZeaburDeployRequest
	deployHermesTenantOnZeabur = func(_ context.Context, req service.HermesTenantZeaburDeployRequest) (*service.HermesTenantZeaburDeployResult, error) {
		sawRequest = req
		return &service.HermesTenantZeaburDeployResult{
			ProjectID:     "project-id",
			EnvironmentID: "env-id",
			ServiceID:     "service-id",
			ServiceName:   req.Tenant.ServiceName,
			VolumeName:    req.Tenant.VolumeName,
			DashboardURL:  "http://hermes-user-42.zeabur.internal:8642",
		}, nil
	}

	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/hermes/tenants/user/42/deploy", nil, 1)
	ctx.Params = gin.Params{{Key: "user_id", Value: "42"}}
	ctx.Request.Host = "mangou-newapi.example.test"
	ctx.Request.Header.Set("X-Forwarded-Proto", "https")

	AdminDeployHermesTenantByUser(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	resp := decodeAPIResponse(t, recorder)
	require.True(t, resp.Success)
	require.Equal(t, 42, sawRequest.Tenant.UserID)
	require.Equal(t, "https://mangou-newapi.example.test", sawRequest.NewAPIBaseURL)
	require.NotEmpty(t, sawRequest.TenantToken)
	require.NotEmpty(t, sawRequest.AdminToken)
	require.NotContains(t, recorder.Body.String(), sawRequest.TenantToken)
	require.NotContains(t, recorder.Body.String(), sawRequest.AdminToken)

	var data map[string]any
	require.NoError(t, common.Unmarshal(resp.Data, &data))
	tenant := data["tenant"].(map[string]any)
	require.Equal(t, "deploying", tenant["status"])
	require.Equal(t, "project-id", tenant["zeabur_project_id"])
	require.Equal(t, "service-id", tenant["zeabur_service_id"])
	require.Equal(t, "http://hermes-user-42.zeabur.internal:8642", tenant["dashboard_url"])
}

func TestAdminDeployHermesTenantByUserReusesExistingDeployment(t *testing.T) {
	setupHermesTenantControllerTestDB(t)

	tenant, _, err := model.EnsureHermesTenantForUser(42)
	require.NoError(t, err)
	require.NoError(t, model.MarkHermesTenantDeploying(tenant, "project-id", "env-id", "service-id", "", ""))

	original := deployHermesTenantOnZeabur
	t.Cleanup(func() {
		deployHermesTenantOnZeabur = original
	})
	deployHermesTenantOnZeabur = func(_ context.Context, _ service.HermesTenantZeaburDeployRequest) (*service.HermesTenantZeaburDeployResult, error) {
		require.FailNow(t, "existing Hermes deployment should be reused")
		return nil, nil
	}

	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/hermes/tenants/user/42/deploy", nil, 1)
	ctx.Params = gin.Params{{Key: "user_id", Value: "42"}}

	AdminDeployHermesTenantByUser(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	resp := decodeAPIResponse(t, recorder)
	require.True(t, resp.Success)
	var data map[string]any
	require.NoError(t, common.Unmarshal(resp.Data, &data))
	require.Equal(t, true, data["reused"])
	tenantData := data["tenant"].(map[string]any)
	require.Equal(t, "service-id", tenantData["zeabur_service_id"])
	require.Equal(t, "http://hermes-user-42.zeabur.internal:8642", tenantData["dashboard_url"])
}

func TestAdminGetHermesProvisioningConfigDoesNotLeakSecrets(t *testing.T) {
	t.Setenv("ZEABUR_API_TOKEN", "secret-token")
	t.Setenv("HERMES_ZEABUR_PROJECT_ID", "project-id")

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/hermes/tenants/provisioning/config", nil, 1)
	AdminGetHermesProvisioningConfig(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	resp := decodeAPIResponse(t, recorder)
	require.True(t, resp.Success)
	require.NotContains(t, recorder.Body.String(), "secret-token")

	var data map[string]any
	require.NoError(t, common.Unmarshal(resp.Data, &data))
	require.Equal(t, true, data["configured"])
	require.Empty(t, data["missing"])
}

func TestGetHermesTenantSelfDoesNotLeakAdminToken(t *testing.T) {
	setupHermesTenantControllerTestDB(t)

	tenant, _, err := model.EnsureHermesTenantForUser(42)
	require.NoError(t, err)
	_, _, err = model.EnsureHermesTenantAdminToken(tenant)
	require.NoError(t, err)

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/hermes/tenant/self", nil, 42)
	GetHermesTenantSelf(ctx)

	resp := decodeAPIResponse(t, recorder)
	require.True(t, resp.Success)
	require.NotContains(t, recorder.Body.String(), "hermes_admin_token")
}

func TestAdminListHermesTenantUsersShowsAllUsersWithoutSecrets(t *testing.T) {
	db := setupHermesTenantControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{Id: 1, Username: "mangou", DisplayName: "Mangou", Email: "root@example.com", Role: common.RoleRootUser, Status: common.UserStatusEnabled, AffCode: "root-aff"}).Error)
	require.NoError(t, db.Create(&model.User{Id: 42, Username: "customer", DisplayName: "Customer", Email: "customer@example.com", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AffCode: "customer-aff"}).Error)

	tenant, _, err := model.EnsureHermesTenantForUser(42)
	require.NoError(t, err)
	_, _, err = model.EnsureHermesTenantAdminToken(tenant)
	require.NoError(t, err)
	session, err := model.CreateHermesPairingSession(tenant, 12345)
	require.NoError(t, err)
	require.NoError(t, model.RecordHermesPairingURL(tenant, session, "https://open.feishu.cn/pair?state=abc", 0, "url generated"))

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/hermes/tenants?p=1&page_size=20", nil, 1)
	AdminListHermesTenantUsers(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	resp := decodeAPIResponse(t, recorder)
	require.True(t, resp.Success)
	require.NotContains(t, recorder.Body.String(), "hermes_admin_token")

	var page map[string]any
	require.NoError(t, common.Unmarshal(resp.Data, &page))
	require.Equal(t, float64(2), page["total"])
	items := page["items"].([]any)
	require.Len(t, items, 2)

	customer := items[0].(map[string]any)
	require.Equal(t, float64(42), customer["user_id"])
	require.Equal(t, "customer", customer["username"])
	require.NotNil(t, customer["tenant"])
	tenantData := customer["tenant"].(map[string]any)
	require.Equal(t, "hermes-user-42", tenantData["service_name"])
	latestPairing := customer["latest_pairing"].(map[string]any)
	require.Equal(t, "url_generated", latestPairing["status"])

	root := items[1].(map[string]any)
	require.Equal(t, float64(1), root["user_id"])
	require.Nil(t, root["tenant"])
}

func TestProxyHermesTenantDashboardForwardsThroughNewAPI(t *testing.T) {
	setupHermesTenantControllerTestDB(t)

	var sawAdminToken string
	var sawNewAPIUser string
	var sawForwardedPrefix string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/dashboard/page", r.URL.Path)
		require.Equal(t, "1", r.URL.Query().Get("tab"))
		sawAdminToken = r.Header.Get("X-Hermes-Admin-Token")
		sawNewAPIUser = r.Header.Get("New-Api-User")
		sawForwardedPrefix = r.Header.Get("X-Forwarded-Prefix")
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("dashboard ok"))
	}))
	t.Cleanup(server.Close)

	tenant, _, err := model.EnsureHermesTenantForUser(42)
	require.NoError(t, err)
	adminToken, _, err := model.EnsureHermesTenantAdminToken(tenant)
	require.NoError(t, err)
	require.NoError(t, model.UpdateHermesTenantProvisioning(tenant, "project", "env", "service", "volume", server.URL+"/dashboard"))

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/hermes/tenant/dashboard/page?tab=1", nil, 42)
	ctx.Params = gin.Params{{Key: "proxy_path", Value: "/page"}}
	ProxyHermesTenantDashboard(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "dashboard ok", recorder.Body.String())
	require.Equal(t, adminToken, sawAdminToken)
	require.Equal(t, "42", sawNewAPIUser)
	require.Equal(t, "/api/hermes/tenant/dashboard", sawForwardedPrefix)
}

func TestProxyHermesTenantDashboardRewritesStaticBasePath(t *testing.T) {
	setupHermesTenantControllerTestDB(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/", r.URL.Path)
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!doctype html><html><head><link rel="icon" href="/favicon.ico"><script type="module" src="/assets/app.js"></script><link rel="stylesheet" href="/assets/app.css"></head><body><div id="root"></div></body></html>`))
	}))
	t.Cleanup(server.Close)

	tenant, _, err := model.EnsureHermesTenantForUser(42)
	require.NoError(t, err)
	_, _, err = model.EnsureHermesTenantAdminToken(tenant)
	require.NoError(t, err)
	require.NoError(t, model.UpdateHermesTenantProvisioning(tenant, "project", "env", "service", "volume", server.URL))

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/hermes/tenant/dashboard/", nil, 42)
	ctx.Params = gin.Params{{Key: "proxy_path", Value: "/"}}
	ProxyHermesTenantDashboard(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	require.Contains(t, body, `window.__HERMES_BASE_PATH__="/api/hermes/tenant/dashboard"`)
	require.Contains(t, body, `href="/api/hermes/tenant/dashboard/favicon.ico"`)
	require.Contains(t, body, `src="/api/hermes/tenant/dashboard/assets/app.js"`)
	require.Contains(t, body, `href="/api/hermes/tenant/dashboard/assets/app.css"`)
}

func TestProxyHermesTenantDashboardInjectsSessionTokenForProtectedAPI(t *testing.T) {
	setupHermesTenantControllerTestDB(t)

	var sawSessionToken string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/dashboard":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<script>window.__HERMES_SESSION_TOKEN__="session-token";</script>`))
		case "/dashboard/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<script>window.__HERMES_SESSION_TOKEN__="session-token";</script>`))
		case "/dashboard/api/config":
			sawSessionToken = r.Header.Get("X-Hermes-Session-Token")
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	tenant, _, err := model.EnsureHermesTenantForUser(42)
	require.NoError(t, err)
	_, _, err = model.EnsureHermesTenantAdminToken(tenant)
	require.NoError(t, err)
	require.NoError(t, model.UpdateHermesTenantProvisioning(tenant, "project", "env", "service", "volume", server.URL+"/dashboard"))

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/hermes/tenant/dashboard/api/config", nil, 42)
	ctx.Params = gin.Params{{Key: "proxy_path", Value: "/api/config"}}
	ProxyHermesTenantDashboard(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "session-token", sawSessionToken)
}

func TestProxyHermesTenantDashboardByHost(t *testing.T) {
	setupHermesTenantControllerTestDB(t)
	t.Setenv("HERMES_DASHBOARD_DOMAIN_SUFFIX", "mangou.art")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/dashboard/api/status", r.URL.Path)
		require.Equal(t, "42", r.Header.Get("New-Api-User"))
		require.Equal(t, "secret-admin", r.Header.Get("X-Hermes-Admin-Token"))
		require.Empty(t, r.Header.Get("X-Forwarded-Prefix"))
		_, _ = w.Write([]byte("host dashboard ok"))
	}))
	t.Cleanup(server.Close)

	tenant, _, err := model.EnsureHermesTenantForUser(42)
	require.NoError(t, err)
	tenant.HermesAdminToken = "secret-admin"
	require.NoError(t, model.DB.Save(tenant).Error)
	require.NoError(t, model.UpdateHermesTenantProvisioning(tenant, "project", "env", "service", "volume", server.URL+"/dashboard"))

	router := gin.New()
	router.Use(sessions.Sessions("session", cookie.NewStore([]byte("secret"))))
	router.GET("/*proxy_path", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("id", 42)
		session.Set("role", common.RoleCommonUser)
		require.NoError(t, session.Save())
		require.True(t, TryProxyHermesTenantDashboardByHost(c))
	})

	req := httptest.NewRequest(http.MethodGet, "https://hermes-user-42.mangou.art/api/status", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "host dashboard ok", recorder.Body.String())
}

func TestProxyHermesTenantDashboardByHostIgnoresNonHermesSubdomains(t *testing.T) {
	setupHermesTenantControllerTestDB(t)
	t.Setenv("HERMES_DASHBOARD_DOMAIN_SUFFIX", "mangou.art")

	ctx, _ := newAuthenticatedContext(t, http.MethodGet, "/api/status", nil, 42)
	ctx.Request.Host = "api.mangou.art"

	require.False(t, TryProxyHermesTenantDashboardByHost(ctx))
}

func TestProxyHermesTenantDashboardByForwardedHost(t *testing.T) {
	setupHermesTenantControllerTestDB(t)
	t.Setenv("HERMES_DASHBOARD_DOMAIN_SUFFIX", "mangou.art")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/dashboard/", r.URL.Path)
		_, _ = w.Write([]byte("forwarded host dashboard ok"))
	}))
	t.Cleanup(server.Close)

	tenant, _, err := model.EnsureHermesTenantForUser(42)
	require.NoError(t, err)
	_, _, err = model.EnsureHermesTenantAdminToken(tenant)
	require.NoError(t, err)
	require.NoError(t, model.UpdateHermesTenantProvisioning(tenant, "project", "env", "service", "volume", server.URL+"/dashboard"))

	router := gin.New()
	router.Use(sessions.Sessions("session", cookie.NewStore([]byte("secret"))))
	router.GET("/*proxy_path", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("id", 42)
		session.Set("role", common.RoleCommonUser)
		require.NoError(t, session.Save())
		require.True(t, TryProxyHermesTenantDashboardByHost(c))
	})

	req := httptest.NewRequest(http.MethodGet, "https://api.mangou.art/", nil)
	req.Header.Set("X-Hermes-Dashboard-Host", "hermes-user-42.mangou.art")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "forwarded host dashboard ok", recorder.Body.String())
}

func TestAdminProxyHermesTenantDashboardForwardsSelectedUser(t *testing.T) {
	setupHermesTenantControllerTestDB(t)

	var sawNewAPIUser string
	var sawForwardedPrefix string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/tenant-dashboard/", r.URL.Path)
		sawNewAPIUser = r.Header.Get("New-Api-User")
		sawForwardedPrefix = r.Header.Get("X-Forwarded-Prefix")
		_, _ = w.Write([]byte("admin dashboard ok"))
	}))
	t.Cleanup(server.Close)

	tenant, _, err := model.EnsureHermesTenantForUser(42)
	require.NoError(t, err)
	_, _, err = model.EnsureHermesTenantAdminToken(tenant)
	require.NoError(t, err)
	require.NoError(t, model.UpdateHermesTenantProvisioning(tenant, "project", "env", "service", "volume", server.URL+"/tenant-dashboard"))

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/hermes/tenants/user/42/dashboard/", nil, 1)
	ctx.Params = gin.Params{
		{Key: "user_id", Value: "42"},
		{Key: "proxy_path", Value: "/"},
	}
	AdminProxyHermesTenantDashboardByUser(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "admin dashboard ok", recorder.Body.String())
	require.Equal(t, "42", sawNewAPIUser)
	require.Equal(t, "/api/hermes/tenants/user/42/dashboard", sawForwardedPrefix)
}

func TestProxyHermesTenantDashboardBlocksRuntimeAdminPaths(t *testing.T) {
	setupHermesTenantControllerTestDB(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("dashboard proxy should not forward runtime admin paths")
	}))
	t.Cleanup(server.Close)

	tenant, _, err := model.EnsureHermesTenantForUser(42)
	require.NoError(t, err)
	_, _, err = model.EnsureHermesTenantAdminToken(tenant)
	require.NoError(t, err)
	require.NoError(t, model.UpdateHermesTenantProvisioning(tenant, "project", "env", "service", "volume", server.URL+"/dashboard"))

	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/hermes/tenant/dashboard/admin/feishu/pair", nil, 42)
	ctx.Params = gin.Params{{Key: "proxy_path", Value: "/admin/feishu/pair"}}
	ProxyHermesTenantDashboard(ctx)

	resp := decodeAPIResponse(t, recorder)
	require.False(t, resp.Success)
	require.Contains(t, resp.Message, "runtime admin path")
}

func TestHermesPairingSessionControlPlaneStoresURL(t *testing.T) {
	setupHermesTenantControllerTestDB(t)

	createCtx, createRecorder := newAuthenticatedContext(t, http.MethodPost, "/api/hermes/tenants/user/42/pairing-sessions", nil, 1)
	createCtx.Params = gin.Params{{Key: "user_id", Value: "42"}}
	AdminCreateHermesPairingSessionByUser(createCtx)

	require.Equal(t, http.StatusOK, createRecorder.Code)
	createResp := decodeAPIResponse(t, createRecorder)
	require.True(t, createResp.Success)

	var createData map[string]any
	require.NoError(t, common.Unmarshal(createResp.Data, &createData))
	session := createData["session"].(map[string]any)
	sessionID := int(session["id"].(float64))
	require.Equal(t, "pending", session["status"])

	recordCtx, recordRecorder := newAuthenticatedContext(t, http.MethodPut, "/api/hermes/tenants/user/42/pairing-sessions/"+strconv.Itoa(sessionID)+"/url", map[string]any{
		"pairing_url":            "https://open.feishu.cn/pair?state=abc",
		"command_exit_code":      0,
		"command_output_summary": "url generated",
	}, 1)
	recordCtx.Params = gin.Params{
		{Key: "user_id", Value: "42"},
		{Key: "session_id", Value: strconv.Itoa(sessionID)},
	}
	AdminRecordHermesPairingURLByUser(recordCtx)

	require.Equal(t, http.StatusOK, recordRecorder.Code)
	recordResp := decodeAPIResponse(t, recordRecorder)
	require.True(t, recordResp.Success)

	selfCtx, selfRecorder := newAuthenticatedContext(t, http.MethodGet, "/api/hermes/tenant/pairing/latest", nil, 42)
	GetHermesTenantPairingSelf(selfCtx)

	selfResp := decodeAPIResponse(t, selfRecorder)
	require.True(t, selfResp.Success)
	var selfData map[string]any
	require.NoError(t, common.Unmarshal(selfResp.Data, &selfData))
	latest := selfData["session"].(map[string]any)
	require.Equal(t, "url_generated", latest["status"])
	require.Equal(t, "https://open.feishu.cn/pair?state=abc", latest["pairing_url"])

	tenant, err := model.GetHermesTenantByUserID(42)
	require.NoError(t, err)
	require.Equal(t, model.HermesTenantStatusPairingURLGenerated, tenant.Status)
}

func TestStartHermesTenantPairingCallsRuntimeAndStoresURL(t *testing.T) {
	setupHermesTenantControllerTestDB(t)

	var sawAdminToken string
	var sawNewAPIUser string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/runtime/admin/feishu/pair", r.URL.Path)
		require.Equal(t, http.MethodPost, r.Method)
		sawAdminToken = r.Header.Get("X-Hermes-Admin-Token")
		sawNewAPIUser = r.Header.Get("New-Api-User")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"pairing_url":"https://open.feishu.cn/pair?state=abc","expire_in":600}`))
	}))
	t.Cleanup(server.Close)

	tenant, _, err := model.EnsureHermesTenantForUser(42)
	require.NoError(t, err)
	adminToken, _, err := model.EnsureHermesTenantAdminToken(tenant)
	require.NoError(t, err)
	require.NoError(t, model.UpdateHermesTenantProvisioning(tenant, "project", "env", "service", "volume", server.URL+"/runtime"))

	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/hermes/tenant/pairing/start", nil, 42)
	StartHermesTenantPairingSelf(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, adminToken, sawAdminToken)
	require.Equal(t, "42", sawNewAPIUser)

	resp := decodeAPIResponse(t, recorder)
	require.True(t, resp.Success)
	var data map[string]any
	require.NoError(t, common.Unmarshal(resp.Data, &data))
	session := data["session"].(map[string]any)
	require.Equal(t, "url_generated", session["status"])
	require.Equal(t, "https://open.feishu.cn/pair?state=abc", session["pairing_url"])
}

func TestAdminRecordHermesPairingURLRejectsInvalidURL(t *testing.T) {
	setupHermesTenantControllerTestDB(t)

	tenant, _, err := model.EnsureHermesTenantForUser(42)
	require.NoError(t, err)
	session, err := model.CreateHermesPairingSession(tenant, 12345)
	require.NoError(t, err)

	ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/hermes/tenants/user/42/pairing-sessions/1/url", map[string]any{
		"pairing_url": "not-a-url",
	}, 1)
	ctx.Params = gin.Params{
		{Key: "user_id", Value: "42"},
		{Key: "session_id", Value: strconv.Itoa(session.ID)},
	}
	AdminRecordHermesPairingURLByUser(ctx)

	resp := decodeAPIResponse(t, recorder)
	require.False(t, resp.Success)
	require.Contains(t, resp.Message, "invalid pairing url")
}
