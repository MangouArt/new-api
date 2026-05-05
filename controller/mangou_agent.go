package controller

import (
	"bytes"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/boombuler/barcode"
	"github.com/boombuler/barcode/qr"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	mangouAgentDefaultAgentID = "mangou-agent"
	mangouAgentTokenPrefix    = "agent:"
	mangouAgentDefaultGroup   = "auto"
	mangouImageOriginModel    = "mangou-image"
	mangouVideoOriginModel    = "mangou-video"
	mangouDemoPaymentMethod   = "mangou_demo"
	mangouDemoPaymentProvider = "mangou_demo"
)

type mangouAgentRegisterRequest struct {
	Email            string `json:"email"`
	VerificationCode string `json:"verification_code"`
	AgentID          string `json:"agent_id"`
}

type mangouAgentEmailCodeRequest struct {
	Email string `json:"email"`
}

type mangouAgentTaskRequest struct {
	Type     string         `json:"type"`
	Provider string         `json:"provider"`
	Model    string         `json:"model"`
	Prompt   string         `json:"prompt"`
	Params   map[string]any `json:"params"`
}

type mangouAgentRechargeRequest struct {
	AgentID   string `json:"agent_id"`
	Tier      string `json:"tier"`
	Amount    int64  `json:"amount"`
	ReturnURL string `json:"return_url"`
}

type mangouProviderPricingRule struct {
	BaseQuota   int                           `json:"base_quota"`
	Multipliers map[string]map[string]float64 `json:"multipliers"`
}

type mangouUpstreamTaskResult struct {
	ID        string
	Status    model.TaskStatus
	Progress  string
	ResultURL string
	Raw       []byte
}

type mangouUnifiedTaskResponse struct {
	ID       string                       `json:"id"`
	Status   string                       `json:"status"`
	Progress int                          `json:"progress"`
	Results  []string                     `json:"results"`
	Error    map[string]any               `json:"error"`
	Data     []mangouUnifiedGeneratedItem `json:"data"`
}

type mangouUnifiedGeneratedItem struct {
	URL     string `json:"url"`
	B64JSON string `json:"b64_json"`
}

type mangouKIERunwaySubmitResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		TaskID string `json:"taskId"`
	} `json:"data"`
}

type mangouKIERunwayRecordResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		TaskID    string `json:"taskId"`
		State     string `json:"state"`
		FailMsg   string `json:"failMsg"`
		VideoInfo struct {
			VideoURL string `json:"videoUrl"`
			ImageURL string `json:"imageUrl"`
		} `json:"videoInfo"`
	} `json:"data"`
}

func MangouAgentSkill(c *gin.Context) {
	c.Data(http.StatusOK, "text/markdown; charset=utf-8", []byte(`# Mangou NewAPI Agent Skill

Use this NewAPI gateway for Mangou image and video tasks. All protected Agent Gateway endpoints use `+"`Authorization: Bearer ${BILLING_TOKEN}`"+`.

## Token Boundary

- `+"`BILLING_TOKEN`"+` is the NewAPI Agent Gateway token.
- `+"`BILLING_TOKEN`"+` is not a provider API key.
- `+"`BILLING_TOKEN`"+` is not a TikHub `+"`TIKHUB_API_KEY`"+`, MCP session ID, OpenAI key, BLTAI key, KIE key, or Evolink key.
- Never print the full token, never commit it, and never log raw registration responses containing it.

## Register

If `+"`BILLING_TOKEN`"+` is missing, register with the user's email.

1. Request an email code:

`+"```http"+`
POST /v1/agents/register/email-code
Content-Type: application/json
`+"```"+`

Body:

`+"```json"+`
{
  "email": "user@example.com"
}
`+"```"+`

2. Register with email and code:

`+"```http"+`
POST /v1/agents/register
Content-Type: application/json
`+"```"+`

Body:

`+"```json"+`
{
  "email": "user@example.com",
  "verification_code": "123456",
  "agent_id": "mangou-agent"
}
`+"```"+`

The field name is `+"`verification_code`"+`. Do not use `+"`code`"+` or `+"`email_code`"+`.

Store the returned full `+"`billing_token`"+` as `+"`BILLING_TOKEN`"+`. The `+"`token_preview`"+` field is only for display and cannot be used for authentication.

3. Verify the token:

`+"```bash"+`
curl -sS "`+strings.TrimSuffix(system_setting.ServerAddress, "/")+`/v1/agent/auth/check" \
  -H "Authorization: Bearer ${BILLING_TOKEN}"
`+"```"+`

Expected success includes `+"`agent_id`"+`, `+"`user_id`"+`, `+"`balance`"+`, `+"`quota`"+`, and `+"`currency`"+`.

## Balance And Demo Recharge

Check balance:

`+"```bash"+`
curl -sS "`+strings.TrimSuffix(system_setting.ServerAddress, "/")+`/v1/agent/balance" \
  -H "Authorization: Bearer ${BILLING_TOKEN}"
`+"```"+`

`+"`/v1/agent/credits`"+` is an alias for `+"`/v1/agent/balance`"+`.

Request a demo recharge QR:

`+"```http"+`
POST /v1/agent/recharge-qr
Authorization: Bearer ${BILLING_TOKEN}
Content-Type: application/json
`+"```"+`

Body:

`+"```json"+`
{
  "agent_id": "mangou-agent",
  "tier": "gems_100",
  "amount": 100,
  "return_url": "`+strings.TrimSuffix(system_setting.ServerAddress, "/")+`"
}
`+"```"+`

The response includes:

- `+"`payment_id`"+`
- `+"`qr_url`"+`
- `+"`payment_url`"+`
- `+"`amount`"+`
- `+"`currency`"+`

Validation flow:

1. `+"`GET qr_url`"+` and verify the response `+"`Content-Type`"+` contains `+"`image/svg+xml`"+`.
2. Show the QR code or open `+"`payment_url`"+` to simulate a user scan.
3. Recheck `+"`/v1/agent/balance`"+` and confirm the balance increased.
4. Open the same `+"`payment_url`"+` again and confirm the balance does not increase again. Demo payments are idempotent.

Legacy protected aliases also exist: `+"`POST /v1/agent/recharge`"+`, `+"`POST /v1/agent/topup`"+`, `+"`POST /v1/agent/payment`"+`, and `+"`POST /v1/agents/recharge-qr`"+`.

## Submit Task

`+"```http"+`
POST /v1/agent/tasks
Authorization: Bearer ${BILLING_TOKEN}
Content-Type: application/json
`+"```"+`

Image example using official `+"`gpt-image-2`"+` pricing:

Body:

`+"```json"+`
{
  "type": "image",
  "provider": "bltai",
  "model": "gpt-image-2",
  "prompt": "A mango robot checking official pricing tables, no text.",
  "params": {
    "quality": "medium",
    "image_size": "1024x1024",
    "response_format": "url"
  }
}
`+"```"+`

Video example using Seedance 2.0 pricing:

`+"```json"+`
{
  "type": "video",
  "provider": "kie",
  "model": "doubao-seedance-2-0-fast-260128",
  "prompt": "A mango robot flipping through a storyboard, cinematic, no text.",
  "params": {
    "duration": 5,
    "resolution": "720p",
    "aspect_ratio": "16:9",
    "quality": "720p"
  }
}
`+"```"+`

Parameter notes:

- For `+"`gpt-image-2`"+` image tasks, use `+"`params.image_size`"+` such as `+"`1024x1024`"+`; do not send only `+"`params.size`"+`.
- For KIE Seedance video tasks, send `+"`params.quality`"+` such as `+"`720p`"+`. If omitted, the gateway tries to copy `+"`params.resolution`"+`; if both are missing, the request is rejected before provider submission.
- API errors return `+"`success: false`"+` and an actionable `+"`message`"+`. Do not guess missing fields; read the returned `+"`message`"+`, fix the named field, and retry.

Supported official pricing rows currently include:

- `+"`bltai`"+` image: `+"`gpt-image-2`"+`
- `+"`kie`"+` image: `+"`gpt-image-2`"+`
- `+"`evolink`"+` image: `+"`gpt-image-2`"+`
- `+"`kie`"+` video: Seedance 2.0 series
- `+"`evolink`"+` video: Seedance 2.0 series

Images and videos are asynchronous tasks. The submit response includes `+"`task_id`"+`, `+"`status`"+`, and `+"`estimated_quota`"+`.

Poll until terminal status:

`+"```bash"+`
curl -sS "`+strings.TrimSuffix(system_setting.ServerAddress, "/")+`/v1/agent/tasks/${TASK_ID}" \
  -H "Authorization: Bearer ${BILLING_TOKEN}"
`+"```"+`

Terminal statuses are `+"`completed`"+` and `+"`failed`"+`. On success, use the returned result URL. On failure, report the task ID and error without exposing secrets.

## Security Checklist

- Do not print full `+"`BILLING_TOKEN`"+`.
- Do not print provider keys.
- Do not commit `+"`.env`"+`.
- Do not commit email verification codes.
- Do not commit raw registration responses.
- Redact secrets as `+"`[REDACTED]`"+` in summaries and logs.
`))
}

