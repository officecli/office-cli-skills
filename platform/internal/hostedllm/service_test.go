package hostedllm

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/officecli/officecli-internal/platform/internal/apikey"
	"github.com/officecli/officecli-internal/platform/internal/clisession"
	licensesvc "github.com/officecli/officecli-internal/platform/internal/license"
	"github.com/officecli/officecli-internal/platform/internal/model"
)

type fakeAPIKeyStore struct {
	key            *model.APIKey
	keysByHash     map[string]*model.APIKey
	sessionsByHash map[string]*model.CLISession
	userKeys       map[uint64]*model.UserAIGatewayAPIKey
	settings       *model.HostedPricingSetting
	rules          []model.HostedPricingRule
	modelConfigs   []model.HostedModelPricingConfig
	reservations   []int
	hostedRequests []string
	releases       []int
	settlements    []int
	events         []*model.UsageEvent
	releaseCtxErrs []error
	settleCtxErrs  []error
	eventCtxErrs   []error
}

func (f *fakeAPIKeyStore) FindAPIKeyByHash(_ context.Context, hash string) (*model.APIKey, error) {
	if f.keysByHash != nil {
		if key, ok := f.keysByHash[hash]; ok && key != nil {
			cloned := *key
			return &cloned, nil
		}
		return nil, nil
	}
	if f.key == nil {
		return nil, nil
	}
	cloned := *f.key
	return &cloned, nil
}

func (f *fakeAPIKeyStore) ReserveCreditsByHash(_ context.Context, hash string, credits int) (*model.APIKey, error) {
	f.reservations = append(f.reservations, credits)
	return f.FindAPIKeyByHash(context.Background(), hash)
}

func (f *fakeAPIKeyStore) ReleaseReservedCredits(ctx context.Context, apiKeyID uint64, reserved int) (*model.APIKey, error) {
	f.releaseCtxErrs = append(f.releaseCtxErrs, ctx.Err())
	f.releases = append(f.releases, reserved)
	return f.FindAPIKeyByHash(context.Background(), "")
}

func (f *fakeAPIKeyStore) SettleReservedCredits(ctx context.Context, apiKeyID uint64, reserved int, settled int) (*model.APIKey, error) {
	f.settleCtxErrs = append(f.settleCtxErrs, ctx.Err())
	f.settlements = append(f.settlements, settled)
	key, err := f.FindAPIKeyByHash(context.Background(), "")
	if key == nil && f.keysByHash != nil {
		for _, candidate := range f.keysByHash {
			if candidate != nil && candidate.ID == apiKeyID {
				cloned := *candidate
				key = &cloned
				break
			}
		}
	}
	if key != nil {
		key.CreditBalance -= settled
	}
	return key, err
}

func (f *fakeAPIKeyStore) ReserveHostedCreditsByUser(_ context.Context, userID uint64, requestID string, credits int) (*model.UserHostedCreditAccount, error) {
	if requestID == "" {
		return nil, fmt.Errorf("request_id is required")
	}
	f.hostedRequests = append(f.hostedRequests, requestID)
	f.reservations = append(f.reservations, credits)
	return &model.UserHostedCreditAccount{UserID: userID, CreditBalance: 100, CreditReserved: credits}, nil
}

func (f *fakeAPIKeyStore) ReleaseHostedCreditsByUser(ctx context.Context, userID uint64, requestID string, reserved int) (*model.UserHostedCreditAccount, error) {
	f.releaseCtxErrs = append(f.releaseCtxErrs, ctx.Err())
	f.releases = append(f.releases, reserved)
	return &model.UserHostedCreditAccount{UserID: userID, CreditBalance: 100}, nil
}

func (f *fakeAPIKeyStore) SettleHostedCreditsByUser(ctx context.Context, userID uint64, requestID string, reserved int, settled int) (*model.UserHostedCreditAccount, error) {
	f.settleCtxErrs = append(f.settleCtxErrs, ctx.Err())
	f.settlements = append(f.settlements, settled)
	return &model.UserHostedCreditAccount{UserID: userID, CreditBalance: 100 - settled}, nil
}

func (f *fakeAPIKeyStore) FindCLISessionByTokenHash(_ context.Context, tokenHash string) (*model.CLISession, error) {
	if f.sessionsByHash != nil {
		if session, ok := f.sessionsByHash[tokenHash]; ok && session != nil {
			cloned := *session
			return &cloned, nil
		}
	}
	return nil, nil
}

func (f *fakeAPIKeyStore) TouchCLISession(_ context.Context, id uint64, usedAt time.Time) error {
	return nil
}

func (f *fakeAPIKeyStore) CreateUsageEvent(ctx context.Context, event *model.UsageEvent) error {
	f.eventCtxErrs = append(f.eventCtxErrs, ctx.Err())
	f.events = append(f.events, event)
	return nil
}

func (f *fakeAPIKeyStore) HostedPricingSettings(_ context.Context) (*model.HostedPricingSetting, error) {
	if f.settings != nil {
		cloned := *f.settings
		return &cloned, nil
	}
	return &model.HostedPricingSetting{ID: 1, MarkupBPS: 3000, Currency: "usd"}, nil
}

func (f *fakeAPIKeyStore) ListHostedPricingRules(_ context.Context, enabledOnly bool) ([]model.HostedPricingRule, error) {
	var out []model.HostedPricingRule
	for _, rule := range f.rules {
		if enabledOnly && !rule.Enabled {
			continue
		}
		out = append(out, rule)
	}
	return out, nil
}

func (f *fakeAPIKeyStore) ListHostedModelPricingConfigs(_ context.Context, enabledOnly bool) ([]model.HostedModelPricingConfig, error) {
	var out []model.HostedModelPricingConfig
	for _, cfg := range f.modelConfigs {
		if enabledOnly && !cfg.Enabled {
			continue
		}
		out = append(out, cfg)
	}
	return out, nil
}

func (f *fakeAPIKeyStore) FindUserAIGatewayAPIKeyByUserID(_ context.Context, userID uint64) (*model.UserAIGatewayAPIKey, error) {
	if f.userKeys == nil {
		return nil, nil
	}
	if key, ok := f.userKeys[userID]; ok && key != nil {
		cloned := *key
		return &cloned, nil
	}
	return nil, nil
}

func (f *fakeAPIKeyStore) ClaimUserAIGatewayAPIKeyCreation(_ context.Context, userID uint64, upstreamName string) (*model.UserAIGatewayAPIKey, error) {
	if f.userKeys == nil {
		f.userKeys = map[uint64]*model.UserAIGatewayAPIKey{}
	}
	if key, ok := f.userKeys[userID]; ok && key != nil {
		if key.Status == model.UserAIGatewayAPIKeyStatusError {
			key.Status = model.UserAIGatewayAPIKeyStatusCreating
			key.LastError = ""
			key.CreationClaimed = true
		} else {
			key.CreationClaimed = false
		}
		cloned := *key
		return &cloned, nil
	}
	key := &model.UserAIGatewayAPIKey{
		ID:              uint64(len(f.userKeys) + 1),
		UserID:          userID,
		Status:          model.UserAIGatewayAPIKeyStatusCreating,
		UpstreamName:    upstreamName,
		CreationClaimed: true,
	}
	f.userKeys[userID] = key
	cloned := *key
	return &cloned, nil
}

func (f *fakeAPIKeyStore) ActivateUserAIGatewayAPIKey(_ context.Context, userID uint64, ciphertext, prefix, upstreamID, upstreamName string) (*model.UserAIGatewayAPIKey, error) {
	if f.userKeys == nil {
		f.userKeys = map[uint64]*model.UserAIGatewayAPIKey{}
	}
	key := f.userKeys[userID]
	if key == nil {
		key = &model.UserAIGatewayAPIKey{ID: uint64(len(f.userKeys) + 1), UserID: userID}
		f.userKeys[userID] = key
	}
	key.Status = model.UserAIGatewayAPIKeyStatusActive
	key.KeyCiphertext = ciphertext
	key.KeyPrefix = prefix
	key.UpstreamID = upstreamID
	key.UpstreamName = upstreamName
	key.LastError = ""
	cloned := *key
	return &cloned, nil
}

func (f *fakeAPIKeyStore) MarkUserAIGatewayAPIKeyCreationError(_ context.Context, userID uint64, upstreamName, message string) (*model.UserAIGatewayAPIKey, error) {
	if f.userKeys == nil {
		f.userKeys = map[uint64]*model.UserAIGatewayAPIKey{}
	}
	key := f.userKeys[userID]
	if key == nil {
		key = &model.UserAIGatewayAPIKey{ID: uint64(len(f.userKeys) + 1), UserID: userID}
		f.userKeys[userID] = key
	}
	key.Status = model.UserAIGatewayAPIKeyStatusError
	key.UpstreamName = upstreamName
	key.LastError = message
	cloned := *key
	return &cloned, nil
}

