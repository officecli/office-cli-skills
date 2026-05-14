package app

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gotomicro/ego/server/egin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/officecli/officecli-internal/platform/internal/officesdk"
	"github.com/officecli/officecli-internal/platform/internal/previewshare"
)

type fakePreviewRouteObjectStore struct {
	objects map[string][]byte
}

func (f *fakePreviewRouteObjectStore) PutObject(_ context.Context, key string, reader io.Reader, _ int64, _ string) error {
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

func (f *fakePreviewRouteObjectStore) GetObject(_ context.Context, key string) (io.ReadCloser, error) {
	if data, ok := f.objects[key]; ok {
		return io.NopCloser(bytes.NewReader(data)), nil
	}
	return nil, io.EOF
}

func (f *fakePreviewRouteObjectStore) DeleteObject(_ context.Context, key string) error {
	delete(f.objects, key)
	return nil
}

func newPreviewShareService(t *testing.T) *previewshare.Service {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), url.QueryEscape(t.Name())+".db")
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s", dbPath)), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&previewshare.PreviewShare{}))
	return previewshare.NewService(db, "secret", ".officecli.io", nil, nil)
}

func TestRegisterPreviewRoutesRendersLoginPromptWithoutAppSession(t *testing.T) {
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
		PlatformBaseURL: "https://platform.officecli.io",
		OfficeSDKHost:   "http://127.0.0.1:19101",
	}, &fakeAuthRouteService{}, shares, sdkHandler, sdkProvider)

	req := httptest.NewRequest(http.MethodGet, "/p/"+share.ShareToken, nil)
	req.Host = "officecli.io"
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	component.Engine.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Header().Get("Content-Type"), "text/html")
	require.Contains(t, rec.Body.String(), "Sign In To Open Preview")
	require.Contains(t, rec.Body.String(), "Continue To Sign In")
	require.Contains(t, rec.Body.String(), "return_to=https%3A%2F%2Fofficecli.io%2Fp%2F"+share.ShareToken)
}

func TestRegisterPreviewRoutesRendersPasswordPageForSignedInUserWithoutAccessCookie(t *testing.T) {
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
		PlatformBaseURL: "https://platform.officecli.io",
		OfficeSDKHost:   "http://127.0.0.1:19101",
	}, &fakeAuthRouteService{}, shares, sdkHandler, sdkProvider)

	req := httptest.NewRequest(http.MethodGet, "/p/"+share.ShareToken, nil)
	req.Host = "officecli.io"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.AddCookie(&http.Cookie{Name: "cop_app_session", Value: "existing"})
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
	req.AddCookie(&http.Cookie{Name: "cop_app_session", Value: "existing"})
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
	req.AddCookie(&http.Cookie{Name: "cop_app_session", Value: "existing"})
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

func TestRegisterPreviewRoutesServesSandboxedReportPreviewAfterPasswordSubmit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	shares := newPreviewShareService(t)
	result, err := shares.CreateWithPassword(t.Context(), previewshare.CreateParams{
		FileID:     "file-report-1",
		StorageKey: "preview/file-report-1/original/q2-business-review.html",
		FileName:   "q2-business-review.html",
		FileType:   "report",
		ExpiresAt:  time.Now().UTC().Add(time.Hour),
	})
	require.NoError(t, err)
	share := result.Share

	objects := &fakePreviewRouteObjectStore{
		objects: map[string][]byte{
			share.StorageKey: []byte(`<!DOCTYPE html><html lang="en"><body><h1>Q2 Business Review</h1><script>window.__reportLoaded = true;</script></body></html>`),
		},
	}
	sdkStore := officesdk.NewFileStore(nil)
	sdkProvider := officesdk.NewFileProvider(sdkStore, objects, shares)
	sdkHandler := officesdk.NewHandler(sdkStore, sdkProvider, "https://officecli.io/sdk/turbo-ai", "secret")
	component := egin.DefaultContainer().Build()
	registerPreviewRoutes(component, Config{
		OfficeSDKHost: "http://127.0.0.1:19101",
	}, &fakeAuthRouteService{}, shares, sdkHandler, sdkProvider)

	req := httptest.NewRequest(http.MethodPost, "/p/"+share.ShareToken, strings.NewReader("password="+result.Password))
	req.Host = "officecli.io"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "cop_app_session", Value: "existing"})
	rec := httptest.NewRecorder()
	component.Engine.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Header().Get("Content-Type"), "text/html")
	require.Contains(t, rec.Body.String(), `sandbox="allow-scripts allow-downloads"`)
	require.Contains(t, rec.Body.String(), "Loading report preview...")
	require.Contains(t, rec.Body.String(), "Q2 Business Review")
	require.NotContains(t, rec.Body.String(), "OfficeCLI Preview")
}

