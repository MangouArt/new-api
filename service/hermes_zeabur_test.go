package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestDeployHermesTenantOnZeaburUsesRawTemplateMutation(t *testing.T) {
	var payloads []struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer test-zeabur-token", r.Header.Get("Authorization"))
		var payload struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		payloads = append(payloads, payload)
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(payload.Query, "deployTemplate") {
			_, _ = w.Write([]byte(`{"data":{"deployTemplate":{"_id":"project-id"}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"services":{"edges":[{"node":{"_id":"service-id","name":"hermes-user-42"}}]}}}`))
	}))
	defer server.Close()

	originalEndpoint := zeaburGraphQLEndpoint
	zeaburGraphQLEndpoint = server.URL
	t.Cleanup(func() {
		zeaburGraphQLEndpoint = originalEndpoint
	})

	t.Setenv("ZEABUR_API_TOKEN", "test-zeabur-token")
	t.Setenv("HERMES_ZEABUR_PROJECT_ID", "project-id")
	t.Setenv("HERMES_ZEABUR_ENVIRONMENT_ID", "env-id")

	result, err := DeployHermesTenantOnZeabur(context.Background(), HermesTenantZeaburDeployRequest{
		Tenant: &model.HermesTenant{
			UserID:      42,
			TenantID:    "hermes-42",
			ServiceName: "hermes-user-42",
			VolumeName:  "hermes-user-42-data",
		},
		TenantToken:   "tenant-token",
		AdminToken:    "admin-token",
		NewAPIBaseURL: "https://newapi.example.test",
	})

	require.NoError(t, err)
	require.Equal(t, "project-id", result.ProjectID)
	require.Equal(t, "service-id", result.ServiceID)
	require.Equal(t, "http://hermes-user-42.zeabur.internal:8642", result.DashboardURL)
	require.Empty(t, result.DeploymentID)
	require.Len(t, payloads, 2)
	require.Contains(t, payloads[0].Query, "deployTemplate(rawSpecYaml: $rawSpecYaml, projectID: $projectID)")
	require.NotContains(t, payloads[0].Query, "DeployTemplateInput")
	require.Equal(t, "project-id", payloads[0].Variables["projectID"])
	rawSpecYaml, ok := payloads[0].Variables["rawSpecYaml"].(string)
	require.True(t, ok)
	require.Contains(t, rawSpecYaml, "name: hermes-user-42")
	require.Contains(t, rawSpecYaml, "id: hermes-user-42-data")
	require.NotContains(t, rawSpecYaml, "id: hermes-data")
	require.Contains(t, rawSpecYaml, "NEWAPI_USER_ID:")
	require.Contains(t, rawSpecYaml, "default: 42")
	require.Contains(t, rawSpecYaml, "default: tenant-token")
	require.Contains(t, rawSpecYaml, "default: admin-token")
	require.Contains(t, rawSpecYaml, "OPENAI_MODEL:")
	require.Contains(t, rawSpecYaml, "default: gpt-5.5")
	require.Contains(t, rawSpecYaml, "HERMES_DEFAULT_PROVIDER:")
	require.Contains(t, rawSpecYaml, "default: newapi")
	require.Contains(t, rawSpecYaml, "HERMES_INFERENCE_PROVIDER:")
	require.Contains(t, rawSpecYaml, "default: custom:newapi")
	require.Contains(t, rawSpecYaml, "HERMES_NEWAPI_TRANSPORT:")
	require.Contains(t, rawSpecYaml, "default: codex_responses")
	require.Contains(t, payloads[1].Query, "services(projectID: $projectID")
	require.Equal(t, "project-id", payloads[1].Variables["projectID"])
}