func MangouAgentSendEmailCode(c *gin.Context) {
	var req mangouAgentEmailCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	email := strings.TrimSpace(strings.ToLower(req.Email))
	if err := validateMangouAgentEmail(email); err != nil {
		common.ApiError(c, err)
		return
	}

	code := common.GenerateVerificationCode(6)
	common.RegisterVerificationCodeWithKey(email, code, common.EmailVerificationPurpose)
	subject := fmt.Sprintf("%s Mangou Agent 邮箱验证码", common.SystemName)
	content := fmt.Sprintf("<p>您好，你正在注册或登录 Mangou Agent。</p>"+
		"<p>您的验证码为: <strong>%s</strong></p>"+
		"<p>验证码 %d 分钟内有效，如果不是本人操作，请忽略。</p>", code, common.VerificationValidMinutes)
	if err := common.SendEmail(subject, email, content); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"email": email})
}

func MangouAgentRegister(c *gin.Context) {
	var req mangouAgentRegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	email := strings.TrimSpace(strings.ToLower(req.Email))
	agentID := strings.TrimSpace(req.AgentID)
	if agentID == "" {
		agentID = mangouAgentDefaultAgentID
	}
	if err := validateMangouAgentEmail(email); err != nil {
		common.ApiError(c, err)
		return
	}
	if !common.VerifyCodeWithKey(email, strings.TrimSpace(req.VerificationCode), common.EmailVerificationPurpose) {
		common.ApiErrorMsg(c, "invalid email verification code")
		return
	}

	user, err := findOrCreateMangouAgentUser(email)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	token, tokenCreated, err := findOrCreateMangouAgentToken(user.Id, agentID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.DeleteKey(email, common.EmailVerificationPurpose)

	tokenValue := token.GetFullKey()
	common.ApiSuccess(c, gin.H{
		"agent_id":      agentID,
		"email":         user.Email,
		"user_id":       user.Id,
		"token":         tokenValue,
		"billing_token": tokenValue,
		"token_preview": token.GetMaskedKey(),
		"token_new":     tokenCreated,
		"balance":       user.Quota,
		"base_url":      strings.TrimSuffix(system_setting.ServerAddress, "/"),
		"skill_url":     strings.TrimSuffix(system_setting.ServerAddress, "/") + "/skills/mangou-newapi/SKILL.md",
		"token_store":   "BILLING_TOKEN",
	})
}

func MangouAgentAuthCheck(c *gin.Context) {
	respondMangouAgentBalance(c)
}

func MangouAgentBalance(c *gin.Context) {
	respondMangouAgentBalance(c)
}

func respondMangouAgentBalance(c *gin.Context) {
	userID := c.GetInt("id")
	if userID <= 0 {
		common.ApiErrorMsg(c, "authenticated user is required")
		return
	}
	user, err := model.GetUserById(userID, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	agentID := strings.TrimPrefix(c.GetString("token_name"), mangouAgentTokenPrefix)
	if agentID == "" {
		agentID = mangouAgentDefaultAgentID
	}
	common.ApiSuccess(c, gin.H{
		"agent_id": agentID,
		"email":    user.Email,
		"user_id":  user.Id,
		"balance":  user.Quota,
		"quota":    user.Quota,
		"currency": "credits",
	})
}

func MangouAgentRechargeQR(c *gin.Context) {
	var req mangouAgentRechargeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	userID := c.GetInt("id")
	if userID <= 0 {
		common.ApiErrorMsg(c, "authenticated user is required")
		return
	}
	amount, tier, err := normalizeMangouRechargeAmount(req)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	agentID := strings.TrimSpace(req.AgentID)
	if agentID == "" {
		agentID = strings.TrimPrefix(c.GetString("token_name"), mangouAgentTokenPrefix)
	}
	if agentID == "" {
		agentID = mangouAgentDefaultAgentID
	}
	paymentID := "pay_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	baseURL := mangouPublicBaseURL(c)
	if baseURL == "" {
		common.ApiErrorMsg(c, "public base URL is unavailable")
		return
	}
	paymentURL := buildMangouDemoPaymentURL(baseURL, paymentID, req.ReturnURL)

	topUp := &model.TopUp{
		UserId:          userID,
		Amount:          amount,
		Money:           float64(amount),
		TradeNo:         paymentID,
		PaymentMethod:   mangouDemoPaymentMethod,
		PaymentProvider: mangouDemoPaymentProvider,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	if err := topUp.Insert(); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"agent_id":       agentID,
		"user_id":        userID,
		"payment_id":     paymentID,
		"payment_status": topUp.Status,
		"tier":           tier,
		"amount":         amount,
		"currency":       "credits",
		"demo":           true,
		"qr_url":         baseURL + "/v1/payments/" + paymentID + "/qr.svg",
		"payment_url":    paymentURL,
		"instructions":   "Show qr_url or payment_url to the user. Opening payment_url simulates a completed payment and credits the account.",
	})
}

func MangouPaymentQRSVG(c *gin.Context) {
	paymentID := strings.TrimSpace(c.Param("payment_id"))
	topUp := model.GetTopUpByTradeNo(paymentID)
	if topUp == nil || topUp.PaymentProvider != mangouDemoPaymentProvider {
		c.String(http.StatusNotFound, "payment not found")
		return
	}
	paymentURL := buildMangouDemoPaymentURL(mangouPublicBaseURL(c), paymentID, "")
	svg, err := mangouQRCodeSVG(paymentURL)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.Data(http.StatusOK, "image/svg+xml; charset=utf-8", []byte(svg))
}

func MangouDemoPaymentScan(c *gin.Context) {
	paymentID := strings.TrimSpace(c.Param("payment_id"))
	payment, user, err := completeMangouDemoPayment(paymentID, c.ClientIP())
	if err != nil {
		c.String(http.StatusBadRequest, html.EscapeString(err.Error()))
		return
	}
	returnURL := strings.TrimSpace(c.Query("returnUrl"))
	body := "<!doctype html><html><head><meta charset=\"utf-8\"><title>Payment complete</title>"
	if returnURL != "" && common.ValidateRedirectURL(returnURL) == nil {
		body += "<meta http-equiv=\"refresh\" content=\"1;url=" + html.EscapeString(returnURL) + "\">"
	}
	body += "</head><body><h1>Demo payment complete</h1>"
	body += "<p>Payment <code>" + html.EscapeString(payment.TradeNo) + "</code> is paid.</p>"
	body += "<p>User <code>" + strconv.Itoa(user.Id) + "</code> balance: <code>" + strconv.Itoa(user.Quota) + "</code> credits.</p>"
	body += "</body></html>"
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(body))
}