func TestRegisterPreviewRoutesServesSandboxedReportPreviewWithExistingCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	shares := newPreviewShareService(t)
	result, err := shares.CreateWithPassword(t.Context(), previewshare.CreateParams{
		FileID:     "file-report-2",
		StorageKey: "preview/file-report-2/original/q2-business-review.html",
		FileName:   "q2-business-review.html",
		FileType:   "report",
		ExpiresAt:  time.Now().UTC().Add(time.Hour),
	})
	require.NoError(t, err)

	objects := &fakePreviewRouteObjectStore{
		objects: map[string][]byte{
			result.Share.StorageKey: []byte(`<!DOCTYPE html><html lang="en"><body><h1>Demand momentum</h1></body></html>`),
		},
	}
	sdkStore := officesdk.NewFileStore(nil)
	sdkProvider := officesdk.NewFileProvider(sdkStore, objects, shares)
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
	require.Contains(t, rec.Body.String(), `sandbox="allow-scripts allow-downloads"`)
	require.Contains(t, rec.Body.String(), "Demand momentum")
}

func TestRegisterPreviewRoutesServesImagePreviewAfterPasswordSubmit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	shares := newPreviewShareService(t)
	result, err := shares.CreateWithPassword(t.Context(), previewshare.CreateParams{
		FileID:     "file-img-1",
		StorageKey: "preview/file-img-1/original/launch-visual.png",
		FileName:   "launch-visual.png",
		FileType:   "img",
		ExpiresAt:  time.Now().UTC().Add(time.Hour),
	})
	require.NoError(t, err)
	share := result.Share

	objects := &fakePreviewRouteObjectStore{
		objects: map[string][]byte{
			share.StorageKey: []byte("png-bytes"),
		},
	}
	sdkStore := officesdk.NewFileStore(nil)
	sdkProvider := officesdk.NewFileProvider(sdkStore, objects, shares)
	sdkHandler := officesdk.NewHandler(sdkStore, sdkProvider, "https://officecli.io/sdk/turbo-ai", "secret")
	component := egin.DefaultContainer().Build()
	registerPreviewRoutes(component, Config{
		OfficeSDKHost: "http://127.0.0.1:19101",
	}, &fakeAuthRouteService{}, shares, sdkHandler, sdkProvider)

	req := httptest.NewRequest(http.MethodPost, "/p/"+share.ShareToken, strings.NewReader("password="+result.Password))
	req.Host = "officecli.io"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "cop_app_session", Value: "existing"})
	rec := httptest.NewRecorder()
	component.Engine.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Header().Get("Content-Type"), "text/html")
	require.Contains(t, rec.Body.String(), "OfficeCLI Image Preview")
	require.Contains(t, rec.Body.String(), "/p/"+share.ShareToken+"/raw")
	require.Contains(t, rec.Body.String(), "download=1")
	require.True(t, strings.Contains(strings.Join(rec.Header().Values("Set-Cookie"), "\n"), "cop_preview_access="))
}

