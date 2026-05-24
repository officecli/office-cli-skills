package billing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v82"

	"github.com/officecli/officecli-internal/platform/internal/model"
)

type fakeGateway struct {
	called                bool
	lastCustomerID        string
	customerIDs           []string
	createCheckoutErrs    []error
	createCheckoutResults []*CheckoutSession
	checkoutSession       *CheckoutSessionDetails
	webhookEvent          *WebhookEvent
}

func (f *fakeGateway) CreateCheckoutSession(_ context.Context, _ CheckoutRequest, _ model.PricingPack, customerID string) (*CheckoutSession, error) {
	f.called = true
	f.lastCustomerID = customerID
	f.customerIDs = append(f.customerIDs, customerID)
	if len(f.createCheckoutErrs) > 0 {
		err := f.createCheckoutErrs[0]
		f.createCheckoutErrs = f.createCheckoutErrs[1:]
		if err != nil {
			return nil, err
		}
	}
	if len(f.createCheckoutResults) > 0 {
		result := f.createCheckoutResults[0]
		f.createCheckoutResults = f.createCheckoutResults[1:]
		if result == nil {
			return nil, nil
		}
		cloned := *result
		return &cloned, nil
	}
	return &CheckoutSession{
		ID:         "cs_test_123",
		URL:        "https://checkout.stripe.test/session/cs_test_123",
		CustomerID: "cus_test_123",
	}, nil
}

func (f *fakeGateway) GetCheckoutSession(_ context.Context, _ string) (*CheckoutSessionDetails, error) {
	if f.checkoutSession == nil {
		return &CheckoutSessionDetails{}, nil
	}
	cloned := *f.checkoutSession
	return &cloned, nil
}

func (f *fakeGateway) ParseWebhook(_ []byte, _ string) (*WebhookEvent, error) {
	if f.webhookEvent == nil {
		return nil, nil
	}
	cloned := *f.webhookEvent
	return &cloned, nil
}

type fakeStore struct {
	apiKeys              map[uint64]*model.APIKey
	orders               []*model.Order
	billingEvents        map[string]*model.BillingEvent
	hostedPacks          []model.HostedCreditPack
	userCredits          map[uint64]int
	updateOrderCalls     int
	stripeCustomer       *model.StripeCustomer
	createdAuditCount    int
	finalizePaymentCalls int
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
	key.HostedEnabled = true
	switch key.AllowedModes {
	case "", "external_only":
		key.AllowedModes = "hybrid"
	}
	if key.DefaultRuntimeMode == nil || *key.DefaultRuntimeMode == "" || *key.DefaultRuntimeMode == "external" {
		defaultRuntimeMode := "hosted"
		key.DefaultRuntimeMode = &defaultRuntimeMode
	}
	cloned := *key
	return &cloned, nil
}

func (f *fakeStore) CreateBillingEvent(_ context.Context, event *model.BillingEvent) error {
	if event == nil {
		return nil
	}
	if f.billingEvents == nil {
		f.billingEvents = make(map[string]*model.BillingEvent)
	}
	cloned := *event
	f.billingEvents[event.EventID] = &cloned
	return nil
}

func (f *fakeStore) GetBillingEventByEventID(_ context.Context, eventID string) (*model.BillingEvent, error) {
	if event := f.billingEvents[eventID]; event != nil {
		cloned := *event
		return &cloned, nil
	}
	return nil, nil
}

