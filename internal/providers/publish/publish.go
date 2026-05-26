package publish

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/officecli/officecli/internal/httpclient"
)

type Config struct {
	Provider   string `json:"provider"`
	BaseURL    string `json:"base_url"`
	APIKey     string `json:"api_key"`
	Enabled    bool   `json:"enabled"`
	TimeoutSec int    `json:"timeout_sec"`
}

type PublishRequest struct {
	LocalFilePath string
	DocumentType  string
	DocumentName  string
}

type PublishResult struct {
	AccessURL string `json:"access_url"`
	Password  string `json:"password"`
	FileID    string `json:"file_id,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

type Publisher interface {
	Publish(ctx context.Context, req PublishRequest) (*PublishResult, error)
}

var EmbeddedPublishBaseURL = "https://platform.officecli.io"

func NewPublisher(cfg Config) (Publisher, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "", "http", "internal":
		return &httpPublisher{
			baseURL: strings.TrimRight(strings.TrimSpace(effectiveBaseURL(cfg)), "/"),
			apiKey:  strings.TrimSpace(cfg.APIKey),
			client:  httpclient.New(timeoutFor(cfg.TimeoutSec)),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported publish provider: %s", cfg.Provider)
	}
}

func ValidateConfig(cfg Config) error {
	if !cfg.Enabled {
		return nil
	}
	if strings.TrimSpace(effectiveBaseURL(cfg)) == "" {
		return errors.New("missing required configuration: online preview publishing service URL")
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return errors.New("missing required configuration: online preview publishing credential")
	}
	return nil
}

func effectiveBaseURL(cfg Config) string {
	if baseURL := strings.TrimSpace(cfg.BaseURL); baseURL != "" {
		return baseURL
	}
	return strings.TrimSpace(EmbeddedPublishBaseURL)
}

func timeoutFor(seconds int) time.Duration {
	if seconds <= 0 {
		return 60 * time.Second
	}
	return time.Duration(seconds) * time.Second
}

type httpPublisher struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func (p *httpPublisher) Publish(ctx context.Context, req PublishRequest) (*PublishResult, error) {
	if p == nil || p.baseURL == "" {
		return nil, fmt.Errorf("publish endpoint is unavailable")
	}
	file, err := os.Open(req.LocalFilePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("document_type", req.DocumentType)
	_ = writer.WriteField("document_name", req.DocumentName)
	part, err := writer.CreateFormFile("file", filepath.Base(req.LocalFilePath))
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/api/publish", &body)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("publish request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var result PublishResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decode publish response: %w", err)
	}
	if strings.TrimSpace(result.AccessURL) == "" {
		return nil, fmt.Errorf("publish response missing access_url")
	}
	return &result, nil
}
