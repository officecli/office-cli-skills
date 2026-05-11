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

	"github.com/officecli/officecli/platform/internal/apikey"
	licensesvc "github.com/officecli/officecli/platform/internal/license"
	"github.com/officecli/officecli/platform/internal/model"
)

type fakeAPIKeyStore struct {
	key          *model.APIKey
	keysByHash   map[string]*model.APIKey
	userKeys     map[uint64]*model.UserAIGatewayAPIKey
	reservations []int
	releases     []int
	settlements  []int
	events       []*model.UsageEvent
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

func (f *fakeAPIKeyStore) ReleaseReservedCredits(_ context.Context, apiKeyID uint64, reserved int) (*model.APIKey, error) {
	f.releases = append(f.releases, reserved)
	return f.FindAPIKeyByHash(context.Background(), "")
}

func (f *fakeAPIKeyStore) SettleReservedCredits(_ context.Context, apiKeyID uint64, reserved int, settled int) (*model.APIKey, error) {
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

func (f *fakeAPIKeyStore) CreateUsageEvent(_ context.Context, event *model.UsageEvent) error {
	f.events = append(f.events, event)
	return nil
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
}

func (f *fakeGenerationQuotaManager) ValidateCommitToken(req licensesvc.ConsumeRequest) error {
	f.validations = append(f.validations, req)
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
			DocumentProfile:      "docx-xlsx",
			ReservationCredits:   1,
			MinimumChargeCredits: 1,
		}},
		TimeoutSec: 5,
	})

	for i := 0; i < 2; i++ {
		resp, err := svc.Complete(context.Background(), "Bearer hosted-key", CompletionRequest{
			Model:    "hosted/docx-xlsx",
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
			DocumentProfile:      "docx-xlsx",
			ReservationCredits:   1,
			MinimumChargeCredits: 1,
		}},
		TimeoutSec: 5,
	})

	for _, bearer := range []string{"Bearer office-key-42", "Bearer office-key-43"} {
		_, err := svc.Complete(context.Background(), bearer, CompletionRequest{
			Model:    "hosted/docx-xlsx",
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
		Model:    "hosted/docx-xlsx",
		Messages: []ChatMessage{{Role: "user", Content: "hello"}},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "hosted mode requires an owner user")
	require.Empty(t, store.reservations)
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
		Model:    "hosted/docx-xlsx",
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

	key, _, err := svc.authorize(context.Background(), "Bearer demo")
	require.Error(t, err)
	require.Nil(t, key)
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
			DocumentProfile:      "img",
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
		Model:       "hosted/img",
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
			DocumentProfile:      "img",
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
		Model:       "hosted/img",
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
		Model:           "hosted/img",
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
		Model:           "hosted/img",
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
			DocumentProfile:      "img",
			ImagePerAssetCredits: 1,
			ReservationCredits:   1,
			MinimumChargeCredits: 1,
		}},
		TimeoutSec:           5,
		AIGatewayKeyCipher:   testAIGatewayCipher(t),
		AIGatewayAdminClient: &fakeAIGatewayAdminClient{keys: []string{"upstream-key"}},
	})

	resp, err := svc.GenerateImage(context.Background(), "Bearer hosted-key", ImageRequest{
		Model:       "hosted/img",
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
