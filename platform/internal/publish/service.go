package publish

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/officecli/officecli/platform/internal/model"
)

type APIKeyStore interface {
	FindByHash(ctx context.Context, hash string) (*model.APIKey, error)
	TouchLastUsedAt(ctx context.Context, id uint64, usedAt time.Time) error
}

type Config struct {
	BaseURL              string
	AuthKey              string
	AuthKeyID            string
	AuthSharedSecret     string
	HashSalt             string
	TimeoutSec           int
	DefaultExpireSeconds int
}

type Service struct {
	store  APIKeyStore
	cfg    Config
	client *http.Client
}

type Request struct {
	FileName         string
	DocumentType     string
	DocumentName     string
	ExpiresInSeconds int
	ContentType      string
	Reader           io.Reader
}

type Result struct {
	AccessURL string     `json:"access_url"`
	Password  string     `json:"password"`
	FileID    string     `json:"file_id,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

func NewService(store APIKeyStore, cfg Config) *Service {
	return &Service{
		store:  store,
		cfg:    cfg,
		client: &http.Client{Timeout: timeoutFor(cfg.TimeoutSec)},
	}
}

func (s *Service) Publish(ctx context.Context, bearer string, req Request) (*Result, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("publish service unavailable")
	}
	if strings.TrimSpace(s.cfg.BaseURL) == "" {
		return nil, fmt.Errorf("claudeoffice base url is required")
	}
	key, err := s.authorize(ctx, bearer)
	if err != nil {
		return nil, err
	}
	if err := validateRequest(req); err != nil {
		return nil, err
	}

	upload, err := s.requestUploadToken(ctx, req.FileName)
	if err != nil {
		return nil, err
	}
	if err := s.upload(ctx, upload.UploadURL, req.ContentType, req.Reader); err != nil {
		return nil, err
	}

	expiresIn := req.ExpiresInSeconds
	if expiresIn <= 0 {
		expiresIn = s.cfg.DefaultExpireSeconds
	}
	if expiresIn <= 0 {
		expiresIn = 24 * 60 * 60
	}
	expiresAt := time.Now().UTC().Add(time.Duration(expiresIn) * time.Second)
	result, err := s.createPreviewShare(ctx, upload.StorageKey, req.DocumentName, req.DocumentType, expiresAt)
	if err != nil {
		_ = s.deleteUploadedObject(ctx, upload.StorageKey)
		return nil, err
	}
	_ = s.store.TouchLastUsedAt(ctx, key.ID, time.Now().UTC())
	return result, nil
}

type uploadTokenResponse struct {
	UploadURL  string `json:"upload_url"`
	StorageKey string `json:"storage_key"`
}

func (s *Service) authorize(ctx context.Context, bearer string) (*model.APIKey, error) {
	keyValue := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(bearer), "Bearer "))
	if keyValue == "" {
		return nil, fmt.Errorf("missing api key")
	}
	hash := hashAPIKey(keyValue, s.cfg.HashSalt)
	key, err := s.store.FindByHash(ctx, hash)
	if err != nil {
		return nil, err
	}
	switch {
	case key == nil:
		return nil, fmt.Errorf("invalid api key")
	case key.Status != model.APIKeyStatusActive:
		return nil, fmt.Errorf("api key is disabled")
	case key.ExpiresAt != nil && key.ExpiresAt.Before(time.Now().UTC()):
		return nil, fmt.Errorf("api key is expired")
	case !hasPublishEntitlement(key):
		return nil, fmt.Errorf("publish entitlement is required")
	default:
		return key, nil
	}
}

func hasPublishEntitlement(key *model.APIKey) bool {
	if key == nil {
		return false
	}
	if key.QuotaTotal != nil && *key.QuotaTotal > 0 {
		return true
	}
	if key.QuotaUsed > 0 {
		return true
	}
	if key.CreditBalance > 0 || key.CreditReserved > 0 {
		return true
	}
	return false
}

func validateRequest(req Request) error {
	fileName := strings.TrimSpace(req.FileName)
	docName := strings.TrimSpace(req.DocumentName)
	docType := strings.ToLower(strings.TrimSpace(req.DocumentType))
	if fileName == "" || docName == "" || req.Reader == nil {
		return fmt.Errorf("file, document_type and document_name are required")
	}
	switch docType {
	case "pptx", "docx", "xlsx":
		return nil
	default:
		return fmt.Errorf("unsupported document_type")
	}
}

func (s *Service) requestUploadToken(ctx context.Context, fileName string) (*uploadTokenResponse, error) {
	rawBody, err := s.postJSON(ctx, "/api/attachment/upload/token", map[string]string{"fileName": fileName})
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Data uploadTokenResponse `json:"data"`
	}
	if err := json.Unmarshal(rawBody, &envelope); err != nil {
		return nil, fmt.Errorf("decode upload token response: %w", err)
	}
	if strings.TrimSpace(envelope.Data.UploadURL) == "" || strings.TrimSpace(envelope.Data.StorageKey) == "" {
		return nil, fmt.Errorf("upload token response missing fields")
	}
	return &envelope.Data, nil
}

func (s *Service) upload(ctx context.Context, uploadURL, contentType string, reader io.Reader) error {
	if strings.TrimSpace(contentType) == "" {
		contentType = "application/octet-stream"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("upload file failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func (s *Service) createPreviewShare(ctx context.Context, storageKey, documentName, documentType string, expiresAt time.Time) (*Result, error) {
	rawBody, err := s.postJSON(ctx, "/api/preview-shares", map[string]any{
		"source_type":   "storage_key",
		"source_value":  storageKey,
		"file_name":     documentName,
		"file_type":     documentType,
		"expires_at":    expiresAt.Format(time.RFC3339),
		"readonly":      true,
		"password_mode": "auto",
	})
	if err != nil {
		return nil, err
	}
	var result Result
	if err := json.Unmarshal(rawBody, &result); err != nil {
		return nil, fmt.Errorf("decode preview share response: %w", err)
	}
	if strings.TrimSpace(result.AccessURL) == "" || strings.TrimSpace(result.Password) == "" {
		return nil, fmt.Errorf("preview share response missing fields")
	}
	return &result, nil
}

func (s *Service) deleteUploadedObject(ctx context.Context, storageKey string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(s.cfg.BaseURL, "/")+"/api/attachment/delete?storage_key="+url.QueryEscape(storageKey), nil)
	if err != nil {
		return err
	}
	s.attachAuth(req, nil)
	resp, err := s.client.Do(req)
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

func (s *Service) postJSON(ctx context.Context, path string, payload any) ([]byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(s.cfg.BaseURL, "/")+path, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	s.attachAuth(req, raw)
	resp, err := s.client.Do(req)
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

func (s *Service) attachAuth(req *http.Request, body []byte) {
	if req == nil {
		return
	}
	if strings.TrimSpace(s.cfg.AuthSharedSecret) != "" {
		timestamp := strconv.FormatInt(time.Now().UTC().Unix(), 10)
		req.Header.Set("X-Auth-Key-Id", strings.TrimSpace(s.cfg.AuthKeyID))
		req.Header.Set("X-Auth-Timestamp", timestamp)
		req.Header.Set("X-Auth-Signature", signDynamic(strings.TrimSpace(s.cfg.AuthSharedSecret), timestamp, req.Method, canonicalPath(req), bodySHA256Hex(body)))
		return
	}
	if strings.TrimSpace(s.cfg.AuthKey) != "" {
		req.Header.Set("X-Auth-Key", strings.TrimSpace(s.cfg.AuthKey))
	}
}

func timeoutFor(seconds int) time.Duration {
	if seconds <= 0 {
		return 60 * time.Second
	}
	return time.Duration(seconds) * time.Second
}

func hashAPIKey(apiKey, salt string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(salt) + ":" + strings.TrimSpace(apiKey)))
	return hex.EncodeToString(sum[:])
}
