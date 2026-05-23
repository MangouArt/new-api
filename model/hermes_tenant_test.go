package model

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestEnsureHermesTenantForUserCreatesStableTenant(t *testing.T) {
	truncateTables(t)

	tenant, created, err := EnsureHermesTenantForUser(42)
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, 42, tenant.UserID)
	require.Equal(t, "hermes-42", tenant.TenantID)
	require.Equal(t, "hermes-user-42", tenant.ServiceName)
	require.Equal(t, "hermes-user-42-data", tenant.VolumeName)
	require.Equal(t, HermesTenantStatusCreated, tenant.Status)

	sameTenant, created, err := EnsureHermesTenantForUser(42)
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, tenant.ID, sameTenant.ID)
}

func TestEnsureHermesTenantRuntimeTokenCreatesAndReusesToken(t *testing.T) {
	truncateTables(t)

	tenant, _, err := EnsureHermesTenantForUser(42)
	require.NoError(t, err)

	token, created, err := EnsureHermesTenantRuntimeToken(tenant)
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, 42, token.UserId)
	require.Equal(t, HermesTenantTokenName, token.Name)
	require.NotEmpty(t, token.GetFullKey())
	require.Equal(t, common.TokenStatusEnabled, token.Status)
	require.True(t, token.UnlimitedQuota)
	require.Equal(t, hermesTenantTokenGroup, token.Group)
	require.True(t, token.CrossGroupRetry)

	tenant, err = GetHermesTenantByUserID(42)
	require.NoError(t, err)
	require.Equal(t, token.Id, tenant.TenantTokenID)

	sameToken, created, err := EnsureHermesTenantRuntimeToken(tenant)
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, token.Id, sameToken.Id)
	require.Equal(t, token.GetFullKey(), sameToken.GetFullKey())
}

func TestEnsureHermesTenantAdminTokenCreatesAndReusesSecret(t *testing.T) {
	truncateTables(t)

	tenant, _, err := EnsureHermesTenantForUser(42)
	require.NoError(t, err)

	adminToken, created, err := EnsureHermesTenantAdminToken(tenant)
	require.NoError(t, err)
	require.True(t, created)
	require.NotEmpty(t, adminToken)

	tenant, err = GetHermesTenantByUserID(42)
	require.NoError(t, err)
	require.Equal(t, adminToken, tenant.HermesAdminToken)
	encodedTenant, err := json.Marshal(tenant)
	require.NoError(t, err)
	require.False(t, strings.Contains(string(encodedTenant), "hermes_admin_token"))
	require.False(t, strings.Contains(string(encodedTenant), adminToken))

	sameToken, created, err := EnsureHermesTenantAdminToken(tenant)
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, adminToken, sameToken)
}

func TestUpdateHermesTenantProvisioningStoresZeaburBinding(t *testing.T) {
	truncateTables(t)

	tenant, _, err := EnsureHermesTenantForUser(7)
	require.NoError(t, err)

	err = UpdateHermesTenantProvisioning(
		tenant,
		"project-id",
		"env-id",
		"service-id",
		"volume-id",
		"https://hermes-user-7.example.test",
	)
	require.NoError(t, err)

	tenant, err = GetHermesTenantByUserID(7)
	require.NoError(t, err)
	require.Equal(t, HermesTenantStatusDeployed, tenant.Status)
	require.Equal(t, "project-id", tenant.ZeaburProjectID)
	require.Equal(t, "env-id", tenant.ZeaburEnvironmentID)
	require.Equal(t, "service-id", tenant.ZeaburServiceID)
	require.Equal(t, "volume-id", tenant.ZeaburVolumeID)
	require.Equal(t, "https://hermes-user-7.example.test", tenant.PublicURL)
	require.Equal(t, "https://hermes-user-7.example.test", tenant.DashboardURL)
}

func TestCreateHermesPairingSession(t *testing.T) {
	truncateTables(t)

	tenant, _, err := EnsureHermesTenantForUser(9)
	require.NoError(t, err)

	session, err := CreateHermesPairingSession(tenant, 12345)
	require.NoError(t, err)
	require.Equal(t, tenant.ID, session.TenantID)
	require.Equal(t, 9, session.UserID)
	require.Equal(t, HermesPairingSessionStatusPending, session.Status)
	require.Equal(t, int64(12345), session.ExpiresAt)

	latest, err := GetLatestHermesPairingSession(tenant.ID)
	require.NoError(t, err)
	require.Equal(t, session.ID, latest.ID)
}

func TestRecordHermesPairingURLUpdatesSessionAndTenant(t *testing.T) {
	truncateTables(t)

	tenant, _, err := EnsureHermesTenantForUser(9)
	require.NoError(t, err)
	session, err := CreateHermesPairingSession(tenant, 12345)
	require.NoError(t, err)

	err = RecordHermesPairingURL(tenant, session, "https://open.feishu.cn/pair", 0, "ok")
	require.NoError(t, err)

	updatedSession, err := GetHermesPairingSession(tenant.ID, session.ID)
	require.NoError(t, err)
	require.Equal(t, HermesPairingSessionStatusURLGenerated, updatedSession.Status)
	require.Equal(t, "https://open.feishu.cn/pair", updatedSession.PairingURL)
	require.NotNil(t, updatedSession.CommandExitCode)
	require.Equal(t, 0, *updatedSession.CommandExitCode)
	require.Equal(t, "ok", updatedSession.CommandOutputSummary)
	require.NotZero(t, updatedSession.CompletedAt)

	updatedTenant, err := GetHermesTenantByUserID(9)
	require.NoError(t, err)
	require.Equal(t, HermesTenantStatusPairingURLGenerated, updatedTenant.Status)
}
