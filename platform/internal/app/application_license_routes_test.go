package app

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/officecli/officecli/platform/internal/httpapi"
	licensesvc "github.com/officecli/officecli/platform/internal/license"
	"github.com/officecli/officecli/platform/internal/model"
	rewardsvc "github.com/officecli/officecli/platform/internal/reward"
)

type testAPIKeyStore struct{}

func (testAPIKeyStore) FindByHash(_ context.Context, _ string) (*model.APIKey, error) {
	return nil, nil
}

func (testAPIKeyStore) TouchLastUsedAt(_ context.Context, _ uint64, _ time.Time) error {
	return nil
}

func (testAPIKeyStore) ConsumePaidByHash(_ context.Context, _ string) (*model.APIKey, error) {
	return nil, licensesvc.ErrPaidQuotaExhausted
}

type testFreeQuotaStore struct {
	mu     sync.Mutex
	quotas map[string]*model.DailyFreeQuota
}

func newTestFreeQuotaStore() *testFreeQuotaStore {
	return &testFreeQuotaStore{quotas: map[string]*model.DailyFreeQuota{}}
}

func (f *testFreeQuotaStore) GetOrCreateByFingerprint(_ context.Context, fingerprint string, usageDate string, defaultLimit int) (*model.DailyFreeQuota, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if quota, ok := f.quotas[fingerprint+"|"+usageDate]; ok {
		copied := *quota
		return &copied, false, nil
	}
	quota := &model.DailyFreeQuota{FingerprintHash: fingerprint, UsageDate: usageDate, DailyLimit: defaultLimit}
	f.quotas[fingerprint+"|"+usageDate] = quota
	copied := *quota
	return &copied, true, nil
}

func (f *testFreeQuotaStore) GetByFingerprint(_ context.Context, fingerprint string, usageDate string) (*model.DailyFreeQuota, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	quota := f.quotas[fingerprint+"|"+usageDate]
	if quota == nil {
		return nil, nil
	}
	copied := *quota
	return &copied, nil
}

func (f *testFreeQuotaStore) Consume(_ context.Context, fingerprint string, usageDate string, defaultLimit int) (*model.DailyFreeQuota, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := fingerprint + "|" + usageDate
	quota := f.quotas[key]
	if quota == nil {
		quota = &model.DailyFreeQuota{FingerprintHash: fingerprint, UsageDate: usageDate, DailyLimit: defaultLimit}
		f.quotas[key] = quota
	}
	if quota.DailyUsed >= quota.DailyLimit {
		return nil, licensesvc.ErrQuotaExhausted
	}
	quota.DailyUsed++
	copied := *quota
	return &copied, nil
}

type testUsageStore struct {
	mu        sync.Mutex
	byRequest map[string]*model.UsageEvent
}

type testRewardManager struct {
	mu       sync.Mutex
	balances map[uint64]int
}

func newTestRewardManager() *testRewardManager {
	return &testRewardManager{balances: map[uint64]int{}}
}

func (m *testRewardManager) Balance(_ context.Context, userID uint64) (*rewardsvc.Balance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return &rewardsvc.Balance{UserID: userID, Remaining: m.balances[userID]}, nil
}

func (m *testRewardManager) Consume(_ context.Context, userID uint64) (*rewardsvc.ConsumeResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.balances[userID] <= 0 {
		return nil, rewardsvc.ErrQuotaExhausted
	}
	m.balances[userID]--
	return &rewardsvc.ConsumeResult{Remaining: m.balances[userID]}, nil
}

func newTestUsageStore() *testUsageStore {
	return &testUsageStore{byRequest: map[string]*model.UsageEvent{}}
}

func (s *testUsageStore) Create(_ context.Context, event *model.UsageEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if event.RequestID != nil {
		cloned := *event
		s.byRequest[*event.RequestID] = &cloned
	}
	return nil
}

func (s *testUsageStore) FindByRequestID(_ context.Context, requestID string) (*model.UsageEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if event, ok := s.byRequest[requestID]; ok {
		cloned := *event
		return &cloned, nil
	}
	return nil, nil
}

type testIdemStore struct{}

func (testIdemStore) GetConsumeResult(_ context.Context, _ string) (*licensesvc.ConsumeResponse, error) {
	return nil, nil
}

