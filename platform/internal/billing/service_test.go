package billing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/officecli/officecli/platform/internal/model"
)

type fakeGateway struct {
	called         bool
	lastCustomerID string
}

func (f *fakeGateway) CreateCheckoutSession(_ context.Context, _ CheckoutRequest, _ model.PricingPack, customerID string) (*CheckoutSession, error) {
	f.called = true
	f.lastCustomerID = customerID
	return &CheckoutSession{
		ID:         "cs_test_123",
		URL:        "https://checkout.stripe.test/session/cs_test_123",
		CustomerID: "cus_test_123",
	}, nil
}

func (f *fakeGateway) ParseWebhook(_ []byte, _ string) (*WebhookEvent, error) {
	return nil, nil
}

type fakeStore struct {
	apiKeys           map[uint64]*model.APIKey
	orders            []*model.Order
	updateOrderCalls  int
	stripeCustomer    *model.StripeCustomer
	createdAuditCount int
}

func (f *fakeStore) GetOrCreateStripeCustomer(_ context.Context, userID uint64, customerID string) (*model.StripeCustomer, error) {
	f.stripeCustomer = &model.StripeCustomer{UserID: userID, StripeCustomerID: customerID}
	return f.stripeCustomer, nil
}

func (f *fakeStore) GetStripeCustomerByUserID(_ context.Context, _ uint64) (*model.StripeCustomer, error) {
	return f.stripeCustomer, nil
}

func (f *fakeStore) CreateOrder(_ context.Context, order *model.Order) error {
	cloned := *order
	cloned.ID = uint64(len(f.orders) + 1)
	*order = cloned
	f.orders = append(f.orders, &cloned)
	return nil
}

func (f *fakeStore) GetOrderByCheckoutSessionID(_ context.Context, sessionID string) (*model.Order, error) {
	for _, order := range f.orders {
		if order.StripeCheckoutSessionID != nil && *order.StripeCheckoutSessionID == sessionID {
			cloned := *order
			return &cloned, nil
		}
	}
	return nil, nil
}

func (f *fakeStore) GetOrderByID(_ context.Context, id uint64) (*model.Order, error) {
	for _, order := range f.orders {
		if order.ID == id {
			cloned := *order
			return &cloned, nil
		}
	}
	return nil, nil
}

func (f *fakeStore) UpdateOrder(_ context.Context, id uint64, values map[string]any) error {
	f.updateOrderCalls++
	for _, order := range f.orders {
		if order.ID != id {
			continue
		}
		if sessionID, ok := values["stripe_checkout_session_id"].(string); ok {
			order.StripeCheckoutSessionID = &sessionID
		}
		if customerID, ok := values["stripe_customer_id"].(string); ok {
			order.StripeCustomerID = &customerID
		}
	}
	return nil
}

func (f *fakeStore) AddPaidQuotaToAPIKey(_ context.Context, apiKeyID uint64, quotaAmount int) (*model.APIKey, error) {
	key := f.apiKeys[apiKeyID]
	if key == nil {
		return nil, nil
	}
	if key.QuotaTotal == nil {
		total := 0
		key.QuotaTotal = &total
	}
	*key.QuotaTotal += quotaAmount
	cloned := *key
	return &cloned, nil
}

func (f *fakeStore) AddCreditBalanceToAPIKey(_ context.Context, apiKeyID uint64, creditAmount int) (*model.APIKey, error) {
	key := f.apiKeys[apiKeyID]
	if key == nil {
		return nil, nil
	}
	key.CreditBalance += creditAmount
	cloned := *key
	return &cloned, nil
}

func (f *fakeStore) CreateBillingEvent(_ context.Context, _ *model.BillingEvent) error {
	return nil
}

func (f *fakeStore) GetBillingEventByEventID(_ context.Context, _ string) (*model.BillingEvent, error) {
	return nil, nil
}

func (f *fakeStore) ListOrdersByUser(_ context.Context, _ uint64) ([]model.Order, error) {
	return nil, nil
}

func (f *fakeStore) ListOrders(_ context.Context) ([]model.Order, error) {
	return nil, nil
}

func (f *fakeStore) ListBillingEvents(_ context.Context) ([]model.BillingEvent, error) {
	return nil, nil
}

