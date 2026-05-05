package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
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
	model.InitSQLColumnNames()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Task{}, &model.Log{}, &model.TopUp{}, &model.MangouProviderPricing{}))

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func seedMangouPricing(t *testing.T, db *gorm.DB, provider string, taskType string, baseQuota int, multipliers string) {
	t.Helper()
	pricing := model.MangouProviderPricing{
		Provider:        provider,
		TaskType:        taskType,
		BaseQuota:       baseQuota,
		MultipliersJSON: multipliers,
		Enabled:         true,
	}
	require.NoError(t, db.Create(&pricing).Error)
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
	require.Equal(t, data["token"], data["billing_token"])
	require.NotContains(t, data["token"], "*")
	require.NotContains(t, data["token"], "...")
	require.Contains(t, data["token_preview"], "*")

	var user model.User
	require.NoError(t, db.Where("email = ?", "agent@example.com").First(&user).Error)
	require.Equal(t, "auto", user.Group)
	var token model.Token
	require.NoError(t, db.Where("user_id = ? AND name = ?", user.Id, "agent:hermes-mangou").First(&token).Error)
	require.True(t, token.UnlimitedQuota)
	require.Equal(t, "auto", token.Group)
}

func TestMangouAgentRegisterReturnsFullExistingTokenForAgentReuse(t *testing.T) {
	db := setupMangouAgentTestDB(t)
	common.RegisterVerificationCodeWithKey("agent@example.com", "123456", common.EmailVerificationPurpose)

	user := model.User{
		Username:    "agentuser",
		DisplayName: "agentuser",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Email:       "agent@example.com",
		Group:       "default",
	}
	require.NoError(t, db.Create(&user).Error)
	token := model.Token{
		UserId:             user.Id,
		Name:               "agent:hermes-mangou",
		Key:                "ExistingFullAgentToken12345678901234567890",
		Status:             common.TokenStatusEnabled,
		ExpiredTime:        -1,
		UnlimitedQuota:     true,
		ModelLimitsEnabled: false,
		Group:              "auto",
		CrossGroupRetry:    true,
	}
	require.NoError(t, db.Create(&token).Error)

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
	require.Equal(t, false, data["token_new"])
	require.Equal(t, "ExistingFullAgentToken12345678901234567890", data["token"])
	require.Equal(t, data["token"], data["billing_token"])
	require.Contains(t, data["token_preview"], "*")

	var updated model.Token
	require.NoError(t, db.First(&updated, token.Id).Error)
	require.Equal(t, "auto", updated.Group)

	var updatedUser model.User
	require.NoError(t, db.First(&updatedUser, user.Id).Error)
	require.Equal(t, "auto", updatedUser.Group)
}