func (f *fakeStore) FinalizeOrderPayment(_ context.Context, params FinalizeOrderPaymentParams) (*model.Order, bool, error) {
	f.finalizePaymentCalls++
	for _, order := range f.orders {
		if order.ID != params.OrderID {
			continue
		}
		if order.Status != model.OrderStatusPending {
			cloned := *order
			return &cloned, false, nil
		}
		order.Status = model.OrderStatusPaid
		if params.PaymentIntentID != "" {
			paymentIntentID := params.PaymentIntentID
			order.StripePaymentIntentID = &paymentIntentID
		}
		if params.CustomerID != "" {
			customerID := params.CustomerID
			order.StripeCustomerID = &customerID
			f.stripeCustomer = &model.StripeCustomer{UserID: order.UserID, StripeCustomerID: customerID}
		}
		switch order.PackKind {
		case model.PackKindHostedCredits:
			if f.userCredits == nil {
				f.userCredits = map[uint64]int{}
			}
			f.userCredits[order.UserID] += order.CreditAmount
		default:
			if order.TargetAPIKeyID != nil {
				_, _ = f.AddPaidQuotaToAPIKey(context.Background(), *order.TargetAPIKeyID, order.QuotaAmount)
			}
		}
		if params.BillingEvent != nil {
			if f.billingEvents == nil {
				f.billingEvents = make(map[string]*model.BillingEvent)
			}
			clonedEvent := *params.BillingEvent
			clonedEvent.OrderID = &order.ID
			f.billingEvents[clonedEvent.EventID] = &clonedEvent
		}
		cloned := *order
		return &cloned, true, nil
	}
	return nil, false, nil
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

func (f *fakeStore) ListHostedCreditPacks(_ context.Context, enabledOnly bool) ([]model.HostedCreditPack, error) {
	var out []model.HostedCreditPack
	for _, pack := range f.hostedPacks {
		if enabledOnly && !pack.Enabled {
			continue
		}
		out = append(out, pack)
	}
	return out, nil
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

func TestCreateCheckoutNoLongerRequiresTargetAPIKey(t *testing.T) {
	t.Parallel()

	store := &fakeStore{}
	gateway := &fakeGateway{}
	svc := NewService(store, gateway, []model.PricingPack{{Code: "hosted-300", Name: "Hosted 300", Currency: "usd", AmountTotal: 990, CreditAmount: 300, PackKind: string(model.PackKindHostedCredits)}})

	order, checkoutURL, err := svc.CreateCheckout(context.Background(), CheckoutRequest{
		UserID:   42,
		PackCode: "hosted-300",
	})

	require.NoError(t, err)
	require.NotNil(t, order)
	require.NotEmpty(t, checkoutURL)
	require.Nil(t, order.TargetAPIKeyID)
	require.True(t, gateway.called)
	require.Equal(t, 1, store.updateOrderCalls)
}

func TestCreateCheckoutPreservesLegacyTargetAPIKeyIDWhenProvided(t *testing.T) {
	t.Parallel()

	store := &fakeStore{}
	gateway := &fakeGateway{}
	svc := NewService(store, gateway, []model.PricingPack{{Code: "hosted-300", Name: "Hosted 300", Currency: "usd", AmountTotal: 990, CreditAmount: 300, PackKind: string(model.PackKindHostedCredits)}})

	order, checkoutURL, err := svc.CreateCheckout(context.Background(), CheckoutRequest{
		UserID:         42,
		PackCode:       "hosted-300",
		TargetAPIKeyID: 7,
	})

	require.NoError(t, err)
	require.NotNil(t, order)
	require.NotEmpty(t, checkoutURL)
	require.NotNil(t, order.TargetAPIKeyID)
	require.Equal(t, uint64(7), *order.TargetAPIKeyID)
	require.True(t, gateway.called)
}

func TestPricingOmitsExternalGenerationPacks(t *testing.T) {
	t.Parallel()

	svc := NewService(&fakeStore{}, &fakeGateway{}, []model.PricingPack{
		{Code: "external-100", Name: "External 100", Currency: "usd", AmountTotal: 500, QuotaAmount: 100, PackKind: string(model.PackKindExternalGeneration)},
		{Code: "hosted-300", Name: "Hosted 300", Currency: "usd", AmountTotal: 300, CreditAmount: 300, PackKind: string(model.PackKindHostedCredits)},
	})

	packs := svc.Pricing()

	require.Len(t, packs, 1)
	require.Equal(t, "hosted-300", packs[0].Code)
}

func TestCreateCheckoutRejectsExternalGenerationPacks(t *testing.T) {
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
	}
	gateway := &fakeGateway{}
	svc := NewService(store, gateway, []model.PricingPack{{Code: "external-100", Name: "External 100", Currency: "usd", AmountTotal: 500, QuotaAmount: 100, PackKind: string(model.PackKindExternalGeneration)}})

	order, checkoutURL, err := svc.CreateCheckout(context.Background(), CheckoutRequest{
		UserID:         42,
		PackCode:       "external-100",
		TargetAPIKeyID: 7,
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "external mode is free and no longer sold as a paid pack")
	require.Nil(t, order)
	require.Empty(t, checkoutURL)
	require.False(t, gateway.called)
	require.Empty(t, store.orders)
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
	svc := NewService(store, gateway, []model.PricingPack{{Code: "hosted-300", Name: "Hosted 300", Currency: "usd", AmountTotal: 990, CreditAmount: 300, PackKind: string(model.PackKindHostedCredits)}})

	order, checkoutURL, err := svc.CreateCheckout(context.Background(), CheckoutRequest{
		UserID:         42,
		PackCode:       "hosted-300",
		TargetAPIKeyID: 7,
	})

	require.NoError(t, err)
	require.NotNil(t, order)
	require.NotEmpty(t, checkoutURL)
	require.True(t, gateway.called)
	require.Equal(t, "cus_existing_123", gateway.lastCustomerID)
}

func TestCreateCheckoutRetriesWithoutStoredStripeCustomerWhenStripeCustomerIsMissing(t *testing.T) {
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
			StripeCustomerID: "cus_test_legacy",
		},
	}
	gateway := &fakeGateway{
		createCheckoutErrs: []error{
			&stripe.Error{
				Type:  stripe.ErrorTypeInvalidRequest,
				Code:  stripe.ErrorCodeResourceMissing,
				Param: "customer",
				Msg:   "No such customer: 'cus_test_legacy'",
			},
			nil,
		},
	}
	svc := NewService(store, gateway, []model.PricingPack{{Code: "hosted-300", Name: "Hosted 300", Currency: "usd", AmountTotal: 990, CreditAmount: 300, PackKind: string(model.PackKindHostedCredits)}})

	order, checkoutURL, err := svc.CreateCheckout(context.Background(), CheckoutRequest{
		UserID:         42,
		PackCode:       "hosted-300",
		TargetAPIKeyID: 7,
	})

	require.NoError(t, err)
	require.NotNil(t, order)
	require.NotEmpty(t, checkoutURL)
	require.Equal(t, []string{"cus_test_legacy", ""}, gateway.customerIDs)
}

