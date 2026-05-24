package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/stripe/stripe-go/v82"

	"github.com/officecli/officecli-internal/platform/internal/model"
)

type CheckoutSession struct {
	ID         string
	URL        string
	CustomerID string
}

type CheckoutSessionDetails struct {
	ID              string
	PaymentStatus   string
	PaymentIntentID string
	CustomerID      string
	Metadata        map[string]string
	RawPayload      string
}

type WebhookEvent struct {
	ID                string
	Type              string
	CheckoutSessionID string
	PaymentIntentID   string
	CustomerID        string
	Metadata          map[string]string
	RawPayload        string
}

type Gateway interface {
	CreateCheckoutSession(ctx context.Context, req CheckoutRequest, pack model.PricingPack, customerID string) (*CheckoutSession, error)
	GetCheckoutSession(ctx context.Context, sessionID string) (*CheckoutSessionDetails, error)
	ParseWebhook(payload []byte, signature string) (*WebhookEvent, error)
}

type FinalizeOrderPaymentParams struct {
	OrderID         uint64
	PaymentIntentID string
	CustomerID      string
	BillingEvent    *model.BillingEvent
}

type Store interface {
	GetOrCreateStripeCustomer(ctx context.Context, userID uint64, customerID string) (*model.StripeCustomer, error)
	GetStripeCustomerByUserID(ctx context.Context, userID uint64) (*model.StripeCustomer, error)
	FindAPIKeyByID(ctx context.Context, id uint64) (*model.APIKey, error)
	CreateOrder(ctx context.Context, order *model.Order) error
	GetOrderByCheckoutSessionID(ctx context.Context, sessionID string) (*model.Order, error)
	GetOrderByID(ctx context.Context, id uint64) (*model.Order, error)
	UpdateOrder(ctx context.Context, id uint64, values map[string]any) error
	CreateBillingEvent(ctx context.Context, event *model.BillingEvent) error
	GetBillingEventByEventID(ctx context.Context, eventID string) (*model.BillingEvent, error)
	FinalizeOrderPayment(ctx context.Context, params FinalizeOrderPaymentParams) (*model.Order, bool, error)
	ListOrdersByUser(ctx context.Context, userID uint64) ([]model.Order, error)
	ListOrders(ctx context.Context) ([]model.Order, error)
	ListBillingEvents(ctx context.Context) ([]model.BillingEvent, error)
	ListHostedCreditPacks(ctx context.Context, enabledOnly bool) ([]model.HostedCreditPack, error)
	CreateAuditLog(ctx context.Context, action, targetType, targetID string, payload string) error
}

type CheckoutRequest struct {
	UserID         uint64 `json:"-"`
	PackCode       string `json:"pack_code"`
	TargetAPIKeyID uint64 `json:"target_api_key_id,omitempty"`
}

type ReconcileOrderRequest struct {
	UserID            uint64 `json:"-"`
	CheckoutSessionID string `json:"checkout_session_id"`
}

type Service struct {
	store   Store
	gateway Gateway
	mu      sync.RWMutex
	packs   map[string]model.PricingPack
}

var (
	ErrCheckoutSessionIDRequired = errors.New("checkout session id is required")
	ErrOrderNotFound             = errors.New("order not found")
	ErrOrderForbidden            = errors.New("order does not belong to current user")
)

func NewService(store Store, gateway Gateway, packs []model.PricingPack) *Service {
	byCode := make(map[string]model.PricingPack, len(packs))
	for _, pack := range packs {
		byCode[pack.Code] = pack
	}
	return &Service{store: store, gateway: gateway, packs: byCode}
}

func (s *Service) Pricing() []model.PricingPack {
	s.mu.RLock()
	defer s.mu.RUnlock()
	byCode := make(map[string]model.PricingPack, len(s.packs))
	for _, pack := range s.packs {
		if pack.PackKind == string(model.PackKindHostedCredits) {
			byCode[pack.Code] = pack
		}
	}
	if s.store != nil {
		if hostedPacks, err := s.store.ListHostedCreditPacks(context.Background(), true); err == nil {
			for _, pack := range hostedPacks {
				if _, exists := byCode[pack.Code]; exists {
					continue
				}
				byCode[pack.Code] = pack.PricingPack()
			}
		}
	}
	result := make([]model.PricingPack, 0, len(byCode))
	for _, pack := range byCode {
		result = append(result, pack)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].AmountTotal == result[j].AmountTotal {
			return result[i].Code < result[j].Code
		}
		return result[i].AmountTotal < result[j].AmountTotal
	})
	return result
}

