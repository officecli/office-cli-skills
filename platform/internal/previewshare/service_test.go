package previewshare

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", url.QueryEscape(t.Name()))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
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
