package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	licensesvc "github.com/officecli/officecli/platform/internal/license"
	"github.com/officecli/officecli/platform/internal/model"
)

func TestRegisterAdminRoutesRateLimitsLogin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api")
	adminSvc := &fakeAdminRouteService{loginCookie: "encoded-admin-cookie"}
	registerAdminRoutes(api, Config{AppEnv: "production", AdminSessionTTL: time.Hour}, adminSvc)

	body, _ := json.Marshal(map[string]string{"password": "secret"})
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/admin/login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code == http.StatusTooManyRequests {
			t.Fatalf("request %d unexpectedly rate limited", i+1)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/api/admin/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestRegisterLicenseRoutesRateLimitsCheck(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api")
	lic := licensesvc.NewService(testAPIKeyStore{}, newTestFreeQuotaStore(), newTestUsageStore(), testIdemStore{}, nil, nil, "salt", 100, time.Hour)
	registerLicenseRoutes(api, lic)

	body, _ := json.Marshal(licensesvc.CheckRequest{FingerprintHash: "fp-1", RequestNonce: "nonce-fp-1-generate", Action: "generate"})
	for i := 0; i < 30; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/license/check", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code == http.StatusTooManyRequests {
			t.Fatalf("request %d unexpectedly rate limited", i+1)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/api/license/check", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestRegisterLicenseRoutesRateLimitsConsume(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api")
	quotas := newTestFreeQuotaStore()
	quotas.quotas["fp-1"] = &model.FreeQuota{FingerprintHash: "fp-1", FreeLimit: 100, FreeUsed: 0}
	lic := licensesvc.NewService(testAPIKeyStore{}, quotas, newTestUsageStore(), testIdemStore{}, nil, nil, "salt", 100, time.Hour)
	registerLicenseRoutes(api, lic)

	for i := 0; i < 30; i++ {
		token := issueRouteCommitToken(t, lic, "fp-1", model.AccessModeFree, func(req *licensesvc.CheckRequest) {
			req.RequestNonce = fmt.Sprintf("nonce-fp-1-generate-%d", i)
		})
		body, _ := json.Marshal(licensesvc.ConsumeRequest{
			FingerprintHash: "fp-1",
			RequestID:       token.RequestID,
			UsageType:       "generate",
			AccessMode:      model.AccessModeFree,
			CommitToken:     token,
		})
		req := httptest.NewRequest(http.MethodPost, "/api/license/consume", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code == http.StatusTooManyRequests {
			t.Fatalf("request %d unexpectedly rate limited", i+1)
		}
	}

	token := issueRouteCommitToken(t, lic, "fp-1", model.AccessModeFree, func(req *licensesvc.CheckRequest) {
		req.RequestNonce = "nonce-fp-1-generate-final"
	})
	body, _ := json.Marshal(licensesvc.ConsumeRequest{
		FingerprintHash: "fp-1",
		RequestID:       token.RequestID,
		UsageType:       "generate",
		AccessMode:      model.AccessModeFree,
		CommitToken:     token,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/license/consume", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