func TestCreateCheckoutDoesNotRetryWithoutCustomerForOtherStripeErrors(t *testing.T) {
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
			StripeCustomerID: "cus_live_existing",
		},
	}
	gateway := &fakeGateway{
		createCheckoutErrs: []error{
			&stripe.Error{
				Type:  stripe.ErrorTypeInvalidRequest,
				Code:  stripe.ErrorCodeParameterMissing,
				Param: "success_url",
				Msg:   "Missing required param: success_url",
			},
		},
	}
	svc := NewService(store, gateway, []model.PricingPack{{Code: "hosted-300", Name: "Hosted 300", Currency: "usd", AmountTotal: 990, CreditAmount: 300, PackKind: string(model.PackKindHostedCredits)}})

	order, checkoutURL, err := svc.CreateCheckout(context.Background(), CheckoutRequest{
		UserID:         42,
		PackCode:       "hosted-300",
		TargetAPIKeyID: 7,
	})

	require.Error(t, err)
	require.Nil(t, order)
	require.Empty(t, checkoutURL)
	require.Equal(t, []string{"cus_live_existing"}, gateway.customerIDs)
}

func TestCreateCheckoutAcceptsHostedCreditPack(t *testing.T) {
	t.Parallel()

	store := &fakeStore{
		apiKeys: map[uint64]*model.APIKey{
			7: {
				ID:            7,
				OwnerUserID:   uint64Ptr(42),
				Status:        model.APIKeyStatusActive,
				PlanName:      "Hosted",
				AllowedModes:  "external_only",
				HostedEnabled: false,
			},
		},
	}
	gateway := &fakeGateway{}
	svc := NewService(store, gateway, []model.PricingPack{{
		Code:         "hosted-300",
		Name:         "Hosted 300",
		Currency:     "usd",
		AmountTotal:  300,
		CreditAmount: 300,
		PackKind:     string(model.PackKindHostedCredits),
	}})

	order, checkoutURL, err := svc.CreateCheckout(context.Background(), CheckoutRequest{
		UserID:         42,
		PackCode:       "hosted-300",
		TargetAPIKeyID: 7,
	})

	require.NoError(t, err)
	require.NotNil(t, order)
	require.NotEmpty(t, checkoutURL)
	require.Equal(t, model.PackKindHostedCredits, order.PackKind)
	require.Equal(t, 300, order.CreditAmount)
	require.True(t, gateway.called)
}

