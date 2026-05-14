package officesdk

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/officecli/officecli-internal/platform/internal/previewshare"
)

func newPreviewShareServiceForOfficeSDK(t *testing.T) *previewshare.Service {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", url.QueryEscape(t.Name()))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&previewshare.PreviewShare{}))
	return previewshare.NewService(db, "secret", ".officecli.io", nil, nil)
}

func issuePreviewCookies(t *testing.T, shares *previewshare.Service, share *previewshare.PreviewShare) []*http.Cookie {
	t.Helper()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/p/"+share.ShareToken, nil)
	shares.IssueAccessCookie(c, share)
	return rec.Result().Cookies()
}

func attachCookies(req *http.Request, cookies []*http.Cookie) {
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
}

func TestServePageFallsBackToPreviewShareMeta(t *testing.T) {
	gin.SetMode(gin.TestMode)
	shares := newPreviewShareServiceForOfficeSDK(t)
	share, err := shares.Create(t.Context(), previewshare.CreateParams{
		FileID:     "file-1",
		StorageKey: "preview/file-1/original/demo.pptx",
		FileName:   "demo.pptx",
		FileType:   "pptx",
		ExpiresAt:  time.Now().UTC().Add(time.Hour),
	})
	require.NoError(t, err)

	provider := NewFileProvider(NewFileStore(nil), nil, shares)
	handler := NewHandler(NewFileStore(nil), provider, "https://officecli.io/sdk/turbo-ai", "secret")
	req := httptest.NewRequest(http.MethodGet, "/officesdk/page?file_id=file-1", nil)
	attachCookies(req, issuePreviewCookies(t, shares, share))

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	handler.ServePage(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "OfficeCLI Preview")
	require.Contains(t, rec.Body.String(), `"file-1"`)
}

func TestGetSDKParamsFallsBackToPreviewShareMeta(t *testing.T) {
	gin.SetMode(gin.TestMode)
	shares := newPreviewShareServiceForOfficeSDK(t)
	share, err := shares.Create(t.Context(), previewshare.CreateParams{
		FileID:     "file-1",
		StorageKey: "preview/file-1/original/demo.pptx",
		FileName:   "demo.pptx",
		FileType:   "pptx",
		ExpiresAt:  time.Now().UTC().Add(time.Hour),
	})
	require.NoError(t, err)

	provider := NewFileProvider(NewFileStore(nil), nil, shares)
	handler := NewHandler(NewFileStore(nil), provider, "https://officecli.io/sdk/turbo-ai", "secret")
	req := httptest.NewRequest(http.MethodGet, "/officesdk/sdk-params?file_id=file-1", nil)
	req.Host = "officecli.io"
	req.Header.Set("X-Forwarded-Proto", "https")
	attachCookies(req, issuePreviewCookies(t, shares, share))

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	handler.GetSDKParams(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"fileId":"file-1"`)
	require.Contains(t, rec.Body.String(), `"version":1`)
}

func TestGetFileDownloadFallsBackToPreviewShareMeta(t *testing.T) {
	gin.SetMode(gin.TestMode)
	shares := newPreviewShareServiceForOfficeSDK(t)
	share, err := shares.Create(t.Context(), previewshare.CreateParams{
		FileID:     "file-1",
		StorageKey: "preview/file-1/original/demo.pptx",
		FileName:   "demo.pptx",
		FileType:   "pptx",
		ExpiresAt:  time.Now().UTC().Add(time.Hour),
	})
	require.NoError(t, err)

	provider := NewFileProvider(NewFileStore(nil), nil, shares)
	req := httptest.NewRequest(http.MethodGet, "/sdk/turbo-ai/v1/api/file/download/file-1", nil)
	req.Host = "officecli.io"
	req.Header.Set("X-Forwarded-Proto", "https")
	attachCookies(req, issuePreviewCookies(t, shares, share))

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req

	resp, err := provider.GetFileDownload(c, "file-1")
	require.NoError(t, err)
	require.NotNil(t, resp)
	parsed, parseErr := url.Parse(resp.URL)
	require.NoError(t, parseErr)
	require.Equal(t, "https", parsed.Scheme)
	require.Equal(t, "officecli.io", parsed.Host)
	require.Equal(t, "/officesdk/proxy/download", parsed.Path)
	require.Equal(t, "preview/file-1/original/demo.pptx", parsed.Query().Get("key"))
	require.NotEmpty(t, strings.TrimSpace(parsed.Query().Get("access_token")))
}