func MangouAgentSubmitTask(c *gin.Context) {
	startedAt := time.Now()
	var req mangouAgentTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	req.Type = strings.TrimSpace(strings.ToLower(req.Type))
	req.Provider = strings.TrimSpace(strings.ToLower(req.Provider))
	req.Model = strings.TrimSpace(req.Model)
	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.Params == nil {
		req.Params = map[string]any{}
	}
	if req.Type != "image" && req.Type != "video" {
		common.ApiErrorMsg(c, "type must be image or video")
		return
	}
	if req.Provider == "" || req.Model == "" || req.Prompt == "" {
		common.ApiErrorMsg(c, "provider, model and prompt are required")
		return
	}
	if err := normalizeMangouAgentTaskRequest(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	pricingParams := buildMangouPricingParams(req)
	quota, ratios, err := calculateMangouTaskQuota(req.Provider, req.Type, pricingParams)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	userID := c.GetInt("id")
	if userID <= 0 {
		common.ApiErrorMsg(c, "authenticated user is required")
		return
	}
	usingGroup := mangouAgentUsingGroup(c, req.Provider)
	channelID := ensureMangouAgentProviderChannel(req.Provider, req.Type, req.Model, usingGroup)
	if err := preConsumeMangouAgentQuota(c, userID, quota); err != nil {
		common.ApiError(c, err)
		return
	}

	taskID := model.GenerateTaskID()
	now := time.Now().Unix()
	originModel := mangouImageOriginModel
	if req.Type == "video" {
		originModel = mangouVideoOriginModel
	}
	data := gin.H{
		"type":      req.Type,
		"provider":  req.Provider,
		"model":     req.Model,
		"prompt":    req.Prompt,
		"params":    req.Params,
		"quota":     quota,
		"submitted": now,
	}
	dataBytes, err := common.Marshal(data)
	if err != nil {
		refundMangouAgentQuota(c, userID, quota)
		common.ApiError(c, err)
		return
	}

	task := &model.Task{
		CreatedAt:  now,
		UpdatedAt:  now,
		TaskID:     taskID,
		Platform:   constant.TaskPlatform(req.Provider),
		UserId:     userID,
		Group:      usingGroup,
		ChannelId:  channelID,
		Quota:      quota,
		Action:     req.Type + ".generate",
		Status:     model.TaskStatusSubmitted,
		SubmitTime: now,
		Progress:   "10%",
		Properties: model.Properties{
			Input:             req.Prompt,
			UpstreamModelName: req.Model,
			OriginModelName:   originModel,
		},
		PrivateData: model.TaskPrivateData{
			TokenId: c.GetInt("token_id"),
			BillingContext: &model.TaskBillingContext{
				ModelPrice:      float64(quota),
				GroupRatio:      1,
				OtherRatios:     ratios,
				OriginModelName: originModel,
				PerCallBilling:  true,
			},
		},
		Data: dataBytes,
	}
	if upstream, attempted, err := submitMangouUpstreamTask(req); err != nil {
		refundMangouAgentQuota(c, userID, quota)
		common.ApiError(c, err)
		return
	} else if attempted {
		task.PrivateData.UpstreamTaskID = upstream.ID
		if upstream.Status != "" {
			task.Status = upstream.Status
		}
		if upstream.Progress != "" {
			task.Progress = upstream.Progress
		}
		if upstream.ResultURL != "" {
			task.PrivateData.ResultURL = upstream.ResultURL
		}
		task.SetData(gin.H{
			"request":  data,
			"upstream": mangouRawJSONValue(upstream.Raw),
		})
	}
	if err := task.Insert(); err != nil {
		refundMangouAgentQuota(c, userID, quota)
		common.ApiError(c, err)
		return
	}

	recordMangouAgentTaskConsumeLog(c, task, req, quota, startedAt)
	common.ApiSuccess(c, gin.H{
		"status":          "submitted",
		"task_id":         task.TaskID,
		"provider":        req.Provider,
		"type":            req.Type,
		"model":           req.Model,
		"estimated_quota": quota,
		"poll_url":        "/v1/agent/tasks/" + task.TaskID,
	})
}

func recordMangouAgentTaskConsumeLog(c *gin.Context, task *model.Task, req mangouAgentTaskRequest, quota int, startedAt time.Time) {
	other := map[string]interface{}{
		"is_task":        true,
		"task_id":        task.TaskID,
		"provider":       req.Provider,
		"task_type":      req.Type,
		"request_path":   c.Request.URL.Path,
		"pricing_params": buildMangouPricingParams(req),
	}
	model.RecordTaskBillingLog(model.RecordTaskBillingLogParams{
		UserId:           task.UserId,
		LogType:          model.LogTypeConsume,
		Content:          fmt.Sprintf("Mangou %s task submitted", req.Type),
		ChannelId:        task.ChannelId,
		ModelName:        req.Model,
		Quota:            quota,
		PromptTokens:     1,
		CompletionTokens: 1,
		TokenId:          task.PrivateData.TokenId,
		UseTimeSeconds:   int(time.Since(startedAt).Seconds()),
		Group:            task.Group,
		Ip:               c.ClientIP(),
		Other:            other,
	})
	model.UpdateUserUsedQuotaAndRequestCount(task.UserId, quota)
	if task.ChannelId > 0 {
		model.UpdateChannelUsedQuota(task.ChannelId, quota)
	}
}

func mangouAgentUsingGroup(c *gin.Context, fallback string) string {
	group := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	if group == "" {
		group = strings.TrimSpace(c.GetString("group"))
	}
	if group == "" {
		group = fallback
	}
	return group
}

func ensureMangouAgentProviderChannel(provider string, taskType string, modelName string, group string) int {
	if model.DB == nil {
		return 0
	}
	provider = strings.TrimSpace(strings.ToLower(provider))
	modelName = strings.TrimSpace(modelName)
	group = strings.TrimSpace(group)
	if provider == "" || modelName == "" || group == "" {
		return 0
	}

	name := fmt.Sprintf("Mangou %s %s", strings.ToUpper(provider), taskType)
	var channel model.Channel
	err := model.DB.Where("name = ?", name).First(&channel).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		common.SysLog("failed to lookup mangou provider channel: " + err.Error())
		return 0
	}

	if errors.Is(err, gorm.ErrRecordNotFound) {
		baseURL := mangouProviderBaseURL(provider)
		tag := "mangou:" + provider
		weight := uint(100)
		priority := int64(0)
		autoBan := 0
		channel = model.Channel{
			Type:        constant.ChannelTypeCustom,
			Key:         mangouProviderChannelKey(provider),
			Status:      common.ChannelStatusEnabled,
			Name:        name,
			Weight:      &weight,
			CreatedTime: common.GetTimestamp(),
			TestTime:    common.GetTimestamp(),
			BaseURL:     &baseURL,
			Models:      modelName,
			Group:       group,
			Priority:    &priority,
			AutoBan:     &autoBan,
			Tag:         &tag,
			Remark:      mangouStringPtr("Managed by Mangou Agent Gateway"),
		}
		if createErr := model.DB.Create(&channel).Error; createErr != nil {
			common.SysLog("failed to create mangou provider channel: " + createErr.Error())
			return 0
		}
		if abilityErr := channel.AddAbilities(nil); abilityErr != nil {
			common.SysLog("failed to create mangou provider channel abilities: " + abilityErr.Error())
		}
		model.InitChannelCache()
		return channel.Id
	}

	changed := false
	if !mangouCSVContains(channel.Models, modelName) {
		channel.Models = mangouAppendCSV(channel.Models, modelName)
		changed = true
	}
	if !mangouCSVContains(channel.Group, group) {
		channel.Group = mangouAppendCSV(channel.Group, group)
		changed = true
	}
	if changed {
		if saveErr := model.DB.Model(&channel).Select("models", "group").Updates(&channel).Error; saveErr != nil {
			common.SysLog("failed to update mangou provider channel: " + saveErr.Error())
		} else if abilityErr := channel.UpdateAbilities(nil); abilityErr != nil {
			common.SysLog("failed to update mangou provider channel abilities: " + abilityErr.Error())
		} else {
			model.InitChannelCache()
		}
	}
	return channel.Id
}

