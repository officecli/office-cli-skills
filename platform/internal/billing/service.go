package billing

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/officecli/officecli/platform/internal/model"
)

type CheckoutSession struct {
	ID         string
	URL        string
	CustomerID string
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
	ParseWebhook(payload []byte, signature string) (*WebhookEvent, error)
}

type Store interface {
	GetOrCreateStripeCustomer(ctx context.Context, userID uint64, customerID string) (*model.StripeCustomer, error)
	GetStripeCustomerByUserID(ctx context.Context, userID uint64) (*model.StripeCustomer, error)
	FindAPIKeyByID(ctx context.Context, id uint64) (*model.APIKey, error)
	CreateOrder(ctx context.Context, order *model.Order) error
	GetOrderByCheckoutSessionID(ctx context.Context, sessionID string) (*model.Order, error)
	GetOrderByID(ctx context.Context, id uint64) (*model.Order, error)
	UpdateOrder(ctx context.Context, id uint64, values map[string]any) error
	AddPaidQuotaToAPIKey(ctx context.Context, apiKeyID uint64, quotaAmount int) (*model.APIKey, error)
	AddCreditBalanceToAPIKey(ctx context.Context, apiKeyID uint64, creditAmount int) (*model.APIKey, error)
	CreateBillingEvent(ctx context.Context, event *model.BillingEvent) error
	GetBillingEventByEventID(ctx context.Context, eventID string) (*model.BillingEvent, error)
	ListOrdersByUser(ctx context.Context, userID uint64) ([]model.Order, error)
	ListOrders(ctx context.Context) ([]model.Order, error)
	ListBillingEvents(ctx context.Context) ([]model.BillingEvent, error)
	CreateAuditLog(ctx context.Context, action, targetType, targetID string, payload string) error
}

type CheckoutRequest struct {
	UserID         uint64 `json:"-"`
	PackCode       string `json:"pack_code"`
	TargetAPIKeyID uint64 `json:"target_api_key_id"`
}

type Service struct {
	store   Store
	gateway Gateway
	mu      sync.RWMutex
	packs   map[string]model.PricingPack
}

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
	result := make([]model.PricingPack, 0, len(s.packs))
	for _, pack := range s.packs {
		result = append(result, pack)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].AmountTotal < result[j].AmountTotal })
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
	s.mu.RLock()
	pack, ok := s.packs[req.PackCode]
	s.mu.RUnlock()
	if !ok || pack.PackKind != string(model.PackKindExternalGeneration) {
		return nil, "", fmt.Errorf("unknown pack_code")
	}
	targetKey, err := s.store.FindAPIKeyByID(ctx, req.TargetAPIKeyID)
	if err != nil {
		return nil, "", err
	}
	switch {
	case targetKey == nil:
		return nil, "", fmt.Errorf("target api key not found")
	case targetKey.OwnerUserID == nil || *targetKey.OwnerUserID != req.UserID:
		return nil, "", fmt.Errorf("target api key does not belong to current user")
	case targetKey.Status != model.APIKeyStatusActive:
		return nil, "", fmt.Errorf("target api key is disabled")
	}
	order := &model.Order{
		UserID:         req.UserID,
		Status:         model.OrderStatusPending,
		Currency:       pack.Currency,
		AmountTotal:    pack.AmountTotal,
		PackCode:       pack.Code,
		PackName:       pack.Name,
		PackKind:       model.PackKind(pack.PackKind),
		QuotaAmount:    pack.QuotaAmount,
		CreditAmount:   pack.CreditAmount,
		TargetAPIKeyID: &req.TargetAPIKeyID,
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
		updates := map[string]any{"status": model.OrderStatusPaid}
		if event.PaymentIntentID != "" {
			updates["stripe_payment_intent_id"] = event.PaymentIntentID
		}
		if event.CustomerID != "" {
			updates["stripe_customer_id"] = event.CustomerID
			_, _ = s.store.GetOrCreateStripeCustomer(ctx, order.UserID, event.CustomerID)
		}
		if err := s.store.UpdateOrder(ctx, order.ID, updates); err != nil {
			return err
		}
		if order.TargetAPIKeyID != nil {
			switch order.PackKind {
			case model.PackKindHostedCredits:
				if _, err := s.store.AddCreditBalanceToAPIKey(ctx, *order.TargetAPIKeyID, order.CreditAmount); err != nil {
					return err
				}
			default:
				if _, err := s.store.AddPaidQuotaToAPIKey(ctx, *order.TargetAPIKeyID, order.QuotaAmount); err != nil {
					return err
				}
			}
		}
		processedAt := time.Now().UTC()
		billEvent.OrderID = &order.ID
		billEvent.ProcessedAt = &processedAt
		if err := s.store.CreateBillingEvent(ctx, billEvent); err != nil {
			return err
		}
		_ = s.store.CreateAuditLog(ctx, "order.paid", "order", fmt.Sprintf("%d", order.ID), event.RawPayload)
		return nil
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
