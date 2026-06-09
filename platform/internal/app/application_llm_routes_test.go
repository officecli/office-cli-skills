package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/officecli/officecli/platform/internal/admin"
	"github.com/officecli/officecli/platform/internal/hostedllm"
	"github.com/officecli/officecli/platform/internal/httpapi"
	"github.com/officecli/officecli/platform/internal/model"
	sqlstore "github.com/officecli/officecli/platform/internal/store/sqlstore"
)

func TestHostedBillingRequestIDPrefersBodyThenHeaderThenMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		bodyID     string
		headerID   string
		contextID  string
		want       string
		wantNonNil bool
	}{
		{
			name:      "body request id wins over header",
			bodyID:    "body-req",
			headerID:  "header-req",
			contextID: "ctx-req",
			want:      "body-req",
		},
		{
			name:      "header request id is fallback",
			headerID:  "header-req",
			contextID: "ctx-req",
			want:      "header-req",
		},
		{
			name:      "middleware request id is final fallback",
			contextID: "ctx-req",
			want:      "ctx-req",
		},
		{
			name:       "missing all ids still returns generated id",
			wantNonNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/api/llm/v1/image", nil)
			if tt.headerID != "" {
				c.Request.Header.Set("X-Request-Id", tt.headerID)
			}
			if tt.contextID != "" {
				c.Set(httpapi.ContextRequestID, tt.contextID)
			}

			got := hostedBillingRequestID(c, tt.bodyID)
			if tt.wantNonNil {
				require.NotEmpty(t, got)
				return
			}
			require.Equal(t, tt.want, got)
		})
	}
}

func TestHostedImageRouteReplaysSavedProvenanceByRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:hosted_image_route_replay?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.APIKey{},
		&model.UserHostedCreditAccount{},
		&model.UserHostedCreditLedger{},
		&model.UsageEvent{},
		&model.ImageGenerationProvenance{},
	))

	store := sqlstore.NewWithDB(db)
	userID := uint64(42)
	require.NoError(t, db.Create(&model.User{ID: userID, GoogleSub: model.StringPtr("sub-route-replay"), Email: "route-replay@example.com", Name: "Route Replay", InviteCode: "route-replay", Status: model.UserStatusActive}).Error)
	_, _, _, err = store.GrantHostedCreditsToUser(context.Background(), userID, model.HostedCreditLedgerSourcePurchase, "order:route-replay", 120, "purchase", "{}")
	require.NoError(t, err)
	apiKey := "hosted-route-key"
	hashBytes := sha256.Sum256([]byte("salt:" + apiKey))
	defaultRuntimeMode := "hosted"
	require.NoError(t, store.CreateAPIKey(context.Background(), &model.APIKey{
		OwnerUserID:        &userID,
		KeyHash:            fmt.Sprintf("%x", hashBytes[:]),
		KeyPrefix:          "cop_route",
		Status:             model.APIKeyStatusActive,
		PlanName:           "Hosted",
		AllowedModes:       "hosted_only",
		HostedEnabled:      true,
		DefaultRuntimeMode: &defaultRuntimeMode,
	}))

	adminSvc := admin.NewService(store, nil, "secret", time.Hour, "cookie", testRouteCodec{}, "salt", nil, nil, nil)
	adminSvc.SetImageTemplateObjectStore(&routeImageTemplateObjectStore{})
	_, err = adminSvc.RecordImageGenerationProvenance(context.Background(), admin.RecordImageGenerationProvenanceRequest{
		RequestID:   "req-route-replay",
		UserID:      userID,
		Prompt:      "saved prompt",
		ImageName:   "generated.png",
		ContentType: "image/png",
		Image:       bytes.NewReader([]byte("saved-image")),
		ImageSize:   int64(len("saved-image")),
	})
	require.NoError(t, err)

	router := gin.New()
	registerHostedLLMRoutes(router.Group("/api"), hostedllm.NewService(store, hostedllm.Config{HashSalt: "salt"}), adminSvc)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/llm/v1/image", bytes.NewReader([]byte(`{
		"request_id":"req-route-replay",
		"model":"hosted/image",
		"prompt":"saved prompt",
		"aspect_ratio":1
	}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var payload struct {
		Data           string `json:"data"`
		MIME           string `json:"mime"`
		RequestID      string `json:"request_id"`
		CreditBalance  int    `json:"credit_balance"`
		CreditsCharged int    `json:"credits_charged"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	data, err := base64.StdEncoding.DecodeString(payload.Data)
	require.NoError(t, err)
	require.Equal(t, []byte("saved-image"), data)
	require.Equal(t, "image/png", payload.MIME)
	require.Equal(t, "req-route-replay", payload.RequestID)
	require.Equal(t, 120, payload.CreditBalance)
	require.Zero(t, payload.CreditsCharged)
}