type fakeGenerationQuotaManager struct {
	validations []licensesvc.ConsumeRequest
	consumes    []licensesvc.ConsumeRequest
	result      *licensesvc.ConsumeResponse
	validateErr error
}

func (f *fakeGenerationQuotaManager) ValidateCommitToken(req licensesvc.ConsumeRequest) error {
	f.validations = append(f.validations, req)
	if f.validateErr != nil {
		return f.validateErr
	}
	return nil
}

func (f *fakeGenerationQuotaManager) Consume(_ context.Context, req licensesvc.ConsumeRequest) (*licensesvc.ConsumeResponse, error) {
	f.consumes = append(f.consumes, req)
	if f.result != nil {
		return f.result, nil
	}
	return &licensesvc.ConsumeResponse{AccessMode: model.AccessModePaid, Remaining: 8, PaidQuotaRemaining: 8}, nil
}

type fakeAIGatewayAdminClient struct {
	keys       []string
	upstreamID string
	err        error
}

func (f *fakeAIGatewayAdminClient) CreateAPIKey(_ context.Context, req CreateAIGatewayAPIKeyRequest) (*CreatedAIGatewayAPIKey, error) {
	if f.err != nil {
		return nil, f.err
	}
	key := fmt.Sprintf("sk-user-%s", req.Name)
	if len(f.keys) > 0 {
		key = f.keys[0]
		f.keys = f.keys[1:]
	}
	upstreamID := f.upstreamID
	if upstreamID == "" {
		upstreamID = "upstream-" + req.Name
	}
	return &CreatedAIGatewayAPIKey{PlaintextKey: key, UpstreamID: upstreamID, Name: req.Name}, nil
}

func testAIGatewayCipher(t *testing.T) *apikey.Cipher {
	t.Helper()
	cipher, err := apikey.NewCipher(apikey.DefaultDevEncryptionKey)
	require.NoError(t, err)
	return cipher
}

func TestCompleteWithCLISessionGeneratesMissingRequestID(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/chat/completions", r.URL.Path)
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer upstream.Close()

	cipher := testAIGatewayCipher(t)
	ciphertext, err := cipher.Encrypt("sk-user-42")
	require.NoError(t, err)

	sessionToken := "ocli_sess_test"
	store := &fakeAPIKeyStore{
		sessionsByHash: map[string]*model.CLISession{
			clisession.HashToken(sessionToken): {
				ID:        9,
				UserID:    42,
				ExpiresAt: time.Now().UTC().Add(time.Hour),
			},
		},
		userKeys: map[uint64]*model.UserAIGatewayAPIKey{
			42: {
				UserID:        42,
				Status:        model.UserAIGatewayAPIKeyStatusActive,
				KeyCiphertext: ciphertext,
			},
		},
		rules: []model.HostedPricingRule{{
			DocumentProfile:      "text",
			ReservationCredits:   1,
			MinimumChargeCredits: 1,
		}},
	}
	svc := NewService(store, Config{
		BaseURL:            upstream.URL,
		TextModel:          "gpt-test",
		AIGatewayKeyCipher: cipher,
		TimeoutSec:         5,
	})

	resp, err := svc.Complete(context.Background(), "Bearer "+sessionToken, CompletionRequest{
		Model:    "hosted/text",
		Messages: []ChatMessage{{Role: "user", Content: "hello"}},
	})
	require.NoError(t, err)
	require.Equal(t, "ok", resp.Content)
	require.Len(t, store.hostedRequests, 1)
	require.NotEmpty(t, store.hostedRequests[0])
	require.NotNil(t, store.events[0].RequestID)
	require.Equal(t, store.hostedRequests[0], *store.events[0].RequestID)
}

func TestCompleteCreatesAndReusesUserAIGatewayAPIKey(t *testing.T) {
	var upstreamAuth []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/chat/completions", r.URL.Path)
		upstreamAuth = append(upstreamAuth, r.Header.Get("Authorization"))
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer upstream.Close()

	defaultRuntimeMode := "hosted"
	userID := uint64(42)
	store := &fakeAPIKeyStore{
		key: &model.APIKey{
			ID:                 7,
			OwnerUserID:        &userID,
			Status:             model.APIKeyStatusActive,
			PlanName:           "Hosted",
			KeyPrefix:          "cop_hosted",
			AllowedModes:       "hosted_only",
			HostedEnabled:      true,
			DefaultRuntimeMode: &defaultRuntimeMode,
			CreditBalance:      100,
		},
	}
	adminClient := &fakeAIGatewayAdminClient{keys: []string{"sk-user-42"}}
	svc := NewService(store, Config{
		BaseURL:              upstream.URL,
		HashSalt:             "salt",
		TextModel:            "gpt-test",
		AIGatewayKeyCipher:   testAIGatewayCipher(t),
		AIGatewayAdminClient: adminClient,
		Rules: []model.HostedPricingRule{{
			DocumentProfile:      "text",
			ReservationCredits:   1,
			MinimumChargeCredits: 1,
		}},
		TimeoutSec: 5,
	})

	for i := 0; i < 2; i++ {
		resp, err := svc.Complete(context.Background(), "Bearer hosted-key", CompletionRequest{
			Model:    "hosted/text",
			Messages: []ChatMessage{{Role: "user", Content: "hello"}},
		})
		require.NoError(t, err)
		require.Equal(t, "ok", resp.Content)
	}

	require.Equal(t, []string{"Bearer sk-user-42", "Bearer sk-user-42"}, upstreamAuth)
	require.Len(t, store.userKeys, 1)
	require.Equal(t, model.UserAIGatewayAPIKeyStatusActive, store.userKeys[userID].Status)
	require.Equal(t, "sk-user-42", store.userKeys[userID].KeyPrefix)
	require.NotContains(t, store.userKeys[userID].KeyCiphertext, "sk-user-42")
}