func mangouProviderChannelKey(provider string) string {
	if key := mangouProviderAPIKey(provider); key != "" {
		return key
	}
	return "managed-by-mangou-agent-gateway"
}

func mangouProviderBaseURL(provider string) string {
	switch provider {
	case "bltai":
		return strings.TrimRight(common.GetEnvOrDefaultString("BLTAI_BASE_URL", "https://api.bltcy.ai/v1"), "/")
	case "evolink":
		return strings.TrimRight(common.GetEnvOrDefaultString("EVOLINK_BASE_URL", ""), "/")
	case "kie":
		return strings.TrimRight(common.GetEnvOrDefaultString("KIE_BASE_URL", "https://api.kie.ai"), "/")
	default:
		return ""
	}
}

func mangouCSVContains(csv string, value string) bool {
	for _, item := range strings.Split(csv, ",") {
		if strings.TrimSpace(item) == value {
			return true
		}
	}
	return false
}

func mangouAppendCSV(csv string, value string) string {
	if strings.TrimSpace(csv) == "" {
		return value
	}
	return strings.TrimRight(csv, ",") + "," + value
}

func mangouStringPtr(value string) *string {
	return &value
}

func MangouAgentGetTask(c *gin.Context) {
	userID := c.GetInt("id")
	taskID := c.Param("task_id")
	task, exists, err := model.GetByTaskId(userID, taskID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !exists {
		common.ApiErrorMsg(c, "task not found")
		return
	}
	if task.PrivateData.UpstreamTaskID != "" {
		upstream, err := pollMangouUpstreamTask(task)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		changed := false
		if upstream.Status != "" && task.Status != upstream.Status {
			task.Status = upstream.Status
			changed = true
		}
		if upstream.Progress != "" && task.Progress != upstream.Progress {
			task.Progress = upstream.Progress
			changed = true
		}
		if upstream.ResultURL != "" && task.PrivateData.ResultURL != upstream.ResultURL {
			task.PrivateData.ResultURL = upstream.ResultURL
			changed = true
		}
		if len(upstream.Raw) > 0 {
			var existing any
			_ = common.Unmarshal(task.Data, &existing)
			task.SetData(gin.H{
				"request":  existing,
				"upstream": mangouRawJSONValue(upstream.Raw),
			})
			changed = true
		}
		if changed {
			task.UpdatedAt = time.Now().Unix()
			if err := task.Update(); err != nil {
				common.ApiError(c, err)
				return
			}
		}
	}
	common.ApiSuccess(c, gin.H{
		"task_id":    task.TaskID,
		"provider":   string(task.Platform),
		"type":       strings.TrimSuffix(task.Action, ".generate"),
		"model":      task.Properties.UpstreamModelName,
		"status":     mangouTaskPublicStatus(task.Status),
		"progress":   task.Progress,
		"quota":      task.Quota,
		"result_url": task.GetResultURL(),
	})
}

func normalizeMangouRechargeAmount(req mangouAgentRechargeRequest) (int64, string, error) {
	tier := strings.TrimSpace(req.Tier)
	if tier == "" {
		tier = "gems_100"
	}
	if req.Amount > 0 {
		return req.Amount, tier, nil
	}
	switch tier {
	case "gems_10":
		return 10, tier, nil
	case "gems_100":
		return 100, tier, nil
	case "gems_1000":
		return 1000, tier, nil
	default:
		return 0, tier, fmt.Errorf("unknown recharge tier: %s", tier)
	}
}

func mangouPublicBaseURL(c *gin.Context) string {
	requestBase := mangouRequestBaseURL(c)
	configuredBase := strings.TrimRight(system_setting.ServerAddress, "/")
	if configuredBase != "" && !mangouLooksLocalBaseURL(configuredBase) {
		return configuredBase
	}
	if requestBase != "" {
		return requestBase
	}
	return configuredBase
}

func mangouRequestBaseURL(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	host := c.Request.Host
	if host == "" {
		host = c.GetHeader("Host")
	}
	if host == "" {
		return ""
	}
	proto := c.GetHeader("X-Forwarded-Proto")
	if proto == "" {
		proto = "https"
	}
	return strings.TrimRight(proto+"://"+host, "/")
}

func mangouLooksLocalBaseURL(baseURL string) bool {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func buildMangouDemoPaymentURL(baseURL string, paymentID string, returnURL string) string {
	u := strings.TrimRight(baseURL, "/") + "/v1/payments/demo-scan/" + url.PathEscape(paymentID)
	if strings.TrimSpace(returnURL) == "" {
		return u
	}
	values := url.Values{}
	values.Set("returnUrl", returnURL)
	return u + "?" + values.Encode()
}

func completeMangouDemoPayment(paymentID string, callerIP string) (*model.TopUp, *model.User, error) {
	if paymentID == "" {
		return nil, nil, errors.New("payment_id is required")
	}
	refCol := "`trade_no`"
	if common.UsingPostgreSQL {
		refCol = `"trade_no"`
	}
	var completed model.TopUp
	var user model.User
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Set("gorm:query_option", "FOR UPDATE").Where(refCol+" = ?", paymentID).First(&completed).Error; err != nil {
			return errors.New("payment not found")
		}
		if completed.PaymentProvider != mangouDemoPaymentProvider {
			return errors.New("payment provider mismatch")
		}
		if completed.Status == common.TopUpStatusSuccess {
			return tx.First(&user, completed.UserId).Error
		}
		if completed.Status != common.TopUpStatusPending {
			return errors.New("payment is not pending")
		}
		if completed.Amount <= 0 {
			return errors.New("invalid payment amount")
		}
		completed.Status = common.TopUpStatusSuccess
		completed.CompleteTime = common.GetTimestamp()
		if err := tx.Save(&completed).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.User{}).Where("id = ?", completed.UserId).Update("quota", gorm.Expr("quota + ?", int(completed.Amount))).Error; err != nil {
			return err
		}
		return tx.First(&user, completed.UserId).Error
	})
	if err != nil {
		return nil, nil, err
	}
	model.RecordTopupLog(completed.UserId, fmt.Sprintf("Mangou demo payment complete, credited %d credits", completed.Amount), callerIP, completed.PaymentMethod, mangouDemoPaymentProvider)
	return &completed, &user, nil
}

