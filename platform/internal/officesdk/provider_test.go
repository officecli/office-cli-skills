package officesdk

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/officecli/officecli/platform/internal/previewshare"
)

type fakeDownloadObjectStore struct {
	objects map[string][]byte
}

func (f *fakeDownloadObjectStore) PutObject(_ context.Context, key string, reader io.Reader, _ int64, _ string) error {
	if f.objects == nil {
		f.objects = map[string][]byte{}
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	f.objects[key] = data
	return nil
}

func (f *fakeDownloadObjectStore) GetObject(_ context.Context, key string) (io.ReadCloser, error) {
	if data, ok := f.objects[key]; ok {
		return io.NopCloser(bytes.NewReader(data)), nil
	}
	return nil, io.EOF
}

func (f *fakeDownloadObjectStore) DeleteObject(_ context.Context, key string) error {
	delete(f.objects, key)
	return nil
}

func TestGetDownloadURLContentFallsBackToSDKContentObjectWhenPendingKeyExpired(t *testing.T) {
	gin.SetMode(gin.TestMode)
	shares := newPreviewShareServiceForOfficeSDK(t)
	share, err := shares.Create(t.Context(), previewshare.CreateParams{
		FileID:     "file-fallback-1",
		StorageKey: "preview/file-fallback-1/original/demo.pptx",
		FileName:   "demo.pptx",
		FileType:   "pptx",
		ExpiresAt:  time.Now().UTC().Add(time.Hour),
	})
	require.NoError(t, err)

	provider := NewFileProvider(NewFileStore(nil), &fakeDownloadObjectStore{
		objects: map[string][]byte{
			sdkContentStorageKey("file-fallback-1"): []byte(`{"ops":[]}`),
		},
	}, shares)

	req := httptest.NewRequest(http.MethodGet, "/download?object_name=content", nil)
	req.Host = "officecli.io"
	req.Header.Set("X-Forwarded-Proto", "https")
	attachCookies(req, issuePreviewCookies(t, shares, share))

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req

	resp, err := provider.GetDownloadURL(c, "file-fallback-1")
	require.NoError(t, err)
	require.NotNil(t, resp)

	parsed, parseErr := url.Parse(resp.URL)
	require.NoError(t, parseErr)
	require.Equal(t, "https", parsed.Scheme)
	require.Equal(t, "officecli.io", parsed.Host)
	require.Equal(t, "/officesdk/proxy/download", parsed.Path)
	require.Equal(t, sdkContentStorageKey("file-fallback-1"), parsed.Query().Get("key"))
	require.NotEmpty(t, parsed.Query().Get("access_token"))
}

func TestGetDownloadURLContentFallsBackToOriginalFileWhenSDKContentObjectMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	shares := newPreviewShareServiceForOfficeSDK(t)
	share, err := shares.Create(t.Context(), previewshare.CreateParams{
		FileID:     "file-fallback-2",
		StorageKey: "preview/file-fallback-2/original/demo.pptx",
		FileName:   "demo.pptx",
		FileType:   "pptx",
		ExpiresAt:  time.Now().UTC().Add(time.Hour),
	})
	require.NoError(t, err)

	provider := NewFileProvider(NewFileStore(nil), &fakeDownloadObjectStore{}, shares)

	req := httptest.NewRequest(http.MethodGet, "/download?object_name=content", nil)
	req.Host = "officecli.io"
	req.Header.Set("X-Forwarded-Proto", "https")
	attachCookies(req, issuePreviewCookies(t, shares, share))

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req

	resp, err := provider.GetDownloadURL(c, "file-fallback-2")
	require.NoError(t, err)
	require.NotNil(t, resp)

	parsed, parseErr := url.Parse(resp.URL)
	require.NoError(t, parseErr)
	require.Equal(t, "https", parsed.Scheme)
	require.Equal(t, "officecli.io", parsed.Host)
	require.Equal(t, "/officesdk/proxy/download", parsed.Path)
	require.Equal(t, "preview/file-fallback-2/original/demo.pptx", parsed.Query().Get("key"))
	require.NotEmpty(t, parsed.Query().Get("access_token"))
}