func TestCompleteRotatesStoredUserAIGatewayAPIKeyAfterUpstreamAuthFailure(t *testing.T) {
	var upstreamAuth []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/chat/completions", r.URL.Path)
		auth := r.Header.Get("Authorization")
		upstreamAuth = append(upstreamAuth, auth)
		if auth == "Bearer stale-key" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = fmt.Fprint(w, `{"error":{"message":"invalid token"}}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer upstream.Close()

	defaultRuntimeMode := "hosted"
	userID := uint64(42)
	cipher := testAIGatewayCipher(t)
	staleCiphertext, err := cipher.Encrypt("stale-key")
	require.NoError(t, err)
	store := &fakeAPIKeyStore{
		key: &model.APIKey{
			ID:                 7,
			OwnerUserID:        &userID,
			Status:             model.APIKeyStatusActive,
			PlanName:           "Hosted",
			KeyPrefix:          "cop_hosted",
			AllowedModes:       "hosted_only",
			HostedEnabled:      true,
			DefaultRuntimeMode: &defaultRuntimeMode,
			CreditBalance:      100,
		},
		userKeys: map[uint64]*model.UserAIGatewayAPIKey{
			userID: {
				ID:            1,
				UserID:        userID,
				Status:        model.UserAIGatewayAPIKeyStatusActive,
				KeyCiphertext: staleCiphertext,
				KeyPrefix:     "stale-key",
				UpstreamName:  "officecli-user-42",
			},
		},
	}
	svc := NewService(store, Config{
		BaseURL:              upstream.URL,
		HashSalt:             "salt",
		TextModel:            "gpt-test",
		AIGatewayKeyCipher:   cipher,
		AIGatewayAdminClient: &fakeAIGatewayAdminClient{keys: []string{"fresh-key"}},
		Rules: []model.HostedPricingRule{{
			DocumentProfile:      "text",
			ReservationCredits:   1,
			MinimumChargeCredits: 1,
		}},
		TimeoutSec: 5,
	})

	resp, err := svc.Complete(context.Background(), "Bearer hosted-key", CompletionRequest{
		Model:    "hosted/text",
		Messages: []ChatMessage{{Role: "user", Content: "hello"}},
	})

	require.NoError(t, err)
	require.Equal(t, "ok", resp.Content)
	require.Equal(t, []string{"Bearer stale-key", "Bearer fresh-key"}, upstreamAuth)
	require.Equal(t, model.UserAIGatewayAPIKeyStatusActive, store.userKeys[userID].Status)
	require.Equal(t, "fresh-key", store.userKeys[userID].KeyPrefix)
	require.Equal(t, []int{1}, store.releases)
	require.Equal(t, []int{1}, store.settlements)
}

func TestCompleteWithCommitTokenUsesAnonymousQuotaAccess(t *testing.T) {
	var upstreamAuth []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/chat/completions", r.URL.Path)
		upstreamAuth = append(upstreamAuth, r.Header.Get("Authorization"))
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer upstream.Close()

	quota := &fakeGenerationQuotaManager{}
	store := &fakeAPIKeyStore{}
	svc := NewService(store, Config{
		BaseURL:    upstream.URL,
		APIKey:     "platform-upstream-key",
		HashSalt:   "salt",
		TextModel:  "gpt-test",
		TimeoutSec: 5,
	}, quota)

	resp, err := svc.Complete(context.Background(), "", CompletionRequest{
		Model:           "hosted/text",
		Messages:        []ChatMessage{{Role: "user", Content: "hello"}},
		FingerprintHash: "fp-anon",
		AccessMode:      model.AccessModeFree,
		CommitToken: &licensesvc.CommitToken{
			FingerprintHash: "fp-anon",
			RequestID:       "req-anon",
			AccessMode:      model.AccessModeFree,
			RuntimeMode:     "hosted",
			Action:          "generate",
			DocumentType:    "docx",
		},
	})

	require.NoError(t, err)
	require.Equal(t, "ok", resp.Content)
	require.Equal(t, []string{"Bearer platform-upstream-key"}, upstreamAuth)
	require.Len(t, quota.validations, 1)
	require.Len(t, quota.consumes, 1)
	require.Equal(t, "req-anon", quota.consumes[0].RequestID)
	require.Empty(t, store.reservations)
	require.Empty(t, store.settlements)
}

func TestRecordUsageFingerprintSource(t *testing.T) {
	tests := []struct {
		name string
		req  CompletionRequest
		want string
	}{
		{
			name: "request fingerprint wins",
			req: CompletionRequest{
				RequestID:       "req-text-request",
				FingerprintHash: "fp-request",
				CommitToken:     &licensesvc.CommitToken{FingerprintHash: "fp-token"},
			},
			want: "fp-request",
		},
		{
			name: "commit token fallback",
			req: CompletionRequest{
				RequestID:   "req-text-token",
				CommitToken: &licensesvc.CommitToken{FingerprintHash: "fp-token"},
			},
			want: "fp-token",
		},
		{
			name: "missing fingerprint fallback",
			req:  CompletionRequest{RequestID: "req-text-hosted"},
			want: "hosted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeAPIKeyStore{}
			svc := NewService(store, Config{})
			svc.recordUsage(context.Background(), nil, tt.req, "gpt-text-test", usageSummary{}, 1, 1, 0, hostedPriceSnapshot{})

			require.Len(t, store.events, 1)
			require.Equal(t, tt.want, store.events[0].FingerprintHash)
		})
	}
}

func TestRecordImageUsageFingerprintSource(t *testing.T) {
	tests := []struct {
		name string
		req  ImageRequest
		want string
	}{
		{
			name: "request fingerprint wins",
			req: ImageRequest{
				RequestID:       "req-image-request",
				FingerprintHash: "fp-request",
				CommitToken:     &licensesvc.CommitToken{FingerprintHash: "fp-token"},
			},
			want: "fp-request",
		},
		{
			name: "commit token fallback",
			req: ImageRequest{
				RequestID:   "req-image-token",
				CommitToken: &licensesvc.CommitToken{FingerprintHash: "fp-token"},
			},
			want: "fp-token",
		},
		{
			name: "missing fingerprint fallback",
			req:  ImageRequest{RequestID: "req-image-hosted"},
			want: "hosted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeAPIKeyStore{}
			svc := NewService(store, Config{})
			svc.recordImageUsage(context.Background(), nil, tt.req, "gpt-image-test", usageSummary{ImageCount: 1}, 1, 1, 0, hostedPriceSnapshot{})

			require.Len(t, store.events, 1)
			require.Equal(t, tt.want, store.events[0].FingerprintHash)
			require.NotNil(t, store.events[0].DocumentType)
			require.Equal(t, "img", *store.events[0].DocumentType)
		})
	}
}

func TestCompleteWithInvalidCommitTokenRejectsBeforeUpstream(t *testing.T) {
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		t.Fatalf("upstream should not be called for invalid commit token")
	}))
	defer upstream.Close()

	quota := &fakeGenerationQuotaManager{validateErr: fmt.Errorf("invalid commit_token: license proof runtime mode mismatch")}
	svc := NewService(&fakeAPIKeyStore{}, Config{
		BaseURL:    upstream.URL,
		APIKey:     "platform-upstream-key",
		HashSalt:   "salt",
		TextModel:  "gpt-test",
		TimeoutSec: 5,
	}, quota)

	_, err := svc.Complete(context.Background(), "", CompletionRequest{
		Model:           "hosted/text",
		Messages:        []ChatMessage{{Role: "user", Content: "hello"}},
		FingerprintHash: "fp-anon",
		AccessMode:      model.AccessModeFree,
		CommitToken: &licensesvc.CommitToken{
			FingerprintHash: "fp-anon",
			RequestID:       "req-anon",
			AccessMode:      model.AccessModeFree,
			RuntimeMode:     "external",
			Action:          "generate",
			DocumentType:    "docx",
		},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "runtime mode mismatch")
	require.Len(t, quota.validations, 1)
	require.Equal(t, 0, upstreamCalls)
}

func TestCompleteUsesDifferentAIGatewayAPIKeysForDifferentUsers(t *testing.T) {
	var upstreamAuth []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamAuth = append(upstreamAuth, r.Header.Get("Authorization"))
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer upstream.Close()

	defaultRuntimeMode := "hosted"
	user42 := uint64(42)
	user43 := uint64(43)
	store := &fakeAPIKeyStore{
		keysByHash: map[string]*model.APIKey{
			hashAPIKey("office-key-42", "salt"): {
				ID:                 7,
				OwnerUserID:        &user42,
				Status:             model.APIKeyStatusActive,
				PlanName:           "Hosted",
				KeyPrefix:          "cop_hosted_a",
				AllowedModes:       "hosted_only",
				HostedEnabled:      true,
				DefaultRuntimeMode: &defaultRuntimeMode,
				CreditBalance:      100,
			},
			hashAPIKey("office-key-43", "salt"): {
				ID:                 8,
				OwnerUserID:        &user43,
				Status:             model.APIKeyStatusActive,
				PlanName:           "Hosted",
				KeyPrefix:          "cop_hosted_b",
				AllowedModes:       "hosted_only",
				HostedEnabled:      true,
				DefaultRuntimeMode: &defaultRuntimeMode,
				CreditBalance:      100,
			},
		},
	}
	svc := NewService(store, Config{
		BaseURL:              upstream.URL,
		HashSalt:             "salt",
		TextModel:            "gpt-test",
		AIGatewayKeyCipher:   testAIGatewayCipher(t),
		AIGatewayAdminClient: &fakeAIGatewayAdminClient{keys: []string{"sk-user-42", "sk-user-43"}},
		Rules: []model.HostedPricingRule{{
			DocumentProfile:      "text",
			ReservationCredits:   1,
			MinimumChargeCredits: 1,
		}},
		TimeoutSec: 5,
	})

	for _, bearer := range []string{"Bearer office-key-42", "Bearer office-key-43"} {
		_, err := svc.Complete(context.Background(), bearer, CompletionRequest{
			Model:    "hosted/text",
			Messages: []ChatMessage{{Role: "user", Content: "hello"}},
		})
		require.NoError(t, err)
	}

	require.Equal(t, []string{"Bearer sk-user-42", "Bearer sk-user-43"}, upstreamAuth)
	require.Len(t, store.userKeys, 2)
}

func TestCompleteRejectsHostedOfficeKeyWithoutOwner(t *testing.T) {
	defaultRuntimeMode := "hosted"
	store := &fakeAPIKeyStore{
		key: &model.APIKey{
			ID:                 7,
			Status:             model.APIKeyStatusActive,
			PlanName:           "Hosted",
			KeyPrefix:          "cop_hosted",
			AllowedModes:       "hosted_only",
			HostedEnabled:      true,
			DefaultRuntimeMode: &defaultRuntimeMode,
			CreditBalance:      100,
		},
	}
	svc := NewService(store, Config{BaseURL: "https://example.com", HashSalt: "salt", TextModel: "gpt-test", TimeoutSec: 5})

	_, err := svc.Complete(context.Background(), "Bearer hosted-key", CompletionRequest{
		Model:    "hosted/text",
		Messages: []ChatMessage{{Role: "user", Content: "hello"}},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "hosted mode requires an owner user")
	require.Empty(t, store.reservations)
}

func TestCompleteRejectsRemovedHostedPricingProfile(t *testing.T) {
	defaultRuntimeMode := "hosted"
	userID := uint64(42)
	store := &fakeAPIKeyStore{
		key: &model.APIKey{
			ID:                 7,
			OwnerUserID:        &userID,
			Status:             model.APIKeyStatusActive,
			PlanName:           "Hosted",
			KeyPrefix:          "cop_hosted",
			AllowedModes:       "hosted_only",
			HostedEnabled:      true,
			DefaultRuntimeMode: &defaultRuntimeMode,
			CreditBalance:      100,
		},
	}
	svc := NewService(store, Config{
		BaseURL:              "https://example.com",
		HashSalt:             "salt",
		TextModel:            "gpt-test",
		AIGatewayKeyCipher:   testAIGatewayCipher(t),
		AIGatewayAdminClient: &fakeAIGatewayAdminClient{keys: []string{"upstream-key"}},
		TimeoutSec:           5,
	})

	_, err := svc.Complete(context.Background(), "Bearer hosted-key", CompletionRequest{
		Model:    "hosted/" + "docx-" + "xlsx",
		Messages: []ChatMessage{{Role: "user", Content: "hello"}},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported hosted pricing profile")
	require.Empty(t, store.reservations)
	require.Empty(t, store.userKeys)
}

func TestCompleteDoesNotReserveCreditsWhenAIGatewayKeyCreationFails(t *testing.T) {
	defaultRuntimeMode := "hosted"
	userID := uint64(42)
	store := &fakeAPIKeyStore{
		key: &model.APIKey{
			ID:                 7,
			OwnerUserID:        &userID,
			Status:             model.APIKeyStatusActive,
			PlanName:           "Hosted",
			KeyPrefix:          "cop_hosted",
			AllowedModes:       "hosted_only",
			HostedEnabled:      true,
			DefaultRuntimeMode: &defaultRuntimeMode,
			CreditBalance:      100,
		},
	}
	svc := NewService(store, Config{
		BaseURL:              "https://example.com",
		HashSalt:             "salt",
		TextModel:            "gpt-test",
		AIGatewayKeyCipher:   testAIGatewayCipher(t),
		AIGatewayAdminClient: &fakeAIGatewayAdminClient{err: errors.New("gateway unavailable")},
		TimeoutSec:           5,
	})

	_, err := svc.Complete(context.Background(), "Bearer hosted-key", CompletionRequest{
		Model:    "hosted/text",
		Messages: []ChatMessage{{Role: "user", Content: "hello"}},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "gateway unavailable")
	require.Empty(t, store.reservations)
	require.Equal(t, model.UserAIGatewayAPIKeyStatusError, store.userKeys[userID].Status)
	require.Contains(t, store.userKeys[userID].LastError, "gateway unavailable")
}

func TestAuthorizeRejectsDisabledKey(t *testing.T) {
	defaultRuntimeMode := "hosted"
	svc := NewService(&fakeAPIKeyStore{
		key: &model.APIKey{
			ID:                 7,
			Status:             model.APIKeyStatusDisabled,
			PlanName:           "Hosted",
			KeyPrefix:          "cop_hosted",
			AllowedModes:       "hybrid",
			HostedEnabled:      true,
			DefaultRuntimeMode: &defaultRuntimeMode,
			CreditBalance:      100,
		},
	}, Config{
		BaseURL:    "https://example.com",
		HashSalt:   "salt",
		TextModel:  "gpt-test",
		ImageModel: "gpt-image-test",
		TimeoutSec: 5,
	})

	subject, err := svc.authorizeSubject(context.Background(), "Bearer demo")
	require.Error(t, err)
	require.Nil(t, subject)
	require.Contains(t, err.Error(), "disabled")
}

func TestNewServiceConfiguresHTTPTimeout(t *testing.T) {
	svc := NewService(&fakeAPIKeyStore{}, Config{TimeoutSec: 7})
	require.NotNil(t, svc.client)
	require.Equal(t, 7*time.Second, svc.client.Timeout)
}

func TestGenerateImageUsesImgPricingAndRecordsImgDocumentType(t *testing.T) {
	imageData := base64.StdEncoding.EncodeToString([]byte("png-bytes"))
	var upstreamPayload map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/images/generations", r.URL.Path)
		require.Equal(t, "Bearer upstream-key", r.Header.Get("Authorization"))
		require.NoError(t, json.NewDecoder(r.Body).Decode(&upstreamPayload))
		_, _ = fmt.Fprintf(w, `{"data":[{"b64_json":"%s"}]}`, imageData)
	}))
	defer upstream.Close()

	defaultRuntimeMode := "hosted"
	userID := uint64(42)
	store := &fakeAPIKeyStore{
		key: &model.APIKey{
			ID:                 7,
			OwnerUserID:        &userID,
			Status:             model.APIKeyStatusActive,
			PlanName:           "Hosted",
			KeyPrefix:          "cop_hosted",
			AllowedModes:       "hosted_only",
			HostedEnabled:      true,
			DefaultRuntimeMode: &defaultRuntimeMode,
			CreditBalance:      100,
		},
	}
	svc := NewService(store, Config{
		BaseURL:    upstream.URL,
		APIKey:     "upstream-key",
		HashSalt:   "salt",
		ImageModel: "gpt-image-test",
		Rules: []model.HostedPricingRule{{
			DocumentProfile:      "image",
			Provider:             "openai",
			Model:                "gpt-image-test",
			ImagePerAssetCredits: 1,
			ReservationCredits:   1,
			MinimumChargeCredits: 1,
		}},
		TimeoutSec:           5,
		AIGatewayKeyCipher:   testAIGatewayCipher(t),
		AIGatewayAdminClient: &fakeAIGatewayAdminClient{keys: []string{"upstream-key"}},
	})

	resp, err := svc.GenerateImage(context.Background(), "Bearer hosted-key", ImageRequest{
		Model:       "hosted/image",
		Prompt:      "A polished product launch hero image",
		AspectRatio: 16.0 / 9.0,
	})
	require.NoError(t, err)
	require.Equal(t, []byte("png-bytes"), resp.Data)
	require.Equal(t, "image/png", resp.MIME)
	require.Equal(t, 99, resp.CreditBalance)
	require.Equal(t, []int{1}, store.reservations)
	require.Equal(t, []int{1}, store.settlements)
	require.Empty(t, store.releases)
	require.Len(t, store.events, 1)
	require.NotNil(t, store.events[0].DocumentType)
	require.Equal(t, "img", *store.events[0].DocumentType)
	require.Equal(t, 1, store.events[0].BilledUnits)
	require.Equal(t, 1, store.events[0].ImageCount)
	require.Equal(t, "gpt-image-test", upstreamPayload["model"])
	require.Equal(t, "1536x1024", upstreamPayload["size"])
}

func TestGenerateImageFallsBackToResponsesWhenImageEndpointFails(t *testing.T) {
	imageData := base64.StdEncoding.EncodeToString([]byte("png-bytes"))
	var sawImages bool
	var sawResponses bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/images/generations":
			sawImages = true
			w.WriteHeader(http.StatusBadGateway)
			_, _ = fmt.Fprint(w, "<html><body>bad gateway</body></html>")
		case "/responses":
			sawResponses = true
			require.Equal(t, "Bearer upstream-key", r.Header.Get("Authorization"))
			var payload map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
			require.Equal(t, "gpt-image-test", payload["model"])
			require.Equal(t, "A polished product launch hero image", payload["input"])
			_, _ = fmt.Fprintf(w, `{"output":[{"type":"image_generation_call","result":"%s"}],"usage":{"prompt_tokens":3000,"completion_tokens":1000}}`, imageData)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	defaultRuntimeMode := "hosted"
	userID := uint64(42)
	store := &fakeAPIKeyStore{
		key: &model.APIKey{
			ID:                 7,
			OwnerUserID:        &userID,
			Status:             model.APIKeyStatusActive,
			PlanName:           "Hosted",
			KeyPrefix:          "cop_hosted",
			AllowedModes:       "hosted_only",
			HostedEnabled:      true,
			DefaultRuntimeMode: &defaultRuntimeMode,
			CreditBalance:      100,
		},
	}
	svc := NewService(store, Config{
		BaseURL:              upstream.URL,
		HashSalt:             "salt",
		ImageModel:           "gpt-image-test",
		TimeoutSec:           5,
		AIGatewayKeyCipher:   testAIGatewayCipher(t),
		AIGatewayAdminClient: &fakeAIGatewayAdminClient{keys: []string{"upstream-key"}},
	})

	resp, err := svc.GenerateImage(context.Background(), "Bearer hosted-key", ImageRequest{
		Model:       "hosted/image",
		Prompt:      "A polished product launch hero image",
		AspectRatio: 1,
	})
	require.NoError(t, err)
	require.Equal(t, []byte("png-bytes"), resp.Data)
	require.True(t, sawImages)
	require.True(t, sawResponses)
	require.Len(t, store.events, 1)
}

func TestCompleteSettlesCreditsFromAIGatewayCostMarkupAndRecordsSnapshot(t *testing.T) {
	var upstreamPayload map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/chat/completions", r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&upstreamPayload))
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":1000,"completion_tokens":500,"reasoning_tokens":250}}`)
	}))
	defer upstream.Close()

	defaultRuntimeMode := "hosted"
	userID := uint64(42)
	ruleID := uint64(9)
	markupOverride := 5000
	store := &fakeAPIKeyStore{
		key: &model.APIKey{
			ID:                 7,
			OwnerUserID:        &userID,
			Status:             model.APIKeyStatusActive,
			PlanName:           "Hosted",
			KeyPrefix:          "cop_hosted",
			AllowedModes:       "hosted_only",
			HostedEnabled:      true,
			DefaultRuntimeMode: &defaultRuntimeMode,
			CreditBalance:      1000,
		},
	}
	svc := NewService(store, Config{
		BaseURL:    upstream.URL,
		APIKey:     "upstream-key",
		HashSalt:   "salt",
		TextModel:  "gpt-test",
		Provider:   "aigateway",
		MarkupBPS:  3000,
		TimeoutSec: 5,
		Rules: []model.HostedPricingRule{{
			ID:                         ruleID,
			DocumentProfile:            "text",
			Provider:                   "aigateway",
			Model:                      "gpt-test",
			PromptPer1KCostMicrousd:    10000,
			OutputPer1KCostMicrousd:    20000,
			ReasoningPer1KCostMicrousd: 40000,
			ReservationCredits:         20,
			MinimumChargeCredits:       2,
			MarkupBPS:                  &markupOverride,
			Enabled:                    true,
		}},
		AIGatewayKeyCipher:   testAIGatewayCipher(t),
		AIGatewayAdminClient: &fakeAIGatewayAdminClient{keys: []string{"upstream-key"}},
	})

	resp, err := svc.Complete(context.Background(), "Bearer hosted-key", CompletionRequest{
		Model:    "hosted/text",
		Messages: []ChatMessage{{Role: "user", Content: "hello"}},
	})

	require.NoError(t, err)
	require.Equal(t, "ok", resp.Content)
	require.Equal(t, "gpt-test", upstreamPayload["model"])
	require.Equal(t, []int{20}, store.reservations)
	require.Equal(t, []int{5}, store.settlements)
	require.Len(t, store.events, 1)
	event := store.events[0]
	require.Equal(t, ruleID, event.HostedPricingRuleID)
	require.Equal(t, 5000, event.MarkupBPS)
	require.Equal(t, int64(30000), event.UpstreamCostMicrousd)
	require.Equal(t, 5, event.UncappedChargeCredits)
	require.Equal(t, 5, event.SettledCredits)
	require.Equal(t, int64(20000), event.ProfitMicrousd)
	require.False(t, event.CapApplied)
}

