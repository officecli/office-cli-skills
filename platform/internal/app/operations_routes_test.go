package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/officecli/officecli/platform/internal/model"
	"github.com/officecli/officecli/platform/internal/operations"
)

type operationalEventRouteStore struct {
	events []model.OperationalEvent
}

func (s *operationalEventRouteStore) CreateOperationalEvent(_ context.Context, event *model.OperationalEvent) error {
	s.events = append(s.events, *event)
	return nil
}

func TestRegisterOperationsRoutesTracksAnonymousEventWithVisitorCookieAndAppUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &operationalEventRouteStore{}
	router := gin.New()
	api := router.Group("/api")
	registerOperationsRoutes(api, Config{AppEnv: "production"}, &fakeAuthRouteService{}, operations.NewService(store))

	body, err := json.Marshal(map[string]any{
		"event_name": "pricing_view",
		"surface":    "site",
		"visitor_id": "visitor_123",
		"page_path":  "/pricing",
		"metadata":   map[string]any{"placement": "hero"},
	})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/events/track", bytes.NewReader(body))
	req.Host = "officecli.io"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "route-test")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.AddCookie(&http.Cookie{Name: "cop_app_session", Value: "existing-app"})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Len(t, store.events, 1)
	require.Equal(t, "pricing_view", store.events[0].EventName)
	require.NotNil(t, store.events[0].UserID)
	require.EqualValues(t, 1, *store.events[0].UserID)
	require.NotNil(t, store.events[0].VisitorID)
	require.Equal(t, "visitor_123", *store.events[0].VisitorID)
	setCookie := rec.Header().Values("Set-Cookie")
	require.NotEmpty(t, setCookie)
	require.Contains(t, strings.Join(setCookie, ";"), "ocli_visitor_id=visitor_123")
	require.Contains(t, strings.Join(setCookie, ";"), "Domain=officecli.io")
	require.Contains(t, strings.Join(setCookie, ";"), "Secure")
}

func TestRegisterOperationsRoutesRejectsUnsafePayloads(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name   string
		body   string
		status int
	}{
		{name: "unknown field", body: `{"event_name":"pricing_view","surface":"site","user_id":1}`, status: http.StatusBadRequest},
		{name: "invalid event", body: `{"event_name":"not_allowed","surface":"site"}`, status: http.StatusBadRequest},
		{name: "invalid surface", body: `{"event_name":"pricing_view","surface":"admin"}`, status: http.StatusBadRequest},
		{name: "metadata array", body: `{"event_name":"pricing_view","surface":"site","metadata":[]}`, status: http.StatusBadRequest},
		{name: "metadata too deep", body: `{"event_name":"pricing_view","surface":"site","metadata":{"a":{"b":{"c":{"d":{"e":1}}}}}}`, status: http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			api := router.Group("/api")
			registerOperationsRoutes(api, Config{}, &fakeAuthRouteService{}, operations.NewService(&operationalEventRouteStore{}))
			req := httptest.NewRequest(http.MethodPost, "/api/events/track", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			require.Equal(t, tc.status, rec.Code, rec.Body.String())
		})
	}
}

func TestRegisterOperationsRoutesRejectsOversizedBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api")
	registerOperationsRoutes(api, Config{}, &fakeAuthRouteService{}, operations.NewService(&operationalEventRouteStore{}))

	req := httptest.NewRequest(http.MethodPost, "/api/events/track", strings.NewReader(strings.Repeat("x", 17<<10)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code, rec.Body.String())
}

func TestAdminOperationsFunnelRequiresAdminAndValidRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api")
	adminSvc := &fakeAdminRouteService{}
	registerAdminRoutes(api, Config{AppEnv: "production", AdminSessionTTL: time.Hour}, adminSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/operations/funnel?range=30d", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	req = httptest.NewRequest(http.MethodGet, "/api/admin/operations/funnel?range=90d", nil)
	req.AddCookie(&http.Cookie{Name: "cop_admin_session", Value: "existing-admin"})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)

	req = httptest.NewRequest(http.MethodGet, "/api/admin/operations/funnel?range=24h", nil)
	req.AddCookie(&http.Cookie{Name: "cop_admin_session", Value: "existing-admin"})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), `"window_start"`)
}
