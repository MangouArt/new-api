package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupMangouAgentTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	common.BatchUpdateEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Task{}, &model.Log{}))

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func newMangouJSONContext(t *testing.T, method string, target string, body any) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	payload, err := common.Marshal(body)
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, bytes.NewReader(payload))
	ctx.Request.Header.Set("Content-Type", "application/json")
	return ctx, recorder
}

func decodeMangouResponse(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var resp map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &resp))
	return resp
}

func TestMangouAgentRegisterCreatesUserAndTokenWithVerifiedEmail(t *testing.T) {
	db := setupMangouAgentTestDB(t)
	common.RegisterVerificationCodeWithKey("agent@example.com", "123456", common.EmailVerificationPurpose)

	ctx, recorder := newMangouJSONContext(t, http.MethodPost, "/v1/agents/register", map[string]any{
		"email":             "agent@example.com",
		"verification_code": "123456",
		"agent_id":          "hermes-mangou",
	})

	MangouAgentRegister(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	resp := decodeMangouResponse(t, recorder)
	require.Equal(t, true, resp["success"])
	data := resp["data"].(map[string]any)
	require.Equal(t, "agent@example.com", data["email"])
	require.Equal(t, "hermes-mangou", data["agent_id"])
	require.NotEmpty(t, data["token"])

	var user model.User
	require.NoError(t, db.Where("email = ?", "agent@example.com").First(&user).Error)
	var token model.Token
	require.NoError(t, db.Where("user_id = ? AND name = ?", user.Id, "agent:hermes-mangou").First(&token).Error)
	require.True(t, token.UnlimitedQuota)
	require.Equal(t, "auto", token.Group)
}

func TestMangouAgentRegisterRejectsInvalidVerificationCode(t *testing.T) {
	setupMangouAgentTestDB(t)

	ctx, recorder := newMangouJSONContext(t, http.MethodPost, "/v1/agents/register", map[string]any{
		"email":             "agent@example.com",
		"verification_code": "wrong",
		"agent_id":          "hermes-mangou",
	})

	MangouAgentRegister(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	resp := decodeMangouResponse(t, recorder)
	require.Equal(t, false, resp["success"])
}

func TestMangouAgentSubmitTaskCreatesAsyncImageTaskWithParameterPricing(t *testing.T) {
	db := setupMangouAgentTestDB(t)
	t.Setenv("MANGOU_PROVIDER_PRICING", `{"bltai":{"image":{"base_quota":100,"multipliers":{"image_size":{"1K":1,"2K":2},"quality":{"standard":1,"hd":1.5}}}}}`)

	user := model.User{
		Username:    "agentuser",
		DisplayName: "agentuser",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Email:       "agent@example.com",
		Quota:       1000,
		Group:       "default",
	}
	require.NoError(t, db.Create(&user).Error)

	ctx, recorder := newMangouJSONContext(t, http.MethodPost, "/v1/agent/tasks", map[string]any{
		"type":     "image",
		"provider": "bltai",
		"model":    "nano-banana-2",
		"prompt":   "A mango robot painting a storyboard, no text.",
		"params": map[string]any{
			"image_size": "2K",
			"quality":    "hd",
		},
	})
	ctx.Set("id", user.Id)
	ctx.Set("token_id", 77)
	ctx.Set("token_key", "test-token")
	ctx.Set("token_unlimited_quota", true)

	MangouAgentSubmitTask(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	resp := decodeMangouResponse(t, recorder)
	require.Equal(t, true, resp["success"])
	data := resp["data"].(map[string]any)
	require.Equal(t, "submitted", data["status"])
	require.Equal(t, "bltai", data["provider"])
	require.Equal(t, "image", data["type"])
	require.EqualValues(t, 300, data["estimated_quota"])
	require.NotEmpty(t, data["task_id"])

	var task model.Task
	require.NoError(t, db.Where("task_id = ?", data["task_id"]).First(&task).Error)
	require.Equal(t, "bltai", string(task.Platform))
	require.Equal(t, "bltai", task.Group)
	require.Equal(t, "image.generate", task.Action)
	require.EqualValues(t, 300, task.Quota)
	require.EqualValues(t, model.TaskStatusSubmitted, task.Status)
	require.NotNil(t, task.PrivateData.BillingContext)
	require.Equal(t, "nano-banana-2", task.Properties.UpstreamModelName)
	require.Equal(t, "mangou-image", task.Properties.OriginModelName)

	var updated model.User
	require.NoError(t, db.First(&updated, user.Id).Error)
	require.Equal(t, 700, updated.Quota)
}

func TestMangouAgentSubmitTaskRejectsMissingProviderPricing(t *testing.T) {
	setupMangouAgentTestDB(t)
	t.Setenv("MANGOU_PROVIDER_PRICING", `{"bltai":{"image":{"base_quota":100}}}`)

	ctx, recorder := newMangouJSONContext(t, http.MethodPost, "/v1/agent/tasks", map[string]any{
		"type":     "video",
		"provider": "kie",
		"model":    "video-fast",
		"prompt":   "A mango robot walks through a neon city.",
	})
	ctx.Set("id", 1)

	MangouAgentSubmitTask(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	resp := decodeMangouResponse(t, recorder)
	require.Equal(t, false, resp["success"])
}

func TestMangouProviderPricingConfigDoesNotReadStaleEnv(t *testing.T) {
	t.Setenv("MANGOU_PROVIDER_PRICING", `{"bltai":{"image":{"base_quota":100}}}`)
	first, err := loadMangouProviderPricing()
	require.NoError(t, err)
	require.Equal(t, 100, first["bltai"]["image"].BaseQuota)

	t.Setenv("MANGOU_PROVIDER_PRICING", `{"bltai":{"image":{"base_quota":250}}}`)
	second, err := loadMangouProviderPricing()
	require.NoError(t, err)
	require.Equal(t, 250, second["bltai"]["image"].BaseQuota)

	_ = os.Unsetenv("MANGOU_PROVIDER_PRICING")
}