func (s *Service) UpdatePricing(ctx context.Context, packs []model.PricingPack) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	byCode := make(map[string]model.PricingPack, len(packs))
	for _, pack := range packs {
		if pack.Code == "" {
			return fmt.Errorf("pricing pack code is required")
		}
		byCode[pack.Code] = pack
	}
	s.packs = byCode
	_ = s.store.CreateAuditLog(ctx, "pricing.update", "pricing", "packs", fmt.Sprintf("%d packs", len(packs)))
	return nil
}

func (s *Service) CreateCheckout(ctx context.Context, req CheckoutRequest) (*model.Order, string, error) {
	pack, ok := s.pricingPack(ctx, req.PackCode)
	if !ok || (pack.PackKind != string(model.PackKindExternalGeneration) && pack.PackKind != string(model.PackKindHostedCredits)) {
		return nil, "", fmt.Errorf("unknown pack_code")
	}
	if pack.PackKind == string(model.PackKindExternalGeneration) {
		return nil, "", fmt.Errorf("external mode is free and no longer sold as a paid pack")
	}
	order := &model.Order{
		UserID:       req.UserID,
		Status:       model.OrderStatusPending,
		Currency:     pack.Currency,
		AmountTotal:  pack.AmountTotal,
		PackCode:     pack.Code,
		PackName:     pack.Name,
		PackKind:     model.PackKind(pack.PackKind),
		QuotaAmount:  pack.QuotaAmount,
		CreditAmount: pack.CreditAmount,
	}
	if req.TargetAPIKeyID > 0 {
		order.TargetAPIKeyID = &req.TargetAPIKeyID
	}
	if err := s.store.CreateOrder(ctx, order); err != nil {
		return nil, "", err
	}

	customer, err := s.store.GetStripeCustomerByUserID(ctx, req.UserID)
	if err != nil {
		return nil, "", err
	}
	customerID := ""
	if customer != nil {
		customerID = customer.StripeCustomerID
	}

	session, err := s.gateway.CreateCheckoutSession(ctx, req, pack, customerID)
	if err != nil && shouldRetryCheckoutWithoutCustomer(err, customerID) {
		session, err = s.gateway.CreateCheckoutSession(ctx, req, pack, "")
	}
	if err != nil {
		return nil, "", err
	}
	updates := map[string]any{"stripe_checkout_session_id": session.ID}
	if session.CustomerID != "" {
		updates["stripe_customer_id"] = session.CustomerID
		if _, err := s.store.GetOrCreateStripeCustomer(ctx, req.UserID, session.CustomerID); err != nil {
			return nil, "", err
		}
	}
	if err := s.store.UpdateOrder(ctx, order.ID, updates); err != nil {
		return nil, "", err
	}
	order, err = s.store.GetOrderByID(ctx, order.ID)
	if err != nil {
		return nil, "", err
	}
	return order, session.URL, nil
}

func (s *Service) pricingPack(ctx context.Context, code string) (model.PricingPack, bool) {
	s.mu.RLock()
	pack, ok := s.packs[code]
	s.mu.RUnlock()
	if ok {
		return pack, true
	}
	if s.store != nil {
		if hostedPacks, err := s.store.ListHostedCreditPacks(ctx, true); err == nil {
			for _, hostedPack := range hostedPacks {
				if hostedPack.Code == code {
					return hostedPack.PricingPack(), true
				}
			}
		}
	}
	return model.PricingPack{}, false
}

