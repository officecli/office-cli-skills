package issuereports

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/officecli/officecli-internal/platform/internal/httpapi"
	"github.com/officecli/officecli-internal/platform/internal/model"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

// TestRateLimit429 wires the rate-limit middleware in front of the Authenticated
// handler and proves a 429 surfaces once the per-key budget is exhausted.
// This is the only piece of the M1a acceptance criteria that the unit-level
// handler_test cannot cover, because the rate limiter is a route-group middleware.
func TestRateLimit429(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store := &mockStore{}
	resolver := &mockCLISession{
		resolveFn: func(_ context.Context, _ string) (*model.CLISession, error) {
			return &model.CLISession{ID: 1, UserID: 42}, nil
		},
	}
	metrics := NewMetrics(nil)
	now := func() time.Time { return time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC) }
	svc := NewService(store, metrics, now)
	h := NewHandler(svc, resolver)

	const burst = 3
	limiter := httpapi.NewRateLimitMiddlewareWithKey(
		rate.Every(time.Minute/60), burst, time.Minute,
		func(c *gin.Context) string { return "test-user" },
	)

	r := gin.New()
	r.POST("/api/issue-reports", limiter, h.Authenticated)

	body := []byte(`{"schema_version":1,"runtimeMode":"hosted","description":"this is a sufficiently long description","timestamp":"2026-05-24T12:00:00Z","via":"http"}`)

	got429 := false
	for i := 0; i < burst+5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/issue-reports", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer test-token")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code == http.StatusTooManyRequests {
			got429 = true
			require.True(t, strings.Contains(w.Body.String(), "rate limit"),
				"429 body should mention rate limit, got: %s", w.Body.String())
			break
		}
	}
	require.True(t, got429, "expected at least one 429 after exhausting burst")
}