func TestCompletePersistsAuditContextOnUsageEvent(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/chat/completions", r.URL.Path)
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":10,"completion_tokens":4}}`)
	}))
	defer upstream.Close()

	defaultRuntimeMode := "hosted"
	userID := uint64(42)
	store := &fakeAPIKeyStore{
		key: &model.APIKey{
			ID:                 77,
			OwnerUserID:        &userID,
			KeyPrefix:          "cop_hosted",
			Status:             model.APIKeyStatusActive,
			AllowedModes:       "hosted_only",
			HostedEnabled:      true,
			DefaultRuntimeMode: &defaultRuntimeMode,
			CreditBalance:      100,
		},
		rules: []model.HostedPricingRule{{
			DocumentProfile:      "text",
			ReservationCredits:   1,
			MinimumChargeCredits: 1,
			Enabled:              true,
		}},
	}
	svc := NewService(store, Config{
		BaseURL:              upstream.URL,
		APIKey:               "upstream-key",
		HashSalt:             "salt",
		TextModel:            "gpt-test",
		TimeoutSec:           5,
		AIGatewayKeyCipher:   testAIGatewayCipher(t),
		AIGatewayAdminClient: &fakeAIGatewayAdminClient{keys: []string{"upstream-key"}},
	})

	_, err := svc.Complete(context.Background(), "Bearer hosted-key", CompletionRequest{
		Model:    "hosted/text",
		Messages: []ChatMessage{{Role: "user", Content: "hello"}},
		AuditContext: model.UsageAuditContext{
			ClientIP:      "203.0.113.77",
			ForwardedFor:  "198.51.100.77, 10.0.0.2",
			UserAgent:     "officecli/0.2.65 hosted-test",
			RequestHost:   "platform.officecli.io",
			RequestPath:   "/api/llm/v1/text",
			RequestMethod: http.MethodPost,
		},
	})
	require.NoError(t, err)
	require.Len(t, store.events, 1)
	event := store.events[0]
	require.NotNil(t, event.ClientIP)
	require.Equal(t, "203.0.113.77", *event.ClientIP)
	require.NotNil(t, event.ForwardedFor)
	require.Equal(t, "198.51.100.77, 10.0.0.2", *event.ForwardedFor)
	require.NotNil(t, event.UserAgent)
	require.Equal(t, "officecli/0.2.65 hosted-test", *event.UserAgent)
	require.NotNil(t, event.RequestHost)
	require.Equal(t, "platform.officecli.io", *event.RequestHost)
	require.NotNil(t, event.RequestPath)
	require.Equal(t, "/api/llm/v1/text", *event.RequestPath)
	require.NotNil(t, event.RequestMethod)
	require.Equal(t, http.MethodPost, *event.RequestMethod)
}

