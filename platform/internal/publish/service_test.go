package publish

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

func (f *fakeAPIKeyStore) FindByHash(_ context.Context, _ string) (*model.APIKey, error) {
	if f.key == nil {
		return nil, nil
	}
	cloned := *f.key
	return &cloned, nil
}

func (f *fakeAPIKeyStore) TouchLastUsedAt(_ context.Context, _ uint64, _ time.Time) error {
	return nil
}

func TestAuthorizeRejectsZeroEntitlementKey(t *testing.T) {
	svc := NewService(&fakeAPIKeyStore{
		key: &model.APIKey{
			ID:            1,
			Status:        model.APIKeyStatusActive,
			PlanName:      "Starter",
			KeyPrefix:     "cop_test",
			AllowedModes:  "external_only",
			HostedEnabled: false,
		},
	}, Config{HashSalt: "salt"})

	key, err := svc.authorize(context.Background(), "Bearer demo")
	require.Error(t, err)
	require.Nil(t, key)
	require.Contains(t, err.Error(), "publish entitlement is required")
}

func TestAuthorizeAcceptsPaidQuotaKey(t *testing.T) {
	quotaTotal := 100
	svc := NewService(&fakeAPIKeyStore{
		key: &model.APIKey{
			ID:            1,
			Status:        model.APIKeyStatusActive,
			PlanName:      "Growth",
			KeyPrefix:     "cop_paid",
			AllowedModes:  "external_only",
			HostedEnabled: false,
			QuotaTotal:    &quotaTotal,
		},
	}, Config{HashSalt: "salt"})

	key, err := svc.authorize(context.Background(), "Bearer demo")
	require.NoError(t, err)
	require.NotNil(t, key)
}

func TestAuthorizeAcceptsHostedCreditKey(t *testing.T) {
	svc := NewService(&fakeAPIKeyStore{
		key: &model.APIKey{
			ID:            2,
			Status:        model.APIKeyStatusActive,
			PlanName:      "Hosted",
			KeyPrefix:     "cop_hosted",
			AllowedModes:  "hybrid",
			HostedEnabled: true,
			CreditBalance: 200,
		},
	}, Config{HashSalt: "salt"})

	key, err := svc.authorize(context.Background(), "Bearer demo")
	require.NoError(t, err)
	require.NotNil(t, key)
}