func TestMangouAgentAuthCheckReturnsAgentContext(t *testing.T) {
	db := setupMangouAgentTestDB(t)
	user := model.User{
		Username:    "agentuser",
		DisplayName: "agentuser",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Email:       "agent@example.com",
		Quota:       123,
		Group:       "default",
	}
	require.NoError(t, db.Create(&user).Error)

	ctx, recorder := newMangouJSONContext(t, http.MethodGet, "/v1/agent/auth/check", nil)
	ctx.Set("id", user.Id)
	ctx.Set("token_name", "agent:hermes-mangou")

	MangouAgentAuthCheck(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	resp := decodeMangouResponse(t, recorder)
	require.Equal(t, true, resp["success"])
	data := resp["data"].(map[string]any)
	require.Equal(t, "hermes-mangou", data["agent_id"])
	require.Equal(t, "agent@example.com", data["email"])
	require.EqualValues(t, 123, data["balance"])
	require.EqualValues(t, 123, data["quota"])
	require.Equal(t, "credits", data["currency"])
}

func TestMangouAgentRechargeQRCreatesDemoPayment(t *testing.T) {
	db := setupMangouAgentTestDB(t)
	originalServerAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "http://localhost:3000"
	t.Cleanup(func() {
		system_setting.ServerAddress = originalServerAddress
	})
	user := model.User{
		Username:    "agentuser",
		DisplayName: "agentuser",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Email:       "agent@example.com",
		Quota:       0,
		Group:       "auto",
	}
	require.NoError(t, db.Create(&user).Error)

	ctx, recorder := newMangouJSONContext(t, http.MethodPost, "/v1/agent/recharge-qr", map[string]any{
		"agent_id": "hermes-mangou",
		"tier":     "gems_100",
	})
	ctx.Request.Host = "mangou-newapi.example.com"
	ctx.Request.Header.Set("X-Forwarded-Proto", "https")
	ctx.Set("id", user.Id)
	ctx.Set("token_name", "agent:hermes-mangou")

	MangouAgentRechargeQR(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	resp := decodeMangouResponse(t, recorder)
	require.Equal(t, true, resp["success"])
	data := resp["data"].(map[string]any)
	require.Equal(t, "hermes-mangou", data["agent_id"])
	require.EqualValues(t, 100, data["amount"])
	require.Equal(t, true, data["demo"])
	require.NotEmpty(t, data["payment_id"])
	require.Contains(t, data["payment_url"], "https://mangou-newapi.example.com/v1/payments/demo-scan/")
	require.Contains(t, data["qr_url"], "https://mangou-newapi.example.com/v1/payments/")

	var topUp model.TopUp
	require.NoError(t, db.Where("trade_no = ?", data["payment_id"]).First(&topUp).Error)
	require.Equal(t, common.TopUpStatusPending, topUp.Status)
	require.Equal(t, "mangou_demo", topUp.PaymentProvider)
	require.EqualValues(t, 100, topUp.Amount)
}

func TestMangouDemoPaymentScanMarksPaymentPaidAndCreditsUser(t *testing.T) {
	db := setupMangouAgentTestDB(t)
	user := model.User{
		Username:    "agentuser",
		DisplayName: "agentuser",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Email:       "agent@example.com",
		Quota:       0,
		Group:       "auto",
	}
	require.NoError(t, db.Create(&user).Error)
	topUp := model.TopUp{
		UserId:          user.Id,
		Amount:          25,
		Money:           25,
		TradeNo:         "pay_demo_scan",
		PaymentMethod:   "mangou_demo",
		PaymentProvider: "mangou_demo",
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, db.Create(&topUp).Error)

	ctx, recorder := newMangouJSONContext(t, http.MethodGet, "/v1/payments/demo-scan/pay_demo_scan", nil)
	ctx.Params = gin.Params{{Key: "payment_id", Value: "pay_demo_scan"}}

	MangouDemoPaymentScan(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "Demo payment complete")

	var updatedUser model.User
	require.NoError(t, db.First(&updatedUser, user.Id).Error)
	require.Equal(t, 25, updatedUser.Quota)

	var updatedTopUp model.TopUp
	require.NoError(t, db.Where("trade_no = ?", "pay_demo_scan").First(&updatedTopUp).Error)
	require.Equal(t, common.TopUpStatusSuccess, updatedTopUp.Status)

	secondCtx, _ := newMangouJSONContext(t, http.MethodGet, "/v1/payments/demo-scan/pay_demo_scan", nil)
	secondCtx.Params = gin.Params{{Key: "payment_id", Value: "pay_demo_scan"}}
	MangouDemoPaymentScan(secondCtx)
	require.NoError(t, db.First(&updatedUser, user.Id).Error)
	require.Equal(t, 25, updatedUser.Quota)
}

func TestMangouAgentRegisteredTokenPassesAuthMiddleware(t *testing.T) {
	db := setupMangouAgentTestDB(t)
	common.RegisterVerificationCodeWithKey("agent@example.com", "123456", common.EmailVerificationPurpose)

	router := gin.New()
	router.POST("/v1/agents/register", MangouAgentRegister)
	agentRouter := router.Group("/v1/agent")
	agentRouter.Use(middleware.TokenAuth())
	agentRouter.GET("/auth/check", MangouAgentAuthCheck)

	registerPayload, err := common.Marshal(map[string]any{
		"email":             "agent@example.com",
		"verification_code": "123456",
		"agent_id":          "hermes-mangou",
	})
	require.NoError(t, err)
	registerReq := httptest.NewRequest(http.MethodPost, "/v1/agents/register", bytes.NewReader(registerPayload))
	registerReq.Header.Set("Content-Type", "application/json")
	registerRecorder := httptest.NewRecorder()

	router.ServeHTTP(registerRecorder, registerReq)

	require.Equal(t, http.StatusOK, registerRecorder.Code)
	registerResp := decodeMangouResponse(t, registerRecorder)
	require.Equal(t, true, registerResp["success"])
	registerData := registerResp["data"].(map[string]any)
	billingToken := registerData["billing_token"].(string)
	require.NotEmpty(t, billingToken)
	require.NotContains(t, billingToken, "*")
	require.NotContains(t, billingToken, "...")

	checkReq := httptest.NewRequest(http.MethodGet, "/v1/agent/auth/check", nil)
	checkReq.Header.Set("Authorization", "Bearer "+billingToken)
	checkRecorder := httptest.NewRecorder()

	router.ServeHTTP(checkRecorder, checkReq)

	require.Equal(t, http.StatusOK, checkRecorder.Code)
	checkResp := decodeMangouResponse(t, checkRecorder)
	require.Equal(t, true, checkResp["success"])
	checkData := checkResp["data"].(map[string]any)
	require.Equal(t, "hermes-mangou", checkData["agent_id"])
	require.Equal(t, "agent@example.com", checkData["email"])

	var token model.Token
	require.NoError(t, db.Where("name = ?", "agent:hermes-mangou").First(&token).Error)
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
	seedMangouPricing(t, db, "bltai", "image", 100, `{"image_size":{"1K":1,"2K":2},"quality":{"standard":1,"hd":1.5}}`)

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

func TestMangouProviderPricingUsesDatabaseOnly(t *testing.T) {
	db := setupMangouAgentTestDB(t)
	t.Setenv("MANGOU_PROVIDER_PRICING", `{"bltai":{"image":{"base_quota":999}}}`)
	t.Setenv("MANGOU_PROVIDER_PRICING_B64", "eyJibHRhaSI6eyJpbWFnZSI6eyJiYXNlX3F1b3RhIjo5OTl9fX0=")
	seedMangouPricing(t, db, "bltai", "image", 100, "")

	rule, err := loadMangouProviderPricingRule("bltai", "image")
	require.NoError(t, err)
	require.Equal(t, 100, rule.BaseQuota)
}

func TestMangouAgentSubmitTaskSubmitsUnifiedProviderTask(t *testing.T) {
	db := setupMangouAgentTestDB(t)
	seedMangouPricing(t, db, "evolink", "image", 100, "")
	t.Setenv("EVOLINK_API_KEY", "test-evolink-key")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer test-evolink-key", r.Header.Get("Authorization"))
		switch r.URL.Path {
		case "/v1/images/generations":
			require.Equal(t, http.MethodPost, r.Method)
			var payload map[string]any
			require.NoError(t, common.DecodeJson(r.Body, &payload))
			require.Equal(t, "gemini-3.1-flash-image-preview", payload["model"])
			require.Equal(t, "A mango robot.", payload["prompt"])
			require.Equal(t, "16:9", payload["aspect_ratio"])
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"remote-image-task","status":"pending","progress":0,"type":"image","model":"gemini-3.1-flash-image-preview"}`))
		case "/v1/tasks/remote-image-task":
			require.Equal(t, http.MethodGet, r.Method)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"remote-image-task","status":"completed","progress":100,"results":["https://cdn.example/image.png"],"type":"image","model":"gemini-3.1-flash-image-preview"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("EVOLINK_BASE_URL", server.URL)

	user := model.User{Username: "agentuser", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Quota: 1000, Group: "default"}
	require.NoError(t, db.Create(&user).Error)

	ctx, recorder := newMangouJSONContext(t, http.MethodPost, "/v1/agent/tasks", map[string]any{
		"type":     "image",
		"provider": "evolink",
		"model":    "gemini-3.1-flash-image-preview",
		"prompt":   "A mango robot.",
		"params": map[string]any{
			"aspect_ratio": "16:9",
		},
	})
	ctx.Set("id", user.Id)
	ctx.Set("token_unlimited_quota", true)

	MangouAgentSubmitTask(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	resp := decodeMangouResponse(t, recorder)
	require.Equal(t, true, resp["success"])
	data := resp["data"].(map[string]any)

	var task model.Task
	require.NoError(t, db.Where("task_id = ?", data["task_id"]).First(&task).Error)
	require.Equal(t, "remote-image-task", task.PrivateData.UpstreamTaskID)

	getCtx, getRecorder := newMangouJSONContext(t, http.MethodGet, "/v1/agent/tasks/"+task.TaskID, nil)
	getCtx.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
	getCtx.Set("id", user.Id)
	MangouAgentGetTask(getCtx)

	require.Equal(t, http.StatusOK, getRecorder.Code)
	getResp := decodeMangouResponse(t, getRecorder)
	require.Equal(t, true, getResp["success"])
	getData := getResp["data"].(map[string]any)
	require.Equal(t, "completed", getData["status"])
	require.Equal(t, "https://cdn.example/image.png", getData["result_url"])
}

func TestMangouAgentSubmitTaskAcceptsOpenAICompatibleImageResponse(t *testing.T) {
	db := setupMangouAgentTestDB(t)
	seedMangouPricing(t, db, "bltai", "image", 100, "")
	t.Setenv("BLTAI_API_KEY", "test-bltai-key")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer test-bltai-key", r.Header.Get("Authorization"))
		require.Equal(t, "/v1/images/generations", r.URL.Path)
		require.Equal(t, http.MethodPost, r.Method)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"created":1777821713,"data":[{"url":"https://cdn.example/sync-image.jpg"}],"model":"nano-banana-pro"}`))
	}))
	defer server.Close()
	t.Setenv("BLTAI_BASE_URL", server.URL+"/v1")

	user := model.User{Username: "agentuser", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Quota: 1000, Group: "default"}
	require.NoError(t, db.Create(&user).Error)

	ctx, recorder := newMangouJSONContext(t, http.MethodPost, "/v1/agent/tasks", map[string]any{
		"type":     "image",
		"provider": "bltai",
		"model":    "nano-banana-2",
		"prompt":   "A mango robot.",
	})
	ctx.Set("id", user.Id)
	ctx.Set("token_unlimited_quota", true)

	MangouAgentSubmitTask(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	resp := decodeMangouResponse(t, recorder)
	require.Equal(t, true, resp["success"])
	data := resp["data"].(map[string]any)

	var task model.Task
	require.NoError(t, db.Where("task_id = ?", data["task_id"]).First(&task).Error)
	require.Empty(t, task.PrivateData.UpstreamTaskID)
	require.EqualValues(t, model.TaskStatusSuccess, task.Status)
	require.Equal(t, "100%", task.Progress)
	require.Equal(t, "https://cdn.example/sync-image.jpg", task.PrivateData.ResultURL)
}

