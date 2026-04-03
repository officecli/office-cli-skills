package httpapi

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

func LogInfo(event string, args ...any) {
	slog.Info(event, args...)
}

func LogWarn(event string, args ...any) {
	slog.Warn(event, args...)
}

func LogError(event string, args ...any) {
	slog.Error(event, args...)
}

func LogWarnRequest(c *gin.Context, event string, args ...any) {
	args = append([]any{"request_id", RequestID(c)}, args...)
	slog.Warn(event, args...)
}

func LogInfoRequest(c *gin.Context, event string, args ...any) {
	args = append([]any{"request_id", RequestID(c)}, args...)
	slog.Info(event, args...)
}

func AccessLogMiddleware(slowThreshold time.Duration) gin.HandlerFunc {
	if slowThreshold <= 0 {
		slowThreshold = time.Second
	}
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		if c.Request != nil && c.Request.URL != nil && c.Request.URL.Path == "/healthz" {
			return
		}

		latency := time.Since(start)
		args := []any{
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"client_ip", c.ClientIP(),
			"latency_ms", latency.Milliseconds(),
		}
		if latency >= slowThreshold {
			LogWarnRequest(c, "http_request_slow", args...)
			return
		}
		LogInfoRequest(c, "http_request_completed", args...)
	}
}
