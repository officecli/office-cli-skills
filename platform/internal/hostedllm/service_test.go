package hostedllm

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/officecli/officecli/platform/internal/model"
)

type fakeAPIKeyStore struct {
	key          *model.APIKey
	reservations []int
	releases     []int
	settlements  []int
	events       []*model.UsageEvent
}

func (f *fakeAPIKeyStore) FindAPIKeyByHash(_ context.Context, _ string) (*model.APIKey, error) {
	if f.key == nil {
		return nil, nil
	}
	cloned := *f.key
	return &cloned, nil
}

func (f *fakeAPIKeyStore) ReserveCreditsByHash(_ context.Context, _ string, credits int) (*model.APIKey, error) {
	f.reservations = append(f.reservations, credits)
	return f.FindAPIKeyByHash(context.Background(), "")
}

func (f *fakeAPIKeyStore) ReleaseReservedCredits(_ context.Context, apiKeyID uint64, reserved int) (*model.APIKey, error) {
	f.releases = append(f.releases, reserved)
	return f.FindAPIKeyByHash(context.Background(), "")
}

func (f *fakeAPIKeyStore) SettleReservedCredits(_ context.Context, apiKeyID uint64, reserved int, settled int) (*model.APIKey, error) {
	f.settlements = append(f.settlements, settled)
	key, err := f.FindAPIKeyByHash(context.Background(), "")
	if key != nil {
		key.CreditBalance -= settled
	}
	return key, err
}

func (f *fakeAPIKeyStore) CreateUsageEvent(_ context.Context, event *model.UsageEvent) error {
	f.events = append(f.events, event)
	return nil
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
		TimeoutSec: 5,
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

func TestGenerateImageFailureReleasesReservationWithoutCharge(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
	}))
	defer upstream.Close()

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
		TimeoutSec: 5,
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
