package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gotomicro/ego/server/egin"
)

func TestRegisterRoutesHealthzIncludesRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := egin.DefaultContainer().Build()
	registerRoutes(router, Config{}, nil, nil, nil, nil, nil, fakeDiscordOAuthRouteService{})

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("data = %#v", body["data"])
	}
	if data["status"] != "ok" {
		t.Fatalf("data.status = %#v", data["status"])
	}
	requestID, ok := body["request_id"].(string)
	if !ok || requestID == "" {
		t.Fatalf("request_id = %#v", body["request_id"])
	}
}
