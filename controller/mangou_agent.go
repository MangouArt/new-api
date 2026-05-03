package controller

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
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

type mangouUpstreamTaskResult struct {
	ID        string
	Status    model.TaskStatus
	Progress  string
	ResultURL string
	Raw       []byte
}

type mangouUnifiedTaskResponse struct {
	ID       string         `json:"id"`
	Status   string         `json:"status"`
	Progress int            `json:"progress"`
	Results  []string       `json:"results"`
	Error    map[string]any `json:"error"`
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

	quota, ratios, err := calculateMangouTaskQuota(req.Provider, req.Type, req.Params)
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
	if parsed.ID == "" {
		return nil, true, fmt.Errorf("%s upstream response missing task id", req.Provider)
	}
	return &mangouUpstreamTaskResult{
		ID:       parsed.ID,
		Status:   mapMangouUnifiedStatus(parsed.Status),
		Progress: mapMangouProgress(parsed.Progress),
		Raw:      respBody,
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
	resultURL := ""
	if len(parsed.Results) > 0 {
		resultURL = parsed.Results[0]
	}
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