func TestCompleteRecordsCapAppliedWhenChargeExceedsReservation(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":10000}}`)
	}))
	defer upstream.Close()

	defaultRuntimeMode := "hosted"
	userID := uint64(42)
	store := &fakeAPIKeyStore{
		settings: &model.HostedPricingSetting{ID: 1, MarkupBPS: 0, Currency: "usd", CreditsPerUSD: 100},
		key: &model.APIKey{
			ID:                 7,
			OwnerUserID:        &userID,
			Status:             model.APIKeyStatusActive,
			PlanName:           "Hosted",
			KeyPrefix:          "cop_hosted",
			AllowedModes:       "hosted_only",
			HostedEnabled:      true,
			DefaultRuntimeMode: &defaultRuntimeMode,
			CreditBalance:      1000,
		},
	}
	svc := NewService(store, Config{
		BaseURL:    upstream.URL,
		APIKey:     "upstream-key",
		HashSalt:   "salt",
		TextModel:  "gpt-test",
		MarkupBPS:  0,
		TimeoutSec: 5,
		Rules: []model.HostedPricingRule{{
			DocumentProfile:         "text",
			Model:                   "gpt-test",
			PromptPer1KCostMicrousd: 100000,
			ReservationCredits:      5,
			MinimumChargeCredits:    1,
			Enabled:                 true,
		}},
		AIGatewayKeyCipher:   testAIGatewayCipher(t),
		AIGatewayAdminClient: &fakeAIGatewayAdminClient{keys: []string{"upstream-key"}},
	})

	_, err := svc.Complete(context.Background(), "Bearer hosted-key", CompletionRequest{
		Model:    "hosted/text",
		Messages: []ChatMessage{{Role: "user", Content: "hello"}},
	})

	require.NoError(t, err)
	require.Equal(t, []int{5}, store.reservations)
	require.Equal(t, []int{5}, store.settlements)
	require.Len(t, store.events, 1)
	require.Equal(t, 100, store.events[0].UncappedChargeCredits)
	require.Equal(t, 5, store.events[0].SettledCredits)
	require.True(t, store.events[0].CapApplied)
}

