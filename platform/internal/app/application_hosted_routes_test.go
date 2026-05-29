package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gotomicro/ego/server/egin"

	"github.com/officecli/officecli/platform/internal/hostedllm"
)

func TestRegisterRoutesWithHostedMountsLLMImageRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := egin.DefaultContainer().Build()
	hostedSvc := hostedllm.NewService(nil, hostedllm.Config{})
	registerRoutesWithHosted(server, Config{}, nil, nil, nil, nil, nil, hostedSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/llm/v1/image", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Engine.ServeHTTP(rec, req)

	if rec.Code == http.StatusNotFound {
		t.Fatalf("hosted image route was not mounted: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// TestHostedLLMConfigPropagatesReconcileFlag verifies the LoadConfig env
// → app.Config → hostedllm.Config wiring for the reconcile flag (the
// last remaining hosted feature toggle after Phase 5 removed
// ChargeOnlyMode).
func TestHostedLLMConfigPropagatesReconcileFlag(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("HOSTED_RECONCILE_ENABLED", "true")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	svc := hostedllm.NewService(nil, hostedllm.Config{
		BaseURL:          cfg.HostedLLMBaseURL,
		ReconcileEnabled: cfg.HostedReconcileEnabled,
	})
	if !svc.CurrentReconcileEnabled() {
		t.Fatalf("CurrentReconcileEnabled = false, want true")
	}
}

// TestHostedLLMConfigDefaultsReconcileOff guards the safety default —
// reconcile must remain off by default so charge_failed_post_upstream
// ledger rows are never written without an explicit opt-in.
func TestHostedLLMConfigDefaultsReconcileOff(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("HOSTED_RECONCILE_ENABLED", "")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	svc := hostedllm.NewService(nil, hostedllm.Config{
		ReconcileEnabled: cfg.HostedReconcileEnabled,
	})
	if svc.CurrentReconcileEnabled() {
		t.Fatalf("CurrentReconcileEnabled = true, want false")
	}
}
