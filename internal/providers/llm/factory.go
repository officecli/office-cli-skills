package llm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/officecli/officecli/engine"
)

type Config struct {
	Provider   string `json:"provider"`
	BaseURL    string `json:"base_url"`
	APIKey     string `json:"api_key"`
	Model      string `json:"model"`
	ImageModel string `json:"image_model"`
	TimeoutSec int    `json:"timeout_sec"`
}

type Provider interface {
	NewClient() (engine.LLMClient, error)
}

func NewProvider(cfg Config) (Provider, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "", "openai":
		return &openAIProvider{cfg: cfg}, nil
	case "internal":
		return &internalProvider{cfg: cfg}, nil
	default:
		return nil, fmt.Errorf("unsupported llm provider: %s", cfg.Provider)
	}
}

func NewClient(cfg Config) (engine.LLMClient, error) {
	provider, err := NewProvider(cfg)
	if err != nil {
		return nil, err
	}
	return provider.NewClient()
}

type openAIProvider struct{ cfg Config }

func (p *openAIProvider) NewClient() (engine.LLMClient, error) {
	if strings.TrimSpace(p.cfg.BaseURL) == "" {
		return nil, fmt.Errorf("llm base_url is required")
	}
	if strings.TrimSpace(p.cfg.Model) == "" {
		return nil, fmt.Errorf("llm model is required")
	}
	return &openAIClient{
		baseURL:    strings.TrimRight(strings.TrimSpace(p.cfg.BaseURL), "/"),
		apiKey:     strings.TrimSpace(p.cfg.APIKey),
		model:      strings.TrimSpace(p.cfg.Model),
		imageModel: strings.TrimSpace(p.cfg.ImageModel),
		client:     &http.Client{Timeout: timeoutFor(p.cfg.TimeoutSec)},
	}, nil
}

type internalProvider struct{ cfg Config }

func (p *internalProvider) NewClient() (engine.LLMClient, error) {
	if strings.TrimSpace(p.cfg.BaseURL) == "" {
		return nil, fmt.Errorf("llm base_url is required")
	}
	return &internalClient{
		baseURL: strings.TrimRight(strings.TrimSpace(p.cfg.BaseURL), "/"),
		apiKey:  strings.TrimSpace(p.cfg.APIKey),
		model:   strings.TrimSpace(p.cfg.Model),
		client:  &http.Client{Timeout: timeoutFor(p.cfg.TimeoutSec)},
	}, nil
}

func timeoutFor(seconds int) time.Duration {
	if seconds <= 0 {
		return 60 * time.Second
	}
	return time.Duration(seconds) * time.Second
}

type openAIClient struct {
	baseURL    string
	apiKey     string
	model      string
	imageModel string
	client     *http.Client
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func toChatMessages(messages []engine.LLMMessage) []chatMessage {
	out := make([]chatMessage, 0, len(messages))
	for _, msg := range messages {
		out = append(out, chatMessage{Role: msg.Role, Content: msg.Content})
	}
	return out
}

func (c *openAIClient) CompleteText(ctx context.Context, messages []engine.LLMMessage) (string, error) {
	return c.chatCompletion(ctx, map[string]any{
		"model":    c.model,
		"messages": toChatMessages(messages),
	})
}

func (c *openAIClient) CompleteJSON(ctx context.Context, messages []engine.LLMMessage) (string, error) {
	return c.chatCompletion(ctx, map[string]any{
		"model":           c.model,
		"messages":        toChatMessages(messages),
		"response_format": map[string]any{"type": "json_object"},
	})
}

func (c *openAIClient) CompleteStructured(ctx context.Context, req engine.StructuredCompletionRequest) (string, error) {
	var schema map[string]any
	if err := json.Unmarshal(req.Schema.JSONSchema, &schema); err != nil {
		return "", fmt.Errorf("parse json schema: %w", err)
	}
	return c.chatCompletion(ctx, map[string]any{
		"model":    c.model,
		"messages": toChatMessages(req.Messages),
		"response_format": map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   req.Schema.Name,
				"strict": req.Schema.Strict,
				"schema": schema,
			},
		},
	})
}

