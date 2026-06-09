package hostedllm

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/officecli/officecli/platform/internal/apikey"
	"github.com/officecli/officecli/platform/internal/clisession"
	licensesvc "github.com/officecli/officecli/platform/internal/license"
	"github.com/officecli/officecli/platform/internal/model"
)

// ErrHostedCreditsExhausted is returned by the precheck path when the
// subject's balance is below the minimum charge threshold. Mirrors the
// store-layer error so callers can compare without importing sqlstore.
var ErrHostedCreditsExhausted = errors.New("hosted credits exhausted")

type APIKeyStore interface {
	FindAPIKeyByHash(ctx context.Context, hash string) (*model.APIKey, error)
	GetHostedCreditAccountByFingerprint(ctx context.Context, fingerprint string) (*model.FingerprintCreditAccount, error)
	ChargeHostedCreditsByUser(ctx context.Context, userID uint64, requestID string, credits int, usageEventID *uint64) (*model.UserHostedCreditAccount, error)
	ChargeHostedCreditsByFingerprint(ctx context.Context, fingerprint string, requestID string, credits int, usageEventID *uint64) (*model.FingerprintCreditAccount, error)
	ChargeAPIKeyCredits(ctx context.Context, apiKeyID uint64, requestID string, credits int, usageEventID *uint64) (*model.APIKey, error)
	PrecheckHostedCreditsByUser(ctx context.Context, userID uint64, minCredits int) error
	PrecheckHostedCreditsByFingerprint(ctx context.Context, fingerprint string, minCredits int) error
	PrecheckAPIKeyCredits(ctx context.Context, apiKeyID uint64, minCredits int) error
	WriteChargeFailedLedgerForUser(ctx context.Context, userID uint64, requestID string, credits int, metadataJSON string) error
	WriteChargeFailedLedgerForFingerprint(ctx context.Context, fingerprint string, requestID string, credits int, metadataJSON string) error
	WriteChargeFailedLedgerForAPIKey(ctx context.Context, apiKeyID uint64, requestID string, credits int, metadataJSON string) error
	FindCLISessionByTokenHash(ctx context.Context, tokenHash string) (*model.CLISession, error)
	TouchCLISession(ctx context.Context, id uint64, usedAt time.Time) error
	CreateUsageEvent(ctx context.Context, event *model.UsageEvent) error
	HostedPricingSettings(ctx context.Context) (*model.HostedPricingSetting, error)
	ListHostedPricingRules(ctx context.Context, enabledOnly bool) ([]model.HostedPricingRule, error)
	ListHostedModelPricingConfigs(ctx context.Context, enabledOnly bool) ([]model.HostedModelPricingConfig, error)
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
	MarkupBPS              int
	ModelConfigs           []model.HostedModelPricingConfig
	Rules                  []model.HostedPricingRule
	TimeoutSec             int
	AIGatewayAdminBaseURL  string
	AIGatewayAdminAPIKey   string
	AIGatewayAPIKeyGroup   string
	AIGatewayCreateKeyPath string
	AIGatewayKeyCipher     *apikey.Cipher
	AIGatewayAdminClient   AIGatewayAdminClient
	ReconcileEnabled       bool
}

type Service struct {
	store            APIKeyStore
	quota            GenerationQuotaManager
	cfg              Config
	mu               sync.RWMutex
	createKeyLocks   sync.Map
	client           *http.Client
	reconcileEnabled atomic.Bool
}