func (s *Service) HandleWebhook(ctx context.Context, payload []byte, signature string) error {
	event, err := s.gateway.ParseWebhook(payload, signature)
	if err != nil {
		return err
	}
	existing, err := s.store.GetBillingEventByEventID(ctx, event.ID)
	if err != nil {
		return err
	}
	if existing != nil {
		return nil
	}

	billEvent := &model.BillingEvent{EventID: event.ID, EventType: event.Type, Status: model.BillingEventStatusHandled, PayloadJSON: event.RawPayload}
	switch event.Type {
	case "checkout.session.completed":
		order, err := s.store.GetOrderByCheckoutSessionID(ctx, event.CheckoutSessionID)
		if err != nil {
			return err
		}
		if order == nil {
			msg := "order not found for checkout session"
			billEvent.Status = model.BillingEventStatusFailed
			billEvent.ErrorMessage = &msg
			_ = s.store.CreateBillingEvent(ctx, billEvent)
			return fmt.Errorf("%s", msg)
		}
		_, _, err = s.finalizePaidOrder(ctx, order, billEvent, event.PaymentIntentID, event.CustomerID)
		return err
	case "payment_intent.payment_failed":
		order, _ := s.store.GetOrderByCheckoutSessionID(ctx, event.CheckoutSessionID)
		if order != nil {
			billEvent.OrderID = &order.ID
			_ = s.store.UpdateOrder(ctx, order.ID, map[string]any{"status": model.OrderStatusFailed, "stripe_payment_intent_id": event.PaymentIntentID})
		}
	case "charge.refunded":
		order, _ := s.store.GetOrderByCheckoutSessionID(ctx, event.CheckoutSessionID)
		if order != nil {
			billEvent.OrderID = &order.ID
			_ = s.store.UpdateOrder(ctx, order.ID, map[string]any{"status": model.OrderStatusRefunded})
		}
	default:
		billEvent.Status = model.BillingEventStatusIgnored
	}
	processedAt := time.Now().UTC()
	billEvent.ProcessedAt = &processedAt
	return s.store.CreateBillingEvent(ctx, billEvent)
}

func (s *Service) ListOrdersByUser(ctx context.Context, userID uint64) ([]model.Order, error) {
	return s.store.ListOrdersByUser(ctx, userID)
}

func (s *Service) ListOrders(ctx context.Context) ([]model.Order, error) {
	return s.store.ListOrders(ctx)
}

func (s *Service) ListBillingEvents(ctx context.Context) ([]model.BillingEvent, error) {
	return s.store.ListBillingEvents(ctx)
}

func shouldRetryCheckoutWithoutCustomer(err error, customerID string) bool {
	if strings.TrimSpace(customerID) == "" {
		return false
	}
	var stripeErr *stripe.Error
	if !errors.As(err, &stripeErr) || stripeErr == nil {
		return false
	}
	return stripeErr.Type == stripe.ErrorTypeInvalidRequest &&
		stripeErr.Code == stripe.ErrorCodeResourceMissing &&
		strings.TrimSpace(stripeErr.Param) == "customer"
}

func (s *Service) ReconcileCheckoutSession(ctx context.Context, req ReconcileOrderRequest) (*model.Order, error) {
	sessionID := strings.TrimSpace(req.CheckoutSessionID)
	if sessionID == "" {
		return nil, ErrCheckoutSessionIDRequired
	}
	order, err := s.store.GetOrderByCheckoutSessionID(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	switch {
	case order == nil:
		return nil, ErrOrderNotFound
	case order.UserID != req.UserID:
		return nil, ErrOrderForbidden
	case order.Status != model.OrderStatusPending:
		return order, nil
	}

	session, err := s.gateway.GetCheckoutSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session == nil || session.PaymentStatus != "paid" {
		refreshed, getErr := s.store.GetOrderByID(ctx, order.ID)
		if getErr != nil {
			return nil, getErr
		}
		if refreshed == nil {
			return nil, ErrOrderNotFound
		}
		return refreshed, nil
	}

	payload, err := json.Marshal(map[string]any{
		"id":             session.ID,
		"payment_status": session.PaymentStatus,
		"payment_intent": session.PaymentIntentID,
		"customer":       session.CustomerID,
		"metadata":       session.Metadata,
	})
	if err != nil {
		return nil, err
	}
	billEvent := &model.BillingEvent{
		EventID:     "reconcile:checkout_session.paid:" + session.ID,
		EventType:   "checkout.session.reconciled",
		Status:      model.BillingEventStatusHandled,
		PayloadJSON: string(payload),
	}
	updated, _, err := s.finalizePaidOrder(ctx, order, billEvent, session.PaymentIntentID, session.CustomerID)
	return updated, err
}

func (s *Service) finalizePaidOrder(ctx context.Context, order *model.Order, billEvent *model.BillingEvent, paymentIntentID string, customerID string) (*model.Order, bool, error) {
	updated, transitioned, err := s.store.FinalizeOrderPayment(ctx, FinalizeOrderPaymentParams{
		OrderID:         order.ID,
		PaymentIntentID: paymentIntentID,
		CustomerID:      customerID,
		BillingEvent:    billEvent,
	})
	if err != nil {
		return nil, false, err
	}
	if transitioned && billEvent != nil {
		_ = s.store.CreateAuditLog(ctx, "order.paid", "order", fmt.Sprintf("%d", order.ID), billEvent.PayloadJSON)
	}
	return updated, transitioned, nil
}
