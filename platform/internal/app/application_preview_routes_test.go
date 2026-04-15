package app

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gotomicro/ego/server/egin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/officecli/officecli/platform/internal/officesdk"
	"github.com/officecli/officecli/platform/internal/previewshare"
)

func newPreviewShareService(t *testing.T) *previewshare.Service {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&previewshare.PreviewShare{}))
	return previewshare.NewService(db, "secret", ".officecli.io", nil, nil)
}

func TestRegisterPreviewRoutesRedirectsToGoogleLoginWhenAppSessionMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	shares := newPreviewShareService(t)
	share, err := shares.Create(t.Context(), previewshare.CreateParams{
		FileID:     "file-1",
		StorageKey: "preview/file-1/original/demo.pptx",
		FileName:   "demo.pptx",
		FileType:   "pptx",
		ExpiresAt:  time.Now().UTC().Add(time.Hour),
	})
	require.NoError(t, err)

	sdkStore := officesdk.NewFileStore(nil)
	sdkProvider := officesdk.NewFileProvider(sdkStore, nil, shares)
	sdkHandler := officesdk.NewHandler(sdkStore, sdkProvider, "https://officecli.io/sdk/turbo-ai", "secret")
	component := egin.DefaultContainer().Build()
	registerPreviewRoutes(component, Config{
		PlatformBaseURL: "https://platform.officecli.io",
		OfficeSDKHost:   "http://127.0.0.1:19101",
	}, &fakeAuthRouteService{meErr: errors.New("unauthorized")}, shares, sdkHandler, sdkProvider)

	req := httptest.NewRequest(http.MethodGet, "/p/"+share.ShareToken, nil)
	req.Host = "officecli.io"
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	component.Engine.ServeHTTP(rec, req)

	require.Equal(t, http.StatusFound, rec.Code)
	require.Contains(t, rec.Header().Get("Location"), "https://platform.officecli.io/api/auth/google/login")
	require.Contains(t, rec.Header().Get("Location"), "return_to=https%3A%2F%2Fofficecli.io%2Fp%2F")
}
