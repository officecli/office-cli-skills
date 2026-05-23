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