func TestCompleteUsesConfiguredCreditsPerUSDForHostedCharges(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":10000,"completion_tokens":0}}`)
	}))
	defer upstream.Close()

	defaultRuntimeMode := "hosted"
	userID := uint64(42)
	store := &fakeAPIKeyStore{
		settings: &model.HostedPricingSetting{ID: 1, MarkupBPS: 0, Currency: "usd", CreditsPerUSD: 200},
		key: &model.APIKey{
			ID:                 7,
			OwnerUserID:        &userID,
			Status:             model.APIKeyStatusActive,
			PlanName:           "Hosted",
			KeyPrefix:          "cop_hosted",
			AllowedModes:       "hosted_only",
			HostedEnabled:      true,
			DefaultRuntimeMode: &defaultRuntimeMode,
			CreditBalance:      1000,
		},
		modelConfigs: []model.HostedModelPricingConfig{{
			Key:                     "text_default",
			Kind:                    model.HostedModelPricingKindText,
			Provider:                "aigateway",
			Model:                   "gpt-credit-ratio",
			PromptPer1MCostMicrousd: 1000000,
			Enabled:                 true,
		}},
		rules: []model.HostedPricingRule{{
			ID:                   31,
			DocumentProfile:      "text",
			TextModelKey:         "text_default",
			MinimumChargeCredits: 0,
			Enabled:              true,
		}},
	}
	svc := NewService(store, Config{
		BaseURL:              upstream.URL,
		APIKey:               "upstream-key",
		HashSalt:             "salt",
		TextModel:            "legacy-text",
		TimeoutSec:           5,
		AIGatewayKeyCipher:   testAIGatewayCipher(t),
		AIGatewayAdminClient: &fakeAIGatewayAdminClient{keys: []string{"upstream-key"}},
	})

	_, err := svc.Complete(context.Background(), "Bearer hosted-key", CompletionRequest{
		Model:    "hosted/text",
		Messages: []ChatMessage{{Role: "user", Content: "hello"}},
	})

	require.NoError(t, err)
	require.Len(t, store.events, 1)
	require.Equal(t, int64(10000), store.events[0].UpstreamCostMicrousd)
	require.Equal(t, 2, store.events[0].SettledCredits)
}

func TestCompletePricesSharedTextModelConfigPer1MTokens(t *testing.T) {
	var upstreamPayloads []map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
		upstreamPayloads = append(upstreamPayloads, payload)
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":1000,"completion_tokens":2000,"reasoning_tokens":1000}}`)
	}))
	defer upstream.Close()

	defaultRuntimeMode := "hosted"
	userID := uint64(42)
	store := &fakeAPIKeyStore{
		settings: &model.HostedPricingSetting{ID: 1, MarkupBPS: 0, Currency: "usd"},
		key: &model.APIKey{
			ID:                 7,
			OwnerUserID:        &userID,
			Status:             model.APIKeyStatusActive,
			PlanName:           "Hosted",
			KeyPrefix:          "cop_hosted",
			AllowedModes:       "hosted_only",
			HostedEnabled:      true,
			DefaultRuntimeMode: &defaultRuntimeMode,
			CreditBalance:      1000,
		},
		modelConfigs: []model.HostedModelPricingConfig{{
			Key:                        "text_default",
			Kind:                       model.HostedModelPricingKindText,
			Provider:                   "aigateway",
			Model:                      "gpt-shared-text",
			PromptPer1MCostMicrousd:    1000000,
			OutputPer1MCostMicrousd:    2000000,
			ReasoningPer1MCostMicrousd: 3000000,
			Enabled:                    true,
		}},
		rules: []model.HostedPricingRule{{
			ID:                   11,
			DocumentProfile:      "text",
			TextModelKey:         "text_default",
			ReservationCredits:   20,
			MinimumChargeCredits: 1,
			Enabled:              true,
		}},
	}
	svc := NewService(store, Config{
		BaseURL:              upstream.URL,
		APIKey:               "upstream-key",
		HashSalt:             "salt",
		TextModel:            "legacy-text",
		TimeoutSec:           5,
		AIGatewayKeyCipher:   testAIGatewayCipher(t),
		AIGatewayAdminClient: &fakeAIGatewayAdminClient{keys: []string{"upstream-key"}},
	})

	for _, requested := range []string{"", "text", "hosted/text"} {
		_, err := svc.Complete(context.Background(), "Bearer hosted-key", CompletionRequest{
			Model:    requested,
			Messages: []ChatMessage{{Role: "user", Content: "hello"}},
		})
		require.NoError(t, err)
	}

	require.Len(t, upstreamPayloads, 3)
	require.Equal(t, "gpt-shared-text", upstreamPayloads[0]["model"])
	require.Equal(t, "gpt-shared-text", upstreamPayloads[1]["model"])
	require.Equal(t, "gpt-shared-text", upstreamPayloads[2]["model"])
	require.Len(t, store.events, 3)
	for _, event := range store.events {
		require.Equal(t, int64(8000), event.UpstreamCostMicrousd)
		require.Equal(t, 1, event.SettledCredits)
	}
}

func TestGenerateImagePricesModelConfigPer1MTokens(t *testing.T) {
	imageData := base64.StdEncoding.EncodeToString([]byte("png-bytes"))
	var upstreamPayload map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&upstreamPayload))
		_, _ = fmt.Fprintf(w, `{"data":[{"b64_json":"%s"}],"usage":{"prompt_tokens":3000,"completion_tokens":1000}}`, imageData)
	}))
	defer upstream.Close()

	defaultRuntimeMode := "hosted"
	userID := uint64(42)
	store := &fakeAPIKeyStore{
		settings: &model.HostedPricingSetting{ID: 1, MarkupBPS: 0, Currency: "usd"},
		key: &model.APIKey{
			ID:                 7,
			OwnerUserID:        &userID,
			Status:             model.APIKeyStatusActive,
			PlanName:           "Hosted",
			KeyPrefix:          "cop_hosted",
			AllowedModes:       "hosted_only",
			HostedEnabled:      true,
			DefaultRuntimeMode: &defaultRuntimeMode,
			CreditBalance:      1000,
		},
		modelConfigs: []model.HostedModelPricingConfig{{
			Key:                     "image_default",
			Kind:                    model.HostedModelPricingKindImage,
			Provider:                "aigateway",
			Model:                   "gpt-image-shared",
			PromptPer1MCostMicrousd: 1000000,
			OutputPer1MCostMicrousd: 2000000,
			Enabled:                 true,
		}},
		rules: []model.HostedPricingRule{{
			ID:                   21,
			DocumentProfile:      "image",
			ImageModelKey:        "image_default",
			ImagePerAssetCredits: 99,
			ReservationCredits:   12,
			MinimumChargeCredits: 1,
			Enabled:              true,
		}},
	}
	svc := NewService(store, Config{
		BaseURL:              upstream.URL,
		APIKey:               "upstream-key",
		HashSalt:             "salt",
		ImageModel:           "legacy-image",
		TimeoutSec:           5,
		AIGatewayKeyCipher:   testAIGatewayCipher(t),
		AIGatewayAdminClient: &fakeAIGatewayAdminClient{keys: []string{"upstream-key"}},
	})

	resp, err := svc.GenerateImage(context.Background(), "Bearer hosted-key", ImageRequest{
		Model:       "image",
		Prompt:      "A product image",
		AspectRatio: 1,
	})

	require.NoError(t, err)
	require.Equal(t, []byte("png-bytes"), resp.Data)
	require.Equal(t, "gpt-image-shared", upstreamPayload["model"])
	require.Len(t, store.events, 1)
	require.Equal(t, int64(5000), store.events[0].UpstreamCostMicrousd)
	require.Equal(t, 1, store.events[0].SettledCredits)
}

