package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/officecli/officecli/platform/internal/admin"
	"github.com/officecli/officecli/platform/internal/httpapi"
	"github.com/officecli/officecli/platform/internal/model"
	sqlstore "github.com/officecli/officecli/platform/internal/store/sqlstore"
)

type fakeAuthRouteFailureService struct{}

func (fakeAuthRouteFailureService) LoginURL(_ context.Context, returnTo, inviteCode string) (string, error) {
	return "", nil
}
func (fakeAuthRouteFailureService) HandleCallback(_ context.Context, code, state string) (*model.User, string, string, error) {
	return nil, "", "", errors.New("oauth failed")
}
func (fakeAuthRouteFailureService) Me(_ context.Context, raw string) (*model.User, error) {
	return nil, errors.New("not implemented")
}
func (fakeAuthRouteFailureService) Logout(_ context.Context, raw string) error { return nil }

type fakeAdminRouteFailureService struct{}

func (fakeAdminRouteFailureService) ResolveSession(cookieValue string) (string, error) {
	return "", nil
}
func (fakeAdminRouteFailureService) LoginURL(_ context.Context, returnTo string) (string, error) {
	return "", errors.New("admin google login url failed")
}
func (fakeAdminRouteFailureService) HandleGoogleCallback(_ context.Context, code, state string) (*admin.AdminIdentity, string, string, error) {
	return nil, "", "", errors.New("admin google callback failed")
}
func (fakeAdminRouteFailureService) Login(_ context.Context, password string) (string, error) {
	return "", errors.New("invalid admin password")
}
func (fakeAdminRouteFailureService) CurrentIdentity(_ context.Context, rawCookie string) (*admin.AdminIdentity, error) {
	return nil, errors.New("bad admin session")
}
func (fakeAdminRouteFailureService) Logout(_ context.Context, rawCookie string) error { return nil }
func (fakeAdminRouteFailureService) Overview(_ context.Context) (*model.OverviewStats, error) {
	return &model.OverviewStats{}, nil
}
func (fakeAdminRouteFailureService) ListAPIKeys(_ context.Context) ([]model.APIKey, error) {
	return nil, nil
}
func (fakeAdminRouteFailureService) CreateAPIKey(_ context.Context, req admin.CreateAPIKeyRequest) (*admin.CreateAPIKeyResponse, *model.APIKey, error) {
	return nil, nil, nil
}
func (fakeAdminRouteFailureService) UpdateAPIKey(_ context.Context, id uint64, req admin.UpdateAPIKeyRequest) error {
	return nil
}
func (fakeAdminRouteFailureService) ListFreeQuotas(_ context.Context, fingerprint string) ([]model.FreeQuota, error) {
	return nil, nil
}
func (fakeAdminRouteFailureService) UpdateFreeQuota(_ context.Context, id uint64, freeLimit int) error {
	return nil
}
func (fakeAdminRouteFailureService) ListUsageEvents(_ context.Context, filter sqlstore.UsageEventFilter) ([]model.UsageEvent, error) {
	return nil, nil
}
func (fakeAdminRouteFailureService) ListUsers(_ context.Context) ([]model.User, error) {
	return nil, nil
}
func (fakeAdminRouteFailureService) UpdateUser(_ context.Context, id uint64, req admin.UpdateUserRequest) error {
	return nil
}
func (fakeAdminRouteFailureService) ListOrders(_ context.Context) ([]model.Order, error) {
	return nil, nil
}
func (fakeAdminRouteFailureService) UpdateOrder(_ context.Context, id uint64, req admin.UpdateOrderRequest) error {
	return nil
}
func (fakeAdminRouteFailureService) ListBillingEvents(_ context.Context) ([]model.BillingEvent, error) {
	return nil, nil
}
func (fakeAdminRouteFailureService) Growth(_ context.Context) (*admin.GrowthSnapshot, error) {
	return &admin.GrowthSnapshot{}, nil
}
func (fakeAdminRouteFailureService) HostedPricingRules(_ context.Context) ([]model.HostedPricingRule, error) {
	return nil, nil
}

func TestRateLimitMiddlewareLogsBlockedRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	prev := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(prev)

	router := gin.New()
	router.Use(httpapi.RequestIDMiddleware())
	router.GET("/limited", httpapi.NewRateLimitMiddleware(1, 1, time.Minute), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	first := httptest.NewRequest(http.MethodGet, "/limited", nil)
	first.RemoteAddr = "127.0.0.1:12345"
	firstRec := httptest.NewRecorder()
	router.ServeHTTP(firstRec, first)

	second := httptest.NewRequest(http.MethodGet, "/limited", nil)
	second.RemoteAddr = "127.0.0.1:12345"
	secondRec := httptest.NewRecorder()
	router.ServeHTTP(secondRec, second)

	if secondRec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, body = %s", secondRec.Code, secondRec.Body.String())
	}
	logs := buf.String()
	if !strings.Contains(logs, "rate_limit_exceeded") || !strings.Contains(logs, "/limited") || !strings.Contains(logs, "request_id") {
		t.Fatalf("logs = %s", logs)
	}
}

func TestRegisterAdminRoutesLogsLoginFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	prev := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(prev)

	router := gin.New()
	router.Use(httpapi.RequestIDMiddleware())
	api := router.Group("/api")
	registerAdminRoutes(api, Config{AppEnv: "production", AdminSessionTTL: time.Hour}, fakeAdminRouteFailureService{})

	req := httptest.NewRequest(http.MethodPost, "/api/admin/login", strings.NewReader(`{"password":"bad"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	logs := buf.String()
	if !strings.Contains(logs, "admin_login_failed") || !strings.Contains(logs, "invalid admin password") || !strings.Contains(logs, "request_id") {
		t.Fatalf("logs = %s", logs)
	}
}

func TestRegisterAuthRoutesLogsCallbackFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	prev := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(prev)

	router := gin.New()
	router.Use(httpapi.RequestIDMiddleware())
	api := router.Group("/api")
	registerAuthRoutes(api, Config{AppEnv: "production", AppSessionTTL: time.Hour}, fakeAuthRouteFailureService{})

	req := httptest.NewRequest(http.MethodGet, "/api/auth/google/callback?code=bad&state=bad", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	logs := buf.String()
	if !strings.Contains(logs, "auth_callback_failed") || !strings.Contains(logs, "oauth failed") || !strings.Contains(logs, "request_id") {
		t.Fatalf("logs = %s", logs)
	}
}

type fakeStripeRouteFailureService struct{}

func (fakeStripeRouteFailureService) HandleWebhook(_ context.Context, payload []byte, signature string) error {
	return errors.New("webhook failed")
}

func TestRegisterStripeRoutesLogsWebhookFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	prev := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(prev)

	router := gin.New()
	router.Use(httpapi.RequestIDMiddleware())
	api := router.Group("/api")
	registerStripeRoutes(api, fakeStripeRouteFailureService{})

	req := httptest.NewRequest(http.MethodPost, "/api/stripe/webhook", strings.NewReader(`{"id":"evt_1"}`))
	req.Header.Set("Stripe-Signature", "bad")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	requestID, ok := body["request_id"].(string)
	if !ok || requestID == "" {
		t.Fatalf("request_id = %#v", body["request_id"])
	}
	logs := buf.String()
	if !strings.Contains(logs, "stripe_webhook_failed") || !strings.Contains(logs, "webhook failed") || !strings.Contains(logs, "request_id") {
		t.Fatalf("logs = %s", logs)
	}
}