type CompletionRequest struct {
	RequestID       string                  `json:"request_id,omitempty"`
	Model           string                  `json:"model"`
	Messages        []ChatMessage           `json:"messages"`
	Kind            string                  `json:"-"`
	JSONMode        bool                    `json:"-"`
	SchemaName      string                  `json:"schema_name,omitempty"`
	Strict          bool                    `json:"strict,omitempty"`
	Schema          json.RawMessage         `json:"schema,omitempty"`
	FingerprintHash string                  `json:"fingerprint_hash,omitempty"`
	UserID          uint64                  `json:"user_id,omitempty"`
	APIKey          string                  `json:"api_key,omitempty"`
	AccessMode      model.AccessMode        `json:"access_mode,omitempty"`
	CommitToken     *licensesvc.CommitToken `json:"commit_token,omitempty"`
	AuditContext    model.UsageAuditContext `json:"-"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type CompletionResponse struct {
	Content        string
	CreditBalance  int
	CreditsCharged int
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
	AuditContext    model.UsageAuditContext `json:"-"`
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
	RequestID          string
	UserID             uint64
	CreditBalance      int
	CreditsCharged     int
	AccessMode         model.AccessMode
	Remaining          int
	RewardRemaining    int
	PaidQuotaRemaining int
}

type hostedSubject struct {
	UserID          uint64
	FingerprintHash string
	APIKeyID        *uint64
	APIKeyHash      string
	Key             *model.APIKey
	Session         *model.CLISession
}

func NewService(store APIKeyStore, cfg Config, quotaManagers ...GenerationQuotaManager) *Service {
	timeout := timeoutFor(cfg.TimeoutSec)
	var quota GenerationQuotaManager
	if len(quotaManagers) > 0 {
		quota = quotaManagers[0]
	}
	svc := &Service{
		store:  store,
		quota:  quota,
		cfg:    cfg,
		client: &http.Client{Timeout: timeout},
	}
	svc.reconcileEnabled.Store(cfg.ReconcileEnabled)
	return svc
}

// SetReconcileEnabled hot-swaps the reconcile (charge_failed ledger) flag.
func (s *Service) SetReconcileEnabled(enabled bool) {
	s.reconcileEnabled.Store(enabled)
}

// CurrentReconcileEnabled returns the live reconcile flag for inspection.
func (s *Service) CurrentReconcileEnabled() bool {
	return s.reconcileEnabled.Load()
}

// precheckSubjectCredits routes a read-only balance check to the right
// store function based on subject type. Returns ErrHostedCreditsExhausted
// when the balance is below minCharge.
func (s *Service) precheckSubjectCredits(ctx context.Context, subject *hostedSubject, minCharge int) error {
	if subject == nil {
		return fmt.Errorf("hosted subject is required")
	}
	if minCharge < 1 {
		minCharge = 1
	}
	if subject.Key != nil && subject.APIKeyID != nil {
		return s.store.PrecheckAPIKeyCredits(ctx, *subject.APIKeyID, minCharge)
	}
	if subject.UserID == 0 && subject.FingerprintHash != "" {
		return s.store.PrecheckHostedCreditsByFingerprint(ctx, subject.FingerprintHash, minCharge)
	}
	return s.store.PrecheckHostedCreditsByUser(ctx, subject.UserID, minCharge)
}

// chargeSubjectCredits routes the post-success charge transaction to the
// right store function. Returns the new balance and any error from the
// charge attempt (including ErrHostedCreditsExhausted on concurrent
// depletion, which callers should treat as a transient overspend under
// the ADR Trade-off 1 boundary).
func (s *Service) chargeSubjectCredits(ctx context.Context, subject *hostedSubject, requestID string, credits int, usageEventID *uint64) (int, error) {
	if subject == nil {
		return 0, fmt.Errorf("hosted subject is required")
	}
	if credits < 1 {
		credits = 1
	}
	opCtx, cancel := settlementContext(ctx)
	defer cancel()
	if subject.Key != nil && subject.APIKeyID != nil {
		key, err := s.store.ChargeAPIKeyCredits(opCtx, *subject.APIKeyID, requestID, credits, usageEventID)
		return creditBalance(key), err
	}
	if subject.UserID == 0 && subject.FingerprintHash != "" {
		account, err := s.store.ChargeHostedCreditsByFingerprint(opCtx, subject.FingerprintHash, requestID, credits, usageEventID)
		return fingerprintAccountCreditBalance(account), err
	}
	account, err := s.store.ChargeHostedCreditsByUser(opCtx, subject.UserID, requestID, credits, usageEventID)
	return accountCreditBalance(account), err
}

// writeChargeFailedForSubject marshals the metadata to JSON and writes a
// charge_failed_post_upstream ledger row via the appropriate store helper.
// Only called when ReconcileEnabled=true, per ADR Trade-off 2.
func (s *Service) writeChargeFailedForSubject(ctx context.Context, subject *hostedSubject, requestID string, credits int, meta map[string]any) error {
	if subject == nil {
		return fmt.Errorf("hosted subject is required")
	}
	if meta == nil {
		meta = map[string]any{}
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal charge_failed metadata: %w", err)
	}
	opCtx, cancel := settlementContext(ctx)
	defer cancel()
	if subject.Key != nil && subject.APIKeyID != nil {
		return s.store.WriteChargeFailedLedgerForAPIKey(opCtx, *subject.APIKeyID, requestID, credits, string(raw))
	}
	if subject.UserID == 0 && subject.FingerprintHash != "" {
		return s.store.WriteChargeFailedLedgerForFingerprint(opCtx, subject.FingerprintHash, requestID, credits, string(raw))
	}
	return s.store.WriteChargeFailedLedgerForUser(opCtx, subject.UserID, requestID, credits, string(raw))
}

func (s *Service) HostedPricingRules() []model.HostedPricingRule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.HostedPricingRule, len(s.cfg.Rules))
	copy(out, s.cfg.Rules)
	return out
}

func (s *Service) Complete(ctx context.Context, bearer string, fingerprint string, req CompletionRequest) (*CompletionResponse, error) {
	if err := validateHostedProfile(req.Model, false); err != nil {
		return nil, err
	}
	req.RequestID = hostedRequestID(req.RequestID, req.CommitToken)
	if req.CommitToken != nil {
		return s.completeQuotaText(ctx, req)
	}
	subject, err := s.authorizeSubject(ctx, bearer, fingerprint)
	if err != nil {
		return nil, err
	}
	upstreamAPIKey, err := s.upstreamAPIKeyForSubject(ctx, subject)
	if err != nil {
		return nil, err
	}
	return s.completeChargeOnly(ctx, subject, req, upstreamAPIKey)
}

func (s *Service) completeChargeOnly(ctx context.Context, subject *hostedSubject, req CompletionRequest, upstreamAPIKey string) (*CompletionResponse, error) {
	modelName := s.normalizeModel(ctx, req.Model, false)
	payload, err := buildChatPayload(req, modelName)
	if err != nil {
		return nil, err
	}
	// Cheap read-only admission check: spare the upstream call and the
	// user-visible error path when the balance is clearly below the
	// minimum charge. NOT the authoritative admission — the DB row lock
	// inside Charge re-validates atomically.
	if err := s.precheckSubjectCredits(ctx, subject, 1); err != nil {
		return nil, err
	}

	body, usage, err := s.post(ctx, strings.TrimRight(s.cfg.BaseURL, "/")+"/chat/completions", payload, upstreamAPIKey)
	if err != nil && isUpstreamAuthError(err) {
		upstreamAPIKey, rotateErr := s.rotateSubjectAIGatewayAPIKey(ctx, subject, err)
		if rotateErr != nil {
			return nil, fmt.Errorf("hosted upstream credential rotation failed: %w", rotateErr)
		}
		body, usage, err = s.post(ctx, strings.TrimRight(s.cfg.BaseURL, "/")+"/chat/completions", payload, upstreamAPIKey)
	}
	if err != nil {
		// Upstream failure: zero side effects on the account (no charge,
		// no charge_failed ledger). This is the core P2 property.
		return nil, err
	}
	content, decodeErr := decodeChatContent(body)
	if decodeErr != nil {
		s.handleDecodeFailureChargeOnly(ctx, subject, req, modelName, usage, false, decodeErr)
		return nil, decodeErr
	}
	pricing := s.priceUsage(ctx, req.Model, usage, false)
	// Record the usage event first so we can attach its primary key to the
	// charge ledger row (fixes F1 — audit chain).
	event := s.buildChatUsageEvent(subject, req, modelName, usage, pricing, pricing.ChargeCredits)
	s.persistUsageEvent(ctx, event)
	var eventIDPtr *uint64
	if event.ID != 0 {
		eid := event.ID
		eventIDPtr = &eid
	}
	balance, chargeErr := s.chargeSubjectCredits(ctx, subject, req.RequestID, pricing.ChargeCredits, eventIDPtr)
	if chargeErr != nil {
		if errors.Is(chargeErr, ErrHostedCreditsExhausted) {
			// Transient overspend ≤ max charge (ADR Trade-off 1). Product
			// was already produced and returned; the account just couldn't
			// be debited because a concurrent request beat us to zero. We
			// eat the cost; no charge_failed row.
			slog.Warn("hosted_charge_overspend",
				"request_id", req.RequestID,
				"subject_user_id", subject.UserID,
				"subject_fingerprint", subject.FingerprintHash,
				"credits", pricing.ChargeCredits,
			)
		} else if s.reconcileEnabled.Load() {
			meta := map[string]any{
				"reason":              "charge_tx_failed",
				"upstream_request_id": req.RequestID,
				"pricing_rule_id":     pricing.RuleID,
				"charge_tx_error":     chargeErr.Error(),
			}
			if writeErr := s.writeChargeFailedForSubject(ctx, subject, req.RequestID, pricing.ChargeCredits, meta); writeErr != nil {
				slog.Warn("hosted_charge_failed_write_failed",
					"request_id", req.RequestID,
					"err", writeErr,
				)
			}
		} else {
			slog.Warn("hosted_charge_tx_failed",
				"request_id", req.RequestID,
				"credits", pricing.ChargeCredits,
				"err", chargeErr,
			)
		}
	}
	return &CompletionResponse{
		Content:        content,
		CreditBalance:  balance,
		CreditsCharged: pricing.ChargeCredits,
	}, nil
}

func buildChatPayload(req CompletionRequest, modelName string) (map[string]any, error) {
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
	return payload, nil
}

func decodeChatContent(body []byte) (string, error) {
	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("decode hosted completion: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("hosted completion is empty")
	}
	return resp.Choices[0].Message.Content, nil
}

func (s *Service) completeQuotaText(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	if s.quota == nil {
		return nil, fmt.Errorf("generation quota service is unavailable")
	}
	consumeReq := textConsumeRequest(req)
	if err := s.quota.ValidateCommitToken(consumeReq); err != nil {
		return nil, err
	}
	modelName := s.normalizeModel(ctx, req.Model, false)
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
	body, usage, err := s.post(ctx, strings.TrimRight(s.cfg.BaseURL, "/")+"/chat/completions", payload, strings.TrimSpace(s.cfg.APIKey))
	if err != nil {
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
		return nil, fmt.Errorf("decode hosted completion: %w", err)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("hosted completion is empty")
	}
	consumeResp, err := s.quota.Consume(ctx, consumeReq)
	if err != nil {
		return nil, err
	}
	_ = usage
	return &CompletionResponse{Content: resp.Choices[0].Message.Content, CreditBalance: consumeResp.CreditBalance}, nil
}

func (s *Service) GenerateImage(ctx context.Context, bearer string, fingerprint string, req ImageRequest) (*ImageResponse, error) {
	if err := validateHostedProfile(req.Model, true); err != nil {
		return nil, err
	}
	req.RequestID = hostedRequestID(req.RequestID, req.CommitToken)
	if req.CommitToken != nil {
		return s.generateQuotaImage(ctx, bearer, req)
	}
	subject, err := s.authorizeSubject(ctx, bearer, fingerprint)
	if err != nil {
		return nil, err
	}
	upstreamAPIKey, err := s.upstreamAPIKeyForSubject(ctx, subject)
	if err != nil {
		return nil, err
	}
	return s.generateImageChargeOnly(ctx, subject, req, upstreamAPIKey)
}

func (s *Service) AuthorizeImageReplay(ctx context.Context, bearer string, fingerprint string, req ImageRequest) (*ImageResponse, error) {
	if err := validateHostedProfile(req.Model, true); err != nil {
		return nil, err
	}
	req.RequestID = hostedRequestID(req.RequestID, req.CommitToken)
	if req.CommitToken != nil {
		if s.quota == nil {
			return nil, fmt.Errorf("generation quota service is unavailable")
		}
		if err := s.quota.ValidateCommitToken(imageConsumeRequest(req, bearer)); err != nil {
			return nil, err
		}
		return &ImageResponse{RequestID: req.RequestID, AccessMode: req.AccessMode}, nil
	}
	subject, err := s.authorizeSubject(ctx, bearer, fingerprint)
	if err != nil {
		return nil, err
	}
	balance := 0
	if subject.Key != nil {
		balance = creditBalance(subject.Key)
	} else if subject.UserID == 0 && subject.FingerprintHash != "" {
		account, err := s.store.GetHostedCreditAccountByFingerprint(ctx, subject.FingerprintHash)
		if err != nil {
			return nil, err
		}
		balance = fingerprintAccountCreditBalance(account)
	}
	return &ImageResponse{
		RequestID:     req.RequestID,
		UserID:        subject.UserID,
		CreditBalance: balance,
	}, nil
}

func (s *Service) generateImageChargeOnly(ctx context.Context, subject *hostedSubject, req ImageRequest, upstreamAPIKey string) (*ImageResponse, error) {
	modelName := s.normalizeModel(ctx, req.Model, true)
	payload := map[string]any{
		"model":           modelName,
		"prompt":          req.Prompt,
		"size":            effectiveImageSize(req),
		"response_format": "b64_json",
	}
	if err := s.precheckSubjectCredits(ctx, subject, 1); err != nil {
		return nil, err
	}
	body, usage, err := s.postImageRequest(ctx, modelName, req, payload, upstreamAPIKey)
	if err != nil && isUpstreamAuthError(err) {
		upstreamAPIKey, rotateErr := s.rotateSubjectAIGatewayAPIKey(ctx, subject, err)
		if rotateErr != nil {
			return nil, fmt.Errorf("hosted upstream credential rotation failed: %w", rotateErr)
		}
		body, usage, err = s.postImageRequest(ctx, modelName, req, payload, upstreamAPIKey)
	}
	if err != nil {
		return nil, err
	}
	data, decodeErr := decodeImageData(body)
	if decodeErr != nil {
		s.handleDecodeFailureChargeOnly(ctx, subject, req, modelName, usage, true, decodeErr)
		return nil, decodeErr
	}
	usage.ImageCount = 1
	pricing := s.priceUsage(ctx, req.Model, usage, true)
	event := s.buildImageUsageEvent(subject, req, modelName, usage, pricing, pricing.ChargeCredits)
	s.persistUsageEvent(ctx, event)
	var eventIDPtr *uint64
	if event.ID != 0 {
		eid := event.ID
		eventIDPtr = &eid
	}
	balance, chargeErr := s.chargeSubjectCredits(ctx, subject, req.RequestID, pricing.ChargeCredits, eventIDPtr)
	if chargeErr != nil {
		if errors.Is(chargeErr, ErrHostedCreditsExhausted) {
			slog.Warn("hosted_charge_overspend",
				"request_id", req.RequestID,
				"subject_user_id", subject.UserID,
				"subject_fingerprint", subject.FingerprintHash,
				"credits", pricing.ChargeCredits,
				"is_image", true,
			)
		} else if s.reconcileEnabled.Load() {
			meta := map[string]any{
				"reason":              "charge_tx_failed",
				"upstream_request_id": req.RequestID,
				"pricing_rule_id":     pricing.RuleID,
				"charge_tx_error":     chargeErr.Error(),
				"is_image":            true,
			}
			if writeErr := s.writeChargeFailedForSubject(ctx, subject, req.RequestID, pricing.ChargeCredits, meta); writeErr != nil {
				slog.Warn("hosted_charge_failed_write_failed",
					"request_id", req.RequestID,
					"err", writeErr,
				)
			}
		} else {
			slog.Warn("hosted_charge_tx_failed",
				"request_id", req.RequestID,
				"credits", pricing.ChargeCredits,
				"err", chargeErr,
				"is_image", true,
			)
		}
	}
	return &ImageResponse{
		Data:           data,
		MIME:           "image/png",
		RequestID:      req.RequestID,
		UserID:         subject.UserID,
		CreditBalance:  balance,
		CreditsCharged: pricing.ChargeCredits,
	}, nil
}

func decodeImageData(body []byte) ([]byte, error) {
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
	return data, nil
}

func (s *Service) generateQuotaImage(ctx context.Context, bearer string, req ImageRequest) (*ImageResponse, error) {
	if err := validateHostedProfile(req.Model, true); err != nil {
		return nil, err
	}
	if s.quota == nil {
		return nil, fmt.Errorf("generation quota service is unavailable")
	}
	consumeReq := imageConsumeRequest(req, bearer)
	if err := s.quota.ValidateCommitToken(consumeReq); err != nil {
		return nil, err
	}
	modelName := s.normalizeModel(ctx, req.Model, true)
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
		RequestID:          req.RequestID,
		AccessMode:         consumeResp.AccessMode,
		Remaining:          consumeResp.Remaining,
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

type upstreamHTTPError struct {
	statusCode int
	body       string
}

func (e upstreamHTTPError) Error() string {
	return fmt.Sprintf("hosted upstream request failed: status=%d body=%s", e.statusCode, strings.TrimSpace(e.body))
}

func isUpstreamAuthError(err error) bool {
	var upstreamErr upstreamHTTPError
	return errors.As(err, &upstreamErr) && (upstreamErr.statusCode == http.StatusUnauthorized || upstreamErr.statusCode == http.StatusForbidden)
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
		return nil, usageSummary{}, upstreamHTTPError{statusCode: resp.StatusCode, body: string(body)}
	}
	return body, parseUsageSummary(body), nil
}

func (s *Service) postImageRequest(ctx context.Context, modelName string, req ImageRequest, payload map[string]any, upstreamAPIKey string) ([]byte, usageSummary, error) {
	refs := effectiveReferenceImages(req)
	if len(refs) == 0 {
		body, usage, err := s.post(ctx, strings.TrimRight(s.cfg.BaseURL, "/")+"/images/generations", payload, upstreamAPIKey)
		if err == nil {
			return body, usage, nil
		}
		if !shouldFallbackToResponsesImage(err) {
			return nil, usageSummary{}, err
		}
		return s.postResponsesImageRequest(ctx, modelName, req, upstreamAPIKey)
	}
	fields := map[string]string{
		"model":  modelName,
		"prompt": req.Prompt,
		"size":   effectiveImageSize(req),
	}
	return s.postImageEdit(ctx, strings.TrimRight(s.cfg.BaseURL, "/")+"/images/edits", fields, refs, upstreamAPIKey)
}

func shouldFallbackToResponsesImage(err error) bool {
	var upstreamErr upstreamHTTPError
	if !errors.As(err, &upstreamErr) {
		return false
	}
	switch upstreamErr.statusCode {
	case http.StatusBadRequest, http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusBadGateway, http.StatusServiceUnavailable:
		return true
	default:
		return false
	}
}

func (s *Service) postResponsesImageRequest(ctx context.Context, modelName string, req ImageRequest, upstreamAPIKey string) ([]byte, usageSummary, error) {
	payload := map[string]any{
		"model": modelName,
		"input": req.Prompt,
		"tools": []map[string]any{{"type": "image_generation"}},
	}
	body, usage, err := s.post(ctx, strings.TrimRight(s.cfg.BaseURL, "/")+"/responses", payload, upstreamAPIKey)
	if err != nil {
		return nil, usageSummary{}, err
	}
	var resp struct {
		Output []struct {
			Type   string `json:"type"`
			Result string `json:"result"`
		} `json:"output"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, usageSummary{}, fmt.Errorf("decode hosted responses image: %w", err)
	}
	for _, item := range resp.Output {
		if item.Type != "image_generation_call" || strings.TrimSpace(item.Result) == "" {
			continue
		}
		normalized, err := json.Marshal(map[string]any{
			"data": []map[string]string{{"b64_json": item.Result}},
		})
		if err != nil {
			return nil, usageSummary{}, err
		}
		return normalized, usage, nil
	}
	return nil, usageSummary{}, fmt.Errorf("hosted responses image is empty")
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
		return nil, usageSummary{}, upstreamHTTPError{statusCode: resp.StatusCode, body: string(respBody)}
	}
	return respBody, parseUsageSummary(respBody), nil
}

