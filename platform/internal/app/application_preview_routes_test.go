package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
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
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", url.QueryEscape(t.Name()))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&previewshare.PreviewShare{}))
	return previewshare.NewService(db, "secret", ".officecli.io", nil, nil)
}

func TestRegisterPreviewRoutesRendersEnglishPasswordPageWithoutAccessCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	shares := newPreviewShareService(t)
	result, err := shares.CreateWithPassword(t.Context(), previewshare.CreateParams{
		FileID:     "file-1",
		StorageKey: "preview/file-1/original/demo.pptx",
		FileName:   "demo.pptx",
		FileType:   "pptx",
		ExpiresAt:  time.Now().UTC().Add(time.Hour),
	})
	require.NoError(t, err)
	share := result.Share

	sdkStore := officesdk.NewFileStore(nil)
	sdkProvider := officesdk.NewFileProvider(sdkStore, nil, shares)
	sdkHandler := officesdk.NewHandler(sdkStore, sdkProvider, "https://officecli.io/sdk/turbo-ai", "secret")
	component := egin.DefaultContainer().Build()
	registerPreviewRoutes(component, Config{
		OfficeSDKHost: "http://127.0.0.1:19101",
	}, &fakeAuthRouteService{}, shares, sdkHandler, sdkProvider)

	req := httptest.NewRequest(http.MethodGet, "/p/"+share.ShareToken, nil)
	req.Host = "officecli.io"
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	component.Engine.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Header().Get("Content-Type"), "text/html")
	require.Contains(t, rec.Body.String(), "Enter Preview Password")
	require.Contains(t, rec.Body.String(), "Password")
	require.Contains(t, rec.Body.String(), "Open Preview")
}

func TestRegisterPreviewRoutesServesPreviewPageAfterPasswordSubmit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	shares := newPreviewShareService(t)
	result, err := shares.CreateWithPassword(t.Context(), previewshare.CreateParams{
		FileID:     "file-1",
		StorageKey: "preview/file-1/original/demo.pptx",
		FileName:   "demo.pptx",
		FileType:   "pptx",
		ExpiresAt:  time.Now().UTC().Add(time.Hour),
	})
	require.NoError(t, err)
	share := result.Share

	sdkStore := officesdk.NewFileStore(nil)
	sdkProvider := officesdk.NewFileProvider(sdkStore, nil, shares)
	sdkHandler := officesdk.NewHandler(sdkStore, sdkProvider, "https://officecli.io/sdk/turbo-ai", "secret")
	component := egin.DefaultContainer().Build()
	registerPreviewRoutes(component, Config{
		OfficeSDKHost: "http://127.0.0.1:19101",
	}, &fakeAuthRouteService{}, shares, sdkHandler, sdkProvider)

	req := httptest.NewRequest(http.MethodPost, "/p/"+share.ShareToken, strings.NewReader("password="+result.Password))
	req.Host = "officecli.io"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	component.Engine.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Empty(t, rec.Header().Get("Location"))
	require.Contains(t, rec.Header().Get("Content-Type"), "text/html")
	require.Contains(t, rec.Body.String(), "OfficeCLI Preview")
	require.Contains(t, rec.Body.String(), `"file-1"`)
	require.True(t, strings.Contains(strings.Join(rec.Header().Values("Set-Cookie"), "\n"), "cop_preview_access="))
}

func TestRegisterPreviewRoutesRejectsInvalidPasswordWithEnglishError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	shares := newPreviewShareService(t)
	result, err := shares.CreateWithPassword(t.Context(), previewshare.CreateParams{
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
		OfficeSDKHost: "http://127.0.0.1:19101",
	}, &fakeAuthRouteService{}, shares, sdkHandler, sdkProvider)

	req := httptest.NewRequest(http.MethodPost, "/p/"+result.Share.ShareToken, strings.NewReader("password=bad"))
	req.Host = "officecli.io"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	component.Engine.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Contains(t, rec.Body.String(), "Enter Preview Password")
	require.Contains(t, rec.Body.String(), "Incorrect password. Please try again.")
	require.NotContains(t, rec.Body.String(), "OfficeCLI Preview")
}

func TestRegisterPreviewRoutesServesPreviewPageWithExistingPreviewCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	shares := newPreviewShareService(t)
	result, err := shares.CreateWithPassword(t.Context(), previewshare.CreateParams{
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
		OfficeSDKHost: "http://127.0.0.1:19101",
	}, &fakeAuthRouteService{}, shares, sdkHandler, sdkProvider)

	issueRec := httptest.NewRecorder()
	issueCtx, _ := gin.CreateTestContext(issueRec)
	issueCtx.Request = httptest.NewRequest(http.MethodGet, "/p/"+result.Share.ShareToken, nil)
	issueCtx.Request.Host = "officecli.io"
	issueCtx.Request.Header.Set("X-Forwarded-Proto", "https")
	shares.IssueAccessCookie(issueCtx, result.Share)

	req := httptest.NewRequest(http.MethodGet, "/p/"+result.Share.ShareToken, nil)
	req.Host = "officecli.io"
	req.Header.Set("X-Forwarded-Proto", "https")
	for _, cookie := range issueRec.Result().Cookies() {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	component.Engine.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "OfficeCLI Preview")
}

func TestRegisterOfficeSDKProxyRewritesAbsoluteRedirectLocationToPublicHost(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://host.docker.internal:29001/officesdk/proxy/download?key=test", http.StatusFound)
	}))
	defer upstream.Close()

	component := egin.DefaultContainer().Build()
	registerOfficeSDKProxy(component.Engine, Config{OfficeSDKHost: upstream.URL})
	server := httptest.NewServer(component.Engine)
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/sdk/turbo-ai/files/file-1/content", nil)
	require.NoError(t, err)
	req.Host = "officecli.io"
	req.Header.Set("X-Forwarded-Proto", "https")

	client := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusFound, resp.StatusCode)
	require.Equal(t, "https://officecli.io/officesdk/proxy/download?key=test", resp.Header.Get("Location"))
}
