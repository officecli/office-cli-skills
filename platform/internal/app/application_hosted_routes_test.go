package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gotomicro/ego/server/egin"

	"github.com/officecli/officecli-internal/platform/internal/hostedllm"
)

func TestRegisterRoutesWithHostedMountsLLMImageRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	server := egin.DefaultContainer().Build()
	hostedSvc := hostedllm.NewService(nil, hostedllm.Config{})
	registerRoutesWithHosted(server, Config{}, nil, nil, nil, nil, nil, hostedSvc, nil, nil, nil, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/llm/v1/image", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	server.Engine.ServeHTTP(rec, req)

	if rec.Code == http.StatusNotFound {
		t.Fatalf("hosted image route was not mounted: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// TestHostedLLMConfigPropagatesChargeOnlyAndReconcileFlags verifies the
// G1/G2 wiring: LoadConfig env vars → app.Config fields →
// hostedllm.Config → live atomic flags reachable via the getters.
func TestHostedLLMConfigPropagatesChargeOnlyAndReconcileFlags(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("HOSTED_CHARGE_ONLY_MODE", "all")
	t.Setenv("HOSTED_RECONCILE_ENABLED", "true")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	// Mirror application.go line ~191 construction so the wiring is
	// exercised by the test (without booting the full app).
	svc := hostedllm.NewService(nil, hostedllm.Config{
		BaseURL:          cfg.HostedLLMBaseURL,
		ChargeOnlyMode:   hostedllm.ChargeOnlyMode(cfg.HostedChargeOnlyMode),
		ReconcileEnabled: cfg.HostedReconcileEnabled,
	})
	if got := svc.CurrentChargeOnlyMode(); got != hostedllm.ChargeOnlyModeAll {
		t.Fatalf("CurrentChargeOnlyMode = %q, want all", got)
	}
	if !svc.CurrentReconcileEnabled() {
		t.Fatalf("CurrentReconcileEnabled = false, want true")
	}
}

// TestHostedLLMConfigDefaultsToDisabledAndReconcileOff guards the safety
// default — Phase 3 §5 calls for flag-off by default so a misconfigured
// deploy cannot silently flip to charge-only.
func TestHostedLLMConfigDefaultsToDisabledAndReconcileOff(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("HOSTED_CHARGE_ONLY_MODE", "")
	t.Setenv("HOSTED_RECONCILE_ENABLED", "")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	svc := hostedllm.NewService(nil, hostedllm.Config{
		ChargeOnlyMode:   hostedllm.ChargeOnlyMode(cfg.HostedChargeOnlyMode),
		ReconcileEnabled: cfg.HostedReconcileEnabled,
	})
	if got := svc.CurrentChargeOnlyMode(); got != hostedllm.ChargeOnlyModeDisabled {
		t.Fatalf("CurrentChargeOnlyMode = %q, want disabled", got)
	}
	if svc.CurrentReconcileEnabled() {
		t.Fatalf("CurrentReconcileEnabled = true, want false")
	}
}