func mangouQRCodeSVG(target string) (string, error) {
	code, err := qr.Encode(target, qr.M, qr.Auto)
	if err != nil {
		return "", err
	}
	scaled, err := barcode.Scale(code, 256, 256)
	if err != nil {
		return "", err
	}
	bounds := scaled.Bounds()
	var b strings.Builder
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 256 256" width="256" height="256" shape-rendering="crispEdges">`)
	b.WriteString(`<rect width="256" height="256" fill="#fff"/>`)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, bl, _ := scaled.At(x, y).RGBA()
			if r+g+bl < 0x18000 {
				b.WriteString(`<rect x="`)
				b.WriteString(strconv.Itoa(x))
				b.WriteString(`" y="`)
				b.WriteString(strconv.Itoa(y))
				b.WriteString(`" width="1" height="1" fill="#000"/>`)
			}
		}
	}
	b.WriteString(`</svg>`)
	return b.String(), nil
}

func submitMangouUpstreamTask(req mangouAgentTaskRequest) (*mangouUpstreamTaskResult, bool, error) {
	switch req.Provider {
	case "evolink", "bltai":
		key := mangouProviderAPIKey(req.Provider)
		if key == "" {
			return nil, false, nil
		}
		return submitMangouUnifiedTask(req, key)
	case "kie":
		key := mangouProviderAPIKey(req.Provider)
		if key == "" {
			return nil, false, nil
		}
		return submitMangouKIERunwayTask(req, key)
	default:
		return nil, false, nil
	}
}

func pollMangouUpstreamTask(task *model.Task) (*mangouUpstreamTaskResult, error) {
	provider := string(task.Platform)
	switch provider {
	case "evolink", "bltai":
		key := mangouProviderAPIKey(provider)
		if key == "" {
			return &mangouUpstreamTaskResult{}, nil
		}
		return pollMangouUnifiedTask(provider, task.PrivateData.UpstreamTaskID, key)
	case "kie":
		key := mangouProviderAPIKey(provider)
		if key == "" {
			return &mangouUpstreamTaskResult{}, nil
		}
		return pollMangouKIERunwayTask(task.PrivateData.UpstreamTaskID, key)
	default:
		return &mangouUpstreamTaskResult{}, nil
	}
}

