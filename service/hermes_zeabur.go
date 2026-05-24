package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/model"
)

var zeaburGraphQLEndpoint = "https://api.zeabur.com/graphql"

type HermesTenantZeaburDeployRequest struct {
	Tenant        *model.HermesTenant
	TenantToken   string
	AdminToken    string
	NewAPIBaseURL string
}

type HermesTenantZeaburDeployResult struct {
	ProjectID     string `json:"project_id"`
	EnvironmentID string `json:"environment_id,omitempty"`
	DeploymentID  string `json:"deployment_id"`
	ServiceName   string `json:"service_name"`
	VolumeName    string `json:"volume_name"`
}

type HermesZeaburConfigStatus struct {
	Configured bool     `json:"configured"`
	Missing    []string `json:"missing"`
}

type zeaburGraphQLResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

type zeaburDeployTemplateData struct {
	DeployTemplate struct {
		ID string `json:"_id"`
	} `json:"deployTemplate"`
}

func HermesNewAPIBaseURL() string {
	return strings.TrimRight(strings.TrimSpace(os.Getenv("HERMES_NEWAPI_BASE_URL")), "/")
}

func HermesZeaburConfig() HermesZeaburConfigStatus {
	missing := make([]string, 0)
	if strings.TrimSpace(os.Getenv("ZEABUR_API_TOKEN")) == "" {
		missing = append(missing, "ZEABUR_API_TOKEN")
	}
	if strings.TrimSpace(os.Getenv("HERMES_ZEABUR_PROJECT_ID")) == "" && strings.TrimSpace(os.Getenv("ZEABUR_PROJECT_ID")) == "" {
		missing = append(missing, "HERMES_ZEABUR_PROJECT_ID")
	}
	return HermesZeaburConfigStatus{
		Configured: len(missing) == 0,
		Missing:    missing,
	}
}

func DeployHermesTenantOnZeabur(ctx context.Context, req HermesTenantZeaburDeployRequest) (*HermesTenantZeaburDeployResult, error) {
	if req.Tenant == nil || req.Tenant.UserID <= 0 {
		return nil, errors.New("invalid hermes tenant")
	}
	apiToken := strings.TrimSpace(os.Getenv("ZEABUR_API_TOKEN"))
	if apiToken == "" {
		return nil, errors.New("ZEABUR_API_TOKEN is required")
	}
	projectID := strings.TrimSpace(os.Getenv("HERMES_ZEABUR_PROJECT_ID"))
	if projectID == "" {
		projectID = strings.TrimSpace(os.Getenv("ZEABUR_PROJECT_ID"))
	}
	if projectID == "" {
		return nil, errors.New("HERMES_ZEABUR_PROJECT_ID or ZEABUR_PROJECT_ID is required")
	}
	environmentID := strings.TrimSpace(os.Getenv("HERMES_ZEABUR_ENVIRONMENT_ID"))
	newAPIBaseURL := strings.TrimRight(strings.TrimSpace(req.NewAPIBaseURL), "/")
	if newAPIBaseURL == "" {
		newAPIBaseURL = HermesNewAPIBaseURL()
	}
	if newAPIBaseURL == "" {
		return nil, errors.New("NewAPI base URL is required")
	}
	if strings.TrimSpace(req.TenantToken) == "" || strings.TrimSpace(req.AdminToken) == "" {
		return nil, errors.New("tenant token and hermes admin token are required")
	}

	variables := map[string]any{
		"projectId":   projectID,
		"rawSpecYaml": renderHermesTenantTemplate(req.Tenant, newAPIBaseURL, req.TenantToken, req.AdminToken),
	}

	body, err := postZeaburGraphQL(ctx, apiToken, `mutation DeployHermesTenant($rawSpecYaml: String!, $projectId: ObjectID!) {
  deployTemplate(rawSpecYaml: $rawSpecYaml, projectID: $projectId) {
    _id
  }
}`, variables)
	if err != nil {
		return nil, err
	}

	var data zeaburDeployTemplateData
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}
	if data.DeployTemplate.ID == "" {
		return nil, errors.New("zeabur deployTemplate response missing deployment id")
	}
	return &HermesTenantZeaburDeployResult{
		ProjectID:     projectID,
		EnvironmentID: environmentID,
		DeploymentID:  data.DeployTemplate.ID,
		ServiceName:   req.Tenant.ServiceName,
		VolumeName:    req.Tenant.VolumeName,
	}, nil
}

func postZeaburGraphQL(ctx context.Context, token string, query string, variables map[string]any) (json.RawMessage, error) {
	payload := map[string]any{
		"query":     query,
		"variables": variables,
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, zeaburGraphQLEndpoint, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("zeabur graphql returned status %d", resp.StatusCode)
	}
	var graphqlResp zeaburGraphQLResponse
	if err := json.Unmarshal(respBytes, &graphqlResp); err != nil {
		return nil, err
	}
	if len(graphqlResp.Errors) > 0 {
		return nil, errors.New(graphqlResp.Errors[0].Message)
	}
	if len(graphqlResp.Data) == 0 {
		return nil, errors.New("zeabur graphql response missing data")
	}
	return graphqlResp.Data, nil
}

func renderHermesTenantTemplate(tenant *model.HermesTenant, newAPIBaseURL string, tenantToken string, adminToken string) string {
	return fmt.Sprintf(`# yaml-language-server: $schema=https://schema.zeabur.app/template.json
apiVersion: zeabur.com/v1
kind: Template
metadata:
  name: Hermes Tenant Runtime
spec:
  description: Dedicated Hermes agent runtime for one NewAPI user.
  icon: https://raw.githubusercontent.com/zeabur/service-icons/main/marketplace/docker.svg
  tags:
    - AI
    - Agent
  services:
    - name: %s
      icon: https://raw.githubusercontent.com/zeabur/service-icons/main/marketplace/docker.svg
      template: GIT
      spec:
        source:
          source: GITHUB
          repo: 1247611351
          branch: main
        ports:
          - id: http
            port: 8642
            type: HTTP
        volumes:
          - id: hermes-data
            dir: /opt/data
        env:
          HERMES_TENANT_ID:
            default: %s
          NEWAPI_USER_ID:
            default: %d
          HERMES_HOME:
            default: /opt/data
          HOME:
            default: /opt/data/home
          NEWAPI_BASE_URL:
            default: %s
          NEWAPI_TENANT_TOKEN:
            default: %s
          OPENAI_API_BASE:
            default: %s/v1
          OPENAI_API_KEY:
            default: %s
          HERMES_ADMIN_TOKEN:
            default: %s
          FEISHU_PAIRING_MODE:
            default: manual
          HERMES_SUPERVISOR_ENABLED:
            default: "true"
          HERMES_BACKEND_PORT:
            default: "8643"
`, tenant.ServiceName, tenant.TenantID, tenant.UserID, newAPIBaseURL, tenantToken, newAPIBaseURL, tenantToken, adminToken)
}
