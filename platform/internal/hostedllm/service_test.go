package hostedllm

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/officecli/officecli/platform/internal/model"
)

type fakeAPIKeyStore struct {
	key *model.APIKey
}

func (f *fakeAPIKeyStore) FindAPIKeyByHash(_ context.Context, _ string) (*model.APIKey, error) {
	if f.key == nil {
		return nil, nil
	}
	cloned := *f.key
	return &cloned, nil
}

func (f *fakeAPIKeyStore) ReserveCreditsByHash(_ context.Context, _ string, _ int) (*model.APIKey, error) {
	return f.FindAPIKeyByHash(context.Background(), "")
}

func (f *fakeAPIKeyStore) ReleaseReservedCredits(_ context.Context, apiKeyID uint64, reserved int) (*model.APIKey, error) {
	return f.FindAPIKeyByHash(context.Background(), "")
}

func (f *fakeAPIKeyStore) SettleReservedCredits(_ context.Context, apiKeyID uint64, reserved int, settled int) (*model.APIKey, error) {
	return f.FindAPIKeyByHash(context.Background(), "")
}

func (f *fakeAPIKeyStore) CreateUsageEvent(_ context.Context, event *model.UsageEvent) error {
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
	}, nil, Config{
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
	svc := NewService(&fakeAPIKeyStore{}, nil, Config{TimeoutSec: 7})
	require.NotNil(t, svc.client)
	require.Equal(t, 7*time.Second, svc.client.Timeout)
}
