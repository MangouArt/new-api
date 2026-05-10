package controller

import (
	"bytes"
	"errors"
	"fmt"
	"html"
	"image"
	"image/png"
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

var genMangouCreemCheckoutLink = genCreemLink

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

type mangouProviderChannelSpec struct {
	Provider    string
	TaskType    string
	Name        string
	BaseURL     string
	APIKey      string
	Models      []string
	Groups      []string
	Description string
}

type mangouProviderSyncResult struct {
	Provider string   `json:"provider"`
	Type     string   `json:"type"`
	Channel  string   `json:"channel"`
	Models   []string `json:"models"`
	Groups   []string `json:"groups"`
	Status   string   `json:"status"`
	Message  string   `json:"message,omitempty"`
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

## Balance And Recharge

Check balance:

`+"```bash"+`
curl -sS "`+strings.TrimSuffix(system_setting.ServerAddress, "/")+`/v1/agent/balance" \
  -H "Authorization: Bearer ${BILLING_TOKEN}"
`+"```"+`

`+"`/v1/agent/credits`"+` is an alias for `+"`/v1/agent/balance`"+`.

Request a recharge QR:

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
- `+"`qr_png_url`"+`
- `+"`payment_url`"+`
- `+"`amount`"+`
- `+"`currency`"+`
- `+"`provider`"+` when a real payment provider is used
- `+"`demo`"+`, where `+"`false`"+` means `+"`payment_url`"+` is a real checkout page and `+"`true`"+` means the demo scan flow is active

Validation flow:

1. In chat channels, send `+"`qr_png_url`"+` as an image attachment whenever image upload is available. Do not only paste JSON or a raw URL.
2. If PNG upload is unavailable, `+"`GET qr_url`"+` and verify the response `+"`Content-Type`"+` contains `+"`image/svg+xml`"+`, then render/show that QR code.
3. As a fallback, show `+"`payment_url`"+` as a clickable checkout link.
4. If `+"`demo`"+` is `+"`false`"+`, the user must complete the external checkout and the payment provider webhook will credit the account.
5. If `+"`demo`"+` is `+"`true`"+`, opening `+"`payment_url`"+` simulates payment completion.
6. Recheck `+"`/v1/agent/balance`"+` and confirm the balance increased.
7. Repeating the same payment callback must be idempotent and must not double-credit the account.

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

Provider and model routing must already be configured in NewAPI `+"`channels`"+`, `+"`abilities`"+`, and `+"`models`"+`. If the requested provider/model/group is not configured, the API returns `+"`success: false`"+` with a message naming the missing provider/model/group; do not guess or retry blindly.

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
	if isCreemWebhookEnabled() {
		if respondMangouAgentCreemRecharge(c, userID, agentID, paymentID, amount, tier, baseURL) {
			return
		}
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
		"qr_png_url":     baseURL + "/v1/payments/" + paymentID + "/qr.png",
		"payment_url":    paymentURL,
		"instructions":   "Show qr_url or payment_url to the user. Opening payment_url simulates a completed payment and credits the account.",
	})
}

func respondMangouAgentCreemRecharge(c *gin.Context, userID int, agentID string, paymentID string, amount int64, tier string, baseURL string) bool {
	product, err := selectMangouAgentCreemProduct(amount, tier)
	if err != nil {
		common.ApiError(c, err)
		return true
	}
	user, err := model.GetUserById(userID, false)
	if err != nil {
		common.ApiError(c, err)
		return true
	}
	topUp := &model.TopUp{
		UserId:          userID,
		Amount:          product.Quota,
		Money:           product.Price,
		TradeNo:         paymentID,
		PaymentMethod:   model.PaymentMethodCreem,
		PaymentProvider: model.PaymentProviderCreem,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}
	if err := topUp.Insert(); err != nil {
		common.ApiError(c, err)
		return true
	}
	checkoutURL, err := genMangouCreemCheckoutLink(c.Request.Context(), paymentID, product, user.Email, user.Username)
	if err != nil {
		topUp.Status = common.TopUpStatusFailed
		_ = topUp.Update()
		common.ApiError(c, fmt.Errorf("create Creem checkout failed: %w", err))
		return true
	}
	qrURL := buildMangouPaymentQRURL(baseURL, paymentID, checkoutURL)
	common.ApiSuccess(c, gin.H{
		"agent_id":       agentID,
		"user_id":        userID,
		"payment_id":     paymentID,
		"payment_status": topUp.Status,
		"tier":           tier,
		"amount":         product.Quota,
		"currency":       "credits",
		"demo":           false,
		"provider":       model.PaymentProviderCreem,
		"qr_url":         qrURL,
		"qr_png_url":     buildMangouPaymentQRPNGURL(baseURL, paymentID, checkoutURL),
		"payment_url":    checkoutURL,
		"instructions":   "Show qr_url or payment_url to the user. Creem webhook will credit the account after checkout.completed is paid.",
	})
	return true
}

