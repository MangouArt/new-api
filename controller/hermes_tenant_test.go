package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
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

func TestProxyHermesTenantDashboardForwardsThroughNewAPI(t *testing.T) {
	setupHermesTenantControllerTestDB(t)

	var sawAdminToken string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/dashboard/page", r.URL.Path)
		require.Equal(t, "1", r.URL.Query().Get("tab"))
		sawAdminToken = r.Header.Get("X-Hermes-Admin-Token")
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
