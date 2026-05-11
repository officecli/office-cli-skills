package hostedllm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"sync"
	"time"

	"github.com/officecli/officecli/platform/internal/apikey"
	licensesvc "github.com/officecli/officecli/platform/internal/license"
	"github.com/officecli/officecli/platform/internal/model"
)

type APIKeyStore interface {
	FindAPIKeyByHash(ctx context.Context, hash string) (*model.APIKey, error)
	ReserveCreditsByHash(ctx context.Context, hash string, credits int) (*model.APIKey, error)
	ReleaseReservedCredits(ctx context.Context, apiKeyID uint64, reserved int) (*model.APIKey, error)
	SettleReservedCredits(ctx context.Context, apiKeyID uint64, reserved int, settled int) (*model.APIKey, error)
	CreateUsageEvent(ctx context.Context, event *model.UsageEvent) error
	FindUserAIGatewayAPIKeyByUserID(ctx context.Context, userID uint64) (*model.UserAIGatewayAPIKey, error)
	ClaimUserAIGatewayAPIKeyCreation(ctx context.Context, userID uint64, upstreamName string) (*model.UserAIGatewayAPIKey, error)
	ActivateUserAIGatewayAPIKey(ctx context.Context, userID uint64, ciphertext, prefix, upstreamID, upstreamName string) (*model.UserAIGatewayAPIKey, error)
	MarkUserAIGatewayAPIKeyCreationError(ctx context.Context, userID uint64, upstreamName, message string) (*model.UserAIGatewayAPIKey, error)
}

type GenerationQuotaManager interface {
	ValidateCommitToken(req licensesvc.ConsumeRequest) error
	Consume(ctx context.Context, req licensesvc.ConsumeRequest) (*licensesvc.ConsumeResponse, error)
}

type Config struct {
	BaseURL                string
	APIKey                 string
	TextModel              string
	ImageModel             string
	Provider               string
	HashSalt               string
	Rules                  []model.HostedPricingRule
	TimeoutSec             int
	AIGatewayAdminBaseURL  string
	AIGatewayAdminAPIKey   string
	AIGatewayCreateKeyPath string
	AIGatewayKeyCipher     *apikey.Cipher
	AIGatewayAdminClient   AIGatewayAdminClient
}

type Service struct {
	store          APIKeyStore
	quota          GenerationQuotaManager
	cfg            Config
	mu             sync.RWMutex
	createKeyLocks sync.Map
	client         *http.Client
}

