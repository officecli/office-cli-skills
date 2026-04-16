package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/officecli/officecli/platform/internal/admin"
	"github.com/officecli/officecli/platform/internal/auth"
	"github.com/officecli/officecli/platform/internal/model"
	sqlstore "github.com/officecli/officecli/platform/internal/store/sqlstore"
)

type fakeAuthRouteService struct {
	handleUser     *model.User
	handleCookie   string
	handleReturnTo string
	logoutCookie   string
	loginInvite    string
	handleErr      error
	meUser         *model.User
	meErr          error
}

func (f *fakeAuthRouteService) LoginURL(_ context.Context, returnTo, inviteCode string) (string, error) {
	f.loginInvite = inviteCode
	return "https://accounts.google.com/o/oauth2/auth?state=state&return_to=" + returnTo + "&invite=" + inviteCode, nil
}

func (f *fakeAuthRouteService) HandleCallback(_ context.Context, code, state string) (*model.User, string, string, error) {
	return f.handleUser, f.handleCookie, f.handleReturnTo, f.handleErr
}

func (f *fakeAuthRouteService) Me(_ context.Context, raw string) (*model.User, error) {
	if f.meUser != nil || f.meErr != nil {
		return f.meUser, f.meErr
	}
	return &model.User{ID: 1, Email: "demo@example.com", Name: "demo"}, nil
}

func (f *fakeAuthRouteService) Logout(_ context.Context, raw string) error {
	f.logoutCookie = raw
	return nil
}

func (f *fakeAuthRouteService) ResolveSession(cookieValue string) (*auth.SessionPayload, error) {
	return &auth.SessionPayload{SessionID: "session", UserID: 1}, nil
}

type fakeAdminRouteService struct {
	loginCookie  string
	logoutCookie string
	sessionEmail string
	sessionName  string
	loginURL     string
	callbackUser *admin.AdminIdentity
	callbackRaw  string
	callbackTo   string
	callbackErr  error
}

func (f *fakeAdminRouteService) ResolveSession(cookieValue string) (string, error) {
	return "session", nil
}
func (f *fakeAdminRouteService) LoginURL(_ context.Context, returnTo string) (string, error) {
	if f.loginURL != "" {
		return f.loginURL, nil
	}
	return "https://accounts.google.com/o/oauth2/auth?state=admin-state&return_to=" + returnTo, nil
}
func (f *fakeAdminRouteService) HandleGoogleCallback(_ context.Context, code, state string) (*admin.AdminIdentity, string, string, error) {
	return f.callbackUser, f.callbackRaw, f.callbackTo, f.callbackErr
}
func (f *fakeAdminRouteService) Login(_ context.Context, password string) (string, error) {
	return f.loginCookie, nil
}
func (f *fakeAdminRouteService) CurrentIdentity(_ context.Context, rawCookie string) (*admin.AdminIdentity, error) {
	return &admin.AdminIdentity{Email: f.sessionEmail, Name: f.sessionName}, nil
}
func (f *fakeAdminRouteService) Logout(_ context.Context, rawCookie string) error {
	f.logoutCookie = rawCookie
	return nil
}
func (f *fakeAdminRouteService) Overview(_ context.Context) (*model.OverviewStats, error) {
	return &model.OverviewStats{}, nil
}
func (f *fakeAdminRouteService) ListAPIKeys(_ context.Context) ([]model.APIKey, error) {
	return nil, nil
}
func (f *fakeAdminRouteService) CreateAPIKey(_ context.Context, req admin.CreateAPIKeyRequest) (*admin.CreateAPIKeyResponse, *model.APIKey, error) {
	return &admin.CreateAPIKeyResponse{}, &model.APIKey{}, nil
}
func (f *fakeAdminRouteService) UpdateAPIKey(_ context.Context, id uint64, req admin.UpdateAPIKeyRequest) error {
	return nil
}
func (f *fakeAdminRouteService) ListFreeQuotas(_ context.Context, fingerprint string, usageDate string) ([]admin.DailyFreeQuotaView, error) {
	return nil, nil
}
func (f *fakeAdminRouteService) UpdateFreeQuota(_ context.Context, id uint64, freeLimit int) error {
	return nil
}
func (f *fakeAdminRouteService) ListUsageEvents(_ context.Context, filter sqlstore.UsageEventFilter) ([]model.UsageEvent, error) {
	return nil, nil
}
func (f *fakeAdminRouteService) ListUsers(_ context.Context) ([]model.User, error) { return nil, nil }
func (f *fakeAdminRouteService) UpdateUser(_ context.Context, id uint64, req admin.UpdateUserRequest) error {
	return nil
}
func (f *fakeAdminRouteService) ListOrders(_ context.Context) ([]model.Order, error) { return nil, nil }
func (f *fakeAdminRouteService) UpdateOrder(_ context.Context, id uint64, req admin.UpdateOrderRequest) error {
	return nil
}
func (f *fakeAdminRouteService) ListBillingEvents(_ context.Context) ([]model.BillingEvent, error) {
	return nil, nil
}
func (f *fakeAdminRouteService) Growth(_ context.Context) (*admin.GrowthSnapshot, error) {
	return &admin.GrowthSnapshot{}, nil
}
func (f *fakeAdminRouteService) QuotaSources(_ context.Context, filter admin.QuotaSourcesFilter) (*admin.QuotaSources, error) {
	return &admin.QuotaSources{}, nil
}
func (f *fakeAdminRouteService) HostedPricingRules(_ context.Context) ([]model.HostedPricingRule, error) {
	return nil, nil
}

