package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSiteCORSMiddleware(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	setup := func(cfg Config) *gin.Engine {
		r := gin.New()
		r.Use(siteCORSMiddleware(cfg))
		r.GET("/api/auth/me", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
		r.POST("/api/app/checkout", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
		r.OPTIONS("/api/app/checkout", func(c *gin.Context) { c.String(http.StatusOK, "should not reach") })
		r.GET("/api/admin/foo", func(c *gin.Context) { c.String(http.StatusOK, "ok") })
		return r
	}

	t.Run("preflight from officecli.io returns 204 with CORS headers", func(t *testing.T) {
		t.Parallel()
		r := setup(Config{AppEnv: "production"})
		req := httptest.NewRequest(http.MethodOptions, "/api/app/checkout", nil)
		req.Header.Set("Origin", "https://officecli.io")
		req.Header.Set("Access-Control-Request-Method", "POST")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusNoContent, w.Code)
		require.Equal(t, "https://officecli.io", w.Header().Get("Access-Control-Allow-Origin"))
		require.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
		require.Equal(t, "Origin", w.Header().Get("Vary"))
		require.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), "Content-Type")
	})

	t.Run("GET from officecli.io passes through with CORS headers", func(t *testing.T) {
		t.Parallel()
		r := setup(Config{AppEnv: "production"})
		req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
		req.Header.Set("Origin", "https://officecli.io")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, "https://officecli.io", w.Header().Get("Access-Control-Allow-Origin"))
		require.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
	})

	t.Run("unknown origin gets no CORS headers", func(t *testing.T) {
		t.Parallel()
		r := setup(Config{AppEnv: "production"})
		req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
		req.Header.Set("Origin", "https://evil.com")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		require.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("localhost allowed in development", func(t *testing.T) {
		t.Parallel()
		r := setup(Config{AppEnv: "development"})
		req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
		req.Header.Set("Origin", "http://localhost:5173")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, "http://localhost:5173", w.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("localhost blocked in production", func(t *testing.T) {
		t.Parallel()
		r := setup(Config{AppEnv: "production"})
		req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
		req.Header.Set("Origin", "http://localhost:5173")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		require.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("non-allowlisted path is untouched even from allowed origin", func(t *testing.T) {
		t.Parallel()
		r := setup(Config{AppEnv: "production"})
		req := httptest.NewRequest(http.MethodGet, "/api/admin/foo", nil)
		req.Header.Set("Origin", "https://officecli.io")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		require.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
		require.Empty(t, w.Header().Get("Access-Control-Allow-Credentials"))
	})

	t.Run("non-allowlisted path OPTIONS is not short-circuited", func(t *testing.T) {
		t.Parallel()
		r := gin.New()
		r.Use(siteCORSMiddleware(Config{AppEnv: "production"}))
		r.OPTIONS("/api/admin/foo", func(c *gin.Context) { c.String(http.StatusOK, "reached") })
		req := httptest.NewRequest(http.MethodOptions, "/api/admin/foo", nil)
		req.Header.Set("Origin", "https://officecli.io")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		require.Equal(t, "reached", w.Body.String())
	})

	t.Run("SITE_CORS_ALLOWED_ORIGINS overrides defaults", func(t *testing.T) {
		t.Parallel()
		r := setup(Config{
			AppEnv:                 "production",
			SiteCORSAllowedOrigins: []string{"https://custom.example"},
		})

		req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
		req.Header.Set("Origin", "https://custom.example")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, "https://custom.example", w.Header().Get("Access-Control-Allow-Origin"))

		req2 := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
		req2.Header.Set("Origin", "https://officecli.io")
		w2 := httptest.NewRecorder()
		r.ServeHTTP(w2, req2)
		require.Empty(t, w2.Header().Get("Access-Control-Allow-Origin"))
	})
}
