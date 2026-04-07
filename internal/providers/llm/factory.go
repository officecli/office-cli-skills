package llm

import (
	"bufio"
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
	Provider     string `json:"provider"`
	BaseURL      string `json:"base_url"`
	APIKey       string `json:"api_key"`
	Model        string `json:"model"`
	ImageBaseURL string `json:"image_base_url,omitempty"`
	ImageAPIKey  string `json:"image_api_key,omitempty"`
	ImageModel   string `json:"image_model"`
	ReviewModel  string `json:"review_model,omitempty"`
	TimeoutSec   int    `json:"timeout_sec"`
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
		baseURL:      strings.TrimRight(strings.TrimSpace(p.cfg.BaseURL), "/"),
		apiKey:       strings.TrimSpace(p.cfg.APIKey),
		model:        strings.TrimSpace(p.cfg.Model),
		imageBaseURL: strings.TrimRight(strings.TrimSpace(p.cfg.ImageBaseURL), "/"),
		imageAPIKey:  strings.TrimSpace(p.cfg.ImageAPIKey),
		imageModel:   strings.TrimSpace(p.cfg.ImageModel),
		client:       &http.Client{Timeout: timeoutFor(p.cfg.TimeoutSec)},
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
	baseURL      string
	apiKey       string
	model        string
	imageBaseURL string
	imageAPIKey  string
	imageModel   string
	client       *http.Client
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
	imageBaseURL := c.baseURL
	if c.imageBaseURL != "" {
		imageBaseURL = c.imageBaseURL
	}
	imageAPIKey := c.apiKey
	if c.imageAPIKey != "" {
		imageAPIKey = c.imageAPIKey
	}
	if isGoogleImageEndpoint(imageBaseURL, model) {
		return c.generateGoogleImage(ctx, imageBaseURL, imageAPIKey, model, req)
	}
	payload := map[string]any{
		"model":           model,
		"prompt":          req.Prompt,
		"size":            pickImageSize(req.TargetAspectRatio),
		"response_format": "b64_json",
	}
	body, err := c.postWithAPIKey(ctx, imageBaseURL+"/images/generations", imageAPIKey, payload)
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

func isGoogleImageEndpoint(baseURL, model string) bool {
	baseURL = strings.ToLower(strings.TrimSpace(baseURL))
	model = strings.ToLower(strings.TrimSpace(model))
	return strings.Contains(baseURL, "generativelanguage.googleapis.com") ||
		strings.HasPrefix(model, "gemini-") ||
		strings.HasPrefix(model, "imagen-")
}

func (c *openAIClient) generateGoogleImage(ctx context.Context, baseURL, apiKey, model string, req engine.ImageGenerationRequest) (*engine.ImageGenerationResult, error) {
	payload := map[string]any{
		"contents": []map[string]any{
			{
				"parts": []map[string]any{
					{
						"text": req.Prompt,
					},
				},
			},
		},
		"generationConfig": map[string]any{
			"responseModalities": []string{"TEXT", "IMAGE"},
		},
	}
	body, err := c.postGoogle(ctx, strings.TrimRight(baseURL, "/")+"/models/"+model+":generateContent", apiKey, payload)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					InlineData *struct {
						MIMEType string `json:"mimeType"`
						Data     string `json:"data"`
					} `json:"inlineData,omitempty"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode google image response: %w", err)
	}
	for _, candidate := range resp.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.InlineData == nil || strings.TrimSpace(part.InlineData.Data) == "" {
				continue
			}
			data, err := base64.StdEncoding.DecodeString(part.InlineData.Data)
			if err != nil {
				return nil, fmt.Errorf("decode google image data: %w", err)
			}
			mime := strings.TrimSpace(part.InlineData.MIMEType)
			if mime == "" {
				mime = "image/png"
			}
			return &engine.ImageGenerationResult{Data: data, MIME: mime}, nil
		}
	}
	return nil, fmt.Errorf("google image response is empty")
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
		if !requiresStreamingFallback(err) {
			return "", err
		}
		return c.chatCompletionStream(ctx, payload)
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
	content := resp.Choices[0].Message.Content
	if strings.TrimSpace(content) != "" {
		return content, nil
	}
	streamContent, err := c.chatCompletionStream(ctx, payload)
	if err != nil {
		return "", fmt.Errorf("chat response is empty: %w", err)
	}
	return streamContent, nil
}

func requiresStreamingFallback(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "Stream must be set to true")
}

func (c *openAIClient) chatCompletionStream(ctx context.Context, payload map[string]any) (string, error) {
	streamPayload := make(map[string]any, len(payload)+1)
	for key, value := range payload {
		streamPayload[key] = value
	}
	streamPayload["stream"] = true

	resp, err := c.postStream(ctx, c.baseURL+"/chat/completions", streamPayload)
	if err != nil {
		return "", err
	}
	defer resp.Close()

	reader := bufio.NewReader(resp)
	var content strings.Builder
	var eventData []string

	flushEvent := func() error {
		if len(eventData) == 0 {
			return nil
		}
		payload := strings.Join(eventData, "\n")
		eventData = eventData[:0]
		if strings.TrimSpace(payload) == "" {
			return nil
		}
		if strings.TrimSpace(payload) == "[DONE]" {
			return io.EOF
		}

		var chunk struct {
			Error *struct {
				Message string `json:"message"`
			} `json:"error,omitempty"`
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return fmt.Errorf("decode streaming chat response: %w", err)
		}
		if chunk.Error != nil {
			return fmt.Errorf("streaming chat response failed: %s", strings.TrimSpace(chunk.Error.Message))
		}
		for _, choice := range chunk.Choices {
			content.WriteString(choice.Delta.Content)
		}
		return nil
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return "", err
		}
		trimmed := strings.TrimRight(line, "\r\n")
		if trimmed == "" {
			if flushErr := flushEvent(); flushErr != nil {
				if flushErr == io.EOF {
					break
				}
				return "", flushErr
			}
		} else if strings.HasPrefix(trimmed, "data:") {
			eventData = append(eventData, strings.TrimSpace(strings.TrimPrefix(trimmed, "data:")))
		}
		if err == io.EOF {
			if flushErr := flushEvent(); flushErr != nil && flushErr != io.EOF {
				return "", flushErr
			}
			break
		}
	}

	if content.Len() == 0 {
		return "", fmt.Errorf("chat response is empty")
	}
	return content.String(), nil
}

func (c *openAIClient) post(ctx context.Context, url string, payload map[string]any) ([]byte, error) {
	return c.postWithAPIKey(ctx, url, c.apiKey, payload)
}

func (c *openAIClient) postWithAPIKey(ctx context.Context, url, apiKey string, payload map[string]any) ([]byte, error) {
	body, closeBody, err := c.doPost(ctx, url, apiKey, payload)
	if err != nil {
		return nil, err
	}
	defer closeBody()
	return io.ReadAll(body)
}

func (c *openAIClient) postStream(ctx context.Context, url string, payload map[string]any) (io.ReadCloser, error) {
	body, _, err := c.doPost(ctx, url, c.apiKey, payload)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func (c *openAIClient) postGoogle(ctx context.Context, url, apiKey string, payload map[string]any) ([]byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(apiKey) != "" {
		req.Header.Set("x-goog-api-key", strings.TrimSpace(apiKey))
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

func (c *openAIClient) doPost(ctx context.Context, url, apiKey string, payload map[string]any) (io.ReadCloser, func(), error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(apiKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	closeBody := func() {
		_ = resp.Body.Close()
	}
	if resp.StatusCode < 300 {
		return resp.Body, closeBody, nil
	}
	body, err := io.ReadAll(resp.Body)
	closeBody()
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, nil, fmt.Errorf("llm request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return io.NopCloser(bytes.NewReader(body)), func() {}, nil
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