func submitMangouUnifiedTask(req mangouAgentTaskRequest, key string) (*mangouUpstreamTaskResult, bool, error) {
	payload := map[string]any{}
	for k, v := range req.Params {
		payload[k] = v
	}
	payload["model"] = req.Model
	payload["prompt"] = req.Prompt

	path := "/v1/images/generations"
	if req.Type == "video" {
		path = "/v1/videos/generations"
	}
	body, err := common.Marshal(payload)
	if err != nil {
		return nil, true, err
	}
	respBody, err := doMangouProviderJSON(http.MethodPost, mangouUnifiedURL(req.Provider, path), key, body)
	if err != nil {
		return nil, true, err
	}
	var parsed mangouUnifiedTaskResponse
	if err := common.Unmarshal(respBody, &parsed); err != nil {
		return nil, true, err
	}
	resultURL := firstMangouUnifiedResultURL(parsed)
	if parsed.ID == "" && resultURL == "" {
		return nil, true, fmt.Errorf("%s upstream response missing task id", req.Provider)
	}
	status := mapMangouUnifiedStatus(parsed.Status)
	progress := mapMangouProgress(parsed.Progress)
	if parsed.ID == "" && resultURL != "" {
		status = model.TaskStatusSuccess
		progress = "100%"
	}
	return &mangouUpstreamTaskResult{
		ID:        parsed.ID,
		Status:    status,
		Progress:  progress,
		ResultURL: resultURL,
		Raw:       respBody,
	}, true, nil
}

func pollMangouUnifiedTask(provider string, upstreamTaskID string, key string) (*mangouUpstreamTaskResult, error) {
	respBody, err := doMangouProviderJSON(http.MethodGet, mangouUnifiedURL(provider, "/v1/tasks/"+upstreamTaskID), key, nil)
	if err != nil {
		return nil, err
	}
	var parsed mangouUnifiedTaskResponse
	if err := common.Unmarshal(respBody, &parsed); err != nil {
		return nil, err
	}
	resultURL := firstMangouUnifiedResultURL(parsed)
	if parsed.Status == "failed" && len(parsed.Error) > 0 {
		if msg, ok := parsed.Error["message"].(string); ok && msg != "" {
			resultURL = msg
		}
	}
	return &mangouUpstreamTaskResult{
		ID:        parsed.ID,
		Status:    mapMangouUnifiedStatus(parsed.Status),
		Progress:  mapMangouProgress(parsed.Progress),
		ResultURL: resultURL,
		Raw:       respBody,
	}, nil
}

func firstMangouUnifiedResultURL(parsed mangouUnifiedTaskResponse) string {
	if len(parsed.Results) > 0 {
		return parsed.Results[0]
	}
	for _, item := range parsed.Data {
		if item.URL != "" {
			return item.URL
		}
		if item.B64JSON != "" {
			return "data:image/png;base64," + item.B64JSON
		}
	}
	return ""
}

func submitMangouKIERunwayTask(req mangouAgentTaskRequest, key string) (*mangouUpstreamTaskResult, bool, error) {
	if req.Type != "video" {
		return nil, true, errors.New("kie runtime currently supports video tasks")
	}
	payload := map[string]any{}
	for k, v := range req.Params {
		payload[k] = v
	}
	payload["prompt"] = req.Prompt
	body, err := common.Marshal(payload)
	if err != nil {
		return nil, true, err
	}
	respBody, err := doMangouProviderJSON(http.MethodPost, mangouKIEURL("/api/v1/runway/generate"), key, body)
	if err != nil {
		return nil, true, err
	}
	var parsed mangouKIERunwaySubmitResponse
	if err := common.Unmarshal(respBody, &parsed); err != nil {
		return nil, true, err
	}
	if parsed.Code != 0 && parsed.Code != 200 {
		if strings.Contains(strings.ToLower(parsed.Msg), "quality") {
			return nil, true, errors.New(`kie submit failed: params.quality is required for KIE video tasks; set params.quality to a supported value such as "720p"`)
		}
		return nil, true, fmt.Errorf("kie submit failed: %s", parsed.Msg)
	}
	if parsed.Data.TaskID == "" {
		return nil, true, errors.New("kie upstream response missing task id")
	}
	return &mangouUpstreamTaskResult{
		ID:       parsed.Data.TaskID,
		Status:   model.TaskStatusSubmitted,
		Progress: "10%",
		Raw:      respBody,
	}, true, nil
}

func pollMangouKIERunwayTask(upstreamTaskID string, key string) (*mangouUpstreamTaskResult, error) {
	respBody, err := doMangouProviderJSON(http.MethodGet, mangouKIEURL("/api/v1/runway/record-detail?taskId="+upstreamTaskID), key, nil)
	if err != nil {
		return nil, err
	}
	var parsed mangouKIERunwayRecordResponse
	if err := common.Unmarshal(respBody, &parsed); err != nil {
		return nil, err
	}
	if parsed.Code != 0 && parsed.Code != 200 {
		return nil, fmt.Errorf("kie task query failed: %s", parsed.Msg)
	}
	status, progress := mapMangouKIEState(parsed.Data.State)
	resultURL := parsed.Data.VideoInfo.VideoURL
	if resultURL == "" {
		resultURL = parsed.Data.FailMsg
	}
	return &mangouUpstreamTaskResult{
		ID:        parsed.Data.TaskID,
		Status:    status,
		Progress:  progress,
		ResultURL: resultURL,
		Raw:       respBody,
	}, nil
}

func doMangouProviderJSON(method string, target string, key string, body []byte) ([]byte, error) {
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, target, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, readErr
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("provider request failed: status=%d body=%s", resp.StatusCode, string(respBody))
	}
	return respBody, nil
}

func mangouRawJSONValue(raw []byte) any {
	var parsed any
	if len(raw) > 0 && common.Unmarshal(raw, &parsed) == nil {
		return parsed
	}
	return string(raw)
}

func mangouProviderAPIKey(provider string) string {
	switch provider {
	case "bltai":
		return strings.TrimSpace(common.GetEnvOrDefaultString("BLTAI_API_KEY", ""))
	case "evolink":
		return strings.TrimSpace(common.GetEnvOrDefaultString("EVOLINK_API_KEY", ""))
	case "kie":
		return strings.TrimSpace(common.GetEnvOrDefaultString("KIE_API_KEY", ""))
	default:
		return ""
	}
}

func mangouUnifiedURL(provider string, path string) string {
	base := ""
	switch provider {
	case "bltai":
		base = common.GetEnvOrDefaultString("BLTAI_BASE_URL", "")
		if base == "" {
			base = "https://api.bltcy.ai/v1"
		}
	case "evolink":
		base = common.GetEnvOrDefaultString("EVOLINK_BASE_URL", "")
		if base == "" {
			base = "https://api.evolink.ai"
		}
	}
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, "/v1") && strings.HasPrefix(path, "/v1/") {
		path = strings.TrimPrefix(path, "/v1")
	}
	return base + path
}

func mangouKIEURL(path string) string {
	base := strings.TrimRight(common.GetEnvOrDefaultString("KIE_BASE_URL", ""), "/")
	if base == "" {
		base = "https://api.kie.ai"
	}
	return base + path
}

