package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/officecli/officecli-internal/platform/internal/clisession"
	"github.com/officecli/officecli-internal/platform/internal/model"
	sqlstore "github.com/officecli/officecli-internal/platform/internal/store/sqlstore"
)

func clisessionTestS256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func newTestCLISessionService(t *testing.T) (*clisession.Service, *sqlstore.Store) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:cli_routes?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.CLILoginChallenge{}, &model.CLISession{}))
	store := sqlstore.NewWithDB(db)
	return clisession.NewService(store, "https://platform.example.com"), store
}

func TestCLILoginVerifyRedirectsAnonymousUserToGoogle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api")
	cliSvc, _ := newTestCLISessionService(t)
	authSvc := &fakeAuthRouteService{}
	registerCLIRoutes(api, Config{}, authSvc, cliSvc)

	req := httptest.NewRequest(http.MethodGet, "/api/cli/login/verify?user_code=ABCD-EFGH", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusFound, rec.Code)
	location := rec.Header().Get("Location")
	require.Contains(t, location, "/api/auth/google/login")
	require.Contains(t, location, "return_to=")
	require.Contains(t, location, "user_code%3DABCD-EFGH")
}

func TestCLILoginVerifyCompletesDeviceChallengeAndPollsCompleted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	api := router.Group("/api")
	cliSvc, store := newTestCLISessionService(t)
	authSvc := &fakeAuthRouteService{meUser: &model.User{ID: 42, Email: "dev@example.com"}}
	registerCLIRoutes(api, Config{}, authSvc, cliSvc)
	require.NoError(t, store.DB().Create(&model.User{ID: 42, GoogleSub: "sub", Email: "dev@example.com", Name: "Dev", InviteCode: "invite-42", Status: model.UserStatusActive}).Error)

	start, err := cliSvc.Start(context.Background(), clisession.StartRequest{
		CodeChallenge:       clisessionTestS256("verifier"),
		CodeChallengeMethod: "S256",
	})
	require.NoError(t, err)

	verifyReq := httptest.NewRequest(http.MethodGet, "/api/cli/login/verify?user_code="+start.UserCode, nil)
	verifyReq.AddCookie(&http.Cookie{Name: "cop_app_session", Value: "cookie"})
	verifyRec := httptest.NewRecorder()
	router.ServeHTTP(verifyRec, verifyReq)
	require.Equal(t, http.StatusOK, verifyRec.Code)
	require.Contains(t, verifyRec.Body.String(), "OfficeCLI login complete")

	body, err := json.Marshal(map[string]string{"challenge_id": start.ChallengeID})
	require.NoError(t, err)
	pollReq := httptest.NewRequest(http.MethodPost, "/api/cli/login/poll", bytes.NewReader(body))
	pollReq.Header.Set("Content-Type", "application/json")
	pollRec := httptest.NewRecorder()
	router.ServeHTTP(pollRec, pollReq)

	require.Equal(t, http.StatusOK, pollRec.Code)
	require.Contains(t, pollRec.Body.String(), `"status":"completed"`)

	resp, err := cliSvc.Exchange(context.Background(), clisession.ExchangeRequest{
		ChallengeID:  start.ChallengeID,
		CodeVerifier: "verifier",
	})
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(resp.Token, "ocli_sess_"))
}