func TestReconcileCheckoutSessionAddsHostedCredits(t *testing.T) {
	t.Parallel()

	sessionID := "cs_test_hosted"
	store := &fakeStore{
		apiKeys: map[uint64]*model.APIKey{
			7: {
				ID:            7,
				OwnerUserID:   uint64Ptr(42),
				Status:        model.APIKeyStatusActive,
				PlanName:      "Hosted",
				AllowedModes:  "external_only",
				HostedEnabled: false,
			},
		},
		orders: []*model.Order{{
			ID:                      1,
			UserID:                  42,
			Status:                  model.OrderStatusPending,
			PackCode:                "hosted-300",
			PackName:                "Hosted 300",
			PackKind:                model.PackKindHostedCredits,
			CreditAmount:            300,
			TargetAPIKeyID:          uint64Ptr(7),
			StripeCheckoutSessionID: &sessionID,
		}},
	}
	gateway := &fakeGateway{
		checkoutSession: &CheckoutSessionDetails{
			ID:              sessionID,
			PaymentStatus:   "paid",
			PaymentIntentID: "pi_hosted_123",
			CustomerID:      "cus_hosted_123",
			Metadata:        map[string]string{"target_api_key_id": "7"},
		},
	}
	svc := NewService(store, gateway, []model.PricingPack{{Code: "hosted-300", Name: "Hosted 300", Currency: "usd", AmountTotal: 300, CreditAmount: 300, PackKind: string(model.PackKindHostedCredits)}})

	order, err := svc.ReconcileCheckoutSession(context.Background(), ReconcileOrderRequest{
		UserID:            42,
		CheckoutSessionID: sessionID,
	})

	require.NoError(t, err)
	require.NotNil(t, order)
	require.Equal(t, model.OrderStatusPaid, order.Status)
	require.Equal(t, 300, store.userCredits[42])
	require.Equal(t, 0, store.apiKeys[7].CreditBalance)
}

func uint64Ptr(v uint64) *uint64 {
	return &v
}

