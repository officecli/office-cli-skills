package previewshare

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type fakeFileCleaner struct {
	deleted string
}

func (f *fakeFileCleaner) DeleteFileMeta(_ context.Context, fileID string) error {
	f.deleted = fileID
	return nil
}

type fakeObjectCleaner struct {
	deleted string
}

func (f *fakeObjectCleaner) DeleteObject(_ context.Context, key string) error {
	f.deleted = key
	return nil
}

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), url.QueryEscape(t.Name())+".db")
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s", dbPath)), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&PreviewShare{}))
	return db
}

func TestIssueAccessCookieAllowsShareAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newTestDB(t)
	service := NewService(db, "secret", ".officecli.io", nil, nil)
	service.now = func() time.Time {
		return time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)
	}

	share, err := service.Create(t.Context(), CreateParams{
		FileID:     "file-1",
		StorageKey: "preview/file-1/original/demo.pptx",
		FileName:   "demo.pptx",
		FileType:   "pptx",
		ExpiresAt:  service.now().Add(time.Hour),
	})
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/p/"+share.ShareToken, nil)
	service.IssueAccessCookie(c, share)

	c2, _ := gin.CreateTestContext(httptest.NewRecorder())
	c2.Request = httptest.NewRequest(http.MethodGet, "/officesdk/page?file_id=file-1", nil)
	for _, cookie := range rec.Result().Cookies() {
		c2.Request.AddCookie(cookie)
	}

	resolved, status, err := service.RequireShareAccess(c2, "file-1")
	require.NoError(t, err)
	require.Equal(t, 0, status)
	require.NotNil(t, resolved)
	require.Equal(t, share.ShareToken, resolved.ShareToken)
}

func TestCreateWithPasswordReturnsVerifiablePassword(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newTestDB(t)
	service := NewService(db, "secret", ".officecli.io", nil, nil)

	result, err := service.CreateWithPassword(t.Context(), CreateParams{
		FileID:     "file-1",
		StorageKey: "preview/file-1/original/demo.pptx",
		FileName:   "demo.pptx",
		FileType:   "pptx",
		ExpiresAt:  time.Now().UTC().Add(time.Hour),
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.Share)
	require.Len(t, result.Password, 6)
	require.NotEmpty(t, result.Share.PasswordHash)
	require.True(t, service.VerifyPassword(result.Share, result.Password))
}

func TestRequireShareAccessAllowsInternalCallbackWithoutCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newTestDB(t)
	service := NewService(db, "secret", ".officecli.io", nil, nil)
	service.now = func() time.Time {
		return time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)
	}

	share, err := service.Create(t.Context(), CreateParams{
		FileID:     "file-1",
		StorageKey: "preview/file-1/original/demo.pptx",
		FileName:   "demo.pptx",
		FileType:   "pptx",
		ExpiresAt:  service.now().Add(time.Hour),
	})
	require.NoError(t, err)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "http://host.docker.internal:29001/officesdk/proxy/download?key=preview%2Ffile-1%2Foriginal%2Fdemo.pptx", nil)
	c.Request.Host = "host.docker.internal:29001"
	c.Request.RemoteAddr = "172.17.0.2:45678"

	resolved, status, err := service.RequireShareAccess(c, "file-1")
	require.NoError(t, err)
	require.Equal(t, 0, status)
	require.NotNil(t, resolved)
	require.Equal(t, share.ShareToken, resolved.ShareToken)
}

func TestRequireShareAccessAllowsTrustedPublicHostFromPrivateIPWithoutCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newTestDB(t)
	service := NewService(db, "secret", ".officecli.io", nil, nil)
	service.now = func() time.Time {
		return time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)
	}

	share, err := service.Create(t.Context(), CreateParams{
		FileID:     "file-1",
		StorageKey: "preview/file-1/original/demo.pptx",
		FileName:   "demo.pptx",
		FileType:   "pptx",
		ExpiresAt:  service.now().Add(time.Hour),
	})
	require.NoError(t, err)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "https://officecli.io/v1/thirdparty/files/file-1", nil)
	c.Request.Host = "officecli.io"
	c.Request.RemoteAddr = "10.42.0.1:45678"

	resolved, status, err := service.RequireShareAccess(c, "file-1")
	require.NoError(t, err)
	require.Equal(t, 0, status)
	require.NotNil(t, resolved)
	require.Equal(t, share.ShareToken, resolved.ShareToken)
}