func parseUsageSummary(body []byte) usageSummary {
	var envelope struct {
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			ReasoningTokens  int `json:"reasoning_tokens"`
			InputTokens      int `json:"input_tokens"`
			OutputTokens     int `json:"output_tokens"`
		} `json:"usage"`
	}
	_ = json.Unmarshal(body, &envelope)
	usage := usageSummary{
		PromptTokens:     envelope.Usage.PromptTokens,
		CompletionTokens: envelope.Usage.CompletionTokens,
		ReasoningTokens:  envelope.Usage.ReasoningTokens,
	}
	if usage.PromptTokens == 0 {
		usage.PromptTokens = envelope.Usage.InputTokens
	}
	if usage.CompletionTokens == 0 {
		usage.CompletionTokens = envelope.Usage.OutputTokens
	}
	return usage
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

func (s *Service) authorizeSubject(ctx context.Context, bearer string, fingerprint string) (*hostedSubject, error) {
	keyValue := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(bearer), "Bearer "))
	fingerprint = strings.TrimSpace(fingerprint)
	if keyValue == "" {
		if fingerprint == "" {
			return nil, fmt.Errorf("missing api key, cli session, or anonymous fingerprint")
		}
		return &hostedSubject{FingerprintHash: fingerprint}, nil
	}
	if strings.HasPrefix(keyValue, "ocli_sess_") {
		session, err := s.store.FindCLISessionByTokenHash(ctx, clisession.HashToken(keyValue))
		if err != nil {
			return nil, err
		}
		switch {
		case session == nil:
			return nil, fmt.Errorf("invalid cli session")
		case session.RevokedAt != nil:
			return nil, fmt.Errorf("cli session has been revoked")
		case time.Now().UTC().After(session.ExpiresAt):
			return nil, fmt.Errorf("cli session has expired")
		}
		_ = s.store.TouchCLISession(ctx, session.ID, time.Now().UTC())
		return &hostedSubject{UserID: session.UserID, Session: session}, nil
	}
	hash := hashAPIKey(keyValue, s.cfg.HashSalt)
	key, err := s.store.FindAPIKeyByHash(ctx, hash)
	if err != nil {
		return nil, err
	}
	switch {
	case key == nil:
		return nil, fmt.Errorf("invalid api key")
	case key.Status != model.APIKeyStatusActive:
		return nil, fmt.Errorf("api key is disabled")
	case !key.SupportsHosted():
		return nil, fmt.Errorf("hosted mode is not enabled for this key")
	case key.OwnerUserID == nil || *key.OwnerUserID == 0:
		return nil, fmt.Errorf("hosted mode requires an owner user for the officecli api key")
	}
	apiKeyID := key.ID
	return &hostedSubject{UserID: *key.OwnerUserID, APIKeyID: &apiKeyID, APIKeyHash: hash, Key: key}, nil
}

