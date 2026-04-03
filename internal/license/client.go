package license

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Service struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func NewService(cfg Config) (*Service, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("缺少平台服务地址，请先完成 officecli 配置")
	}
	return &Service{
		baseURL: baseURL,
		apiKey:  strings.TrimSpace(cfg.APIKey),
		client:  &http.Client{Timeout: timeoutFor(cfg.TimeoutSec)},
	}, nil
}

func timeoutFor(seconds int) time.Duration {
	if seconds <= 0 {
		return 30 * time.Second
	}
	return time.Duration(seconds) * time.Second
}

func (s *Service) Check(ctx context.Context, req CheckRequest) (*CheckResult, error) {
	if s == nil {
		return &CheckResult{Allowed: true, AccessMode: AccessModePaid}, nil
	}
	body, err := s.post(ctx, "/license/check", req)
	if err != nil {
		return nil, err
	}
	var result CheckResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode license check response: %w", err)
	}
	if result.CommitToken.RequestID == "" {
		result.CommitToken = CommitToken{
			FingerprintHash: req.FingerprintHash,
			UserID:          req.UserID,
			RequestID:       buildRequestID(req),
			AccessMode:      result.AccessMode,
		}
	}
	return &result, nil
}

func (s *Service) Consume(ctx context.Context, token CommitToken) (*ConsumeResult, error) {
	if s == nil {
		return nil, nil
	}
	body, err := s.post(ctx, "/license/consume", ConsumeRequest{
		FingerprintHash: token.FingerprintHash,
		UserID:          token.UserID,
		RequestID:       token.RequestID,
		UsageType:       "generate",
		AccessMode:      token.AccessMode,
		APIKey:          s.apiKey,
	})
	if err != nil {
		return nil, err
	}
	var result ConsumeResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode license consume response: %w", err)
	}
	return &result, nil
}

func buildRequestID(req CheckRequest) string {
	raw := strings.TrimSpace(req.FingerprintHash) + "|" + strings.TrimSpace(req.DocumentType) + "|" + strings.TrimSpace(req.Action) + "|" + time.Now().UTC().Format(time.RFC3339Nano)
	return raw
}

func (s *Service) post(ctx context.Context, path string, payload any) ([]byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/api"+path, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if s.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.apiKey)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求平台服务失败: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("license request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}