func TestRequireShareAccessRejectsPublicRequestWithoutCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newTestDB(t)
	service := NewService(db, "secret", ".officecli.io", nil, nil)
	service.now = func() time.Time {
		return time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)
	}

	_, err := service.Create(t.Context(), CreateParams{
		FileID:     "file-1",
		StorageKey: "preview/file-1/original/demo.pptx",
		FileName:   "demo.pptx",
		FileType:   "pptx",
		ExpiresAt:  service.now().Add(time.Hour),
	})
	require.NoError(t, err)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "https://officecli.io/officesdk/proxy/download?key=preview%2Ffile-1%2Foriginal%2Fdemo.pptx", nil)
	c.Request.Host = "officecli.io"
	c.Request.RemoteAddr = "8.8.8.8:45678"

	resolved, status, err := service.RequireShareAccess(c, "file-1")
	require.Error(t, err)
	require.Equal(t, http.StatusUnauthorized, status)
	require.NotNil(t, resolved)
	require.Contains(t, err.Error(), "preview password required")
}

func TestRequireShareDownloadAccessAllowsSignedTokenWithoutCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newTestDB(t)
	service := NewService(db, "secret", ".officecli.io", nil, nil)
	service.now = func() time.Time {
		return time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)
	}

	share, err := service.Create(t.Context(), CreateParams{
		FileID:     "file-1",
		StorageKey: "officesdk/file-1/content/content",
		FileName:   "demo.pptx",
		FileType:   "pptx",
		ExpiresAt:  service.now().Add(time.Hour),
	})
	require.NoError(t, err)

	token, err := service.IssueDownloadToken(t.Context(), share.FileID, share.StorageKey)
	require.NoError(t, err)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(
		http.MethodGet,
		"https://officecli.io/officesdk/proxy/download?key=officesdk%2Ffile-1%2Fcontent%2Fcontent&access_token="+url.QueryEscape(token),
		nil,
	)
	c.Request.Host = "officecli.io"
	c.Request.RemoteAddr = "8.8.8.8:45678"

	resolved, status, err := service.RequireShareDownloadAccess(c, "file-1", "officesdk/file-1/content/content")
	require.NoError(t, err)
	require.Equal(t, 0, status)
	require.NotNil(t, resolved)
	require.Equal(t, share.ShareToken, resolved.ShareToken)
}

func TestRequireShareDownloadAccessRejectsInvalidSignedToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newTestDB(t)
	service := NewService(db, "secret", ".officecli.io", nil, nil)
	service.now = func() time.Time {
		return time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)
	}

	_, err := service.Create(t.Context(), CreateParams{
		FileID:     "file-1",
		StorageKey: "officesdk/file-1/content/content",
		FileName:   "demo.pptx",
		FileType:   "pptx",
		ExpiresAt:  service.now().Add(time.Hour),
	})
	require.NoError(t, err)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(
		http.MethodGet,
		"https://officecli.io/officesdk/proxy/download?key=officesdk%2Ffile-1%2Fcontent%2Fcontent&access_token=bad-token",
		nil,
	)
	c.Request.Host = "officecli.io"
	c.Request.RemoteAddr = "8.8.8.8:45678"

	resolved, status, err := service.RequireShareDownloadAccess(c, "file-1", "officesdk/file-1/content/content")
	require.Error(t, err)
	require.Equal(t, http.StatusUnauthorized, status)
	require.NotNil(t, resolved)
	require.Contains(t, err.Error(), "preview password required")
}

func TestCleanupExpiredRemovesArtifacts(t *testing.T) {
	db := newTestDB(t)
	files := &fakeFileCleaner{}
	objects := &fakeObjectCleaner{}
	service := NewService(db, "secret", "", files, objects)
	now := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }

	_, err := service.Create(t.Context(), CreateParams{
		FileID:     "file-1",
		StorageKey: "preview/file-1/original/demo.pptx",
		FileName:   "demo.pptx",
		FileType:   "pptx",
		ExpiresAt:  now.Add(-time.Minute),
	})
	require.NoError(t, err)

	require.NoError(t, service.CleanupExpired(t.Context()))
	require.Equal(t, "file-1", files.deleted)
	require.Equal(t, "preview/file-1/original/demo.pptx", objects.deleted)
}