func (testIdemStore) SaveConsumeResult(_ context.Context, _ string, _ *licensesvc.ConsumeResponse, _ time.Duration) error {
	return nil
}

func (testIdemStore) AcquireConsumeLock(_ context.Context, _ string, _ time.Duration) (bool, error) {
	return true, nil
}

func (testIdemStore) ReleaseConsumeLock(_ context.Context, _ string) error {
	return nil
}

const routeTestProofSeed = "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA"

func issueRouteCommitToken(t *testing.T, _ *licensesvc.Service, fingerprint string, accessMode model.AccessMode, extras func(*licensesvc.CheckRequest)) *licensesvc.CommitToken {
	t.Helper()
	req := licensesvc.CheckRequest{
		FingerprintHash: fingerprint,
		RequestNonce:    "nonce-" + fingerprint + "-" + string(accessMode),
		Action:          string(model.UsageActionGenerate),
	}
	if extras != nil {
		extras(&req)
	}
	seed, err := base64.RawURLEncoding.DecodeString(routeTestProofSeed)
	if err != nil {
		t.Fatalf("DecodeString() error = %v", err)
	}
	privateKey := ed25519.NewKeyFromSeed(seed)
	now := time.Now().UTC()
	token := &licensesvc.CommitToken{
		FingerprintHash: req.FingerprintHash,
		UserID:          req.UserID,
		RequestID:       "req-" + fingerprint + "-" + string(accessMode),
		AccessMode:      accessMode,
		Action:          req.Action,
		DocumentType:    req.DocumentType,
		RuntimeMode:     req.RuntimeMode,
		RequestNonce:    req.RequestNonce,
		ProofVersion:    "v1",
		IssuedAt:        now,
		ExpiresAt:       now.Add(2 * time.Minute),
	}
	token.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, []byte(routeCommitTokenPayload(*token))))
	return token
}

func routeCommitTokenPayload(token licensesvc.CommitToken) string {
	parts := []string{
		"version=" + strings.TrimSpace(token.ProofVersion),
		"fingerprint_hash=" + strings.TrimSpace(token.FingerprintHash),
		"user_id=" + strconv.FormatUint(token.UserID, 10),
		"request_id=" + strings.TrimSpace(token.RequestID),
		"access_mode=" + strings.TrimSpace(string(token.AccessMode)),
		"api_key_hint=" + strings.TrimSpace(token.APIKeyHint),
		"action=" + strings.TrimSpace(token.Action),
		"document_type=" + strings.TrimSpace(token.DocumentType),
		"runtime_mode=" + strings.TrimSpace(token.RuntimeMode),
		"request_nonce=" + strings.TrimSpace(token.RequestNonce),
		"issued_at=" + token.IssuedAt.UTC().Format(time.RFC3339Nano),
		"expires_at=" + token.ExpiresAt.UTC().Format(time.RFC3339Nano),
	}
	return strings.Join(parts, "\n")
}