func (s *Service) upstreamAPIKeyForSubject(ctx context.Context, subject *hostedSubject) (string, error) {
	if subject == nil {
		return "", fmt.Errorf("hosted subject is required")
	}
	if subject.UserID == 0 && subject.FingerprintHash != "" {
		key := strings.TrimSpace(s.cfg.APIKey)
		if key == "" {
			return "", fmt.Errorf("anonymous hosted mode requires a system upstream api key")
		}
		return key, nil
	}
	if subject.UserID == 0 {
		return "", fmt.Errorf("hosted mode requires an owner user for the officecli api key")
	}
	return s.upstreamAPIKeyForUser(ctx, subject.UserID)
}

func (s *Service) upstreamAPIKeyForUser(ctx context.Context, userID uint64) (string, error) {
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

func (s *Service) rotateSubjectAIGatewayAPIKey(ctx context.Context, subject *hostedSubject, cause error) (string, error) {
	if subject == nil || subject.UserID == 0 {
		return "", fmt.Errorf("hosted mode requires an owner user for the officecli api key")
	}
	userID := subject.UserID
	upstreamName := fmt.Sprintf("officecli-user-%d", userID)
	message := "upstream credential rejected"
	if cause != nil {
		message = cause.Error()
	}
	if _, err := s.store.MarkUserAIGatewayAPIKeyCreationError(ctx, userID, upstreamName, message); err != nil {
		return "", err
	}
	return s.upstreamAPIKeyForUser(ctx, userID)
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

func fingerprintAccountCreditBalance(account *model.FingerprintCreditAccount) int {
	if account == nil {
		return 0
	}
	return account.CreditBalance
}

func settlementContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
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
		AuditContext:    req.AuditContext,
	}
}

