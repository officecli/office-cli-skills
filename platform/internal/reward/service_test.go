package reward

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/officecli/officecli/platform/internal/model"
)

type fakeStore struct {
	grants      []model.RewardGrant
	consumedIDs []uint64
}

func (f *fakeStore) FindRewardGrantByIdempotencyKey(_ context.Context, key string) (*model.RewardGrant, error) {
	for _, grant := range f.grants {
		if grant.IdempotencyKey == key {
			copied := grant
			return &copied, nil
		}
	}
	return nil, nil
}

func (f *fakeStore) CreateRewardGrant(_ context.Context, grant *model.RewardGrant) error {
	copied := *grant
	copied.ID = uint64(len(f.grants) + 1)
	f.grants = append(f.grants, copied)
	grant.ID = copied.ID
	return nil
}

func (f *fakeStore) ListRewardGrantsByUser(_ context.Context, userID uint64) ([]model.RewardGrant, error) {
	var grants []model.RewardGrant
	for _, grant := range f.grants {
		if grant.UserID == userID {
			grants = append(grants, grant)
		}
	}
	return grants, nil
}

func (f *fakeStore) ConsumeRewardGrant(_ context.Context, userID uint64) (*model.RewardGrant, int, error) {
	for i := range f.grants {
		if f.grants[i].UserID != userID || f.grants[i].Remaining() <= 0 {
			continue
		}
		f.grants[i].AmountUsed++
		f.consumedIDs = append(f.consumedIDs, f.grants[i].ID)
		copied := f.grants[i]
		remaining := 0
		for _, grant := range f.grants {
			if grant.UserID == userID {
				remaining += grant.Remaining()
			}
		}
		return &copied, remaining, nil
	}
	return nil, 0, ErrQuotaExhausted
}

func TestGrantIsIdempotent(t *testing.T) {
	store := &fakeStore{}
	svc := NewService(store)

	first, created, err := svc.Grant(context.Background(), GrantRequest{
		UserID:         42,
		SourceType:     model.RewardSourceInviteActivation,
		IdempotencyKey: "invite:42:activation:1",
		AmountTotal:    3,
		Reason:         "invite activation",
		Metadata:       map[string]any{"referral_id": 1},
	})
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, 1, len(store.grants))
	require.Equal(t, 3, first.AmountTotal)

	second, created, err := svc.Grant(context.Background(), GrantRequest{
		UserID:         42,
		SourceType:     model.RewardSourceInviteActivation,
		IdempotencyKey: "invite:42:activation:1",
		AmountTotal:    3,
		Reason:         "invite activation",
	})
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, first.ID, second.ID)
	require.Equal(t, 1, len(store.grants))
}

func TestBalanceAndConsumeUseAvailableGrants(t *testing.T) {
	store := &fakeStore{grants: []model.RewardGrant{
		{ID: 1, UserID: 7, AmountTotal: 2, AmountUsed: 1},
		{ID: 2, UserID: 7, AmountTotal: 5, AmountUsed: 3},
		{ID: 3, UserID: 9, AmountTotal: 4, AmountUsed: 0},
	}}
	svc := NewService(store)

	balance, err := svc.Balance(context.Background(), 7)
	require.NoError(t, err)
	require.Equal(t, 3, balance.Remaining)
	require.Len(t, balance.Grants, 2)

	consume, err := svc.Consume(context.Background(), 7)
	require.NoError(t, err)
	require.NotNil(t, consume.Grant)
	require.Equal(t, uint64(1), consume.Grant.ID)
	require.Equal(t, 2, consume.Remaining)
	require.Equal(t, []uint64{1}, store.consumedIDs)
}