func (f *fakeStore) CreateAuditLog(_ context.Context, _, _, _, _ string) error {
	f.createdAuditCount++
	return nil
}

func (f *fakeStore) FindAPIKeyByID(_ context.Context, id uint64) (*model.APIKey, error) {
	if key := f.apiKeys[id]; key != nil {
		cloned := *key
		return &cloned, nil
	}
	return nil, nil
}

func TestCreateCheckoutRejectsTargetKeyOwnedByAnotherUser(t *testing.T) {
	t.Parallel()

	quota := 10
	store := &fakeStore{
		apiKeys: map[uint64]*model.APIKey{
			7: {
				ID:          7,
				OwnerUserID: uint64Ptr(99),
				Status:      model.APIKeyStatusActive,
				PlanName:    "Growth",
				QuotaTotal:  &quota,
			},
		},
	}
	gateway := &fakeGateway{}
	svc := NewService(store, gateway, []model.PricingPack{{Code: "pack_100", Name: "100 Credits", Currency: "usd", AmountTotal: 990, QuotaAmount: 100, PackKind: string(model.PackKindExternalGeneration)}})

	order, checkoutURL, err := svc.CreateCheckout(context.Background(), CheckoutRequest{
		UserID:         42,
		PackCode:       "pack_100",
		TargetAPIKeyID: 7,
	})

	require.Error(t, err)
	require.Nil(t, order)
	require.Empty(t, checkoutURL)
	require.Contains(t, err.Error(), "target api key")
	require.Empty(t, store.orders)
	require.False(t, gateway.called)
	require.Zero(t, store.updateOrderCalls)
}

func TestCreateCheckoutRejectsDisabledTargetKey(t *testing.T) {
	t.Parallel()

	quota := 10
	store := &fakeStore{
		apiKeys: map[uint64]*model.APIKey{
			7: {
				ID:          7,
				OwnerUserID: uint64Ptr(42),
				Status:      model.APIKeyStatusDisabled,
				PlanName:    "Growth",
				QuotaTotal:  &quota,
			},
		},
	}
	gateway := &fakeGateway{}
	svc := NewService(store, gateway, []model.PricingPack{{Code: "pack_100", Name: "100 Credits", Currency: "usd", AmountTotal: 990, QuotaAmount: 100, PackKind: string(model.PackKindExternalGeneration)}})

	order, checkoutURL, err := svc.CreateCheckout(context.Background(), CheckoutRequest{
		UserID:         42,
		PackCode:       "pack_100",
		TargetAPIKeyID: 7,
	})

	require.Error(t, err)
	require.Nil(t, order)
	require.Empty(t, checkoutURL)
	require.Contains(t, err.Error(), "disabled")
	require.Empty(t, store.orders)
	require.False(t, gateway.called)
}

func TestCreateCheckoutUsesExistingStripeCustomerID(t *testing.T) {
	t.Parallel()

	quota := 10
	store := &fakeStore{
		apiKeys: map[uint64]*model.APIKey{
			7: {
				ID:          7,
				OwnerUserID: uint64Ptr(42),
				Status:      model.APIKeyStatusActive,
				PlanName:    "Growth",
				QuotaTotal:  &quota,
			},
		},
		stripeCustomer: &model.StripeCustomer{
			UserID:           42,
			StripeCustomerID: "cus_existing_123",
		},
	}
	gateway := &fakeGateway{}
	svc := NewService(store, gateway, []model.PricingPack{{Code: "pack_100", Name: "100 Credits", Currency: "usd", AmountTotal: 990, QuotaAmount: 100, PackKind: string(model.PackKindExternalGeneration)}})

	order, checkoutURL, err := svc.CreateCheckout(context.Background(), CheckoutRequest{
		UserID:         42,
		PackCode:       "pack_100",
		TargetAPIKeyID: 7,
	})

	require.NoError(t, err)
	require.NotNil(t, order)
	require.NotEmpty(t, checkoutURL)
	require.True(t, gateway.called)
	require.Equal(t, "cus_existing_123", gateway.lastCustomerID)
}

func uint64Ptr(v uint64) *uint64 {
	return &v
}