func TestRegisterLicenseRoutesCheckReturnsBadRequestOnInvalidBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api")
	lic := licensesvc.NewService(testAPIKeyStore{}, newTestFreeQuotaStore(), newTestUsageStore(), testIdemStore{}, nil, nil, "salt", 10, time.Hour)
	registerLicenseRoutes(api, lic)

	req := httptest.NewRequest(http.MethodPost, "/api/license/check", bytes.NewBufferString("{invalid"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestRegisterLicenseRoutesConsumeReturnsConflictOnFreeQuotaExhausted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api")
	quotas := newTestFreeQuotaStore()
	today := time.Now().UTC().Format("2006-01-02")
	quotas.quotas["fp-1|"+today] = &model.DailyFreeQuota{FingerprintHash: "fp-1", UsageDate: today, DailyLimit: 1, DailyUsed: 1}
	lic := licensesvc.NewService(testAPIKeyStore{}, quotas, newTestUsageStore(), testIdemStore{}, nil, nil, "salt", 10, time.Hour)
	registerLicenseRoutes(api, lic)
	token := issueRouteCommitToken(t, lic, "fp-1", model.AccessModeFree, nil)

	body, _ := json.Marshal(licensesvc.ConsumeRequest{
		FingerprintHash: "fp-1",
		RequestID:       token.RequestID,
		UsageType:       "generate",
		AccessMode:      model.AccessModeFree,
		CommitToken:     token,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/license/consume", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestRegisterLicenseRoutesConsumeReturnsBadRequestOnPaidConsumeWithoutAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api")
	lic := licensesvc.NewService(testAPIKeyStore{}, newTestFreeQuotaStore(), newTestUsageStore(), testIdemStore{}, nil, nil, "salt", 10, time.Hour)
	registerLicenseRoutes(api, lic)
	token := issueRouteCommitToken(t, lic, "fp-paid", model.AccessModePaid, nil)

	body, _ := json.Marshal(licensesvc.ConsumeRequest{
		FingerprintHash: "fp-paid",
		RequestID:       token.RequestID,
		UsageType:       "generate",
		AccessMode:      model.AccessModePaid,
		CommitToken:     token,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/license/consume", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestRegisterLicenseRoutesCheckErrorIncludesRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(httpapi.RequestIDMiddleware())
	api := router.Group("/api")
	lic := licensesvc.NewService(testAPIKeyStore{}, newTestFreeQuotaStore(), newTestUsageStore(), testIdemStore{}, nil, nil, "salt", 10, time.Hour)
	registerLicenseRoutes(api, lic)

	req := httptest.NewRequest(http.MethodPost, "/api/license/check", bytes.NewBufferString("{invalid"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if body["error"] == "" {
		t.Fatalf("error = %#v", body["error"])
	}
	requestID, ok := body["request_id"].(string)
	if !ok || requestID == "" {
		t.Fatalf("request_id = %#v", body["request_id"])
	}
}

func TestRegisterLicenseRoutesCheckSuccessIncludesRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(httpapi.RequestIDMiddleware())
	api := router.Group("/api")
	lic := licensesvc.NewService(testAPIKeyStore{}, newTestFreeQuotaStore(), newTestUsageStore(), testIdemStore{}, nil, nil, "salt", 10, time.Hour)
	registerLicenseRoutes(api, lic)

	bodyBytes, _ := json.Marshal(licensesvc.CheckRequest{FingerprintHash: "fp-1", RequestNonce: "nonce-fp-1-generate", Action: "generate"})
	req := httptest.NewRequest(http.MethodPost, "/api/license/check", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	requestID, ok := body["request_id"].(string)
	if !ok || requestID == "" {
		t.Fatalf("request_id = %#v", body["request_id"])
	}
}

func TestRegisterLicenseRoutesCheckReturnsRewardResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api")
	rewards := newTestRewardManager()
	rewards.balances[42] = 3
	lic := licensesvc.NewService(testAPIKeyStore{}, newTestFreeQuotaStore(), newTestUsageStore(), testIdemStore{}, rewards, nil, "salt", 10, time.Hour)
	registerLicenseRoutes(api, lic)

	body, _ := json.Marshal(licensesvc.CheckRequest{
		FingerprintHash: "fp-reward-check",
		UserID:          42,
		RequestNonce:    "nonce-fp-reward-check-generate",
		Action:          string(model.UsageActionGenerate),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/license/check", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var response struct {
		Data licensesvc.CheckResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if response.Data.AccessMode != model.AccessModeReward || response.Data.RewardRemaining != 3 {
		t.Fatalf("response = %+v", response.Data)
	}
	if response.Data.CommitToken == nil || response.Data.CommitToken.AccessMode != model.AccessModeReward {
		t.Fatalf("commit token = %+v", response.Data.CommitToken)
	}
}

func TestRegisterLicenseRoutesConsumeReturnsConflictOnRewardQuotaExhausted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api")
	rewards := newTestRewardManager()
	lic := licensesvc.NewService(testAPIKeyStore{}, newTestFreeQuotaStore(), newTestUsageStore(), testIdemStore{}, rewards, nil, "salt", 10, time.Hour)
	registerLicenseRoutes(api, lic)
	token := issueRouteCommitToken(t, lic, "fp-reward-consume", model.AccessModeReward, func(req *licensesvc.CheckRequest) {
		req.UserID = 42
	})

	body, _ := json.Marshal(licensesvc.ConsumeRequest{
		FingerprintHash: "fp-reward-consume",
		UserID:          42,
		RequestID:       token.RequestID,
		UsageType:       string(model.UsageActionGenerate),
		AccessMode:      model.AccessModeReward,
		CommitToken:     token,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/license/consume", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
