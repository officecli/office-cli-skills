package llm

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"

	"github.com/officecli/officecli-internal/engine"
	"github.com/officecli/officecli-internal/internal/httpclient"
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
	ImageAccess  *InternalImageAccess
}

type InternalImageAccess struct {
	FingerprintHash string
	UserID          uint64
	APIKey          string
	AccessMode      string
	CommitToken     json.RawMessage
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
		client:       httpclient.New(timeoutFor(p.cfg.TimeoutSec)),
	}, nil
}

type internalProvider struct{ cfg Config }

func (p *internalProvider) NewClient() (engine.LLMClient, error) {
	if strings.TrimSpace(p.cfg.BaseURL) == "" {
		return nil, fmt.Errorf("llm base_url is required")
	}
	return &internalClient{
		baseURL:     strings.TrimRight(strings.TrimSpace(p.cfg.BaseURL), "/"),
		apiKey:      strings.TrimSpace(p.cfg.APIKey),
		model:       strings.TrimSpace(p.cfg.Model),
		imageAccess: p.cfg.ImageAccess,
		client:      httpclient.New(timeoutFor(p.cfg.TimeoutSec)),
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
	if len(req.ReferenceImages) > 0 {
		return c.generateOpenAIImageEdit(ctx, imageBaseURL, imageAPIKey, model, req)
	}
	image, err := c.generateOpenAIImageGeneration(ctx, imageBaseURL, imageAPIKey, model, req)
	if err == nil {
		return image, nil
	}
	return c.generateOpenAIResponsesImage(ctx, imageBaseURL, imageAPIKey, model, req)
}

func (c *openAIClient) generateOpenAIImageGeneration(ctx context.Context, imageBaseURL, imageAPIKey, model string, req engine.ImageGenerationRequest) (*engine.ImageGenerationResult, error) {
	payload := map[string]any{
		"model":  model,
		"prompt": req.Prompt,
		"size":   resolveImageSize(req),
	}
	body, err := c.postWithAPIKey(ctx, imageBaseURL+"/images/generations", imageAPIKey, payload)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
			URL     string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode image response: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("image response is empty")
	}
	if url := strings.TrimSpace(resp.Data[0].URL); url != "" {
		return c.fetchImageURL(ctx, url)
	}
	if strings.TrimSpace(resp.Data[0].B64JSON) == "" {
		return nil, fmt.Errorf("image response is empty")
	}
	data, err := base64.StdEncoding.DecodeString(resp.Data[0].B64JSON)
	if err != nil {
		return nil, fmt.Errorf("decode image data: %w", err)
	}
	return &engine.ImageGenerationResult{Data: data, MIME: "image/png"}, nil
}

func (c *openAIClient) generateOpenAIImageEdit(ctx context.Context, baseURL, apiKey, model string, req engine.ImageGenerationRequest) (*engine.ImageGenerationResult, error) {
	if len(req.ReferenceImages) == 0 {
		return nil, fmt.Errorf("at least one reference image is required")
	}
	body, err := c.postMultipartImageEdit(ctx, strings.TrimRight(baseURL, "/")+"/images/edits", apiKey, map[string]string{
		"model":  model,
		"prompt": req.Prompt,
		"size":   resolveImageSize(req),
	}, req.ReferenceImages)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode image edit response: %w", err)
	}
	if len(resp.Data) == 0 || strings.TrimSpace(resp.Data[0].B64JSON) == "" {
		return nil, fmt.Errorf("image edit response is empty")
	}
	data, err := base64.StdEncoding.DecodeString(resp.Data[0].B64JSON)
	if err != nil {
		return nil, fmt.Errorf("decode image edit data: %w", err)
	}
	return &engine.ImageGenerationResult{Data: data, MIME: "image/png"}, nil
}

func isGoogleImageEndpoint(baseURL, model string) bool {
	baseURL = strings.ToLower(strings.TrimSpace(baseURL))
	return strings.Contains(baseURL, "generativelanguage.googleapis.com")
}

