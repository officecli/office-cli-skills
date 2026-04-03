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

	body, _ := json.Marshal(licensesvc.CheckRequest{FingerprintHash: "fp-1", Action: "generate"})
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
	today := time.Now().UTC().Format("2006-01-02")
	quotas.quotas["fp-1|"+today] = &model.DailyFreeQuota{FingerprintHash: "fp-1", UsageDate: today, DailyLimit: 100, DailyUsed: 0}
	lic := licensesvc.NewService(testAPIKeyStore{}, quotas, newTestUsageStore(), testIdemStore{}, nil, nil, "salt", 100, time.Hour)
	registerLicenseRoutes(api, lic)

	for i := 0; i < 30; i++ {
		body, _ := json.Marshal(licensesvc.ConsumeRequest{
			FingerprintHash: "fp-1",
			RequestID:       fmt.Sprintf("req-%d", i),
			UsageType:       "generate",
			AccessMode:      model.AccessModeFree,
		})
		req := httptest.NewRequest(http.MethodPost, "/api/license/consume", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code == http.StatusTooManyRequests {
			t.Fatalf("request %d unexpectedly rate limited", i+1)
		}
	}

	body, _ := json.Marshal(licensesvc.ConsumeRequest{
		FingerprintHash: "fp-1",
		RequestID:       "req-final",
		UsageType:       "generate",
		AccessMode:      model.AccessModeFree,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/license/consume", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}
