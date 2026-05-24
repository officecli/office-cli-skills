package app

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// siteCORSAllowedPaths is the strict allowlist of request paths that may
// receive cross-origin headers. Mounting the middleware on the wider /api
// group keeps registration simple, but only these two endpoints actually
// participate in cross-origin flows from officecli.io.
var siteCORSAllowedPaths = map[string]struct{}{
	"/api/auth/me":      {},
	"/api/app/checkout": {},
}

func siteCORSMiddleware(cfg Config) gin.HandlerFunc {
	allowed := siteCORSAllowedOrigins(cfg)

	return func(c *gin.Context) {
		if _, ok := siteCORSAllowedPaths[c.Request.URL.Path]; !ok {
			c.Next()
			return
		}

		origin := c.GetHeader("Origin")
		if _, ok := allowed[origin]; !ok {
			c.Next()
			return
		}

		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, X-Request-Id")
		c.Header("Vary", "Origin")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// siteCORSAllowedOrigins resolves the cross-origin allowlist. SITE_CORS_ALLOWED_ORIGINS
// (comma-separated) takes precedence; when unset, fall back to the canonical
// production origins plus localhost in non-production.
func siteCORSAllowedOrigins(cfg Config) map[string]struct{} {
	out := make(map[string]struct{})
	if len(cfg.SiteCORSAllowedOrigins) > 0 {
		for _, origin := range cfg.SiteCORSAllowedOrigins {
			out[origin] = struct{}{}
		}
		return out
	}
	out["https://officecli.io"] = struct{}{}
	out["https://www.officecli.io"] = struct{}{}
	if cfg.AppEnv != "production" {
		out["http://localhost:5173"] = struct{}{}
		out["http://localhost:4173"] = struct{}{}
	}
	return out
}