func textConsumeRequest(req CompletionRequest) licensesvc.ConsumeRequest {
	apiKey := strings.TrimSpace(req.APIKey)
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
		AuditContext:    req.AuditContext,
	}
}

func (s *Service) normalizeModel(ctx context.Context, value string, image bool) string {
	model := strings.TrimSpace(value)
	if model == "" || model == "text" || model == "image" || strings.HasPrefix(model, "hosted/") {
		if rule, ok := s.matchRule(ctx, model); ok {
			if config, ok := s.matchModelConfig(ctx, rule, image); ok && strings.TrimSpace(config.Model) != "" {
				return strings.TrimSpace(config.Model)
			}
		}
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

func hostedRequestID(value string, token *licensesvc.CommitToken) string {
	if requestID := strings.TrimSpace(value); requestID != "" {
		return requestID
	}
	if token != nil {
		if requestID := strings.TrimSpace(token.RequestID); requestID != "" {
			return requestID
		}
	}
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("hosted-%d", time.Now().UnixNano())
	}
	return "hosted-" + hex.EncodeToString(b[:])
}

type hostedPriceSnapshot struct {
	RuleID                uint64
	MarkupBPS             int
	UpstreamCostMicrousd  int64
	ChargeCredits         int
	UncappedChargeCredits int
	ProfitMicrousd        int64
	CapApplied            bool
}

func (s *Service) priceUsage(ctx context.Context, modelName string, usage usageSummary, image bool) (snapshot hostedPriceSnapshot) {
	ctx, cancel := settlementContext(ctx)
	defer cancel()
	// Global floor: every successful generation costs at least 1 credit,
	// even when the matched rule has MinimumChargeCredits=0 or upstream
	// cost rounds to zero. Guards against ImageCount=0 / near-zero token
	// edge cases that would otherwise debit 0.
	defer func() {
		if snapshot.ChargeCredits < 1 {
			snapshot.ChargeCredits = 1
		}
		if snapshot.UncappedChargeCredits < 1 {
			snapshot.UncappedChargeCredits = 1
		}
	}()
	creditsPerUSD := s.effectiveCreditsPerUSD(ctx)
	if rule, ok := s.matchRule(ctx, modelName); ok {
		markupBPS := s.effectiveMarkupBPS(ctx, rule)
		if image && usage.ImageCount > 0 && rule.ImagePerAssetCredits > 0 {
			charge := rule.ImagePerAssetCredits * usage.ImageCount
			if charge < rule.MinimumChargeCredits {
				charge = rule.MinimumChargeCredits
			}
			var cost int64
			if config, ok := s.matchModelConfig(ctx, rule, true); ok {
				cost = hostedUsageModelConfigCostMicrousd(config, usage)
			} else {
				cost = hostedUsageCostMicrousd(rule, usage)
			}
			return hostedPriceSnapshot{
				RuleID:                rule.ID,
				MarkupBPS:             markupBPS,
				UpstreamCostMicrousd:  cost,
				ChargeCredits:         charge,
				UncappedChargeCredits: charge,
				ProfitMicrousd:        microusdFromCredits(charge, creditsPerUSD) - cost,
			}
		}
		if config, ok := s.matchModelConfig(ctx, rule, image); ok {
			cost := hostedUsageModelConfigCostMicrousd(config, usage)
			charge := creditsFromCostMicrousd(cost, markupBPS, creditsPerUSD)
			if charge < rule.MinimumChargeCredits {
				charge = rule.MinimumChargeCredits
			}
			return hostedPriceSnapshot{
				RuleID:                rule.ID,
				MarkupBPS:             markupBPS,
				UpstreamCostMicrousd:  cost,
				ChargeCredits:         charge,
				UncappedChargeCredits: charge,
				ProfitMicrousd:        microusdFromCredits(charge, creditsPerUSD) - cost,
			}
		}
		cost := hostedUsageCostMicrousd(rule, usage)
		charge := creditsFromCostMicrousd(cost, markupBPS, creditsPerUSD)
		if charge < rule.MinimumChargeCredits {
			charge = rule.MinimumChargeCredits
		}
		if charge > 0 {
			return hostedPriceSnapshot{
				RuleID:                rule.ID,
				MarkupBPS:             markupBPS,
				UpstreamCostMicrousd:  cost,
				ChargeCredits:         charge,
				UncappedChargeCredits: charge,
				ProfitMicrousd:        microusdFromCredits(charge, creditsPerUSD) - cost,
			}
		}
	}
	if image {
		charge := usage.ImageCount * 10
		if charge < 10 {
			charge = 10
		}
		return hostedPriceSnapshot{MarkupBPS: s.effectiveMarkupBPS(ctx, model.HostedPricingRule{}), ChargeCredits: charge, UncappedChargeCredits: charge, ProfitMicrousd: microusdFromCredits(charge, creditsPerUSD)}
	}
	totalTokens := usage.PromptTokens + usage.CompletionTokens + usage.ReasoningTokens
	if totalTokens == 0 {
		totalTokens = 200
	}
	charge := int(math.Max(1, math.Ceil(float64(totalTokens)/120.0)))
	return hostedPriceSnapshot{MarkupBPS: s.effectiveMarkupBPS(ctx, model.HostedPricingRule{}), ChargeCredits: charge, UncappedChargeCredits: charge, ProfitMicrousd: microusdFromCredits(charge, creditsPerUSD)}
}

func hostedUsageCostMicrousd(rule model.HostedPricingRule, usage usageSummary) int64 {
	var cost int64
	cost += costByPer1K(usage.PromptTokens, firstPositiveInt64(rule.PromptPer1KCostMicrousd, int64(rule.PromptPer1KCredits)*10000))
	cost += costByPer1K(usage.CompletionTokens, firstPositiveInt64(rule.OutputPer1KCostMicrousd, int64(rule.OutputPer1KCredits)*10000))
	cost += costByPer1K(usage.ReasoningTokens, firstPositiveInt64(rule.ReasoningPer1KCostMicrousd, int64(rule.ReasoningPer1KCredits)*10000))
	cost += int64(usage.ImageCount) * firstPositiveInt64(rule.ImagePerAssetCostMicrousd, int64(rule.ImagePerAssetCredits)*10000)
	return cost
}

func hostedUsageModelConfigCostMicrousd(config model.HostedModelPricingConfig, usage usageSummary) int64 {
	var cost int64
	cost += costByPer1M(usage.PromptTokens, config.PromptPer1MCostMicrousd)
	cost += costByPer1M(usage.CompletionTokens, config.OutputPer1MCostMicrousd)
	cost += costByPer1M(usage.ReasoningTokens, config.ReasoningPer1MCostMicrousd)
	return cost
}

func firstPositiveInt64(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func costByPer1K(tokens int, microusdPer1K int64) int64 {
	if tokens <= 0 || microusdPer1K <= 0 {
		return 0
	}
	return int64(tokens)*microusdPer1K/1000 + ceilRemainder(int64(tokens)*microusdPer1K, 1000)
}

func costByPer1M(tokens int, microusdPer1M int64) int64 {
	if tokens <= 0 || microusdPer1M <= 0 {
		return 0
	}
	return int64(tokens)*microusdPer1M/1000000 + ceilRemainder(int64(tokens)*microusdPer1M, 1000000)
}

func ceilRemainder(value, divisor int64) int64 {
	if value <= 0 || divisor <= 0 || value%divisor == 0 {
		return 0
	}
	return 1
}

func creditsFromCostMicrousd(cost int64, markupBPS int, creditsPerUSD int) int {
	if cost <= 0 {
		return 0
	}
	creditsPerUSD = normalizeCreditsPerUSD(creditsPerUSD)
	numerator := cost * int64(creditsPerUSD) * int64(10000+markupBPS)
	denominator := int64(1_000_000 * 10000)
	credits := numerator / denominator
	if numerator%denominator != 0 {
		credits++
	}
	if credits < 0 {
		return 0
	}
	return int(credits)
}

func microusdFromCredits(credits int, creditsPerUSD int) int64 {
	if credits <= 0 {
		return 0
	}
	creditsPerUSD = normalizeCreditsPerUSD(creditsPerUSD)
	return int64(credits) * 1_000_000 / int64(creditsPerUSD)
}

func (s *Service) effectiveMarkupBPS(ctx context.Context, rule model.HostedPricingRule) int {
	if rule.MarkupBPS != nil {
		return *rule.MarkupBPS
	}
	if s.store != nil {
		if settings, err := s.store.HostedPricingSettings(ctx); err == nil && settings != nil {
			return settings.MarkupBPS
		}
	}
	if s.cfg.MarkupBPS != 0 {
		return s.cfg.MarkupBPS
	}
	return 3000
}

func (s *Service) effectiveCreditsPerUSD(ctx context.Context) int {
	if s.store != nil {
		if settings, err := s.store.HostedPricingSettings(ctx); err == nil && settings != nil {
			return normalizeCreditsPerUSD(settings.CreditsPerUSD)
		}
	}
	return 100
}

func normalizeCreditsPerUSD(value int) int {
	if value <= 0 {
		return 100
	}
	return value
}

func (s *Service) matchRule(ctx context.Context, modelName string) (model.HostedPricingRule, bool) {
	s.mu.RLock()
	cfgRules := make([]model.HostedPricingRule, len(s.cfg.Rules))
	copy(cfgRules, s.cfg.Rules)
	s.mu.RUnlock()
	rules := cfgRules
	if s.store != nil {
		if dbRules, err := s.store.ListHostedPricingRules(ctx, true); err == nil && len(dbRules) > 0 {
			rules = dbRules
		}
	}
	profile := normalizeProfile(modelName)
	for _, rule := range rules {
		if !rule.Enabled && rule.ID != 0 {
			continue
		}
		if rule.DocumentProfile == profile {
			return rule, true
		}
	}
	return model.HostedPricingRule{}, false
}

func (s *Service) matchModelConfig(ctx context.Context, rule model.HostedPricingRule, image bool) (model.HostedModelPricingConfig, bool) {
	key := strings.TrimSpace(rule.TextModelKey)
	kind := model.HostedModelPricingKindText
	if image {
		key = strings.TrimSpace(rule.ImageModelKey)
		kind = model.HostedModelPricingKindImage
	}
	if key == "" {
		return model.HostedModelPricingConfig{}, false
	}
	for _, config := range s.hostedModelPricingConfigs(ctx, true) {
		if strings.TrimSpace(config.Key) == key && config.Kind == kind {
			return config, true
		}
	}
	return model.HostedModelPricingConfig{}, false
}

func (s *Service) hostedModelPricingConfigs(ctx context.Context, enabledOnly bool) []model.HostedModelPricingConfig {
	s.mu.RLock()
	cfgConfigs := make([]model.HostedModelPricingConfig, len(s.cfg.ModelConfigs))
	copy(cfgConfigs, s.cfg.ModelConfigs)
	s.mu.RUnlock()
	configs := cfgConfigs
	if s.store != nil {
		if dbConfigs, err := s.store.ListHostedModelPricingConfigs(ctx, enabledOnly); err == nil && len(dbConfigs) > 0 {
			configs = dbConfigs
		}
	}
	if !enabledOnly {
		return configs
	}
	out := make([]model.HostedModelPricingConfig, 0, len(configs))
	for _, config := range configs {
		if config.Enabled {
			out = append(out, config)
		}
	}
	return out
}

func normalizeProfile(modelName string) string {
	trimmed := strings.TrimSpace(modelName)
	switch {
	case trimmed == "" || trimmed == "text" || trimmed == "hosted/text":
		return "text"
	case trimmed == "image" || trimmed == "hosted/image":
		return "image"
	case strings.HasPrefix(trimmed, "hosted/"):
		return ""
	default:
		return "text"
	}
}

func validateHostedProfile(modelName string, image bool) error {
	trimmed := strings.TrimSpace(modelName)
	if trimmed == "" {
		return nil
	}
	if strings.HasPrefix(trimmed, "hosted/") {
		if image && trimmed == "hosted/image" {
			return nil
		}
		if !image && trimmed == "hosted/text" {
			return nil
		}
		return fmt.Errorf("unsupported hosted pricing profile %q", strings.TrimPrefix(trimmed, "hosted/"))
	}
	if image && trimmed == "text" {
		return fmt.Errorf("unsupported hosted pricing profile %q", trimmed)
	}
	if !image && trimmed == "image" {
		return fmt.Errorf("unsupported hosted pricing profile %q", trimmed)
	}
	return nil
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

func hostedUsageFingerprint(requestFingerprint string, token *licensesvc.CommitToken) string {
	if fingerprint := strings.TrimSpace(requestFingerprint); fingerprint != "" {
		return fingerprint
	}
	if token != nil {
		if fingerprint := strings.TrimSpace(token.FingerprintHash); fingerprint != "" {
			return fingerprint
		}
	}
	return "hosted"
}

func (s *Service) buildChatUsageEvent(subject *hostedSubject, req CompletionRequest, modelName string, usage usageSummary, pricing hostedPriceSnapshot, settled int) *model.UsageEvent {
	return s.buildChatUsageEventFull(subject, req, modelName, usage, pricing, 0, settled, 0)
}

func (s *Service) buildChatUsageEventFull(subject *hostedSubject, req CompletionRequest, modelName string, usage usageSummary, pricing hostedPriceSnapshot, reserved, settled, refund int) *model.UsageEvent {
	runtimeMode := "hosted"
	provider := s.cfg.Provider
	if provider == "" {
		provider = "openai"
	}
	event := &model.UsageEvent{
		RequestID:             stringPtr(req.RequestID),
		FingerprintHash:       hostedUsageFingerprint(req.FingerprintHash, req.CommitToken),
		Mode:                  model.UsageModeHosted,
		Action:                model.UsageActionGenerate,
		Result:                model.UsageResultAllowed,
		Charged:               settled > 0,
		BilledUnits:           settled,
		UnitType:              "credit",
		RuntimeMode:           &runtimeMode,
		Provider:              &provider,
		ModelName:             &modelName,
		PromptTokens:          usage.PromptTokens,
		CompletionTokens:      usage.CompletionTokens,
		ReasoningTokens:       usage.ReasoningTokens,
		ImageCount:            usage.ImageCount,
		ReservedCredits:       reserved,
		SettledCredits:        settled,
		RefundCredits:         refund,
		HostedPricingRuleID:   pricing.RuleID,
		MarkupBPS:             pricing.MarkupBPS,
		UpstreamCostMicrousd:  pricing.UpstreamCostMicrousd,
		UncappedChargeCredits: pricing.UncappedChargeCredits,
		ProfitMicrousd:        int64(settled)*10000 - pricing.UpstreamCostMicrousd,
		CapApplied:            pricing.CapApplied,
	}
	if subject != nil {
		event.UserID = optionalUserID(subject.UserID)
		event.APIKeyID = subject.APIKeyID
	}
	event.ApplyAuditContext(req.AuditContext)
	return event
}

func (s *Service) persistUsageEvent(ctx context.Context, event *model.UsageEvent) {
	opCtx, cancel := settlementContext(ctx)
	defer cancel()
	_ = s.store.CreateUsageEvent(opCtx, event)
}

func (s *Service) buildImageUsageEvent(subject *hostedSubject, req ImageRequest, modelName string, usage usageSummary, pricing hostedPriceSnapshot, settled int) *model.UsageEvent {
	return s.buildImageUsageEventFull(subject, req, modelName, usage, pricing, 0, settled, 0)
}

func (s *Service) buildImageUsageEventFull(subject *hostedSubject, req ImageRequest, modelName string, usage usageSummary, pricing hostedPriceSnapshot, reserved, settled, refund int) *model.UsageEvent {
	runtimeMode := "hosted"
	provider := s.cfg.Provider
	if provider == "" {
		provider = "openai"
	}
	documentType := "img"
	event := &model.UsageEvent{
		RequestID:             stringPtr(req.RequestID),
		FingerprintHash:       hostedUsageFingerprint(req.FingerprintHash, req.CommitToken),
		Mode:                  model.UsageModeHosted,
		Action:                model.UsageActionGenerate,
		Result:                model.UsageResultAllowed,
		DocumentType:          &documentType,
		Charged:               settled > 0,
		BilledUnits:           settled,
		UnitType:              "credit",
		RuntimeMode:           &runtimeMode,
		Provider:              &provider,
		ModelName:             &modelName,
		PromptTokens:          usage.PromptTokens,
		CompletionTokens:      usage.CompletionTokens,
		ReasoningTokens:       usage.ReasoningTokens,
		ImageCount:            usage.ImageCount,
		ReservedCredits:       reserved,
		SettledCredits:        settled,
		RefundCredits:         refund,
		HostedPricingRuleID:   pricing.RuleID,
		MarkupBPS:             pricing.MarkupBPS,
		UpstreamCostMicrousd:  pricing.UpstreamCostMicrousd,
		UncappedChargeCredits: pricing.UncappedChargeCredits,
		ProfitMicrousd:        int64(settled)*10000 - pricing.UpstreamCostMicrousd,
		CapApplied:            pricing.CapApplied,
	}
	if subject != nil {
		event.UserID = optionalUserID(subject.UserID)
		event.APIKeyID = subject.APIKeyID
	}
	event.ApplyAuditContext(req.AuditContext)
	if settled > 0 && usage.ImageCount > 0 && usage.PromptTokens == 0 && usage.CompletionTokens == 0 && usage.ReasoningTokens == 0 && pricing.UpstreamCostMicrousd == 0 {
		slog.Warn("hosted_image_usage_missing",
			"request_id", req.RequestID,
			"model", modelName,
			"size", effectiveImageSize(req),
			"user_id", event.UserID,
			"api_key_id", event.APIKeyID,
		)
	}
	return event
}

// handleDecodeFailureChargeOnly is invoked when the upstream returned 200
// but the body could not be decoded (or was empty). Default behavior is to
// log + return — the product is lost to the user but the account is not
// charged. When ReconcileEnabled is set, a charge_failed_post_upstream
// ledger row is written so an out-of-band reconciliation can compensate.
func (s *Service) handleDecodeFailureChargeOnly(ctx context.Context, subject *hostedSubject, req any, modelName string, usage usageSummary, isImage bool, cause error) {
	var requestID string
	switch r := req.(type) {
	case CompletionRequest:
		requestID = r.RequestID
	case ImageRequest:
		requestID = r.RequestID
	}
	slog.Warn("hosted_decode_failure_post_upstream",
		"request_id", requestID,
		"model", modelName,
		"is_image", isImage,
		"err", cause,
	)
	if !s.reconcileEnabled.Load() {
		return
	}
	pricing := s.priceUsage(ctx, modelName, usage, isImage)
	meta := map[string]any{
		"reason":               "decode_failure",
		"upstream_request_id":  requestID,
		"pricing_rule_id":      pricing.RuleID,
		"decode_error":         cause.Error(),
		"upstream_http_status": 200,
	}
	if writeErr := s.writeChargeFailedForSubject(ctx, subject, requestID, pricing.ChargeCredits, meta); writeErr != nil {
		slog.Warn("hosted_charge_failed_write_failed",
			"request_id", requestID,
			"err", writeErr,
		)
	}
}

func creditBalance(key *model.APIKey) int {
	if key == nil {
		return 0
	}
	return key.CreditBalance
}

func accountCreditBalance(account *model.UserHostedCreditAccount) int {
	if account == nil {
		return 0
	}
	return account.CreditBalance
}

func stringPtr(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func optionalUserID(userID uint64) *uint64 {
	if userID == 0 {
		return nil
	}
	return &userID
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