func TestMangouAgentSubmitTaskSubmitsKIERunwayVideoTask(t *testing.T) {
	db := setupMangouAgentTestDB(t)
	seedMangouPricing(t, db, "kie", "video", 200, "")
	t.Setenv("KIE_API_KEY", "test-kie-key")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer test-kie-key", r.Header.Get("Authorization"))
		switch r.URL.Path {
		case "/api/v1/runway/generate":
			require.Equal(t, http.MethodPost, r.Method)
			var payload map[string]any
			require.NoError(t, common.DecodeJson(r.Body, &payload))
			require.Equal(t, "A mango robot walks.", payload["prompt"])
			require.EqualValues(t, float64(5), payload["duration"])
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":200,"msg":"success","data":{"taskId":"kie-video-task"}}`))
		case "/api/v1/runway/record-detail":
			require.Equal(t, "kie-video-task", r.URL.Query().Get("taskId"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":200,"msg":"success","data":{"taskId":"kie-video-task","state":"success","videoInfo":{"videoUrl":"https://cdn.example/video.mp4","imageUrl":"https://cdn.example/cover.png"}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("KIE_BASE_URL", server.URL)

	user := model.User{Username: "agentuser", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Quota: 1000, Group: "default"}
	require.NoError(t, db.Create(&user).Error)

	ctx, recorder := newMangouJSONContext(t, http.MethodPost, "/v1/agent/tasks", map[string]any{
		"type":     "video",
		"provider": "kie",
		"model":    "runway",
		"prompt":   "A mango robot walks.",
		"params": map[string]any{
			"duration": 5,
		},
	})
	ctx.Set("id", user.Id)
	ctx.Set("token_unlimited_quota", true)

	MangouAgentSubmitTask(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	resp := decodeMangouResponse(t, recorder)
	require.Equal(t, true, resp["success"])
	data := resp["data"].(map[string]any)

	var task model.Task
	require.NoError(t, db.Where("task_id = ?", data["task_id"]).First(&task).Error)
	require.Equal(t, "kie-video-task", task.PrivateData.UpstreamTaskID)

	getCtx, getRecorder := newMangouJSONContext(t, http.MethodGet, "/v1/agent/tasks/"+task.TaskID, nil)
	getCtx.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
	getCtx.Set("id", user.Id)
	MangouAgentGetTask(getCtx)

	require.Equal(t, http.StatusOK, getRecorder.Code)
	getResp := decodeMangouResponse(t, getRecorder)
	require.Equal(t, true, getResp["success"])
	getData := getResp["data"].(map[string]any)
	require.Equal(t, "completed", getData["status"])
	require.Equal(t, "https://cdn.example/video.mp4", getData["result_url"])
}
