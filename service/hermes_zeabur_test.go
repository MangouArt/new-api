package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestDeployHermesTenantOnZeaburUsesRawTemplateMutation(t *testing.T) {
	var payload struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer test-zeabur-token", r.Header.Get("Authorization"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"deployTemplate":{"_id":"deployment-id"}}}`))
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
	require.Equal(t, "deployment-id", result.DeploymentID)
	require.Contains(t, payload.Query, "deployTemplate(rawSpecYaml: $rawSpecYaml, projectID: $projectId)")
	require.NotContains(t, payload.Query, "DeployTemplateInput")
	require.Equal(t, "project-id", payload.Variables["projectId"])
	rawSpecYaml, ok := payload.Variables["rawSpecYaml"].(string)
	require.True(t, ok)
	require.Contains(t, rawSpecYaml, "name: hermes-user-42")
	require.Contains(t, rawSpecYaml, "NEWAPI_USER_ID:")
	require.Contains(t, rawSpecYaml, "default: 42")
	require.Contains(t, rawSpecYaml, "default: tenant-token")
	require.Contains(t, rawSpecYaml, "default: admin-token")
}
