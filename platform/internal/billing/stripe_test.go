package billing

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/officecli/officecli/platform/internal/model"
)

func TestCheckoutSessionParamsReuseExistingCustomer(t *testing.T) {
	t.Parallel()

	gateway := NewStripeGateway("sk_test_123", "", "https://platform.officecli.io/app/billing?status=success", "https://platform.officecli.io/app/billing?status=cancel")
	params := gateway.checkoutSessionParams(
		CheckoutRequest{UserID: 42, PackCode: "external-100", TargetAPIKeyID: 7},
		model.PricingPack{Code: "external-100", Name: "External 100", Description: "External pack", Currency: "usd", AmountTotal: 990, QuotaAmount: 100, PackKind: string(model.PackKindExternalGeneration)},
		"cus_existing_123",
	)

	require.NotNil(t, params.Customer)
	require.Equal(t, "cus_existing_123", *params.Customer)
	require.Nil(t, params.CustomerCreation)
}

func TestCheckoutSessionParamsCreateCustomerForFirstPurchase(t *testing.T) {
	t.Parallel()

	gateway := NewStripeGateway("sk_test_123", "", "https://platform.officecli.io/app/billing?status=success", "https://platform.officecli.io/app/billing?status=cancel")
	params := gateway.checkoutSessionParams(
		CheckoutRequest{UserID: 42, PackCode: "external-100", TargetAPIKeyID: 7},
		model.PricingPack{Code: "external-100", Name: "External 100", Description: "External pack", Currency: "usd", AmountTotal: 990, QuotaAmount: 100, PackKind: string(model.PackKindExternalGeneration)},
		"",
	)

	require.Nil(t, params.Customer)
	require.NotNil(t, params.CustomerCreation)
	require.Equal(t, "always", *params.CustomerCreation)
}

func TestCreateCheckoutSessionFailsFastWhenStripeSecretKeyMissing(t *testing.T) {
	t.Parallel()

	gateway := NewStripeGateway("", "", "https://platform.officecli.io/app/billing?status=success", "https://platform.officecli.io/app/billing?status=cancel")
	_, err := gateway.CreateCheckoutSession(
		t.Context(),
		CheckoutRequest{UserID: 42, PackCode: "external-100", TargetAPIKeyID: 7},
		model.PricingPack{Code: "external-100", Name: "External 100", Description: "External pack", Currency: "usd", AmountTotal: 990, QuotaAmount: 100, PackKind: string(model.PackKindExternalGeneration)},
		"",
	)

	require.Error(t, err)
	require.Contains(t, err.Error(), "stripe secret key is not configured")
}