func mapMangouUnifiedStatus(status string) model.TaskStatus {
	switch strings.ToLower(status) {
	case "completed", "success", "succeeded":
		return model.TaskStatusSuccess
	case "failed", "failure", "error":
		return model.TaskStatusFailure
	case "processing", "running", "in_progress":
		return model.TaskStatusInProgress
	case "pending", "queued", "submitted":
		return model.TaskStatusSubmitted
	default:
		return ""
	}
}

func mapMangouKIEState(state string) (model.TaskStatus, string) {
	switch strings.ToLower(state) {
	case "success", "completed", "succeeded":
		return model.TaskStatusSuccess, "100%"
	case "fail", "failed", "failure", "error":
		return model.TaskStatusFailure, "100%"
	case "processing", "running", "in_progress":
		return model.TaskStatusInProgress, "50%"
	case "waiting", "pending", "queued", "submitted":
		return model.TaskStatusSubmitted, "10%"
	default:
		return "", ""
	}
}

func mapMangouProgress(progress int) string {
	if progress <= 0 {
		return ""
	}
	if progress > 100 {
		progress = 100
	}
	return strconv.Itoa(progress) + "%"
}

func mangouTaskPublicStatus(status model.TaskStatus) string {
	switch status {
	case model.TaskStatusSuccess:
		return "completed"
	case model.TaskStatusFailure:
		return "failed"
	case model.TaskStatusInProgress:
		return "processing"
	case model.TaskStatusSubmitted, model.TaskStatusQueued, model.TaskStatusNotStart:
		return "submitted"
	default:
		return strings.ToLower(string(status))
	}
}

func MangouAgentListTasks(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	userID := c.GetInt("id")
	queryParams := model.SyncTaskQueryParams{
		Platform: constant.TaskPlatform(strings.TrimSpace(strings.ToLower(c.Query("provider")))),
		TaskID:   c.Query("task_id"),
		Status:   c.Query("status"),
		Action:   c.Query("type"),
	}
	if queryParams.Action == "image" || queryParams.Action == "video" {
		queryParams.Action += ".generate"
	}
	items := model.TaskGetAllUserTask(userID, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), queryParams)
	total := model.TaskCountAllUserTask(userID, queryParams)
	rows := make([]gin.H, 0, len(items))
	for _, task := range items {
		rows = append(rows, gin.H{
			"task_id":    task.TaskID,
			"provider":   string(task.Platform),
			"type":       strings.TrimSuffix(task.Action, ".generate"),
			"model":      task.Properties.UpstreamModelName,
			"status":     mangouTaskPublicStatus(task.Status),
			"progress":   task.Progress,
			"quota":      task.Quota,
			"result_url": task.GetResultURL(),
		})
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(rows)
	common.ApiSuccess(c, pageInfo)
}

func loadMangouProviderPricingRule(provider string, taskType string) (mangouProviderPricingRule, error) {
	pricing, exists, err := model.GetMangouProviderPricing(provider, taskType)
	if err != nil {
		return mangouProviderPricingRule{}, err
	}
	if !exists {
		return mangouProviderPricingRule{}, fmt.Errorf("pricing for provider %s type %s is not configured", provider, taskType)
	}
	multipliers, err := pricing.GetMultipliers()
	if err != nil {
		return mangouProviderPricingRule{}, fmt.Errorf("invalid pricing multipliers for provider %s type %s: %w", provider, taskType, err)
	}
	return mangouProviderPricingRule{
		BaseQuota:   pricing.BaseQuota,
		Multipliers: multipliers,
	}, nil
}

func calculateMangouTaskQuota(provider string, taskType string, params map[string]any) (int, map[string]float64, error) {
	rule, err := loadMangouProviderPricingRule(provider, taskType)
	if err != nil {
		return 0, nil, err
	}
	if rule.BaseQuota <= 0 {
		return 0, nil, fmt.Errorf("base_quota for provider %s type %s must be positive", provider, taskType)
	}

	total := float64(rule.BaseQuota)
	ratios := map[string]float64{"base_quota": float64(rule.BaseQuota)}
	hasPricingTier := false
	if _, ok := rule.Multipliers["pricing_tier"]; ok {
		_, hasPricingTier = params["pricing_tier"]
	}
	for paramName, values := range rule.Multipliers {
		if len(values) == 0 {
			continue
		}
		if hasPricingTier && (paramName == "model" || paramName == "quality" || paramName == "resolution" || paramName == "input_mode" || paramName == "image_size" || paramName == "size") {
			continue
		}
		raw, exists := params[paramName]
		if !exists {
			continue
		}
		valueKey := mangouPricingValueKey(raw)
		multiplier, exists := values[valueKey]
		if !exists {
			return 0, nil, fmt.Errorf("pricing multiplier for %s=%s is not configured", paramName, valueKey)
		}
		total *= multiplier
		ratios[paramName] = multiplier
	}
	return int(total + 0.5), ratios, nil
}

func buildMangouPricingParams(req mangouAgentTaskRequest) map[string]any {
	params := make(map[string]any, len(req.Params)+4)
	for key, value := range req.Params {
		params[key] = value
	}
	params["model"] = req.Model

	if _, exists := params["quality"]; !exists {
		if resolution, ok := params["resolution"]; ok {
			params["quality"] = resolution
		}
	}
	if raw, exists := params["duration"]; exists {
		if normalized, ok := normalizeMangouDuration(raw); ok {
			params["duration"] = normalized
		}
	}

	if req.Type == "image" {
		quality := "medium"
		if raw, exists := params["quality"]; exists && strings.TrimSpace(mangouPricingValueKey(raw)) != "" {
			quality = strings.TrimSpace(strings.ToLower(mangouPricingValueKey(raw)))
		}
		size := normalizeMangouImageSize(params["image_size"], params["size"], params["aspect_ratio"])
		params["quality"] = quality
		params["image_size"] = size
		params["pricing_tier"] = req.Model + "|" + quality + "|" + size
	}

	if req.Type == "video" {
		inputMode := "no_video_input"
		if mangouPricingHasNonEmptyList(params["video_urls"]) || mangouPricingHasNonEmptyList(params["videos"]) || strings.TrimSpace(mangouPricingValueKey(params["video_url"])) != "" {
			inputMode = "with_video_input"
		}
		params["input_mode"] = inputMode
		quality := "720p"
		if raw, exists := params["quality"]; exists && strings.TrimSpace(mangouPricingValueKey(raw)) != "" {
			quality = strings.TrimSpace(mangouPricingValueKey(raw))
		}
		params["pricing_tier"] = req.Model + "|" + inputMode + "|" + quality
	}

	return params
}