func (c *openAIClient) generateOpenAIResponsesImage(ctx context.Context, imageBaseURL, imageAPIKey, model string, req engine.ImageGenerationRequest) (*engine.ImageGenerationResult, error) {
	payload := map[string]any{
		"model": model,
		"input": req.Prompt,
		"tools": []map[string]any{{"type": "image_generation"}},
	}
	body, err := c.postWithAPIKey(ctx, imageBaseURL+"/responses", imageAPIKey, payload)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Output []struct {
			Type   string `json:"type"`
			Result string `json:"result"`
		} `json:"output"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode responses image response: %w", err)
	}
	for _, item := range resp.Output {
		if item.Type != "image_generation_call" || strings.TrimSpace(item.Result) == "" {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(item.Result)
		if err != nil {
			return nil, fmt.Errorf("decode responses image data: %w", err)
		}
		return &engine.ImageGenerationResult{Data: data, MIME: "image/png"}, nil
	}
	return nil, fmt.Errorf("responses image response is empty")
}

func (c *openAIClient) fetchImageURL(ctx context.Context, rawURL string) (*engine.ImageGenerationResult, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
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
		return nil, fmt.Errorf("fetch image failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	mime := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if mime == "" {
		mime = "image/png"
	}
	return &engine.ImageGenerationResult{Data: body, MIME: mime}, nil
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

func resolveImageSize(req engine.ImageGenerationRequest) string {
	if size := strings.TrimSpace(req.Size); size != "" {
		return size
	}
	return pickImageSize(req.TargetAspectRatio)
}

func (c *openAIClient) chatCompletion(ctx context.Context, payload map[string]any) (string, error) {
	var lastErr error
	for i, endpoint := range openAICompatibleEndpointCandidates(c.baseURL, "chat/completions") {
		content, err := c.chatCompletionAt(ctx, endpoint, payload)
		if err == nil {
			return content, nil
		}
		lastErr = err
		if i == 0 && shouldRetryV1Endpoint(err) {
			continue
		}
		return "", err
	}
	return "", lastErr
}

func (c *openAIClient) chatCompletionAt(ctx context.Context, endpoint string, payload map[string]any) (string, error) {
	body, err := c.post(ctx, endpoint, payload)
	if err != nil {
		if !requiresStreamingFallback(err) {
			return "", err
		}
		return c.chatCompletionStream(ctx, endpoint, payload)
	}
	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", llmInvalidJSONResponseError(body, err)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("chat response is empty")
	}
	content := resp.Choices[0].Message.Content
	if strings.TrimSpace(content) != "" {
		return content, nil
	}
	streamContent, err := c.chatCompletionStream(ctx, endpoint, payload)
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

func (c *openAIClient) chatCompletionStream(ctx context.Context, endpoint string, payload map[string]any) (string, error) {
	streamPayload := make(map[string]any, len(payload)+1)
	for key, value := range payload {
		streamPayload[key] = value
	}
	streamPayload["stream"] = true

	resp, err := c.postStream(ctx, endpoint, streamPayload)
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
			return llmInvalidStreamingPayloadError(payload, err)
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

func (c *openAIClient) postMultipartImageEdit(ctx context.Context, rawURL, apiKey string, fields map[string]string, refs []engine.ImageReference) ([]byte, error) {
	if len(refs) == 0 {
		return nil, fmt.Errorf("at least one reference image is required")
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return nil, err
		}
	}
	for i, ref := range refs {
		data, err := base64.StdEncoding.DecodeString(ref.Data)
		if err != nil {
			return nil, fmt.Errorf("decode reference image %d: %w", i, err)
		}
		filename := strings.TrimSpace(ref.Filename)
		if filename == "" {
			filename = fmt.Sprintf("reference-%d.png", i)
		}
		part, err := createImageEditPart(writer, filename, ref.MIME)
		if err != nil {
			return nil, err
		}
		if _, err := part.Write(data); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, &body)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	if strings.TrimSpace(apiKey) != "" {
		httpReq.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
	}
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("llm request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return respBody, nil
}

func createImageEditPart(writer *multipart.Writer, filename string, mime string) (io.Writer, error) {
	filename = strings.NewReplacer("\\", "\\\\", `"`, `\"`, "\r", "", "\n", "").Replace(filename)
	mime = strings.TrimSpace(mime)
	if mime == "" {
		mime = "application/octet-stream"
	}
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="image[]"; filename="%s"`, filename))
	header.Set("Content-Type", mime)
	return writer.CreatePart(header)
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
	baseURL     string
	apiKey      string
	model       string
	imageAccess *InternalImageAccess
	client      *http.Client
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
	if c.imageAccess != nil {
		payload["fingerprint_hash"] = c.imageAccess.FingerprintHash
		payload["user_id"] = c.imageAccess.UserID
		payload["api_key"] = c.imageAccess.APIKey
		payload["access_mode"] = c.imageAccess.AccessMode
		if len(c.imageAccess.CommitToken) > 0 {
			payload["commit_token"] = c.imageAccess.CommitToken
		}
	}
	if len(req.ReferenceImages) > 0 {
		payload["reference_image"] = req.ReferenceImages[0]
		payload["reference_images"] = req.ReferenceImages
	}
	if size := strings.TrimSpace(req.Size); size != "" {
		payload["size"] = size
	}
	body, err := c.post(ctx, c.baseURL+"/v1/image", payload)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data               string `json:"data"`
		MIME               string `json:"mime"`
		CreditBalance      *int   `json:"credit_balance,omitempty"`
		AccessMode         string `json:"access_mode,omitempty"`
		Remaining          int    `json:"remaining,omitempty"`
		FreeRemaining      int    `json:"free_remaining,omitempty"`
		RewardRemaining    int    `json:"reward_remaining,omitempty"`
		PaidQuotaRemaining int    `json:"paid_quota_remaining,omitempty"`
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
	return &engine.ImageGenerationResult{
		Data:               data,
		MIME:               resp.MIME,
		CreditBalance:      resp.CreditBalance,
		AccessMode:         resp.AccessMode,
		Remaining:          resp.Remaining,
		FreeRemaining:      resp.FreeRemaining,
		RewardRemaining:    resp.RewardRemaining,
		PaidQuotaRemaining: resp.PaidQuotaRemaining,
	}, nil
}

func (c *internalClient) complete(ctx context.Context, kind string, messages []engine.LLMMessage, extra map[string]any) (string, error) {
	payload := map[string]any{
		"model":    c.model,
		"messages": messages,
	}
	if c.imageAccess != nil {
		payload["fingerprint_hash"] = c.imageAccess.FingerprintHash
		payload["user_id"] = c.imageAccess.UserID
		payload["api_key"] = c.imageAccess.APIKey
		payload["access_mode"] = c.imageAccess.AccessMode
		if len(c.imageAccess.CommitToken) > 0 {
			payload["commit_token"] = c.imageAccess.CommitToken
		}
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
		return "", llmInvalidJSONResponseError(body, err)
	}
	if resp.Content == "" {
		return "", fmt.Errorf("internal completion response is empty")
	}
	return resp.Content, nil
}

func openAICompatibleEndpointCandidates(baseURL, endpointPath string) []string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	endpoints := []string{baseURL + "/" + strings.TrimLeft(endpointPath, "/")}
	if shouldOfferV1Fallback(baseURL) {
		endpoints = append(endpoints, baseURL+"/v1/"+strings.TrimLeft(endpointPath, "/"))
	}
	return endpoints
}

func shouldOfferV1Fallback(baseURL string) bool {
	u, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	return strings.Trim(strings.TrimSpace(u.Path), "/") == ""
}

func shouldRetryV1Endpoint(err error) bool {
	var invalidJSON *llmInvalidJSONError
	if !errors.As(err, &invalidJSON) {
		return false
	}
	return looksLikeHTMLResponse(invalidJSON.body)
}

func looksLikeHTMLResponse(body []byte) bool {
	trimmed := strings.TrimSpace(string(body))
	lower := strings.ToLower(trimmed)
	return strings.HasPrefix(lower, "<!doctype html") || strings.HasPrefix(lower, "<html")
}

type llmInvalidJSONError struct {
	body []byte
	err  error
}

func (e *llmInvalidJSONError) Error() string {
	return fmt.Sprintf("llm request failed: invalid json response: %v body=%s", e.err, strings.TrimSpace(string(e.body)))
}

func (e *llmInvalidJSONError) Unwrap() error {
	return e.err
}

func llmInvalidJSONResponseError(body []byte, err error) error {
	return &llmInvalidJSONError{body: bytes.Clone(body), err: err}
}

func llmInvalidStreamingPayloadError(payload string, err error) error {
	return fmt.Errorf("llm request failed: invalid streaming response: %w body=%s", err, strings.TrimSpace(payload))
}

func (c *internalClient) post(ctx context.Context, url string, payload map[string]any) ([]byte, error) {
	ensureInternalRequestID(payload)
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

func ensureInternalRequestID(payload map[string]any) {
	if payload == nil {
		return
	}
	if requestID, ok := payload["request_id"].(string); ok && strings.TrimSpace(requestID) != "" {
		return
	}
	if requestID := requestIDFromCommitToken(payload["commit_token"]); requestID != "" {
		payload["request_id"] = requestID
		return
	}
	payload["request_id"] = newInternalRequestID()
}

func requestIDFromCommitToken(value any) string {
	if value == nil {
		return ""
	}
	var raw []byte
	switch token := value.(type) {
	case json.RawMessage:
		raw = token
	case []byte:
		raw = token
	case map[string]any:
		if requestID, ok := token["request_id"].(string); ok {
			return strings.TrimSpace(requestID)
		}
	case map[string]string:
		return strings.TrimSpace(token["request_id"])
	default:
		marshaled, err := json.Marshal(token)
		if err != nil {
			return ""
		}
		raw = marshaled
	}
	if len(raw) == 0 {
		return ""
	}
	var parsed struct {
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ""
	}
	return strings.TrimSpace(parsed.RequestID)
}

func newInternalRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("llm-%d", time.Now().UnixNano())
	}
	return "llm-" + hex.EncodeToString(b[:])
}
