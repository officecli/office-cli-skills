package hostedllm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const defaultAIGatewayCreateAPIKeyPath = "/api/open/api-keys"

type AIGatewayAdminClient interface {
	CreateAPIKey(ctx context.Context, req CreateAIGatewayAPIKeyRequest) (*CreatedAIGatewayAPIKey, error)
}

type CreateAIGatewayAPIKeyRequest struct {
	Name string
}

type CreatedAIGatewayAPIKey struct {
	PlaintextKey string
	UpstreamID   string
	Name         string
}

type httpAIGatewayAdminClient struct {
	client *http.Client
	cfg    Config
}

func newHTTPAIGatewayAdminClient(client *http.Client, cfg Config) AIGatewayAdminClient {
	if client == nil {
		client = http.DefaultClient
	}
	return &httpAIGatewayAdminClient{client: client, cfg: cfg}
}

func (c *httpAIGatewayAdminClient) CreateAPIKey(ctx context.Context, req CreateAIGatewayAPIKeyRequest) (*CreatedAIGatewayAPIKey, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(c.cfg.AIGatewayAdminBaseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("AIGATEWAY_ADMIN_BASE_URL is not configured")
	}
	adminKey := strings.TrimSpace(c.cfg.AIGatewayAdminAPIKey)
	if adminKey == "" {
		return nil, fmt.Errorf("AIGATEWAY_ADMIN_API_KEY is not configured")
	}
	path := strings.TrimSpace(c.cfg.AIGatewayCreateKeyPath)
	if path == "" {
		path = defaultAIGatewayCreateAPIKeyPath
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, fmt.Errorf("aigateway api key name is required")
	}
	payload := map[string]any{
		"name":                 name,
		"expired_time":         -1,
		"remain_quota":         0,
		"unlimited_quota":      true,
		"model_limits_enabled": false,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+path, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+adminKey)
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("aigateway api key creation failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	created, err := parseCreatedAIGatewayAPIKey(body)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(created.Name) == "" {
		created.Name = name
	}
	return created, nil
}

func parseCreatedAIGatewayAPIKey(body []byte) (*CreatedAIGatewayAPIKey, error) {
	var envelope struct {
		APIKey string `json:"api_key"`
		Key    string `json:"key"`
		Token  string `json:"token"`
		ID     any    `json:"id"`
		Name   string `json:"name"`
		Data   struct {
			APIKey string `json:"api_key"`
			Key    string `json:"key"`
			Token  string `json:"token"`
			ID     any    `json:"id"`
			Name   string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("decode aigateway api key creation response: %w", err)
	}
	plain := firstNonEmpty(envelope.APIKey, envelope.Key, envelope.Token, envelope.Data.APIKey, envelope.Data.Key, envelope.Data.Token)
	if strings.TrimSpace(plain) == "" {
		return nil, fmt.Errorf("aigateway api key creation response did not include an api key")
	}
	upstreamID := stringifyFirstNonEmpty(envelope.ID, envelope.Data.ID)
	name := firstNonEmpty(envelope.Name, envelope.Data.Name)
	return &CreatedAIGatewayAPIKey{
		PlaintextKey: strings.TrimSpace(plain),
		UpstreamID:   upstreamID,
		Name:         strings.TrimSpace(name),
	}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func stringifyFirstNonEmpty(values ...any) string {
	for _, value := range values {
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return strings.TrimSpace(typed)
			}
		case float64:
			if typed != 0 {
				return fmt.Sprintf("%.0f", typed)
			}
		case json.Number:
			if strings.TrimSpace(string(typed)) != "" {
				return strings.TrimSpace(string(typed))
			}
		}
	}
	return ""
}
