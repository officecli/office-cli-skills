package httpapi

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestAccessLogMiddlewareLogsCompletedRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	prev := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(prev)

	router := gin.New()
	router.Use(RequestIDMiddleware())
	router.Use(AccessLogMiddleware(200 * time.Millisecond))
	router.GET("/ping", func(c *gin.Context) {
		JSON(c, http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	logs := buf.String()
	if !strings.Contains(logs, "http_request_completed") {
		t.Fatalf("logs = %s", logs)
	}
	if !strings.Contains(logs, "\"path\":\"/ping\"") || !strings.Contains(logs, "\"method\":\"GET\"") {
		t.Fatalf("logs = %s", logs)
	}
	if !strings.Contains(logs, "\"status\":200") || !strings.Contains(logs, "request_id") {
		t.Fatalf("logs = %s", logs)
	}
}

func TestAccessLogMiddlewareWarnsOnSlowRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	prev := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(prev)

	router := gin.New()
	router.Use(RequestIDMiddleware())
	router.Use(AccessLogMiddleware(time.Millisecond))
	router.GET("/slow", func(c *gin.Context) {
		time.Sleep(5 * time.Millisecond)
		JSON(c, http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/slow", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	logs := buf.String()
	if !strings.Contains(logs, "http_request_slow") {
		t.Fatalf("logs = %s", logs)
	}
	if !strings.Contains(logs, "\"path\":\"/slow\"") || !strings.Contains(logs, "\"status\":200") {
		t.Fatalf("logs = %s", logs)
	}
	if !strings.Contains(logs, "latency_ms") || !strings.Contains(logs, "request_id") {
		t.Fatalf("logs = %s", logs)
	}
}

func TestAccessLogMiddlewareSkipsHealthz(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	prev := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(prev)

	router := gin.New()
	router.Use(RequestIDMiddleware())
	router.Use(AccessLogMiddleware(200 * time.Millisecond))
	router.GET("/healthz", func(c *gin.Context) {
		JSON(c, http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if logs := strings.TrimSpace(buf.String()); logs != "" {
		t.Fatalf("logs = %s", logs)
	}
}
