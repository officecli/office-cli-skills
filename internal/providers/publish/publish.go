package publish

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
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

var EmbeddedPublishBaseURL = "https://claudeoffice.com"
var EmbeddedPublishAuthKeyID = "officecli-cli"
var EmbeddedPublishAuthKey = ""

func NewPublisher(cfg Config) (Publisher, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "", "http", "internal":
		if canUseEmbeddedDynamicAuth(cfg) {
			return &claudeOfficePublisher{
				baseURL: normalizeEmbeddedBaseURL(effectiveBaseURL(cfg)),
				keyID:   strings.TrimSpace(EmbeddedPublishAuthKeyID),
				secret:  strings.TrimSpace(EmbeddedPublishAuthKey),
				client:  &http.Client{Timeout: timeoutFor(cfg.TimeoutSec)},
			}, nil
		}
		return &httpPublisher{
			baseURL: strings.TrimRight(strings.TrimSpace(effectiveBaseURL(cfg)), "/"),
			apiKey:  strings.TrimSpace(cfg.APIKey),
			client:  &http.Client{Timeout: timeoutFor(cfg.TimeoutSec)},
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
	if strings.TrimSpace(cfg.APIKey) == "" && !canUseEmbeddedDynamicAuth(cfg) {
		return errors.New("missing required configuration: online preview publishing credential")
	}
	return nil
}

func SupportsEmbeddedDynamicAuth() bool {
	return strings.TrimSpace(EmbeddedPublishAuthKey) != ""
}

func effectiveBaseURL(cfg Config) string {
	if baseURL := strings.TrimSpace(cfg.BaseURL); baseURL != "" {
		return baseURL
	}
	if SupportsEmbeddedDynamicAuth() {
		return strings.TrimSpace(EmbeddedPublishBaseURL)
	}
	return ""
}

func canUseEmbeddedDynamicAuth(cfg Config) bool {
	return SupportsEmbeddedDynamicAuth() && strings.TrimSpace(effectiveBaseURL(cfg)) != ""
}

func normalizeEmbeddedBaseURL(raw string) string {
	baseURL := strings.TrimRight(strings.TrimSpace(raw), "/")
	return strings.TrimSuffix(baseURL, "/api")
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

type claudeOfficePublisher struct {
	baseURL string
	keyID   string
	secret  string
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

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/publish", &body)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
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

func (p *claudeOfficePublisher) Publish(ctx context.Context, req PublishRequest) (*PublishResult, error) {
	if p == nil || p.baseURL == "" {
		return nil, fmt.Errorf("publish endpoint is unavailable")
	}
	if strings.TrimSpace(p.secret) == "" {
		return nil, fmt.Errorf("publish embedded auth is unavailable")
	}
	file, err := os.Open(req.LocalFilePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	upload, err := p.uploadAttachment(ctx, filepath.Base(req.LocalFilePath), file)
	if err != nil {
		return nil, err
	}
	result, err := p.createPreviewShare(ctx, req.DocumentName, req.DocumentType, upload.StorageKey)
	if err != nil {
		_ = p.deleteUploadedObject(ctx, upload.StorageKey)
		return nil, err
	}
	return result, nil
}

const defaultPreviewShareTTL = 30 * 24 * time.Hour

type uploadTokenResponse struct {
	StorageKey string `json:"storage_key"`
}

func (p *claudeOfficePublisher) uploadAttachment(ctx context.Context, fileName string, file *os.File) (*uploadTokenResponse, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("fileName", filepath.Base(strings.TrimSpace(fileName))); err != nil {
		return nil, err
	}
	part, err := writer.CreateFormFile("file", filepath.Base(strings.TrimSpace(fileName)))
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(part, file); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/api/attachment/upload", &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	p.attachDynamicAuth(req, body.Bytes())

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("claudeoffice request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(rawBody)))
	}
	var envelope struct {
		Data uploadTokenResponse `json:"data"`
	}
	if err := json.Unmarshal(rawBody, &envelope); err != nil {
		return nil, fmt.Errorf("decode upload response: %w", err)
	}
	if strings.TrimSpace(envelope.Data.StorageKey) == "" {
		return nil, fmt.Errorf("upload response missing storage_key")
	}
	return &envelope.Data, nil
}

func (p *claudeOfficePublisher) createPreviewShare(ctx context.Context, documentName, documentType, storageKey string) (*PublishResult, error) {
	expiresAt := time.Now().UTC().Add(defaultPreviewShareTTL)
	payload := map[string]any{
		"source_type":   "storage_key",
		"source_value":  storageKey,
		"file_name":     documentName,
		"file_type":     documentType,
		"expires_at":    expiresAt.Format(time.RFC3339),
		"readonly":      true,
		"password_mode": "auto",
	}
	rawBody, err := p.postJSON(ctx, "/api/preview-shares", payload)
	if err != nil {
		return nil, err
	}
	var result PublishResult
	if err := json.Unmarshal(rawBody, &result); err != nil {
		return nil, fmt.Errorf("decode preview share response: %w", err)
	}
	if strings.TrimSpace(result.AccessURL) == "" {
		return nil, fmt.Errorf("publish response missing access_url")
	}
	return &result, nil
}

func (p *claudeOfficePublisher) deleteUploadedObject(ctx context.Context, storageKey string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/api/attachment/delete?storage_key="+storageKey, nil)
	if err != nil {
		return err
	}
	p.attachDynamicAuth(req, nil)
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete uploaded object failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func (p *claudeOfficePublisher) postJSON(ctx context.Context, path string, payload any) ([]byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+path, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	p.attachDynamicAuth(req, raw)
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("claudeoffice request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

func (p *claudeOfficePublisher) attachDynamicAuth(req *http.Request, body []byte) {
	nonce, err := newRequestNonce()
	if err != nil {
		return
	}
	timestamp := strconv.FormatInt(time.Now().UTC().Unix(), 10)
	req.Header.Set("X-Auth-Key-Id", strings.TrimSpace(p.keyID))
	req.Header.Set("X-Auth-Timestamp", timestamp)
	req.Header.Set("X-Auth-Nonce", nonce)
	req.Header.Set("X-Auth-Signature", signDynamic(strings.TrimSpace(p.secret), timestamp, req.Method, canonicalPath(req), bodySHA256Hex(body), nonce))
}

func canonicalPath(r *http.Request) string {
	if r == nil || r.URL == nil {
		return "/"
	}
	path := r.URL.Path
	if path == "" {
		path = "/"
	}
	values := r.URL.Query()
	if len(values) == 0 {
		return path
	}
	normalized := make([]string, 0, len(values))
	for key, vals := range values {
		sorted := append([]string(nil), vals...)
		sort.Strings(sorted)
		for _, value := range sorted {
			normalized = append(normalized, key+"="+value)
		}
	}
	sort.Strings(normalized)
	if len(normalized) == 0 {
		return path
	}
	return path + "?" + strings.Join(normalized, "&")
}

func bodySHA256Hex(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func newRequestNonce() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate request nonce: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func signDynamic(secret, timestamp, method, path, bodyHash, nonce string) string {
	base := strings.Join([]string{
		strings.TrimSpace(timestamp),
		strings.ToUpper(strings.TrimSpace(method)),
		strings.TrimSpace(path),
		strings.TrimSpace(bodyHash),
		strings.TrimSpace(nonce),
	}, "\n")
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(base))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