func normalizeMangouAgentTaskRequest(req *mangouAgentTaskRequest) error {
	if req.Params == nil {
		req.Params = map[string]any{}
	}
	switch req.Type {
	case "image":
		quality := "medium"
		if raw, exists := req.Params["quality"]; exists && strings.TrimSpace(mangouPricingValueKey(raw)) != "" {
			quality = strings.TrimSpace(strings.ToLower(mangouPricingValueKey(raw)))
		}
		req.Params["quality"] = quality
		req.Params["image_size"] = normalizeMangouImageSize(req.Params["image_size"], req.Params["size"], req.Params["aspect_ratio"])
	case "video":
		if req.Provider == "kie" {
			quality := strings.TrimSpace(mangouPricingValueKey(req.Params["quality"]))
			if quality == "" {
				quality = strings.TrimSpace(mangouPricingValueKey(req.Params["resolution"]))
			}
			if quality == "" {
				return errors.New(`params.quality is required for KIE video tasks; set params.quality to a supported value such as "720p"`)
			}
			req.Params["quality"] = quality
		}
	}
	return nil
}

func normalizeMangouImageSize(values ...any) string {
	for _, value := range values {
		raw := strings.TrimSpace(mangouPricingValueKey(value))
		if raw == "" {
			continue
		}
		key := strings.ReplaceAll(strings.ToLower(raw), " ", "")
		switch key {
		case "1024x1024", "1k", "square", "1:1":
			return "1024x1024"
		case "1536x1024", "landscape", "3:2", "16:9":
			return "1536x1024"
		case "1024x1536", "portrait", "2:3", "9:16":
			return "1024x1536"
		default:
			return strings.ReplaceAll(raw, " ", "")
		}
	}
	return "1024x1024"
}

func normalizeMangouDuration(value any) (string, bool) {
	key := strings.TrimSpace(mangouPricingValueKey(value))
	if key == "" {
		return "", false
	}
	key = strings.TrimSuffix(strings.ToLower(key), "s")
	key = strings.TrimSpace(key)
	if key == "" {
		return "", false
	}
	return key, true
}

func mangouPricingHasNonEmptyList(value any) bool {
	switch v := value.(type) {
	case []any:
		return len(v) > 0
	case []string:
		return len(v) > 0
	default:
		return false
	}
}

func mangouPricingValueKey(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", value))
	}
}

func validateMangouAgentEmail(email string) error {
	if err := common.Validate.Var(email, "required,email"); err != nil {
		return errors.New("invalid email")
	}
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return errors.New("invalid email")
	}
	localPart := parts[0]
	domainPart := parts[1]
	if common.EmailDomainRestrictionEnabled {
		allowed := false
		for _, domain := range common.EmailDomainWhitelist {
			if domainPart == domain {
				allowed = true
				break
			}
		}
		if !allowed {
			return errors.New("email domain is not allowed")
		}
	}
	if common.EmailAliasRestrictionEnabled && (strings.Contains(localPart, "+") || strings.Contains(localPart, ".")) {
		return errors.New("email alias is not allowed")
	}
	return nil
}

func findOrCreateMangouAgentUser(email string) (*model.User, error) {
	var user model.User
	err := model.DB.Where("email = ?", email).First(&user).Error
	if err == nil {
		if err := ensureMangouAgentUserGroup(&user); err != nil {
			return nil, err
		}
		return &user, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	password, err := common.GenerateRandomCharsKey(16)
	if err != nil {
		return nil, err
	}
	user = model.User{
		Username:    "agent_" + common.GetRandomString(12),
		Password:    password,
		DisplayName: "Mangou Agent",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Email:       email,
		Group:       mangouAgentDefaultGroup,
	}
	if err := user.Insert(0); err != nil {
		return nil, err
	}
	return &user, nil
}

func ensureMangouAgentUserGroup(user *model.User) error {
	if user.Group == mangouAgentDefaultGroup {
		return nil
	}
	if err := model.DB.Model(&model.User{}).Where("id = ?", user.Id).Update("group", mangouAgentDefaultGroup).Error; err != nil {
		return err
	}
	user.Group = mangouAgentDefaultGroup
	return model.UpdateUserGroupCache(user.Id, mangouAgentDefaultGroup)
}

func findOrCreateMangouAgentToken(userID int, agentID string) (*model.Token, bool, error) {
	tokenName := mangouAgentTokenPrefix + agentID
	var token model.Token
	err := model.DB.Where("user_id = ? AND name = ?", userID, tokenName).First(&token).Error
	if err == nil {
		if token.Group != mangouAgentDefaultGroup {
			token.Group = mangouAgentDefaultGroup
			token.CrossGroupRetry = true
			if err := token.Update(); err != nil {
				return nil, false, err
			}
		}
		return &token, false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}

	key, err := common.GenerateKey()
	if err != nil {
		return nil, false, err
	}
	token = model.Token{
		UserId:             userID,
		Name:               tokenName,
		Key:                key,
		Status:             common.TokenStatusEnabled,
		CreatedTime:        common.GetTimestamp(),
		AccessedTime:       common.GetTimestamp(),
		ExpiredTime:        -1,
		RemainQuota:        0,
		UnlimitedQuota:     true,
		ModelLimitsEnabled: false,
		Group:              mangouAgentDefaultGroup,
		CrossGroupRetry:    true,
	}
	if err := token.Insert(); err != nil {
		return nil, false, err
	}
	return &token, true, nil
}

func preConsumeMangouAgentQuota(c *gin.Context, userID int, quota int) error {
	if quota <= 0 {
		return nil
	}
	userQuota, err := model.GetUserQuota(userID, false)
	if err != nil {
		return err
	}
	if userQuota < quota {
		return fmt.Errorf("user quota is not enough, user quota: %d, need quota: %d", userQuota, quota)
	}
	if !c.GetBool("token_unlimited_quota") {
		tokenQuota := c.GetInt("token_quota")
		if tokenQuota < quota {
			return fmt.Errorf("token quota is not enough, token quota: %d, need quota: %d", tokenQuota, quota)
		}
		if err := model.DecreaseTokenQuota(c.GetInt("token_id"), c.GetString("token_key"), quota); err != nil {
			return err
		}
	}
	return model.DecreaseUserQuota(userID, quota, false)
}

func refundMangouAgentQuota(c *gin.Context, userID int, quota int) {
	if quota <= 0 {
		return
	}
	if err := model.IncreaseUserQuota(userID, quota, false); err != nil {
		common.SysLog("failed to refund mangou agent user quota: " + err.Error())
	}
	if !c.GetBool("token_unlimited_quota") {
		if err := model.IncreaseTokenQuota(c.GetInt("token_id"), c.GetString("token_key"), quota); err != nil {
			common.SysLog("failed to refund mangou agent token quota: " + err.Error())
		}
	}
}