func TestRegisterAuthRoutesCallbackSetsSecureLaxCookieInProduction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api")
	authSvc := &fakeAuthRouteService{
		handleUser:     &model.User{ID: 1, Email: "demo@example.com", Name: "demo"},
		handleCookie:   "encoded-app-cookie",
		handleReturnTo: "/app",
	}
	registerAuthRoutes(api, Config{AppEnv: "production", AppSessionTTL: time.Hour, AppSessionCookieDomain: ".officecli.io"}, authSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/google/callback?code=demo&state=state", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	cookie := rec.Header().Get("Set-Cookie")
	if !strings.Contains(cookie, "cop_app_session=encoded-app-cookie") {
		t.Fatalf("Set-Cookie = %q", cookie)
	}
	if !strings.Contains(cookie, "HttpOnly") || !strings.Contains(cookie, "Secure") || !strings.Contains(cookie, "SameSite=Lax") {
		t.Fatalf("Set-Cookie = %q", cookie)
	}
	if !strings.Contains(cookie, "Domain=officecli.io") {
		t.Fatalf("Set-Cookie = %q", cookie)
	}
}

func TestRegisterAuthRoutesLogoutClearsCookieWithSameSiteLax(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api")
	authSvc := &fakeAuthRouteService{}
	registerAuthRoutes(api, Config{AppEnv: "production", AppSessionTTL: time.Hour, AppSessionCookieDomain: ".officecli.io"}, authSvc)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: "cop_app_session", Value: "existing"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	cookie := rec.Header().Get("Set-Cookie")
	if authSvc.logoutCookie != "existing" {
		t.Fatalf("logout cookie = %q", authSvc.logoutCookie)
	}
	if !strings.Contains(cookie, "cop_app_session=") || !strings.Contains(cookie, "Max-Age=0") || !strings.Contains(cookie, "SameSite=Lax") {
		t.Fatalf("Set-Cookie = %q", cookie)
	}
	if !strings.Contains(cookie, "Domain=officecli.io") {
		t.Fatalf("Set-Cookie = %q", cookie)
	}
}

func TestRegisterAuthRoutesLoginPassesInviteCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api")
	authSvc := &fakeAuthRouteService{}
	registerAuthRoutes(api, Config{AppEnv: "production", AppSessionTTL: time.Hour}, authSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/google/login?return_to=%2Fapp&invite=invite-abc", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d", rec.Code)
	}
	if authSvc.loginInvite != "invite-abc" {
		t.Fatalf("loginInvite = %q", authSvc.loginInvite)
	}
	if location := rec.Header().Get("Location"); !strings.Contains(location, "invite=invite-abc") {
		t.Fatalf("location = %q", location)
	}
}

func TestRegisterAdminRoutesLoginSetsSecureLaxCookieInProduction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api")
	adminSvc := &fakeAdminRouteService{loginCookie: "encoded-admin-cookie"}
	registerAdminRoutes(api, Config{AppEnv: "production", AdminSessionTTL: time.Hour}, adminSvc)

	body, _ := json.Marshal(admin.LoginRequest{Password: "secret"})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/login", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	cookie := rec.Header().Get("Set-Cookie")
	if !strings.Contains(cookie, "cop_admin_session=encoded-admin-cookie") {
		t.Fatalf("Set-Cookie = %q", cookie)
	}
	if !strings.Contains(cookie, "HttpOnly") || !strings.Contains(cookie, "Secure") || !strings.Contains(cookie, "SameSite=Lax") {
		t.Fatalf("Set-Cookie = %q", cookie)
	}
}

