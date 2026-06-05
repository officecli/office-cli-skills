package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/officecli/officecli/platform/internal/admin"
	"github.com/officecli/officecli/platform/internal/appuser"
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

func (f *fakeAuthRouteService) GitHubEnabled() bool {
	return false
}

func (f *fakeAuthRouteService) GitHubLoginURL(_ context.Context, returnTo, inviteCode string) (string, error) {
	return "", nil
}

func (f *fakeAuthRouteService) HandleGitHubCallback(_ context.Context, code, state string) (*model.User, string, string, error) {
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
	loginCookie           string
	logoutCookie          string
	sessionEmail          string
	sessionName           string
	preferences           map[string]string
	loginURL              string
	mockEmail             string
	mockName              string
	mockCookie            string
	callbackUser          *admin.AdminIdentity
	callbackRaw           string
	callbackTo            string
	callbackErr           error
	plaintext             string
	plaintextErr          error
	overview              *model.OverviewStats
	fingerprintQuality    *model.FingerprintQuality
	deletedRedemptionID   uint64
	deleteRedemptionActor string
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
func (f *fakeAdminRouteService) MockGoogleLogin(_ context.Context, email, name string) (*admin.AdminIdentity, string, error) {
	f.mockEmail = email
	f.mockName = name
	if f.mockCookie != "" {
		return &admin.AdminIdentity{Email: email, Name: name, AuthMethod: "google_mock"}, f.mockCookie, nil
	}
	return &admin.AdminIdentity{Email: email, Name: name, AuthMethod: "google_mock"}, "encoded-mock-admin-cookie", nil
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
	if f.overview != nil {
		return f.overview, nil
	}
	return &model.OverviewStats{}, nil
}
func (f *fakeAdminRouteService) FingerprintQuality(_ context.Context) (*model.FingerprintQuality, error) {
	if f.fingerprintQuality != nil {
		return f.fingerprintQuality, nil
	}
	return &model.FingerprintQuality{}, nil
}
func (f *fakeAdminRouteService) OperationsFunnel(_ context.Context, windowStart, now time.Time) (*model.OperationsFunnel, error) {
	return &model.OperationsFunnel{WindowStart: windowStart, WindowEnd: now}, nil
}
func (f *fakeAdminRouteService) ListAPIKeys(_ context.Context, _ *uint64) ([]model.APIKey, error) {
	return nil, nil
}
func (f *fakeAdminRouteService) GetAPIKeyPlaintext(_ context.Context, _ uint64, _ string) (string, error) {
	return f.plaintext, f.plaintextErr
}
func (f *fakeAdminRouteService) CreateAPIKey(_ context.Context, req admin.CreateAPIKeyRequest) (*admin.CreateAPIKeyResponse, *model.APIKey, error) {
	return &admin.CreateAPIKeyResponse{}, &model.APIKey{}, nil
}
func (f *fakeAdminRouteService) UpdateAPIKey(_ context.Context, id uint64, req admin.UpdateAPIKeyRequest) error {
	return nil
}
func (f *fakeAdminRouteService) ListUsageEvents(_ context.Context, filter sqlstore.UsageEventFilter) ([]model.UsageEvent, error) {
	return nil, nil
}
func (f *fakeAdminRouteService) GetPreference(_ context.Context, adminEmail, pageKey string) (*model.AdminUserPreference, error) {
	if f.preferences == nil {
		return nil, nil
	}
	if payload, ok := f.preferences[adminEmail+"|"+pageKey]; ok {
		return &model.AdminUserPreference{AdminEmail: adminEmail, PageKey: pageKey, PreferencesJSON: payload}, nil
	}
	return nil, nil
}
func (f *fakeAdminRouteService) SavePreference(_ context.Context, adminEmail, pageKey, preferencesJSON string) (*model.AdminUserPreference, error) {
	if f.preferences == nil {
		f.preferences = map[string]string{}
	}
	f.preferences[adminEmail+"|"+pageKey] = preferencesJSON
	return &model.AdminUserPreference{AdminEmail: adminEmail, PageKey: pageKey, PreferencesJSON: preferencesJSON}, nil
}
func (f *fakeAdminRouteService) ListUsers(_ context.Context, _ string) ([]model.User, error) {
	return nil, nil
}
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
func (f *fakeAdminRouteService) ListCreditLedger(_ context.Context, _ bool) ([]model.UserHostedCreditLedger, error) {
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
func (f *fakeAdminRouteService) HostedBillingConfig(_ context.Context) (*admin.HostedBillingConfig, error) {
	return &admin.HostedBillingConfig{}, nil
}
func (f *fakeAdminRouteService) UpdateHostedPricingSettings(_ context.Context, req admin.UpdateHostedPricingSettingsRequest) (*model.HostedPricingSetting, error) {
	return &model.HostedPricingSetting{}, nil
}
func (f *fakeAdminRouteService) CreateHostedModelPricingConfig(_ context.Context, req admin.UpsertHostedModelPricingConfigRequest) (*model.HostedModelPricingConfig, error) {
	return &model.HostedModelPricingConfig{}, nil
}
func (f *fakeAdminRouteService) UpdateHostedModelPricingConfig(_ context.Context, id uint64, req admin.UpsertHostedModelPricingConfigRequest) (*model.HostedModelPricingConfig, error) {
	return &model.HostedModelPricingConfig{}, nil
}
func (f *fakeAdminRouteService) CreateHostedPricingRule(_ context.Context, req admin.UpsertHostedPricingRuleRequest) (*model.HostedPricingRule, error) {
	return &model.HostedPricingRule{}, nil
}
func (f *fakeAdminRouteService) UpdateHostedPricingRule(_ context.Context, id uint64, req admin.UpsertHostedPricingRuleRequest) (*model.HostedPricingRule, error) {
	return &model.HostedPricingRule{}, nil
}
func (f *fakeAdminRouteService) CreateHostedCreditPack(_ context.Context, req admin.UpsertHostedCreditPackRequest) (*model.HostedCreditPack, error) {
	return &model.HostedCreditPack{}, nil
}
func (f *fakeAdminRouteService) UpdateHostedCreditPack(_ context.Context, id uint64, req admin.UpsertHostedCreditPackRequest) (*model.HostedCreditPack, error) {
	return &model.HostedCreditPack{}, nil
}
func (f *fakeAdminRouteService) CreateRedemptionCode(_ context.Context, _ string, _ admin.CreateRedemptionCodeRequest) (*model.RedemptionCode, error) {
	return &model.RedemptionCode{}, nil
}
func (f *fakeAdminRouteService) ListRedemptionCodes(_ context.Context, _ admin.ListRedemptionCodesRequest) (*admin.ListRedemptionCodesResponse, error) {
	return &admin.ListRedemptionCodesResponse{Items: []model.RedemptionCode{}}, nil
}
func (f *fakeAdminRouteService) GetRedemptionCode(_ context.Context, _ uint64) (*model.RedemptionCode, error) {
	return &model.RedemptionCode{}, nil
}
func (f *fakeAdminRouteService) UpdateRedemptionCode(_ context.Context, _ string, _ uint64, _ admin.UpdateRedemptionCodeRequest) (*model.RedemptionCode, error) {
	return &model.RedemptionCode{}, nil
}
func (f *fakeAdminRouteService) EnableRedemptionCode(_ context.Context, _ string, _ uint64) (*model.RedemptionCode, error) {
	return &model.RedemptionCode{}, nil
}
func (f *fakeAdminRouteService) DisableRedemptionCode(_ context.Context, _ string, _ uint64) (*model.RedemptionCode, error) {
	return &model.RedemptionCode{}, nil
}
func (f *fakeAdminRouteService) DeleteRedemptionCode(_ context.Context, actorEmail string, id uint64) error {
	f.deleteRedemptionActor = actorEmail
	f.deletedRedemptionID = id
	return nil
}
func (f *fakeAdminRouteService) ListRedemptionRecords(_ context.Context, _ admin.ListRedemptionRecordsRequest) (*admin.ListRedemptionRecordsResponse, error) {
	return &admin.ListRedemptionRecordsResponse{Items: []model.RedemptionCodeRedemption{}}, nil
}

func TestAdminOverviewRouteReturnsChartData(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api")
	adminSvc := &fakeAdminRouteService{
		overview: &model.OverviewStats{
			UsageTrend: []model.OverviewUsageTrendPoint{{
				Date:     "2026-05-18",
				Checks:   3,
				Consumes: 2,
				Blocked:  1,
				Allowed:  4,
				Total:    5,
			}},
			ResultBreakdown: []model.OverviewBreakdownItem{{Key: "allowed", Label: "Allowed", Value: 4}},
			ModeBreakdown:   []model.OverviewBreakdownItem{{Key: "hosted", Label: "Hosted", Value: 2}},
			APIKeyStatusBreakdown: []model.OverviewBreakdownItem{{
				Key:   "active",
				Label: "Active",
				Value: 7,
			}},
		},
	}
	registerAdminRoutes(api, Config{AppEnv: "production", AdminSessionTTL: time.Hour}, adminSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/overview", nil)
	req.AddCookie(&http.Cookie{Name: "cop_admin_session", Value: "existing-admin"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Data model.OverviewStats `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data.UsageTrend, 1)
	require.Equal(t, "2026-05-18", body.Data.UsageTrend[0].Date)
	require.EqualValues(t, 5, body.Data.UsageTrend[0].Total)
	require.EqualValues(t, 4, body.Data.ResultBreakdown[0].Value)
	require.EqualValues(t, 2, body.Data.ModeBreakdown[0].Value)
	require.EqualValues(t, 7, body.Data.APIKeyStatusBreakdown[0].Value)
}

func TestAdminFingerprintQualityRouteRequiresAdminAndReturnsEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api")
	adminSvc := &fakeAdminRouteService{
		fingerprintQuality: &model.FingerprintQuality{
			Summary: []model.FingerprintQualityBucketSummary{{
				Bucket:          "candidate_real_or_unknown",
				Reason:          "no machine/test signals matched",
				Fingerprints:    1,
				Events:          2,
				DefaultFiltered: false,
			}},
			Rows: []model.FingerprintQualityRow{{
				Bucket:            "candidate_real_or_unknown",
				Reason:            "no machine/test signals matched",
				FingerprintHash:   strings.Repeat("a", 64),
				FingerprintPrefix: strings.Repeat("a", 12),
				Events:            2,
			}},
		},
	}
	registerAdminRoutes(api, Config{AppEnv: "production", AdminSessionTTL: time.Hour}, adminSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/fingerprint-quality", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	req = httptest.NewRequest(http.MethodGet, "/api/admin/fingerprint-quality", nil)
	req.AddCookie(&http.Cookie{Name: "cop_admin_session", Value: "existing-admin"})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var body struct {
		Data model.FingerprintQuality `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data.Summary, 1)
	require.Len(t, body.Data.Rows, 1)
	require.Equal(t, "candidate_real_or_unknown", body.Data.Rows[0].Bucket)
}

func TestRegisterAdminRoutesDeletesRedemptionCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api")
	adminSvc := &fakeAdminRouteService{sessionEmail: "admin@example.com"}
	registerAdminRoutes(api, Config{AppEnv: "production", AdminSessionTTL: time.Hour}, adminSvc)

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/redemption-codes/42", nil)
	req.AddCookie(&http.Cookie{Name: "cop_admin_session", Value: "existing-admin"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.EqualValues(t, 42, adminSvc.deletedRedemptionID)
	require.Equal(t, "admin@example.com", adminSvc.deleteRedemptionActor)
	require.JSONEq(t, `{"data":{"success":true}}`, rec.Body.String())
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

func TestRegisterAuthRoutesCallbackGrantsSignupCreditsBeforeSettingCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api")
	authSvc := &fakeAuthRouteService{
		handleUser:     &model.User{ID: 42, Email: "dev@example.com", Status: model.UserStatusActive},
		handleCookie:   "encoded-app-cookie",
		handleReturnTo: "/app",
	}
	store := &overviewRouteStore{user: &model.User{ID: 42, InviteCode: "invite-42"}}
	appSvc := appuser.NewService(store, nil, "", nil)
	registerAuthRoutes(api, Config{AppEnv: "production", AppSessionTTL: time.Hour}, authSvc, appSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/oauth2/callback?code=abc&state=state", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusFound, rec.Code)
	require.Contains(t, rec.Header().Get("Set-Cookie"), "cop_app_session=encoded-app-cookie")
	require.Equal(t, appuser.SignupHostedCreditBonus, store.hostedAccount.CreditBalance)
	require.NotNil(t, store.hostedLedgers["signup-hosted-credits:42"])
}

func TestRegisterAuthRoutesCallbackRejectsWhenSignupCreditsFail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api")
	authSvc := &fakeAuthRouteService{
		handleUser:     &model.User{ID: 42, Email: "dev@example.com", Status: model.UserStatusActive},
		handleCookie:   "encoded-app-cookie",
		handleReturnTo: "/app",
	}
	store := &overviewRouteStore{grantUserErr: errors.New("credit store unavailable")}
	appSvc := appuser.NewService(store, nil, "", nil)
	registerAuthRoutes(api, Config{AppEnv: "production", AppSessionTTL: time.Hour}, authSvc, appSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/oauth2/callback?code=abc&state=state", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.NotContains(t, rec.Header().Get("Set-Cookie"), "cop_app_session=")
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

func TestRegisterAuthRoutesMeRejectsDisabledSessionUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api")
	authSvc := auth.NewService(
		nil,
		routeAuthUserStore{user: &model.User{ID: 42, Status: model.UserStatusDisabled}},
		overviewSessionStore{payload: auth.SessionPayload{SessionID: "session-1", UserID: 42}},
		"cop_app_session",
		time.Hour,
		overviewCookieCodec{},
		nil,
		nil,
	)
	registerAuthRoutes(api, Config{AppEnv: "production", AppSessionTTL: time.Hour, AppSessionCookieDomain: ".officecli.io"}, authSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: "cop_app_session", Value: "cookie:session-1"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestRegisterAdminRoutesAPIKeyPlaintextReturnsStoredKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	api := router.Group("/api")
	adminSvc := &fakeAdminRouteService{
		sessionEmail: "admin@example.com",
		plaintext:    "cop_admin_secret_123",
	}
	registerAdminRoutes(api, Config{AppEnv: "production", AdminSessionTTL: time.Hour}, adminSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/api-keys/7/plaintext", nil)
	req.AddCookie(&http.Cookie{Name: "cop_admin_session", Value: "admin-cookie"})
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var body struct {
		Data admin.APIKeyPlaintextResponse `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if body.Data.PlaintextKey != "cop_admin_secret_123" {
		t.Fatalf("plaintext = %q", body.Data.PlaintextKey)
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

func TestRegisterAdminRoutesMockGoogleLoginSetsCookieAndRedirectsInDevelopment(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api")
	adminSvc := &fakeAdminRouteService{}
	registerAdminRoutes(api, Config{
		AppEnv:                 "development",
		AdminSessionTTL:        time.Hour,
		AdminGoogleMockEnabled: true,
		AdminGoogleMockEmail:   "mock-admin@example.com",
		AdminGoogleMockName:    "Mock Admin",
	}, adminSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/auth/google/login?return_to=%2Fadmin%2Fusers", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if location := rec.Header().Get("Location"); location != "/admin/users" {
		t.Fatalf("location = %q", location)
	}
	if adminSvc.mockEmail != "mock-admin@example.com" || adminSvc.mockName != "Mock Admin" {
		t.Fatalf("mock identity = %q %q", adminSvc.mockEmail, adminSvc.mockName)
	}
	cookie := rec.Header().Get("Set-Cookie")
	if !strings.Contains(cookie, "cop_admin_session=encoded-mock-admin-cookie") {
		t.Fatalf("Set-Cookie = %q", cookie)
	}
	if !strings.Contains(cookie, "HttpOnly") || !strings.Contains(cookie, "SameSite=Lax") {
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

func TestRegisterAdminRoutesPreferencesPersistByAdminEmail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api")
	adminSvc := &fakeAdminRouteService{sessionEmail: "ops@example.com", sessionName: "Ops Lead"}
	registerAdminRoutes(api, Config{AppEnv: "production", AdminSessionTTL: time.Hour}, adminSvc)

	body := strings.NewReader(`{"version":1,"columns":[{"key":"created_at","visible":true,"fixed":"left"}]}`)
	putReq := httptest.NewRequest(http.MethodPut, "/api/admin/preferences/usage-events-table", body)
	putReq.Header.Set("Content-Type", "application/json")
	putReq.AddCookie(&http.Cookie{Name: "cop_admin_session", Value: "existing-admin"})
	putRec := httptest.NewRecorder()
	router.ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d body = %s", putRec.Code, putRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/admin/preferences/usage-events-table", nil)
	getReq.AddCookie(&http.Cookie{Name: "cop_admin_session", Value: "existing-admin"})
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET status = %d body = %s", getRec.Code, getRec.Body.String())
	}
	var payload struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Data["version"].(float64) != 1 {
		t.Fatalf("preference payload = %#v", payload.Data)
	}
}

func TestRegisterAdminRoutesPreferencesRejectMissingAdminEmail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api")
	adminSvc := &fakeAdminRouteService{}
	registerAdminRoutes(api, Config{AppEnv: "production", AdminSessionTTL: time.Hour}, adminSvc)

	req := httptest.NewRequest(http.MethodPut, "/api/admin/preferences/usage-events-table", strings.NewReader(`{"version":1}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "cop_admin_session", Value: "existing-admin"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

var _ authRouteService = (*fakeAuthRouteService)(nil)
var _ adminRouteService = (*fakeAdminRouteService)(nil)