func TestGenerateImageParsesInputOutputTokenUsageAndPersistsTokens(t *testing.T) {
	imageData := base64.StdEncoding.EncodeToString([]byte("png-bytes"))
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/images/generations", r.URL.Path)
		_, _ = fmt.Fprintf(w, `{"data":[{"b64_json":"%s"}],"usage":{"input_tokens":120,"output_tokens":1584}}`, imageData)
	}))
	defer upstream.Close()

	defaultRuntimeMode := "hosted"
	userID := uint64(42)
	store := &fakeAPIKeyStore{
		settings: &model.HostedPricingSetting{ID: 1, MarkupBPS: 3000, Currency: "usd", CreditsPerUSD: 100},
		key: &model.APIKey{
			ID:                 7,
			OwnerUserID:        &userID,
			Status:             model.APIKeyStatusActive,
			PlanName:           "Hosted",
			KeyPrefix:          "cop_hosted",
			AllowedModes:       "hosted_only",
			HostedEnabled:      true,
			DefaultRuntimeMode: &defaultRuntimeMode,
			CreditBalance:      1000,
		},
		modelConfigs: []model.HostedModelPricingConfig{{
			Key:                     "image_default",
			Kind:                    model.HostedModelPricingKindImage,
			Provider:                "aigateway",
			Model:                   "gpt-image-shared",
			PromptPer1MCostMicrousd: 710000,
			OutputPer1MCostMicrousd: 40000000,
			Enabled:                 true,
		}},
		rules: []model.HostedPricingRule{{
			ID:                        21,
			DocumentProfile:           "image",
			ImageModelKey:             "image_default",
			ImagePerAssetCredits:      999,
			ImagePerAssetCostMicrousd: 999000000,
			ReservationCredits:        100,
			MinimumChargeCredits:      6,
			Enabled:                   true,
		}},
	}
	svc := NewService(store, Config{
		BaseURL:              upstream.URL,
		APIKey:               "upstream-key",
		HashSalt:             "salt",
		ImageModel:           "legacy-image",
		TimeoutSec:           5,
		AIGatewayKeyCipher:   testAIGatewayCipher(t),
		AIGatewayAdminClient: &fakeAIGatewayAdminClient{keys: []string{"upstream-key"}},
	})

	resp, err := svc.GenerateImage(context.Background(), "Bearer hosted-key", ImageRequest{
		Model:       "image",
		Prompt:      "A breakfast map of Wuhan",
		AspectRatio: 1,
	})

	require.NoError(t, err)
	require.Equal(t, []byte("png-bytes"), resp.Data)
	require.Len(t, store.events, 1)
	event := store.events[0]
	require.Equal(t, 120, event.PromptTokens)
	require.Equal(t, 1584, event.CompletionTokens)
	require.Equal(t, 0, event.ReasoningTokens)
	require.Equal(t, 1, event.ImageCount)
	require.Equal(t, int64(63446), event.UpstreamCostMicrousd)
	require.Equal(t, 9, event.SettledCredits)
	require.Equal(t, 91, event.RefundCredits)
	require.Equal(t, int64(26554), event.ProfitMicrousd)
}

func TestGenerateImageResponsesFallbackParsesInputOutputTokens(t *testing.T) {
	imageData := base64.StdEncoding.EncodeToString([]byte("png-bytes"))
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/images/generations":
			http.Error(w, "not supported", http.StatusBadRequest)
		case "/responses":
			_, _ = fmt.Fprintf(w, `{"output":[{"type":"image_generation_call","result":"%s"}],"usage":{"input_tokens":200,"output_tokens":300}}`, imageData)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	defaultRuntimeMode := "hosted"
	userID := uint64(42)
	store := &fakeAPIKeyStore{
		settings: &model.HostedPricingSetting{ID: 1, MarkupBPS: 0, Currency: "usd", CreditsPerUSD: 100},
		key: &model.APIKey{
			ID:                 7,
			OwnerUserID:        &userID,
			Status:             model.APIKeyStatusActive,
			PlanName:           "Hosted",
			KeyPrefix:          "cop_hosted",
			AllowedModes:       "hosted_only",
			HostedEnabled:      true,
			DefaultRuntimeMode: &defaultRuntimeMode,
			CreditBalance:      1000,
		},
		modelConfigs: []model.HostedModelPricingConfig{{
			Key:                     "image_default",
			Kind:                    model.HostedModelPricingKindImage,
			Model:                   "gpt-image-shared",
			PromptPer1MCostMicrousd: 1000000,
			OutputPer1MCostMicrousd: 2000000,
			Enabled:                 true,
		}},
		rules: []model.HostedPricingRule{{
			ID:                   21,
			DocumentProfile:      "image",
			ImageModelKey:        "image_default",
			ReservationCredits:   12,
			MinimumChargeCredits: 1,
			Enabled:              true,
		}},
	}
	svc := NewService(store, Config{
		BaseURL:              upstream.URL,
		HashSalt:             "salt",
		ImageModel:           "legacy-image",
		TimeoutSec:           5,
		AIGatewayKeyCipher:   testAIGatewayCipher(t),
		AIGatewayAdminClient: &fakeAIGatewayAdminClient{keys: []string{"upstream-key"}},
	})

	resp, err := svc.GenerateImage(context.Background(), "Bearer hosted-key", ImageRequest{
		Model:       "image",
		Prompt:      "A product image",
		AspectRatio: 1,
	})

	require.NoError(t, err)
	require.Equal(t, []byte("png-bytes"), resp.Data)
	require.Len(t, store.events, 1)
	require.Equal(t, 200, store.events[0].PromptTokens)
	require.Equal(t, 300, store.events[0].CompletionTokens)
	require.Equal(t, int64(800), store.events[0].UpstreamCostMicrousd)
	require.Equal(t, 1, store.events[0].SettledCredits)
}

func TestGenerateImageWithReferenceUsesImageEditEndpoint(t *testing.T) {
	imageData := base64.StdEncoding.EncodeToString([]byte("png-bytes"))
	var uploadedImage []byte
	var formValues map[string]string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/images/edits", r.URL.Path)
		require.Equal(t, "Bearer upstream-key", r.Header.Get("Authorization"))
		require.Contains(t, r.Header.Get("Content-Type"), "multipart/form-data")
		require.NoError(t, r.ParseMultipartForm(8<<20))
		formValues = map[string]string{
			"model":  r.FormValue("model"),
			"prompt": r.FormValue("prompt"),
			"size":   r.FormValue("size"),
		}
		file, header, err := r.FormFile("image[]")
		require.NoError(t, err)
		defer file.Close()
		require.Equal(t, "reference.png", header.Filename)
		require.Equal(t, "image/png", header.Header.Get("Content-Type"))
		uploadedImage, err = io.ReadAll(file)
		require.NoError(t, err)
		_, _ = fmt.Fprintf(w, `{"data":[{"b64_json":"%s"}]}`, imageData)
	}))
	defer upstream.Close()

	defaultRuntimeMode := "hosted"
	userID := uint64(42)
	store := &fakeAPIKeyStore{
		key: &model.APIKey{
			ID:                 7,
			OwnerUserID:        &userID,
			Status:             model.APIKeyStatusActive,
			PlanName:           "Hosted",
			KeyPrefix:          "cop_hosted",
			AllowedModes:       "hosted_only",
			HostedEnabled:      true,
			DefaultRuntimeMode: &defaultRuntimeMode,
			CreditBalance:      100,
		},
	}
	svc := NewService(store, Config{
		BaseURL:    upstream.URL,
		APIKey:     "upstream-key",
		HashSalt:   "salt",
		ImageModel: "gpt-image-test",
		Rules: []model.HostedPricingRule{{
			DocumentProfile:      "image",
			Provider:             "openai",
			Model:                "gpt-image-test",
			ImagePerAssetCredits: 1,
			ReservationCredits:   1,
			MinimumChargeCredits: 1,
		}},
		TimeoutSec:           5,
		AIGatewayKeyCipher:   testAIGatewayCipher(t),
		AIGatewayAdminClient: &fakeAIGatewayAdminClient{keys: []string{"upstream-key"}},
	})

	resp, err := svc.GenerateImage(context.Background(), "Bearer hosted-key", ImageRequest{
		Model:       "hosted/image",
		Prompt:      "Use the uploaded reference image as visual context",
		AspectRatio: 1,
		ReferenceImage: &ImageReference{
			Filename: "reference.png",
			MIME:     "image/png",
			Data:     base64.StdEncoding.EncodeToString([]byte("reference-bytes")),
		},
	})
	require.NoError(t, err)
	require.Equal(t, []byte("png-bytes"), resp.Data)
	require.Equal(t, []byte("reference-bytes"), uploadedImage)
	require.Equal(t, map[string]string{
		"model":  "gpt-image-test",
		"prompt": "Use the uploaded reference image as visual context",
		"size":   "1024x1024",
	}, formValues)
	require.Equal(t, []int{1}, store.reservations)
	require.Equal(t, []int{1}, store.settlements)
	require.Empty(t, store.releases)
}

func TestGenerateImageWithCommitTokenConsumesGenerationQuotaNotHostedCredits(t *testing.T) {
	imageData := base64.StdEncoding.EncodeToString([]byte("png-bytes"))
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/images/generations", r.URL.Path)
		_, _ = fmt.Fprintf(w, `{"data":[{"b64_json":"%s"}]}`, imageData)
	}))
	defer upstream.Close()

	store := &fakeAPIKeyStore{}
	quota := &fakeGenerationQuotaManager{
		result: &licensesvc.ConsumeResponse{AccessMode: model.AccessModePaid, Remaining: 89, PaidQuotaRemaining: 89},
	}
	svc := NewService(store, Config{
		BaseURL:    upstream.URL,
		APIKey:     "upstream-key",
		HashSalt:   "salt",
		ImageModel: "gpt-image-test",
		TimeoutSec: 5,
	}, quota)

	resp, err := svc.GenerateImage(context.Background(), "Bearer paid-key", ImageRequest{
		RequestID:       "req-img",
		Model:           "hosted/image",
		Prompt:          "A polished product launch hero image",
		AspectRatio:     1,
		FingerprintHash: "fp-img",
		AccessMode:      model.AccessModePaid,
		CommitToken: &licensesvc.CommitToken{
			FingerprintHash: "fp-img",
			RequestID:       "req-img",
			AccessMode:      model.AccessModePaid,
			DocumentType:    "img",
		},
	})
	require.NoError(t, err)
	require.Equal(t, []byte("png-bytes"), resp.Data)
	require.Equal(t, model.AccessModePaid, resp.AccessMode)
	require.Equal(t, 89, resp.PaidQuotaRemaining)
	require.Empty(t, store.reservations)
	require.Empty(t, store.settlements)
	require.Empty(t, store.events)
	require.Len(t, quota.validations, 1)
	require.Len(t, quota.consumes, 1)
	require.Equal(t, "paid-key", quota.consumes[0].APIKey)
}