func TestRegisterPreviewRoutesImageRawRequiresPreviewAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	shares := newPreviewShareService(t)
	result, err := shares.CreateWithPassword(t.Context(), previewshare.CreateParams{
		FileID:     "file-img-2",
		StorageKey: "preview/file-img-2/original/launch-visual.png",
		FileName:   "launch-visual.png",
		FileType:   "img",
		ExpiresAt:  time.Now().UTC().Add(time.Hour),
	})
	require.NoError(t, err)

	objects := &fakePreviewRouteObjectStore{
		objects: map[string][]byte{
			result.Share.StorageKey: []byte("png-bytes"),
		},
	}
	sdkStore := officesdk.NewFileStore(nil)
	sdkProvider := officesdk.NewFileProvider(sdkStore, objects, shares)
	sdkHandler := officesdk.NewHandler(sdkStore, sdkProvider, "https://officecli.io/sdk/turbo-ai", "secret")
	component := egin.DefaultContainer().Build()
	registerPreviewRoutes(component, Config{
		OfficeSDKHost: "http://127.0.0.1:19101",
	}, &fakeAuthRouteService{}, shares, sdkHandler, sdkProvider)

	unauthorizedReq := httptest.NewRequest(http.MethodGet, "/p/"+result.Share.ShareToken+"/raw", nil)
	unauthorizedReq.Host = "officecli.io"
	unauthorizedReq.Header.Set("X-Forwarded-Proto", "https")
	unauthorizedRec := httptest.NewRecorder()
	component.Engine.ServeHTTP(unauthorizedRec, unauthorizedReq)
	require.Equal(t, http.StatusUnauthorized, unauthorizedRec.Code)

	issueRec := httptest.NewRecorder()
	issueCtx, _ := gin.CreateTestContext(issueRec)
	issueCtx.Request = httptest.NewRequest(http.MethodGet, "/p/"+result.Share.ShareToken, nil)
	issueCtx.Request.Host = "officecli.io"
	issueCtx.Request.Header.Set("X-Forwarded-Proto", "https")
	shares.IssueAccessCookie(issueCtx, result.Share)

	req := httptest.NewRequest(http.MethodGet, "/p/"+result.Share.ShareToken+"/raw", nil)
	req.Host = "officecli.io"
	req.Header.Set("X-Forwarded-Proto", "https")
	for _, cookie := range issueRec.Result().Cookies() {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	component.Engine.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "image/png", rec.Header().Get("Content-Type"))
	require.Equal(t, "inline", rec.Header().Get("Content-Disposition"))
	require.Equal(t, "png-bytes", rec.Body.String())
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

func TestRegisterOfficeSDKProxyForwardsPublicHostAndProtoToUpstream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Upstream-Host", r.Host)
		w.Header().Set("X-Upstream-Forwarded-Host", r.Header.Get("X-Forwarded-Host"))
		w.Header().Set("X-Upstream-Forwarded-Proto", r.Header.Get("X-Forwarded-Proto"))
		w.Header().Set("X-Upstream-Forwarded-Port", r.Header.Get("X-Forwarded-Port"))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	component := egin.DefaultContainer().Build()
	registerOfficeSDKProxy(component.Engine, Config{OfficeSDKHost: upstream.URL})
	server := httptest.NewServer(component.Engine)
	defer server.Close()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/sdk/turbo-ai/files/file-1/content", nil)
	require.NoError(t, err)
	req.Host = "host.docker.internal:29001"
	req.Header.Set("X-Forwarded-Host", "officecli.io")
	req.Header.Set("X-Forwarded-Proto", "https")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	require.Equal(t, "officecli.io", resp.Header.Get("X-Upstream-Host"))
	require.Equal(t, "officecli.io", resp.Header.Get("X-Upstream-Forwarded-Host"))
	require.Equal(t, "https", resp.Header.Get("X-Upstream-Forwarded-Proto"))
	require.Equal(t, "443", resp.Header.Get("X-Upstream-Forwarded-Port"))
}
