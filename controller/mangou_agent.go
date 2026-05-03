package controller

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	mangouAgentDefaultAgentID = "mangou-agent"
	mangouAgentTokenPrefix    = "agent:"
	mangouImageOriginModel    = "mangou-image"
	mangouVideoOriginModel    = "mangou-video"
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

type mangouProviderPricingRule struct {
	BaseQuota   int                           `json:"base_quota"`
	Multipliers map[string]map[string]float64 `json:"multipliers"`
}

type mangouProviderPricing map[string]map[string]mangouProviderPricingRule

func MangouAgentSkill(c *gin.Context) {
	c.Data(http.StatusOK, "text/markdown; charset=utf-8", []byte(`# Mangou NewAPI Agent Skill

Use this NewAPI gateway for Mangou image and video tasks.

## Register

1. Request an email code:

POST /v1/agents/register/email-code

2. Register with email and code:

POST /v1/agents/register

Store the returned token as `+"`BILLING_TOKEN`"+` and send it as `+"`Authorization: Bearer ${BILLING_TOKEN}`"+`.

## Submit Task

POST /v1/agent/tasks

Images and videos are both asynchronous tasks. Poll `+"`/v1/agent/tasks/{task_id}`"+` until the task reaches a terminal status.
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
	if !tokenCreated {
		tokenValue = token.GetMaskedKey()
	}

	common.ApiSuccess(c, gin.H{
		"agent_id":    agentID,
		"email":       user.Email,
		"user_id":     user.Id,
		"token":       tokenValue,
		"token_new":   tokenCreated,
		"balance":     user.Quota,
		"base_url":    strings.TrimSuffix(system_setting.ServerAddress, "/"),
		"skill_url":   strings.TrimSuffix(system_setting.ServerAddress, "/") + "/skills/mangou-newapi/SKILL.md",
		"token_store": "BILLING_TOKEN",
	})
}

func MangouAgentSubmitTask(c *gin.Context) {
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

	pricing, err := loadMangouProviderPricing()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	quota, ratios, err := calculateMangouTaskQuota(pricing, req.Provider, req.Type, req.Params)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	userID := c.GetInt("id")
	if userID <= 0 {
		common.ApiErrorMsg(c, "authenticated user is required")
		return
	}
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
		Group:      req.Provider,
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
	if err := task.Insert(); err != nil {
		refundMangouAgentQuota(c, userID, quota)
		common.ApiError(c, err)
		return
	}

	model.RecordLog(userID, model.LogTypeSystem, fmt.Sprintf("Mangou %s task submitted, provider=%s, quota=%d", req.Type, req.Provider, quota))
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
	common.ApiSuccess(c, gin.H{
		"task_id":    task.TaskID,
		"provider":   string(task.Platform),
		"type":       strings.TrimSuffix(task.Action, ".generate"),
		"model":      task.Properties.UpstreamModelName,
		"status":     strings.ToLower(string(task.Status)),
		"progress":   task.Progress,
		"quota":      task.Quota,
		"result_url": task.GetResultURL(),
	})
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
			"status":     strings.ToLower(string(task.Status)),
			"progress":   task.Progress,
			"quota":      task.Quota,
			"result_url": task.GetResultURL(),
		})
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(rows)
	common.ApiSuccess(c, pageInfo)
}

func loadMangouProviderPricing() (mangouProviderPricing, error) {
	raw := strings.TrimSpace(os.Getenv("MANGOU_PROVIDER_PRICING"))
	if raw == "" {
		return nil, errors.New("MANGOU_PROVIDER_PRICING is not configured")
	}
	var pricing mangouProviderPricing
	if err := common.Unmarshal([]byte(raw), &pricing); err != nil {
		return nil, fmt.Errorf("invalid MANGOU_PROVIDER_PRICING: %w", err)
	}
	if len(pricing) == 0 {
		return nil, errors.New("MANGOU_PROVIDER_PRICING is empty")
	}
	return pricing, nil
}

func calculateMangouTaskQuota(pricing mangouProviderPricing, provider string, taskType string, params map[string]any) (int, map[string]float64, error) {
	providerPricing, ok := pricing[provider]
	if !ok {
		return 0, nil, fmt.Errorf("pricing for provider %s is not configured", provider)
	}
	rule, ok := providerPricing[taskType]
	if !ok {
		return 0, nil, fmt.Errorf("pricing for provider %s type %s is not configured", provider, taskType)
	}
	if rule.BaseQuota <= 0 {
		return 0, nil, fmt.Errorf("base_quota for provider %s type %s must be positive", provider, taskType)
	}

	total := float64(rule.BaseQuota)
	ratios := map[string]float64{"base_quota": float64(rule.BaseQuota)}
	for paramName, values := range rule.Multipliers {
		if len(values) == 0 {
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

func mangouPricingValueKey(value any) string {
	switch v := value.(type) {
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
		Group:       "default",
	}
	if err := user.Insert(0); err != nil {
		return nil, err
	}
	return &user, nil
}

func findOrCreateMangouAgentToken(userID int, agentID string) (*model.Token, bool, error) {
	tokenName := mangouAgentTokenPrefix + agentID
	var token model.Token
	err := model.DB.Where("user_id = ? AND name = ?", userID, tokenName).First(&token).Error
	if err == nil {
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
		Group:              "auto",
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