func TestReconcileCheckoutSessionMarksPendingOrderPaidOnce(t *testing.T) {
	t.Parallel()

	quota := 10
	sessionID := "cs_test_paid"
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
		orders: []*model.Order{{
			ID:                      1,
			UserID:                  42,
			Status:                  model.OrderStatusPending,
			PackCode:                "pack_100",
			PackName:                "100 Credits",
			PackKind:                model.PackKindExternalGeneration,
			QuotaAmount:             100,
			TargetAPIKeyID:          uint64Ptr(7),
			StripeCheckoutSessionID: &sessionID,
		}},
	}
	gateway := &fakeGateway{
		checkoutSession: &CheckoutSessionDetails{
			ID:              sessionID,
			PaymentStatus:   "paid",
			PaymentIntentID: "pi_test_123",
			CustomerID:      "cus_test_123",
			Metadata:        map[string]string{"target_api_key_id": "7"},
		},
	}
	svc := NewService(store, gateway, []model.PricingPack{{Code: "pack_100", Name: "100 Credits", Currency: "usd", AmountTotal: 990, QuotaAmount: 100, PackKind: string(model.PackKindExternalGeneration)}})

	order, err := svc.ReconcileCheckoutSession(context.Background(), ReconcileOrderRequest{
		UserID:            42,
		CheckoutSessionID: sessionID,
	})

	require.NoError(t, err)
	require.NotNil(t, order)
	require.Equal(t, model.OrderStatusPaid, order.Status)
	require.NotNil(t, order.StripePaymentIntentID)
	require.Equal(t, "pi_test_123", *order.StripePaymentIntentID)
	require.Equal(t, 1, store.finalizePaymentCalls)
	require.Equal(t, 110, *store.apiKeys[7].QuotaTotal)
	require.Contains(t, store.billingEvents, "reconcile:checkout_session.paid:"+sessionID)

	order, err = svc.ReconcileCheckoutSession(context.Background(), ReconcileOrderRequest{
		UserID:            42,
		CheckoutSessionID: sessionID,
	})

	require.NoError(t, err)
	require.NotNil(t, order)
	require.Equal(t, model.OrderStatusPaid, order.Status)
	require.Equal(t, 1, store.finalizePaymentCalls)
	require.Equal(t, 110, *store.apiKeys[7].QuotaTotal)
	require.Equal(t, 1, store.createdAuditCount)
}

func TestHandleWebhookAndReconcileRemainIdempotent(t *testing.T) {
	t.Parallel()

	quota := 5
	sessionID := "cs_test_123"
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
		orders: []*model.Order{{
			ID:                      1,
			UserID:                  42,
			Status:                  model.OrderStatusPending,
			PackCode:                "pack_100",
			PackName:                "100 Credits",
			PackKind:                model.PackKindExternalGeneration,
			QuotaAmount:             100,
			TargetAPIKeyID:          uint64Ptr(7),
			StripeCheckoutSessionID: &sessionID,
		}},
	}
	gateway := &fakeGateway{
		webhookEvent: &WebhookEvent{
			ID:                "evt_1",
			Type:              "checkout.session.completed",
			CheckoutSessionID: sessionID,
			PaymentIntentID:   "pi_test_123",
			CustomerID:        "cus_test_123",
			RawPayload:        `{"id":"evt_1"}`,
		},
		checkoutSession: &CheckoutSessionDetails{
			ID:              sessionID,
			PaymentStatus:   "paid",
			PaymentIntentID: "pi_test_123",
			CustomerID:      "cus_test_123",
		},
	}
	svc := NewService(store, gateway, []model.PricingPack{{Code: "pack_100", Name: "100 Credits", Currency: "usd", AmountTotal: 990, QuotaAmount: 100, PackKind: string(model.PackKindExternalGeneration)}})

	require.NoError(t, svc.HandleWebhook(context.Background(), []byte(`{}`), "sig"))
	require.Equal(t, model.OrderStatusPaid, store.orders[0].Status)
	require.Equal(t, 105, *store.apiKeys[7].QuotaTotal)
	require.Equal(t, 1, store.createdAuditCount)

	order, err := svc.ReconcileCheckoutSession(context.Background(), ReconcileOrderRequest{
		UserID:            42,
		CheckoutSessionID: sessionID,
	})

	require.NoError(t, err)
	require.NotNil(t, order)
	require.Equal(t, model.OrderStatusPaid, order.Status)
	require.Equal(t, 105, *store.apiKeys[7].QuotaTotal)
	require.Equal(t, 1, store.createdAuditCount)
}

