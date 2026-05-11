package hostedllm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHTTPAIGatewayAdminClientCreateAPIKeySendsFixedPayloadAndParsesNestedKey(t *testing.T) {
	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/open/api-keys", r.URL.Path)
		require.Equal(t, "Bearer admin-token", r.Header.Get("Authorization"))
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotPayload))
		_, _ = w.Write([]byte(`{"data":{"api_key":"sk-user-42","id":123,"name":"officecli-user-42"}}`))
	}))
	defer server.Close()

	client := newHTTPAIGatewayAdminClient(server.Client(), Config{
		AIGatewayAdminBaseURL: server.URL,
		AIGatewayAdminAPIKey:  "admin-token",
	})

	created, err := client.CreateAPIKey(context.Background(), CreateAIGatewayAPIKeyRequest{Name: "officecli-user-42"})
	require.NoError(t, err)
	require.Equal(t, "sk-user-42", created.PlaintextKey)
	require.Equal(t, "123", created.UpstreamID)
	require.Equal(t, "officecli-user-42", created.Name)
	require.Equal(t, map[string]any{
		"name":                 "officecli-user-42",
		"expired_time":         float64(-1),
		"remain_quota":         float64(0),
		"unlimited_quota":      true,
		"model_limits_enabled": false,
	}, gotPayload)
}

func TestHTTPAIGatewayAdminClientCreateAPIKeySendsConfiguredGroup(t *testing.T) {
	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/open/api-keys", r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotPayload))
		_, _ = w.Write([]byte(`{"data":{"key":"sk-user-42","id":42,"name":"officecli-user-42"}}`))
	}))
	defer server.Close()

	client := newHTTPAIGatewayAdminClient(server.Client(), Config{
		AIGatewayAdminBaseURL:  server.URL,
		AIGatewayAdminAPIKey:   "admin-token",
		AIGatewayAPIKeyGroup:   "gpt_codex_only_with_5.5",
		AIGatewayCreateKeyPath: "/api/open/api-keys",
	})

	_, err := client.CreateAPIKey(context.Background(), CreateAIGatewayAPIKeyRequest{Name: "officecli-user-42"})
	require.NoError(t, err)
	require.Equal(t, "gpt_codex_only_with_5.5", gotPayload["group"])
}

func TestHTTPAIGatewayAdminClientRejectsResponseWithoutAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"id":"missing-key"}}`))
	}))
	defer server.Close()

	client := newHTTPAIGatewayAdminClient(server.Client(), Config{
		AIGatewayAdminBaseURL:  server.URL,
		AIGatewayAdminAPIKey:   "admin-token",
		AIGatewayCreateKeyPath: "api/open/api-keys",
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	created, err := client.CreateAPIKey(ctx, CreateAIGatewayAPIKeyRequest{Name: "officecli-user-42"})
	require.Error(t, err)
	require.Nil(t, created)
	require.Contains(t, err.Error(), "did not include an api key")
}
