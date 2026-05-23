package controller

import (
	"net/http"
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