func TestGenerateImageWithCommitTokenAndReferenceConsumesQuotaAfterEdit(t *testing.T) {
	imageData := base64.StdEncoding.EncodeToString([]byte("png-bytes"))
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/images/edits", r.URL.Path)
		require.NoError(t, r.ParseMultipartForm(8<<20))
		_, _, err := r.FormFile("image[]")
		require.NoError(t, err)
		_, _ = fmt.Fprintf(w, `{"data":[{"b64_json":"%s"}]}`, imageData)
	}))
	defer upstream.Close()

	store := &fakeAPIKeyStore{}
	quota := &fakeGenerationQuotaManager{
		result: &licensesvc.ConsumeResponse{AccessMode: model.AccessModePaid, Remaining: 88, PaidQuotaRemaining: 88},
	}
	svc := NewService(store, Config{
		BaseURL:    upstream.URL,
		APIKey:     "upstream-key",
		HashSalt:   "salt",
		ImageModel: "gpt-image-test",
		TimeoutSec: 5,
	}, quota)

	resp, err := svc.GenerateImage(context.Background(), "Bearer paid-key", ImageRequest{
		RequestID:       "req-img",
		Model:           "hosted/image",
		Prompt:          "Use the uploaded reference image as visual context",
		AspectRatio:     1,
		FingerprintHash: "fp-img",
		AccessMode:      model.AccessModePaid,
		CommitToken: &licensesvc.CommitToken{
			FingerprintHash: "fp-img",
			RequestID:       "req-img",
			AccessMode:      model.AccessModePaid,
			DocumentType:    "img",
		},
		ReferenceImage: &ImageReference{
			Filename: "reference.png",
			MIME:     "image/png",
			Data:     base64.StdEncoding.EncodeToString([]byte("reference-bytes")),
		},
	})
	require.NoError(t, err)
	require.Equal(t, []byte("png-bytes"), resp.Data)
	require.Equal(t, 88, resp.PaidQuotaRemaining)
	require.Empty(t, store.reservations)
	require.Len(t, quota.validations, 1)
	require.Len(t, quota.consumes, 1)
}

func TestGenerateImageFailureReleasesReservationWithoutCharge(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
	}))
	defer upstream.Close()

	defaultRuntimeMode := "hosted"
	userID := uint64(42)
	store := &fakeAPIKeyStore{
		key: &model.APIKey{
			ID:                 7,
			OwnerUserID:        &userID,
			Status:             model.APIKeyStatusActive,
			PlanName:           "Hosted",
			KeyPrefix:          "cop_hosted",
			AllowedModes:       "hosted_only",
			HostedEnabled:      true,
			DefaultRuntimeMode: &defaultRuntimeMode,
			CreditBalance:      100,
		},
	}
	svc := NewService(store, Config{
		BaseURL:    upstream.URL,
		HashSalt:   "salt",
		ImageModel: "gpt-image-test",
		Rules: []model.HostedPricingRule{{
			DocumentProfile:      "image",
			ImagePerAssetCredits: 1,
			ReservationCredits:   1,
			MinimumChargeCredits: 1,
		}},
		TimeoutSec:           5,
		AIGatewayKeyCipher:   testAIGatewayCipher(t),
		AIGatewayAdminClient: &fakeAIGatewayAdminClient{keys: []string{"upstream-key"}},
	})

	resp, err := svc.GenerateImage(context.Background(), "Bearer hosted-key", ImageRequest{
		Model:       "hosted/image",
		Prompt:      "A polished product launch hero image",
		AspectRatio: 1,
	})
	require.Error(t, err)
	require.Nil(t, resp)
	require.Equal(t, []int{1}, store.reservations)
	require.Equal(t, []int{1}, store.releases)
	require.Empty(t, store.settlements)
	require.Len(t, store.events, 1)
	require.False(t, store.events[0].Charged)
	require.Equal(t, 0, store.events[0].SettledCredits)
	require.Equal(t, 1, store.events[0].RefundCredits)
	require.NotNil(t, store.events[0].DocumentType)
	require.Equal(t, "img", *store.events[0].DocumentType)
}

func TestCompleteFailureReleasesReservationWithCanceledRequestContext(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer upstream.Close()

	defaultRuntimeMode := "hosted"
	userID := uint64(42)
	store := &fakeAPIKeyStore{
		key: &model.APIKey{
			ID:                 7,
			OwnerUserID:        &userID,
			Status:             model.APIKeyStatusActive,
			PlanName:           "Hosted",
			KeyPrefix:          "cop_hosted",
			AllowedModes:       "hosted_only",
			HostedEnabled:      true,
			DefaultRuntimeMode: &defaultRuntimeMode,
			CreditBalance:      100,
		},
		rules: []model.HostedPricingRule{{
			DocumentProfile:      "text",
			ReservationCredits:   24,
			MinimumChargeCredits: 2,
			Enabled:              true,
		}},
	}
	svc := NewService(store, Config{
		BaseURL:              upstream.URL,
		HashSalt:             "salt",
		TextModel:            "gpt-text-test",
		TimeoutSec:           5,
		AIGatewayKeyCipher:   testAIGatewayCipher(t),
		AIGatewayAdminClient: &fakeAIGatewayAdminClient{keys: []string{"upstream-key"}},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	resp, err := svc.Complete(ctx, "Bearer hosted-key", CompletionRequest{
		Model:    "hosted/text",
		Messages: []ChatMessage{{Role: "user", Content: "hello"}},
	})

	require.Error(t, err)
	require.Nil(t, resp)
	require.Equal(t, []int{24}, store.reservations)
	require.Equal(t, []int{24}, store.releases)
	require.Empty(t, store.settlements)
	require.Len(t, store.events, 1)
	require.Equal(t, []error{nil}, store.releaseCtxErrs)
	require.Equal(t, []error{nil}, store.eventCtxErrs)
	require.Equal(t, 0, store.events[0].SettledCredits)
	require.Equal(t, 24, store.events[0].RefundCredits)
}

func TestGenerateImageFailureReleasesReservationWithCanceledRequestContext(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer upstream.Close()

	defaultRuntimeMode := "hosted"
	userID := uint64(42)
	store := &fakeAPIKeyStore{
		key: &model.APIKey{
			ID:                 7,
			OwnerUserID:        &userID,
			Status:             model.APIKeyStatusActive,
			PlanName:           "Hosted",
			KeyPrefix:          "cop_hosted",
			AllowedModes:       "hosted_only",
			HostedEnabled:      true,
			DefaultRuntimeMode: &defaultRuntimeMode,
			CreditBalance:      100,
		},
		rules: []model.HostedPricingRule{{
			DocumentProfile:      "image",
			ReservationCredits:   12,
			MinimumChargeCredits: 6,
			Enabled:              true,
		}},
	}
	svc := NewService(store, Config{
		BaseURL:              upstream.URL,
		HashSalt:             "salt",
		ImageModel:           "gpt-image-test",
		TimeoutSec:           5,
		AIGatewayKeyCipher:   testAIGatewayCipher(t),
		AIGatewayAdminClient: &fakeAIGatewayAdminClient{keys: []string{"upstream-key"}},
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	resp, err := svc.GenerateImage(ctx, "Bearer hosted-key", ImageRequest{
		Model:       "hosted/image",
		Prompt:      "A polished product launch hero image",
		AspectRatio: 1,
	})

	require.Error(t, err)
	require.Nil(t, resp)
	require.Equal(t, []int{12}, store.reservations)
	require.Equal(t, []int{12}, store.releases)
	require.Empty(t, store.settlements)
	require.Len(t, store.events, 1)
	require.Equal(t, []error{nil}, store.releaseCtxErrs)
	require.Equal(t, []error{nil}, store.eventCtxErrs)
	require.Equal(t, 0, store.events[0].SettledCredits)
	require.Equal(t, 12, store.events[0].RefundCredits)
}
