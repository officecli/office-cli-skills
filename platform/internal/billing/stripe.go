package billing

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/checkout/session"
	"github.com/stripe/stripe-go/v82/webhook"

	"github.com/officecli/officecli/platform/internal/model"
)

type StripeGateway struct {
	secretKey     string
	webhookSecret string
	successURL    string
	cancelURL     string
}

func NewStripeGateway(secretKey, webhookSecret, successURL, cancelURL string) *StripeGateway {
	stripe.Key = secretKey
	return &StripeGateway{secretKey: secretKey, webhookSecret: webhookSecret, successURL: successURL, cancelURL: cancelURL}
}

func (g *StripeGateway) CreateCheckoutSession(ctx context.Context, req CheckoutRequest, pack model.PricingPack, customerID string) (*CheckoutSession, error) {
	if g.secretKey == "" {
		return nil, fmt.Errorf("stripe secret key is not configured")
	}
	params := g.checkoutSessionParams(req, pack, customerID)
	created, err := session.New(params)
	if err != nil {
		return nil, err
	}
	result := &CheckoutSession{ID: created.ID, URL: created.URL}
	if created.Customer != nil {
		result.CustomerID = created.Customer.ID
	}
	return result, nil
}

func (g *StripeGateway) GetCheckoutSession(ctx context.Context, sessionID string) (*CheckoutSessionDetails, error) {
	if g.secretKey == "" {
		return nil, fmt.Errorf("stripe secret key is not configured")
	}
	params := &stripe.CheckoutSessionParams{}
	params.Context = ctx
	retrieved, err := session.Get(sessionID, params)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(retrieved)
	if err != nil {
		return nil, err
	}
	result := &CheckoutSessionDetails{
		ID:            retrieved.ID,
		PaymentStatus: string(retrieved.PaymentStatus),
		Metadata:      retrieved.Metadata,
		RawPayload:    string(payload),
	}
	if retrieved.PaymentIntent != nil {
		result.PaymentIntentID = retrieved.PaymentIntent.ID
	}
	if retrieved.Customer != nil {
		result.CustomerID = retrieved.Customer.ID
	}
	return result, nil
}

func (g *StripeGateway) checkoutSessionParams(req CheckoutRequest, pack model.PricingPack, customerID string) *stripe.CheckoutSessionParams {
	params := &stripe.CheckoutSessionParams{
		Mode:       stripe.String(string(stripe.CheckoutSessionModePayment)),
		SuccessURL: stripe.String(g.successURL),
		CancelURL:  stripe.String(g.cancelURL),
		LineItems: []*stripe.CheckoutSessionLineItemParams{{
			Quantity: stripe.Int64(1),
			PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
				Currency:   stripe.String(pack.Currency),
				UnitAmount: stripe.Int64(pack.AmountTotal),
				ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
					Name:        stripe.String(pack.Name),
					Description: stripe.String(pack.Description),
				},
			},
		}},
		Metadata: map[string]string{
			"user_id":           fmt.Sprintf("%d", req.UserID),
			"target_api_key_id": fmt.Sprintf("%d", req.TargetAPIKeyID),
			"pack_code":         req.PackCode,
		},
	}
	if customerID != "" {
		params.Customer = stripe.String(customerID)
	} else {
		params.CustomerCreation = stripe.String(string(stripe.CheckoutSessionCustomerCreationAlways))
	}
	return params
}

func (g *StripeGateway) ParseWebhook(payload []byte, signature string) (*WebhookEvent, error) {
	if g.webhookSecret == "" {
		return nil, fmt.Errorf("stripe webhook secret is not configured")
	}
	event, err := webhook.ConstructEventWithOptions(payload, signature, g.webhookSecret, webhook.ConstructEventOptions{
		IgnoreAPIVersionMismatch: true,
	})
	if err != nil {
		return nil, err
	}
	result := &WebhookEvent{ID: event.ID, Type: string(event.Type), RawPayload: string(payload)}
	switch event.Type {
	case "checkout.session.completed", "payment_intent.payment_failed", "charge.refunded":
		var data struct {
			ID            string            `json:"id"`
			PaymentStatus string            `json:"payment_status"`
			PaymentIntent string            `json:"payment_intent"`
			Customer      string            `json:"customer"`
			Metadata      map[string]string `json:"metadata"`
		}
		if err := json.Unmarshal(event.Data.Raw, &data); err != nil {
			return nil, err
		}
		result.CheckoutSessionID = data.ID
		result.PaymentStatus = data.PaymentStatus
		result.PaymentIntentID = data.PaymentIntent
		result.CustomerID = data.Customer
		result.Metadata = data.Metadata
	}
	return result, nil
}