type CompletionRequest struct {
	RequestID  string          `json:"request_id,omitempty"`
	Model      string          `json:"model"`
	Messages   []ChatMessage   `json:"messages"`
	Kind       string          `json:"-"`
	JSONMode   bool            `json:"-"`
	SchemaName string          `json:"schema_name,omitempty"`
	Strict     bool            `json:"strict,omitempty"`
	Schema     json.RawMessage `json:"schema,omitempty"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type CompletionResponse struct {
	Content       string
	CreditBalance int
}

type ImageRequest struct {
	RequestID       string                  `json:"request_id,omitempty"`
	Model           string                  `json:"model"`
	Prompt          string                  `json:"prompt"`
	AspectRatio     float64                 `json:"aspect_ratio"`
	Size            string                  `json:"size,omitempty"`
	FingerprintHash string                  `json:"fingerprint_hash,omitempty"`
	UserID          uint64                  `json:"user_id,omitempty"`
	APIKey          string                  `json:"api_key,omitempty"`
	AccessMode      model.AccessMode        `json:"access_mode,omitempty"`
	CommitToken     *licensesvc.CommitToken `json:"commit_token,omitempty"`
	ReferenceImage  *ImageReference         `json:"reference_image,omitempty"`
	ReferenceImages []ImageReference        `json:"reference_images,omitempty"`
}

type ImageReference struct {
	Filename string `json:"filename,omitempty"`
	MIME     string `json:"mime"`
	Data     string `json:"data"`
}

type ImageResponse struct {
	Data               []byte
	MIME               string
	CreditBalance      int
	AccessMode         model.AccessMode
	Remaining          int
	FreeRemaining      int
	RewardRemaining    int
	PaidQuotaRemaining int
}

func NewService(store APIKeyStore, cfg Config, quotaManagers ...GenerationQuotaManager) *Service {
	timeout := timeoutFor(cfg.TimeoutSec)
	var quota GenerationQuotaManager
	if len(quotaManagers) > 0 {
		quota = quotaManagers[0]
	}
	return &Service{
		store:  store,
		quota:  quota,
		cfg:    cfg,
		client: &http.Client{Timeout: timeout},
	}
}

func (s *Service) HostedPricingRules() []model.HostedPricingRule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.HostedPricingRule, len(s.cfg.Rules))
	copy(out, s.cfg.Rules)
	return out
}

func (s *Service) Complete(ctx context.Context, bearer string, req CompletionRequest) (*CompletionResponse, error) {
	key, hash, err := s.authorize(ctx, bearer)
	if err != nil {
		return nil, err
	}
	upstreamAPIKey, err := s.upstreamAPIKeyForOfficeKey(ctx, key)
	if err != nil {
		return nil, err
	}
	reservation := s.reserveCreditsForModel(req.Model, false)
	key, err = s.store.ReserveCreditsByHash(ctx, hash, reservation)
	if err != nil {
		return nil, err
	}
	modelName := s.normalizeModel(req.Model, false)

	payload := map[string]any{
		"model":    modelName,
		"messages": req.Messages,
	}
	if req.JSONMode {
		payload["response_format"] = map[string]any{"type": "json_object"}
	}
	if len(req.Schema) > 0 {
		var schema map[string]any
		if err := json.Unmarshal(req.Schema, &schema); err != nil {
			_, _ = s.store.ReleaseReservedCredits(ctx, key.ID, reservation)
			return nil, fmt.Errorf("parse schema: %w", err)
		}
		payload["response_format"] = map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   req.SchemaName,
				"strict": req.Strict,
				"schema": schema,
			},
		}
	}

	body, usage, err := s.post(ctx, strings.TrimRight(s.cfg.BaseURL, "/")+"/chat/completions", payload, upstreamAPIKey)
	if err != nil {
		updatedKey, _ := s.store.ReleaseReservedCredits(ctx, key.ID, reservation)
		s.recordUsage(ctx, key.ID, req, modelName, usage, reservation, 0, reservation, updatedKey)
		return nil, err
	}
	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		updatedKey, _ := s.store.ReleaseReservedCredits(ctx, key.ID, reservation)
		s.recordUsage(ctx, key.ID, req, modelName, usage, reservation, 0, reservation, updatedKey)
		return nil, fmt.Errorf("decode hosted completion: %w", err)
	}
	if len(resp.Choices) == 0 {
		updatedKey, _ := s.store.ReleaseReservedCredits(ctx, key.ID, reservation)
		s.recordUsage(ctx, key.ID, req, modelName, usage, reservation, 0, reservation, updatedKey)
		return nil, fmt.Errorf("hosted completion is empty")
	}
	settled := s.settleCredits(req.Model, usage, false)
	if settled > reservation {
		settled = reservation
	}
	updatedKey, err := s.store.SettleReservedCredits(ctx, key.ID, reservation, settled)
	if err != nil {
		return nil, err
	}
	s.recordUsage(ctx, key.ID, req, modelName, usage, reservation, settled, reservation-settled, updatedKey)
	return &CompletionResponse{
		Content:       resp.Choices[0].Message.Content,
		CreditBalance: creditBalance(updatedKey),
	}, nil
}

func (s *Service) GenerateImage(ctx context.Context, bearer string, req ImageRequest) (*ImageResponse, error) {
	if req.CommitToken != nil {
		return s.generateQuotaImage(ctx, bearer, req)
	}
	key, hash, err := s.authorize(ctx, bearer)
	if err != nil {
		return nil, err
	}
	upstreamAPIKey, err := s.upstreamAPIKeyForOfficeKey(ctx, key)
	if err != nil {
		return nil, err
	}
	reservation := s.reserveCreditsForModel(req.Model, true)
	key, err = s.store.ReserveCreditsByHash(ctx, hash, reservation)
	if err != nil {
		return nil, err
	}
	modelName := s.normalizeModel(req.Model, true)
	payload := map[string]any{
		"model":           modelName,
		"prompt":          req.Prompt,
		"size":            effectiveImageSize(req),
		"response_format": "b64_json",
	}
	body, usage, err := s.postImageRequest(ctx, modelName, req, payload, upstreamAPIKey)
	if err != nil {
		updatedKey, _ := s.store.ReleaseReservedCredits(ctx, key.ID, reservation)
		s.recordImageUsage(ctx, key.ID, req, modelName, usage, reservation, 0, reservation, updatedKey)
		return nil, err
	}
	var resp struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		updatedKey, _ := s.store.ReleaseReservedCredits(ctx, key.ID, reservation)
		s.recordImageUsage(ctx, key.ID, req, modelName, usage, reservation, 0, reservation, updatedKey)
		return nil, fmt.Errorf("decode hosted image: %w", err)
	}
	if len(resp.Data) == 0 || strings.TrimSpace(resp.Data[0].B64JSON) == "" {
		updatedKey, _ := s.store.ReleaseReservedCredits(ctx, key.ID, reservation)
		s.recordImageUsage(ctx, key.ID, req, modelName, usage, reservation, 0, reservation, updatedKey)
		return nil, fmt.Errorf("hosted image is empty")
	}
	data, err := base64.StdEncoding.DecodeString(resp.Data[0].B64JSON)
	if err != nil {
		updatedKey, _ := s.store.ReleaseReservedCredits(ctx, key.ID, reservation)
		s.recordImageUsage(ctx, key.ID, req, modelName, usage, reservation, 0, reservation, updatedKey)
		return nil, fmt.Errorf("decode hosted image data: %w", err)
	}
	usage.ImageCount = 1
	settled := s.settleCredits(req.Model, usage, true)
	if settled > reservation {
		settled = reservation
	}
	updatedKey, err := s.store.SettleReservedCredits(ctx, key.ID, reservation, settled)
	if err != nil {
		return nil, err
	}
	s.recordImageUsage(ctx, key.ID, req, modelName, usage, reservation, settled, reservation-settled, updatedKey)
	return &ImageResponse{
		Data:          data,
		MIME:          "image/png",
		CreditBalance: creditBalance(updatedKey),
	}, nil
}

func (s *Service) generateQuotaImage(ctx context.Context, bearer string, req ImageRequest) (*ImageResponse, error) {
	if s.quota == nil {
		return nil, fmt.Errorf("generation quota service is unavailable")
	}
	consumeReq := imageConsumeRequest(req, bearer)
	if err := s.quota.ValidateCommitToken(consumeReq); err != nil {
		return nil, err
	}
	modelName := s.normalizeModel(req.Model, true)
	payload := map[string]any{
		"model":           modelName,
		"prompt":          req.Prompt,
		"size":            effectiveImageSize(req),
		"response_format": "b64_json",
	}
	body, _, err := s.postImageRequest(ctx, modelName, req, payload, strings.TrimSpace(s.cfg.APIKey))
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode hosted image: %w", err)
	}
	if len(resp.Data) == 0 || strings.TrimSpace(resp.Data[0].B64JSON) == "" {
		return nil, fmt.Errorf("hosted image is empty")
	}
	data, err := base64.StdEncoding.DecodeString(resp.Data[0].B64JSON)
	if err != nil {
		return nil, fmt.Errorf("decode hosted image data: %w", err)
	}
	consumeResp, err := s.quota.Consume(ctx, consumeReq)
	if err != nil {
		return nil, err
	}
	return &ImageResponse{
		Data:               data,
		MIME:               "image/png",
		AccessMode:         consumeResp.AccessMode,
		Remaining:          consumeResp.Remaining,
		FreeRemaining:      consumeResp.FreeRemaining,
		RewardRemaining:    consumeResp.RewardRemaining,
		PaidQuotaRemaining: consumeResp.PaidQuotaRemaining,
		CreditBalance:      consumeResp.CreditBalance,
	}, nil
}

type usageSummary struct {
	PromptTokens     int
	CompletionTokens int
	ReasoningTokens  int
	ImageCount       int
}

func (s *Service) post(ctx context.Context, url string, payload map[string]any, upstreamAPIKey string) ([]byte, usageSummary, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, usageSummary{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, usageSummary{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(upstreamAPIKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(upstreamAPIKey))
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, usageSummary{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, usageSummary{}, err
	}
	if resp.StatusCode >= 300 {
		return nil, usageSummary{}, fmt.Errorf("hosted upstream request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var envelope struct {
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			ReasoningTokens  int `json:"reasoning_tokens"`
		} `json:"usage"`
	}
	_ = json.Unmarshal(body, &envelope)
	return body, usageSummary{
		PromptTokens:     envelope.Usage.PromptTokens,
		CompletionTokens: envelope.Usage.CompletionTokens,
		ReasoningTokens:  envelope.Usage.ReasoningTokens,
	}, nil
}

func (s *Service) postImageRequest(ctx context.Context, modelName string, req ImageRequest, payload map[string]any, upstreamAPIKey string) ([]byte, usageSummary, error) {
	refs := effectiveReferenceImages(req)
	if len(refs) == 0 {
		return s.post(ctx, strings.TrimRight(s.cfg.BaseURL, "/")+"/images/generations", payload, upstreamAPIKey)
	}
	fields := map[string]string{
		"model":  modelName,
		"prompt": req.Prompt,
		"size":   effectiveImageSize(req),
	}
	return s.postImageEdit(ctx, strings.TrimRight(s.cfg.BaseURL, "/")+"/images/edits", fields, refs, upstreamAPIKey)
}

func (s *Service) postImageEdit(ctx context.Context, rawURL string, fields map[string]string, refs []ImageReference, upstreamAPIKey string) ([]byte, usageSummary, error) {
	if len(refs) == 0 {
		return nil, usageSummary{}, fmt.Errorf("at least one reference image is required")
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return nil, usageSummary{}, err
		}
	}
	for i, ref := range refs {
		imageData, err := base64.StdEncoding.DecodeString(ref.Data)
		if err != nil {
			return nil, usageSummary{}, fmt.Errorf("decode reference image %d: %w", i, err)
		}
		filename := strings.TrimSpace(ref.Filename)
		if filename == "" {
			filename = fmt.Sprintf("reference-%d.png", i)
		}
		part, err := createImageEditPart(writer, filename, ref.MIME)
		if err != nil {
			return nil, usageSummary{}, err
		}
		if _, err := part.Write(imageData); err != nil {
			return nil, usageSummary{}, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, usageSummary{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, &body)
	if err != nil {
		return nil, usageSummary{}, err
	}
	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	if strings.TrimSpace(upstreamAPIKey) != "" {
		httpReq.Header.Set("Authorization", "Bearer "+strings.TrimSpace(upstreamAPIKey))
	}
	resp, err := s.client.Do(httpReq)
	if err != nil {
		return nil, usageSummary{}, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, usageSummary{}, err
	}
	if resp.StatusCode >= 300 {
		return nil, usageSummary{}, fmt.Errorf("hosted upstream request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	var envelope struct {
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			ReasoningTokens  int `json:"reasoning_tokens"`
		} `json:"usage"`
	}
	_ = json.Unmarshal(respBody, &envelope)
	return respBody, usageSummary{
		PromptTokens:     envelope.Usage.PromptTokens,
		CompletionTokens: envelope.Usage.CompletionTokens,
		ReasoningTokens:  envelope.Usage.ReasoningTokens,
	}, nil
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

func (s *Service) authorize(ctx context.Context, bearer string) (*model.APIKey, string, error) {
	keyValue := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(bearer), "Bearer "))
	if keyValue == "" {
		return nil, "", fmt.Errorf("missing api key")
	}
	hash := hashAPIKey(keyValue, s.cfg.HashSalt)
	key, err := s.store.FindAPIKeyByHash(ctx, hash)
	if err != nil {
		return nil, "", err
	}
	switch {
	case key == nil:
		return nil, "", fmt.Errorf("invalid api key")
	case key.Status != model.APIKeyStatusActive:
		return nil, "", fmt.Errorf("api key is disabled")
	case !key.SupportsHosted():
		return nil, "", fmt.Errorf("hosted mode is not enabled for this key")
	case key.OwnerUserID == nil || *key.OwnerUserID == 0:
		return nil, "", fmt.Errorf("hosted mode requires an owner user for the officecli api key")
	}
	return key, hash, nil
}

func (s *Service) upstreamAPIKeyForOfficeKey(ctx context.Context, key *model.APIKey) (string, error) {
	if key == nil || key.OwnerUserID == nil || *key.OwnerUserID == 0 {
		return "", fmt.Errorf("hosted mode requires an owner user for the officecli api key")
	}
	userID := *key.OwnerUserID
	if existing, err := s.store.FindUserAIGatewayAPIKeyByUserID(ctx, userID); err != nil {
		return "", err
	} else if existing != nil && existing.Status == model.UserAIGatewayAPIKeyStatusActive && strings.TrimSpace(existing.KeyCiphertext) != "" {
		return s.decryptUserAIGatewayAPIKey(existing)
	}

	lockValue, _ := s.createKeyLocks.LoadOrStore(userID, &sync.Mutex{})
	lock := lockValue.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	if existing, err := s.store.FindUserAIGatewayAPIKeyByUserID(ctx, userID); err != nil {
		return "", err
	} else if existing != nil && existing.Status == model.UserAIGatewayAPIKeyStatusActive && strings.TrimSpace(existing.KeyCiphertext) != "" {
		return s.decryptUserAIGatewayAPIKey(existing)
	}

	upstreamName := fmt.Sprintf("officecli-user-%d", userID)
	claimed, err := s.store.ClaimUserAIGatewayAPIKeyCreation(ctx, userID, upstreamName)
	if err != nil {
		return "", err
	}
	if claimed != nil && claimed.Status == model.UserAIGatewayAPIKeyStatusActive && strings.TrimSpace(claimed.KeyCiphertext) != "" {
		return s.decryptUserAIGatewayAPIKey(claimed)
	}
	if claimed == nil || !claimed.CreationClaimed {
		return "", fmt.Errorf("aigateway api key creation is already in progress for user_id=%d", userID)
	}
	created, err := s.aigatewayAdminClient().CreateAPIKey(ctx, CreateAIGatewayAPIKeyRequest{Name: upstreamName})
	if err != nil {
		_, _ = s.store.MarkUserAIGatewayAPIKeyCreationError(ctx, userID, upstreamName, err.Error())
		return "", err
	}
	plain := strings.TrimSpace(created.PlaintextKey)
	if plain == "" {
		err := fmt.Errorf("aigateway api key creation response did not include an api key")
		_, _ = s.store.MarkUserAIGatewayAPIKeyCreationError(ctx, userID, upstreamName, err.Error())
		return "", err
	}
	cipher := s.cfg.AIGatewayKeyCipher
	if cipher == nil {
		err := fmt.Errorf("aigateway api key cipher is not configured")
		_, _ = s.store.MarkUserAIGatewayAPIKeyCreationError(ctx, userID, upstreamName, err.Error())
		return "", err
	}
	ciphertext, err := cipher.Encrypt(plain)
	if err != nil {
		_, _ = s.store.MarkUserAIGatewayAPIKeyCreationError(ctx, userID, upstreamName, err.Error())
		return "", err
	}
	name := strings.TrimSpace(created.Name)
	if name == "" {
		name = upstreamName
	}
	if _, err := s.store.ActivateUserAIGatewayAPIKey(ctx, userID, ciphertext, apiKeyPrefix(plain), created.UpstreamID, name); err != nil {
		return "", err
	}
	return plain, nil
}

func (s *Service) decryptUserAIGatewayAPIKey(key *model.UserAIGatewayAPIKey) (string, error) {
	cipher := s.cfg.AIGatewayKeyCipher
	if cipher == nil {
		return "", fmt.Errorf("aigateway api key cipher is not configured")
	}
	plain, err := cipher.Decrypt(key.KeyCiphertext)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(plain) == "" {
		return "", fmt.Errorf("stored aigateway api key is empty")
	}
	return strings.TrimSpace(plain), nil
}

func (s *Service) aigatewayAdminClient() AIGatewayAdminClient {
	if s.cfg.AIGatewayAdminClient != nil {
		return s.cfg.AIGatewayAdminClient
	}
	return newHTTPAIGatewayAdminClient(s.client, s.cfg)
}

func apiKeyPrefix(key string) string {
	trimmed := strings.TrimSpace(key)
	if len(trimmed) <= 12 {
		return trimmed
	}
	return trimmed[:12]
}

func imageConsumeRequest(req ImageRequest, bearer string) licensesvc.ConsumeRequest {
	apiKey := strings.TrimSpace(req.APIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(bearer), "Bearer "))
	}
	accessMode := req.AccessMode
	if accessMode == "" && req.CommitToken != nil {
		accessMode = req.CommitToken.AccessMode
	}
	requestID := req.RequestID
	if requestID == "" && req.CommitToken != nil {
		requestID = req.CommitToken.RequestID
	}
	fingerprint := strings.TrimSpace(req.FingerprintHash)
	if fingerprint == "" && req.CommitToken != nil {
		fingerprint = req.CommitToken.FingerprintHash
	}
	userID := req.UserID
	if userID == 0 && req.CommitToken != nil {
		userID = req.CommitToken.UserID
	}
	return licensesvc.ConsumeRequest{
		FingerprintHash: fingerprint,
		UserID:          userID,
		RequestID:       requestID,
		UsageType:       string(model.UsageActionGenerate),
		AccessMode:      accessMode,
		APIKey:          apiKey,
		CommitToken:     req.CommitToken,
	}
}

func (s *Service) normalizeModel(value string, image bool) string {
	model := strings.TrimSpace(value)
	if model == "" || strings.HasPrefix(model, "hosted/") {
		if image {
			if strings.TrimSpace(s.cfg.ImageModel) != "" {
				return strings.TrimSpace(s.cfg.ImageModel)
			}
		}
		if strings.TrimSpace(s.cfg.TextModel) != "" {
			return strings.TrimSpace(s.cfg.TextModel)
		}
	}
	return model
}

func (s *Service) reserveCreditsForModel(modelName string, image bool) int {
	if rule, ok := s.matchRule(modelName); ok && rule.ReservationCredits > 0 {
		return rule.ReservationCredits
	}
	switch {
	case image:
		return 32
	case strings.Contains(modelName, "pptx-with-image"):
		return 48
	case strings.Contains(modelName, "pptx-no-image"):
		return 28
	default:
		return 16
	}
}

func (s *Service) settleCredits(modelName string, usage usageSummary, image bool) int {
	if rule, ok := s.matchRule(modelName); ok {
		charge := 0
		charge += creditsByPer1K(usage.PromptTokens, rule.PromptPer1KCredits)
		charge += creditsByPer1K(usage.CompletionTokens, rule.OutputPer1KCredits)
		charge += creditsByPer1K(usage.ReasoningTokens, rule.ReasoningPer1KCredits)
		charge += usage.ImageCount * rule.ImagePerAssetCredits
		if charge < rule.MinimumChargeCredits {
			charge = rule.MinimumChargeCredits
		}
		if charge > 0 {
			return charge
		}
	}
	if image {
		return 24 + usage.ImageCount*8
	}
	totalTokens := usage.PromptTokens + usage.CompletionTokens + usage.ReasoningTokens
	if totalTokens == 0 {
		totalTokens = 200
	}
	return int(math.Max(1, math.Ceil(float64(totalTokens)/120.0)))
}

func creditsByPer1K(tokens int, creditsPer1K int) int {
	if tokens <= 0 || creditsPer1K <= 0 {
		return 0
	}
	return int(math.Ceil(float64(tokens) / 1000.0 * float64(creditsPer1K)))
}

func (s *Service) matchRule(modelName string) (model.HostedPricingRule, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	profile := normalizeProfile(modelName)
	for _, rule := range s.cfg.Rules {
		if rule.DocumentProfile == profile {
			return rule, true
		}
	}
	return model.HostedPricingRule{}, false
}

func normalizeProfile(modelName string) string {
	switch {
	case strings.Contains(modelName, "hosted/img") || strings.TrimSpace(modelName) == "img":
		return "img"
	case strings.Contains(modelName, "pptx-with-image"):
		return "pptx-with-image"
	case strings.Contains(modelName, "pptx-no-image"):
		return "pptx-no-image"
	default:
		return "docx-xlsx"
	}
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

func effectiveImageSize(req ImageRequest) string {
	if size := strings.TrimSpace(req.Size); size != "" {
		return size
	}
	return pickImageSize(req.AspectRatio)
}

func effectiveReferenceImages(req ImageRequest) []ImageReference {
	out := make([]ImageReference, 0, len(req.ReferenceImages)+1)
	if req.ReferenceImage != nil && strings.TrimSpace(req.ReferenceImage.Data) != "" {
		out = append(out, *req.ReferenceImage)
	}
	for _, ref := range req.ReferenceImages {
		if strings.TrimSpace(ref.Data) == "" {
			continue
		}
		out = append(out, ref)
	}
	return out
}

func (s *Service) recordUsage(ctx context.Context, apiKeyID uint64, req CompletionRequest, modelName string, usage usageSummary, reserved, settled, refund int, updatedKey *model.APIKey) {
	runtimeMode := "hosted"
	provider := s.cfg.Provider
	if provider == "" {
		provider = "openai"
	}
	event := &model.UsageEvent{
		RequestID:        stringPtr(req.RequestID),
		FingerprintHash:  "hosted",
		Mode:             model.UsageModeHosted,
		Action:           model.UsageActionGenerate,
		APIKeyID:         &apiKeyID,
		Result:           model.UsageResultAllowed,
		Charged:          settled > 0,
		BilledUnits:      settled,
		UnitType:         "credit",
		RuntimeMode:      &runtimeMode,
		Provider:         &provider,
		ModelName:        &modelName,
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		ReasoningTokens:  usage.ReasoningTokens,
		ImageCount:       usage.ImageCount,
		ReservedCredits:  reserved,
		SettledCredits:   settled,
		RefundCredits:    refund,
	}
	_ = s.store.CreateUsageEvent(ctx, event)
	_ = updatedKey
}

func (s *Service) recordImageUsage(ctx context.Context, apiKeyID uint64, req ImageRequest, modelName string, usage usageSummary, reserved, settled, refund int, updatedKey *model.APIKey) {
	runtimeMode := "hosted"
	provider := s.cfg.Provider
	if provider == "" {
		provider = "openai"
	}
	documentType := "img"
	event := &model.UsageEvent{
		RequestID:       stringPtr(req.RequestID),
		FingerprintHash: "hosted",
		Mode:            model.UsageModeHosted,
		Action:          model.UsageActionGenerate,
		APIKeyID:        &apiKeyID,
		Result:          model.UsageResultAllowed,
		DocumentType:    &documentType,
		Charged:         settled > 0,
		BilledUnits:     settled,
		UnitType:        "credit",
		RuntimeMode:     &runtimeMode,
		Provider:        &provider,
		ModelName:       &modelName,
		ImageCount:      usage.ImageCount,
		ReservedCredits: reserved,
		SettledCredits:  settled,
		RefundCredits:   refund,
	}
	_ = s.store.CreateUsageEvent(ctx, event)
	_ = updatedKey
}

func creditBalance(key *model.APIKey) int {
	if key == nil {
		return 0
	}
	return key.AvailableCredits()
}

func stringPtr(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func timeoutFor(seconds int) time.Duration {
	if seconds <= 0 {
		return 60 * time.Second
	}
	return time.Duration(seconds) * time.Second
}

func hashAPIKey(apiKey, salt string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(salt) + ":" + strings.TrimSpace(apiKey)))
	return fmt.Sprintf("%x", sum[:])
}