func TestPricingDedupesByCodeWithConfigWinningOverStaleDBPack(t *testing.T) {
	t.Parallel()

	store := &fakeStore{
		hostedPacks: []model.HostedCreditPack{
			{ID: 1, Code: "hosted-100", Name: "Hosted 100 (stale)", Currency: "usd", AmountTotal: 500, CreditAmount: 100, Enabled: true},
			{ID: 2, Code: "hosted-2500", Name: "Hosted 2500", Currency: "usd", AmountTotal: 19900, CreditAmount: 2500, Enabled: true},
		},
	}
	svc := NewService(store, &fakeGateway{}, []model.PricingPack{
		{Code: "hosted-100", Name: "Hosted 100", Currency: "usd", AmountTotal: 100, CreditAmount: 100, PackKind: string(model.PackKindHostedCredits)},
		{Code: "hosted-300", Name: "Hosted 300", Currency: "usd", AmountTotal: 2900, CreditAmount: 300, PackKind: string(model.PackKindHostedCredits)},
	})

	packs := svc.Pricing()

	require.Len(t, packs, 3)
	byCode := map[string]model.PricingPack{}
	for _, p := range packs {
		byCode[p.Code] = p
	}
	require.Equal(t, int64(100), byCode["hosted-100"].AmountTotal, "config-defined hosted-100 ($1) must override stale DB row ($5)")
	require.Equal(t, "Hosted 100", byCode["hosted-100"].Name)
	require.Equal(t, int64(2900), byCode["hosted-300"].AmountTotal)
	require.Equal(t, int64(19900), byCode["hosted-2500"].AmountTotal, "admin-added pack with no config equivalent must still surface")
}

func TestCreateCheckoutSnapshotsConfigPriceWhenStaleDBPackShadowsCode(t *testing.T) {
	t.Parallel()

	store := &fakeStore{
		hostedPacks: []model.HostedCreditPack{
			{ID: 1, Code: "hosted-100", Name: "Hosted 100 (stale)", Currency: "usd", AmountTotal: 500, CreditAmount: 100, Enabled: true},
		},
	}
	gateway := &fakeGateway{}
	svc := NewService(store, gateway, []model.PricingPack{
		{Code: "hosted-100", Name: "Hosted 100", Currency: "usd", AmountTotal: 100, CreditAmount: 100, PackKind: string(model.PackKindHostedCredits)},
	})

	order, checkoutURL, err := svc.CreateCheckout(context.Background(), CheckoutRequest{UserID: 42, PackCode: "hosted-100"})

	require.NoError(t, err)
	require.NotNil(t, order)
	require.NotEmpty(t, checkoutURL)
	require.Equal(t, int64(100), order.AmountTotal, "checkout must use config $1 price, not stale DB $5 price")
	require.Equal(t, 100, order.CreditAmount)
	require.Equal(t, "Hosted 100", order.PackName)
}

func TestReconcileCheckoutSessionLeavesOrderPendingWhenStripePaymentNotConfirmed(t *testing.T) {
	t.Parallel()

	sessionID := "cs_live_unpaid_xyz"
	store := &fakeStore{
		orders: []*model.Order{{
			ID:                      77,
			UserID:                  42,
			Status:                  model.OrderStatusPending,
			PackCode:                "hosted-100",
			PackName:                "Hosted 100",
			PackKind:                model.PackKindHostedCredits,
			AmountTotal:             100,
			CreditAmount:            100,
			StripeCheckoutSessionID: &sessionID,
		}},
	}
	gateway := &fakeGateway{
		checkoutSession: &CheckoutSessionDetails{
			ID:            sessionID,
			PaymentStatus: "unpaid",
		},
	}
	svc := NewService(store, gateway, []model.PricingPack{{Code: "hosted-100", Name: "Hosted 100", Currency: "usd", AmountTotal: 100, CreditAmount: 100, PackKind: string(model.PackKindHostedCredits)}})

	order, err := svc.ReconcileCheckoutSession(context.Background(), ReconcileOrderRequest{
		UserID:            42,
		CheckoutSessionID: sessionID,
	})

	require.NoError(t, err)
	require.NotNil(t, order)
	require.Equal(t, model.OrderStatusPending, order.Status, "order must remain pending so the route can return awaiting_confirmation=true")
	require.Equal(t, 0, store.finalizePaymentCalls)
}