func (c *openAIClient) GenerateImage(ctx context.Context, req engine.ImageGenerationRequest) (*engine.ImageGenerationResult, error) {
	model := c.imageModel
	if model == "" {
		model = c.model
	}
	payload := map[string]any{
		"model":           model,
		"prompt":          req.Prompt,
		"size":            pickImageSize(req.TargetAspectRatio),
		"response_format": "b64_json",
	}
	body, err := c.post(ctx, c.baseURL+"/images/generations", payload)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode image response: %w", err)
	}
	if len(resp.Data) == 0 || strings.TrimSpace(resp.Data[0].B64JSON) == "" {
		return nil, fmt.Errorf("image response is empty")
	}
	data, err := base64.StdEncoding.DecodeString(resp.Data[0].B64JSON)
	if err != nil {
		return nil, fmt.Errorf("decode image data: %w", err)
	}
	return &engine.ImageGenerationResult{Data: data, MIME: "image/png"}, nil
}

func pickImageSize(aspectRatio float64) string {
	switch {
	case aspectRatio > 1.2:
		return "1536x1024"
	case aspectRatio > 0 && aspectRatio < 0.9:
		return "1024x1536"
	default:
		return "1024x1024"
	}
}

func (c *openAIClient) chatCompletion(ctx context.Context, payload map[string]any) (string, error) {
	body, err := c.post(ctx, c.baseURL+"/chat/completions", payload)
	if err != nil {
		return "", err
	}
	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("decode chat response: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("chat response is empty")
	}
	return resp.Choices[0].Message.Content, nil
}

func (c *openAIClient) post(ctx context.Context, url string, payload map[string]any) ([]byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("llm request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

type internalClient struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

func (c *internalClient) CompleteText(ctx context.Context, messages []engine.LLMMessage) (string, error) {
	return c.complete(ctx, "text", messages, nil)
}

func (c *internalClient) CompleteJSON(ctx context.Context, messages []engine.LLMMessage) (string, error) {
	return c.complete(ctx, "json", messages, nil)
}

func (c *internalClient) CompleteStructured(ctx context.Context, req engine.StructuredCompletionRequest) (string, error) {
	return c.complete(ctx, "structured", req.Messages, map[string]any{
		"schema_name": req.Schema.Name,
		"strict":      req.Schema.Strict,
		"schema":      json.RawMessage(req.Schema.JSONSchema),
	})
}

func (c *internalClient) GenerateImage(ctx context.Context, req engine.ImageGenerationRequest) (*engine.ImageGenerationResult, error) {
	payload := map[string]any{
		"model":        c.model,
		"prompt":       req.Prompt,
		"aspect_ratio": req.TargetAspectRatio,
	}
	body, err := c.post(ctx, c.baseURL+"/v1/image", payload)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data string `json:"data"`
		MIME string `json:"mime"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode internal image response: %w", err)
	}
	if resp.Data == "" {
		return nil, fmt.Errorf("internal image response is empty")
	}
	data, err := base64.StdEncoding.DecodeString(resp.Data)
	if err != nil {
		return nil, err
	}
	return &engine.ImageGenerationResult{Data: data, MIME: resp.MIME}, nil
}

func (c *internalClient) complete(ctx context.Context, kind string, messages []engine.LLMMessage, extra map[string]any) (string, error) {
	payload := map[string]any{
		"model":    c.model,
		"messages": messages,
	}
	for k, v := range extra {
		payload[k] = v
	}
	body, err := c.post(ctx, c.baseURL+"/v1/"+kind, payload)
	if err != nil {
		return "", err
	}
	var resp struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("decode internal completion response: %w", err)
	}
	if resp.Content == "" {
		return "", fmt.Errorf("internal completion response is empty")
	}
	return resp.Content, nil
}

func (c *internalClient) post(ctx context.Context, url string, payload map[string]any) ([]byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("internal llm request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}