func selectMangouAgentCreemProduct(amount int64, tier string) (*CreemProduct, error) {
	product, err := model.GetActivePaymentProduct(model.PaymentProviderCreem, amount, tier)
	if err != nil {
		return nil, fmt.Errorf("Creem payment product is not configured for tier=%s amount=%d credits", tier, amount)
	}
	return &CreemProduct{
		ProductId: product.ProductId,
		Name:      product.Name,
		Price:     product.Price,
		Currency:  product.Currency,
		Quota:     product.Quota,
	}, nil
}

func MangouPaymentQRSVG(c *gin.Context) {
	mangouPaymentQR(c, "svg")
}

func MangouPaymentQRPNG(c *gin.Context) {
	mangouPaymentQR(c, "png")
}

func mangouPaymentQR(c *gin.Context, format string) {
	paymentID := strings.TrimSpace(c.Param("payment_id"))
	topUp := model.GetTopUpByTradeNo(paymentID)
	if topUp == nil {
		c.String(http.StatusNotFound, "payment not found")
		return
	}
	var paymentURL string
	switch topUp.PaymentProvider {
	case mangouDemoPaymentProvider:
		paymentURL = buildMangouDemoPaymentURL(mangouPublicBaseURL(c), paymentID, "")
	case model.PaymentProviderCreem:
		paymentURL = strings.TrimSpace(c.Query("target"))
		if paymentURL == "" || validateMangouCreemPaymentTarget(paymentURL) != nil {
			c.String(http.StatusBadRequest, "valid target is required")
			return
		}
	default:
		c.String(http.StatusNotFound, "payment not found")
		return
	}
	svg, err := mangouQRCodeSVG(paymentURL)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if format == "png" {
		pngBytes, err := mangouQRCodePNG(paymentURL)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		c.Data(http.StatusOK, "image/png", pngBytes)
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
	channel, err := resolveMangouAgentProviderChannel(req.Provider, req.Type, req.Model, usingGroup)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	channelID := channel.Id
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
	if upstream, attempted, err := submitMangouUpstreamTask(req, channel); err != nil {
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

func resolveMangouAgentProviderChannel(provider string, taskType string, modelName string, group string) (*model.Channel, error) {
	provider = strings.TrimSpace(strings.ToLower(provider))
	modelName = strings.TrimSpace(modelName)
	group = strings.TrimSpace(group)
	if provider == "" || modelName == "" || group == "" {
		return nil, errors.New("provider, model and group are required to resolve a NewAPI channel")
	}

	name := mangouProviderChannelName(provider, taskType)
	tag := "mangou:" + provider
	var ability model.Ability
	err := model.DB.Joins("JOIN channels ON channels.id = abilities.channel_id").
		Where(&model.Ability{Group: group, Model: modelName, Enabled: true}).
		Where("channels.status = ?", common.ChannelStatusEnabled).
		Where("(channels.tag = ? OR channels.name = ?)", tag, name).
		Order("abilities.priority DESC, abilities.weight DESC").
		First(&ability).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("provider/model is not configured in NewAPI channels: provider=%s type=%s model=%s group=%s; ask an admin to run POST /api/mangou/providers/sync or configure channels/models manually", provider, taskType, modelName, group)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to resolve NewAPI channel for provider=%s model=%s group=%s: %w", provider, modelName, group, err)
	}
	var channel model.Channel
	if err := model.DB.First(&channel, ability.ChannelId).Error; err != nil {
		return nil, fmt.Errorf("failed to load NewAPI channel %d for provider=%s model=%s group=%s: %w", ability.ChannelId, provider, modelName, group, err)
	}
	if strings.TrimSpace(channel.Key) == "" {
		return nil, fmt.Errorf("NewAPI channel %d (%s) has no provider key configured", channel.Id, channel.Name)
	}
	if channel.BaseURL == nil || strings.TrimSpace(*channel.BaseURL) == "" {
		return nil, fmt.Errorf("NewAPI channel %d (%s) has no base_url configured", channel.Id, channel.Name)
	}
	return &channel, nil
}

func MangouAdminSyncProviders(c *gin.Context) {
	results := syncMangouProviderChannels()
	created := 0
	updated := 0
	skipped := 0
	for _, result := range results {
		switch result.Status {
		case "created":
			created++
		case "updated", "unchanged":
			updated++
		default:
			skipped++
		}
	}
	common.ApiSuccess(c, gin.H{
		"created": created,
		"updated": updated,
		"skipped": skipped,
		"results": results,
	})
}

func syncMangouProviderChannels() []mangouProviderSyncResult {
	specs := defaultMangouProviderChannelSpecs()
	results := make([]mangouProviderSyncResult, 0, len(specs))
	for _, spec := range specs {
		results = append(results, syncMangouProviderChannel(spec))
	}
	model.InitChannelCache()
	return results
}

func syncMangouProviderChannel(spec mangouProviderChannelSpec) mangouProviderSyncResult {
	result := mangouProviderSyncResult{
		Provider: spec.Provider,
		Type:     spec.TaskType,
		Channel:  spec.Name,
		Models:   spec.Models,
		Groups:   spec.Groups,
	}
	if len(spec.Models) == 0 || len(spec.Groups) == 0 {
		result.Status = "skipped"
		result.Message = "models and groups are required"
		return result
	}

	var channel model.Channel
	err := model.DB.Where("name = ?", spec.Name).First(&channel).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		result.Status = "skipped"
		result.Message = err.Error()
		return result
	}

	key := strings.TrimSpace(spec.APIKey)
	if !errors.Is(err, gorm.ErrRecordNotFound) && strings.TrimSpace(channel.Key) != "" {
		key = strings.TrimSpace(channel.Key)
	}
	if key == "" {
		result.Status = "skipped"
		result.Message = "provider key is missing; configure the NewAPI channel key or set the bootstrap environment variable before sync"
		return result
	}

	baseURL := strings.TrimRight(strings.TrimSpace(spec.BaseURL), "/")
	if !errors.Is(err, gorm.ErrRecordNotFound) && channel.BaseURL != nil && strings.TrimSpace(*channel.BaseURL) != "" {
		baseURL = strings.TrimRight(strings.TrimSpace(*channel.BaseURL), "/")
	}
	if baseURL == "" {
		result.Status = "skipped"
		result.Message = "base_url is missing"
		return result
	}

	tag := "mangou:" + spec.Provider
	weight := uint(100)
	priority := int64(0)
	autoBan := 0
	modelsCSV := strings.Join(spec.Models, ",")
	groupsCSV := strings.Join(spec.Groups, ",")
	if errors.Is(err, gorm.ErrRecordNotFound) {
		channel = model.Channel{
			Type:        constant.ChannelTypeCustom,
			Key:         key,
			Status:      common.ChannelStatusEnabled,
			Name:        spec.Name,
			Weight:      &weight,
			CreatedTime: common.GetTimestamp(),
			TestTime:    common.GetTimestamp(),
			BaseURL:     &baseURL,
			Models:      modelsCSV,
			Group:       groupsCSV,
			Priority:    &priority,
			AutoBan:     &autoBan,
			Tag:         &tag,
			Remark:      mangouStringPtr(spec.Description),
		}
		if createErr := model.DB.Create(&channel).Error; createErr != nil {
			result.Status = "skipped"
			result.Message = createErr.Error()
			return result
		}
		result.Status = "created"
	} else {
		channel.Type = constant.ChannelTypeCustom
		channel.Status = common.ChannelStatusEnabled
		channel.Key = key
		channel.BaseURL = &baseURL
		channel.Models = modelsCSV
		channel.Group = groupsCSV
		channel.Weight = &weight
		channel.Priority = &priority
		channel.AutoBan = &autoBan
		channel.Tag = &tag
		channel.Remark = mangouStringPtr(spec.Description)
		if saveErr := model.DB.Model(&channel).Select("type", "status", "key", "base_url", "models", "group", "weight", "priority", "auto_ban", "tag", "remark").Updates(&channel).Error; saveErr != nil {
			result.Status = "skipped"
			result.Message = saveErr.Error()
			return result
		}
		result.Status = "updated"
	}
	if abilityErr := channel.UpdateAbilities(nil); abilityErr != nil {
		result.Status = "skipped"
		result.Message = abilityErr.Error()
		return result
	}
	for _, modelName := range spec.Models {
		if err := ensureMangouModelMeta(modelName, spec); err != nil {
			common.SysLog("failed to ensure mangou model meta: " + err.Error())
		}
	}
	return result
}

func ensureMangouModelMeta(modelName string, spec mangouProviderChannelSpec) error {
	var existing model.Model
	err := model.DB.Where("model_name = ?", modelName).First(&existing).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	vendorID, err := ensureMangouVendor(spec.Provider)
	if err != nil {
		return err
	}
	now := common.GetTimestamp()
	return model.DB.Create(&model.Model{
		ModelName:    modelName,
		Description:  spec.Description,
		Tags:         "mangou," + spec.Provider + "," + spec.TaskType,
		VendorID:     vendorID,
		Status:       1,
		SyncOfficial: 0,
		CreatedTime:  now,
		UpdatedTime:  now,
		NameRule:     model.NameRuleExact,
	}).Error
}

func ensureMangouVendor(provider string) (int, error) {
	name := "Mangou " + strings.ToUpper(provider)
	var vendor model.Vendor
	err := model.DB.Where("name = ?", name).First(&vendor).Error
	if err == nil {
		return vendor.Id, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}
	now := common.GetTimestamp()
	vendor = model.Vendor{
		Name:        name,
		Description: "Mangou managed " + strings.ToUpper(provider) + " provider",
		Status:      1,
		CreatedTime: now,
		UpdatedTime: now,
	}
	if err := model.DB.Create(&vendor).Error; err != nil {
		return 0, err
	}
	return vendor.Id, nil
}

func defaultMangouProviderChannelSpecs() []mangouProviderChannelSpec {
	groups := []string{"auto", "default"}
	return []mangouProviderChannelSpec{
		{
			Provider:    "bltai",
			TaskType:    "image",
			Name:        mangouProviderChannelName("bltai", "image"),
			BaseURL:     strings.TrimRight(common.GetEnvOrDefaultString("BLTAI_BASE_URL", "https://api.bltcy.ai/v1"), "/"),
			APIKey:      strings.TrimSpace(common.GetEnvOrDefaultString("BLTAI_API_KEY", "")),
			Models:      []string{"gpt-image-2"},
			Groups:      groups,
			Description: "Mangou BLTAI image provider",
		},
		{
			Provider:    "kie",
			TaskType:    "video",
			Name:        mangouProviderChannelName("kie", "video"),
			BaseURL:     strings.TrimRight(common.GetEnvOrDefaultString("KIE_BASE_URL", "https://api.kie.ai"), "/"),
			APIKey:      strings.TrimSpace(common.GetEnvOrDefaultString("KIE_API_KEY", "")),
			Models:      []string{"doubao-seedance-2-0-260128", "doubao-seedance-2-0-fast-260128"},
			Groups:      groups,
			Description: "Mangou KIE video provider",
		},
		{
			Provider:    "evolink",
			TaskType:    "image",
			Name:        mangouProviderChannelName("evolink", "image"),
			BaseURL:     strings.TrimRight(common.GetEnvOrDefaultString("EVOLINK_BASE_URL", ""), "/"),
			APIKey:      strings.TrimSpace(common.GetEnvOrDefaultString("EVOLINK_API_KEY", "")),
			Models:      []string{"gpt-image-2"},
			Groups:      groups,
			Description: "Mangou Evolink image provider",
		},
		{
			Provider:    "evolink",
			TaskType:    "video",
			Name:        mangouProviderChannelName("evolink", "video"),
			BaseURL:     strings.TrimRight(common.GetEnvOrDefaultString("EVOLINK_BASE_URL", ""), "/"),
			APIKey:      strings.TrimSpace(common.GetEnvOrDefaultString("EVOLINK_API_KEY", "")),
			Models:      []string{"doubao-seedance-2-0-260128", "doubao-seedance-2-0-fast-260128"},
			Groups:      groups,
			Description: "Mangou Evolink video provider",
		},
	}
}

func mangouProviderChannelName(provider string, taskType string) string {
	return fmt.Sprintf("Mangou %s %s", strings.ToUpper(provider), taskType)
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

func buildMangouPaymentQRURL(baseURL string, paymentID string, target string) string {
	u := strings.TrimRight(baseURL, "/") + "/v1/payments/" + url.PathEscape(paymentID) + "/qr.svg"
	values := url.Values{}
	values.Set("target", target)
	return u + "?" + values.Encode()
}

func buildMangouPaymentQRPNGURL(baseURL string, paymentID string, target string) string {
	u := strings.TrimRight(baseURL, "/") + "/v1/payments/" + url.PathEscape(paymentID) + "/qr.png"
	values := url.Values{}
	values.Set("target", target)
	return u + "?" + values.Encode()
}

func validateMangouCreemPaymentTarget(rawURL string) error {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL format: %s", err.Error())
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("invalid URL scheme: only http and https are allowed")
	}
	domain := strings.ToLower(parsedURL.Hostname())
	if domain == "creem.io" || strings.HasSuffix(domain, ".creem.io") {
		return nil
	}
	return fmt.Errorf("domain %s is not an allowed Creem checkout domain", domain)
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
	scaled, err := mangouQRCodeImage(target)
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

func mangouQRCodePNG(target string) ([]byte, error) {
	scaled, err := mangouQRCodeImage(target)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	if err := png.Encode(&b, scaled); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func mangouQRCodeImage(target string) (image.Image, error) {
	code, err := qr.Encode(target, qr.M, qr.Auto)
	if err != nil {
		return nil, err
	}
	scaled, err := barcode.Scale(code, 256, 256)
	if err != nil {
		return nil, err
	}
	return scaled, nil
}

func submitMangouUpstreamTask(req mangouAgentTaskRequest, channel *model.Channel) (*mangouUpstreamTaskResult, bool, error) {
	if channel == nil {
		return nil, false, errors.New("NewAPI provider channel is required")
	}
	key := strings.TrimSpace(channel.Key)
	baseURL := ""
	if channel.BaseURL != nil {
		baseURL = strings.TrimSpace(*channel.BaseURL)
	}
	if key == "" {
		return nil, true, fmt.Errorf("NewAPI channel %d (%s) has no provider key configured", channel.Id, channel.Name)
	}
	if baseURL == "" {
		return nil, true, fmt.Errorf("NewAPI channel %d (%s) has no base_url configured", channel.Id, channel.Name)
	}
	switch req.Provider {
	case "evolink", "bltai":
		return submitMangouUnifiedTask(req, key, baseURL)
	case "kie":
		return submitMangouKIERunwayTask(req, key, baseURL)
	default:
		return nil, false, nil
	}
}

func pollMangouUpstreamTask(task *model.Task) (*mangouUpstreamTaskResult, error) {
	provider := string(task.Platform)
	channel, err := model.CacheGetChannel(task.ChannelId)
	if err != nil {
		return nil, fmt.Errorf("failed to load NewAPI channel %d for task %s: %w", task.ChannelId, task.TaskID, err)
	}
	key := strings.TrimSpace(channel.Key)
	baseURL := ""
	if channel.BaseURL != nil {
		baseURL = strings.TrimSpace(*channel.BaseURL)
	}
	if key == "" || baseURL == "" {
		return nil, fmt.Errorf("NewAPI channel %d (%s) is missing key or base_url", channel.Id, channel.Name)
	}
	switch provider {
	case "evolink", "bltai":
		return pollMangouUnifiedTask(provider, task.PrivateData.UpstreamTaskID, key, baseURL)
	case "kie":
		return pollMangouKIERunwayTask(task.PrivateData.UpstreamTaskID, key, baseURL)
	default:
		return &mangouUpstreamTaskResult{}, nil
	}
}

func submitMangouUnifiedTask(req mangouAgentTaskRequest, key string, baseURL string) (*mangouUpstreamTaskResult, bool, error) {
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
	respBody, err := doMangouProviderJSON(http.MethodPost, mangouUnifiedURL(baseURL, path), key, body)
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

func pollMangouUnifiedTask(provider string, upstreamTaskID string, key string, baseURL string) (*mangouUpstreamTaskResult, error) {
	respBody, err := doMangouProviderJSON(http.MethodGet, mangouUnifiedURL(baseURL, "/v1/tasks/"+upstreamTaskID), key, nil)
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

func submitMangouKIERunwayTask(req mangouAgentTaskRequest, key string, baseURL string) (*mangouUpstreamTaskResult, bool, error) {
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
	respBody, err := doMangouProviderJSON(http.MethodPost, mangouKIEURL(baseURL, "/api/v1/runway/generate"), key, body)
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

func pollMangouKIERunwayTask(upstreamTaskID string, key string, baseURL string) (*mangouUpstreamTaskResult, error) {
	respBody, err := doMangouProviderJSON(http.MethodGet, mangouKIEURL(baseURL, "/api/v1/runway/record-detail?taskId="+upstreamTaskID), key, nil)
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

func mangouUnifiedURL(baseURL string, path string) string {
	base := strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(base, "/v1") && strings.HasPrefix(path, "/v1/") {
		path = strings.TrimPrefix(path, "/v1")
	}
	return base + path
}

func mangouKIEURL(baseURL string, path string) string {
	base := strings.TrimRight(baseURL, "/")
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