func TestRegisterAdminRoutesLogoutClearsCookieWithSameSiteLax(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api")
	adminSvc := &fakeAdminRouteService{}
	registerAdminRoutes(api, Config{AppEnv: "production", AdminSessionTTL: time.Hour}, adminSvc)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/logout", nil)
	req.AddCookie(&http.Cookie{Name: "cop_admin_session", Value: "existing-admin"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	cookie := rec.Header().Get("Set-Cookie")
	if adminSvc.logoutCookie != "existing-admin" {
		t.Fatalf("logout cookie = %q", adminSvc.logoutCookie)
	}
	if !strings.Contains(cookie, "cop_admin_session=") || !strings.Contains(cookie, "Max-Age=0") || !strings.Contains(cookie, "SameSite=Lax") {
		t.Fatalf("Set-Cookie = %q", cookie)
	}
}

func TestRegisterAdminRoutesGoogleCallbackSetsSecureCookieAndRedirects(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api")
	adminSvc := &fakeAdminRouteService{
		callbackUser: &admin.AdminIdentity{Email: "ops@example.com", Name: "Ops Lead"},
		callbackRaw:  "encoded-google-admin-cookie",
		callbackTo:   "/admin",
	}
	registerAdminRoutes(api, Config{AppEnv: "production", AdminSessionTTL: time.Hour}, adminSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/auth/google/callback?code=demo&state=admin-state", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d", rec.Code)
	}
	if location := rec.Header().Get("Location"); location != "/admin" {
		t.Fatalf("location = %q", location)
	}
	cookie := rec.Header().Get("Set-Cookie")
	if !strings.Contains(cookie, "cop_admin_session=encoded-google-admin-cookie") {
		t.Fatalf("Set-Cookie = %q", cookie)
	}
	if !strings.Contains(cookie, "HttpOnly") || !strings.Contains(cookie, "Secure") || !strings.Contains(cookie, "SameSite=Lax") {
		t.Fatalf("Set-Cookie = %q", cookie)
	}
}

func TestRegisterAuthRoutesGoogleCallbackRedirectsDeniedUsersWithoutCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api")
	authSvc := &fakeAuthRouteService{
		handleErr: &auth.AccessDeniedError{Email: "blocked@example.com", Reason: "email_not_allowlisted"},
	}
	registerAuthRoutes(api, Config{AppEnv: "production", AppSessionTTL: time.Hour}, authSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/google/callback?code=demo&state=state", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d", rec.Code)
	}
	if location := rec.Header().Get("Location"); location != "/app/access-denied?email=blocked%40example.com" {
		t.Fatalf("location = %q", location)
	}
	if cookie := rec.Header().Get("Set-Cookie"); cookie != "" {
		t.Fatalf("Set-Cookie = %q", cookie)
	}
}

func TestRegisterAdminRoutesGoogleCallbackRedirectsDeniedUsersWithoutCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api")
	adminSvc := &fakeAdminRouteService{
		callbackErr: &admin.AccessDeniedError{Email: "blocked@example.com"},
	}
	registerAdminRoutes(api, Config{AppEnv: "production", AdminSessionTTL: time.Hour}, adminSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/auth/google/callback?code=demo&state=admin-state", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d", rec.Code)
	}
	if location := rec.Header().Get("Location"); location != "/admin/access-denied?email=blocked%40example.com" {
		t.Fatalf("location = %q", location)
	}
	if cookie := rec.Header().Get("Set-Cookie"); cookie != "" {
		t.Fatalf("unexpected Set-Cookie = %q", cookie)
	}
}

func TestRegisterAdminRoutesSessionReturnsCurrentIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api")
	adminSvc := &fakeAdminRouteService{sessionEmail: "ops@example.com", sessionName: "Ops Lead"}
	registerAdminRoutes(api, Config{AppEnv: "production", AdminSessionTTL: time.Hour}, adminSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/session", nil)
	req.AddCookie(&http.Cookie{Name: "cop_admin_session", Value: "session-cookie"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, "ops@example.com") || !strings.Contains(body, "Ops Lead") {
		t.Fatalf("body = %s", body)
	}
}

var _ authRouteService = (*fakeAuthRouteService)(nil)
var _ adminRouteService = (*fakeAdminRouteService)(nil)
