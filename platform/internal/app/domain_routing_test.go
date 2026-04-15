package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestLoadConfigDefaultsUseProductionDomains(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("GOOGLE_REDIRECT_URL", "")
	t.Setenv("STRIPE_SUCCESS_URL", "")
	t.Setenv("STRIPE_CANCEL_URL", "")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if cfg.GoogleRedirectURL != "https://platform.officecli.io/api/auth/google/callback" {
		t.Fatalf("GoogleRedirectURL = %q", cfg.GoogleRedirectURL)
	}
	if cfg.StripeSuccessURL != "https://platform.officecli.io/app/billing?status=success" {
		t.Fatalf("StripeSuccessURL = %q", cfg.StripeSuccessURL)
	}
	if cfg.StripeCancelURL != "https://platform.officecli.io/app/billing?status=cancel" {
		t.Fatalf("StripeCancelURL = %q", cfg.StripeCancelURL)
	}
}

func TestRegisterStaticRedirectsPlatformRootToApp(t *testing.T) {
	gin.SetMode(gin.TestMode)

	root := t.TempDir()
	adminDir := writeIndex(t, filepath.Join(root, "admin"), "admin")
	appDir := writeIndex(t, filepath.Join(root, "app"), "app")
	siteDir := writeIndex(t, filepath.Join(root, "site"), "site")

	engine := gin.New()
	registerStatic(engine, Config{
		AdminStaticDir:  adminDir,
		AppStaticDir:    appDir,
		SiteStaticDir:   siteDir,
		SiteBaseURL:     "https://officecli.io",
		PlatformBaseURL: "https://platform.officecli.io",
	})

	req := httptest.NewRequest(http.MethodGet, "http://platform.officecli.io/", nil)
	req.Host = "platform.officecli.io"
	rec := httptest.NewRecorder()

	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound && rec.Code != http.StatusMovedPermanently && rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if location := rec.Header().Get("Location"); location != "/app" && location != "https://platform.officecli.io/app" {
		t.Fatalf("Location = %q", location)
	}
}

func TestRegisterStaticServesAdminLoginThroughSPA(t *testing.T) {
	gin.SetMode(gin.TestMode)

	root := t.TempDir()
	adminDir := writeIndex(t, filepath.Join(root, "admin"), "admin")
	appDir := writeIndex(t, filepath.Join(root, "app"), "app")
	siteDir := writeIndex(t, filepath.Join(root, "site"), "site")

	engine := gin.New()
	registerStatic(engine, Config{
		AdminStaticDir:  adminDir,
		AppStaticDir:    appDir,
		SiteStaticDir:   siteDir,
		SiteBaseURL:     "https://officecli.io",
		PlatformBaseURL: "https://platform.officecli.io",
	})

	req := httptest.NewRequest(http.MethodGet, "http://platform.officecli.io/admin/login", nil)
	req.Host = "platform.officecli.io"
	rec := httptest.NewRecorder()

	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); body != "admin" {
		t.Fatalf("body = %q", body)
	}
}

func TestRegisterStaticStillServesAccessDenied(t *testing.T) {
	gin.SetMode(gin.TestMode)

	root := t.TempDir()
	adminDir := writeIndex(t, filepath.Join(root, "admin"), "admin")
	appDir := writeIndex(t, filepath.Join(root, "app"), "app")
	siteDir := writeIndex(t, filepath.Join(root, "site"), "site")

	engine := gin.New()
	registerStatic(engine, Config{
		AdminStaticDir:  adminDir,
		AppStaticDir:    appDir,
		SiteStaticDir:   siteDir,
		SiteBaseURL:     "https://officecli.io",
		PlatformBaseURL: "https://platform.officecli.io",
	})

	req := httptest.NewRequest(http.MethodGet, "http://platform.officecli.io/admin/access-denied?email=blocked@example.com", nil)
	req.Host = "platform.officecli.io"
	rec := httptest.NewRecorder()

	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); body != "admin" {
		t.Fatalf("body = %q", body)
	}
}

func TestRegisterStaticServesAppAccessDeniedThroughSPA(t *testing.T) {
	gin.SetMode(gin.TestMode)

	root := t.TempDir()
	adminDir := writeIndex(t, filepath.Join(root, "admin"), "admin")
	appDir := writeIndex(t, filepath.Join(root, "app"), "app")
	siteDir := writeIndex(t, filepath.Join(root, "site"), "site")

	engine := gin.New()
	registerStatic(engine, Config{
		AdminStaticDir:  adminDir,
		AppStaticDir:    appDir,
		SiteStaticDir:   siteDir,
		SiteBaseURL:     "https://officecli.io",
		PlatformBaseURL: "https://platform.officecli.io",
	})

	req := httptest.NewRequest(http.MethodGet, "http://platform.officecli.io/app/access-denied?email=blocked@example.com", nil)
	req.Host = "platform.officecli.io"
	rec := httptest.NewRecorder()

	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); body != "app" {
		t.Fatalf("body = %q", body)
	}
}

func TestRegisterStaticServesSiteRootFiles(t *testing.T) {
	gin.SetMode(gin.TestMode)

	root := t.TempDir()
	adminDir := writeIndex(t, filepath.Join(root, "admin"), "admin")
	appDir := writeIndex(t, filepath.Join(root, "app"), "app")
	siteDir := writeIndex(t, filepath.Join(root, "site"), "site")
	writeFile(t, siteDir, "robots.txt", "User-agent: *")
	writeFile(t, siteDir, "sitemap.xml", "<urlset/>")
	writeFile(t, siteDir, "favicon.svg", "<svg/>")

	engine := gin.New()
	registerStatic(engine, Config{
		AdminStaticDir:  adminDir,
		AppStaticDir:    appDir,
		SiteStaticDir:   siteDir,
		SiteBaseURL:     "https://officecli.io",
		PlatformBaseURL: "https://platform.officecli.io",
	})

	for _, tc := range []struct {
		path string
		want string
	}{
		{path: "/robots.txt", want: "User-agent: *"},
		{path: "/sitemap.xml", want: "<urlset/>"},
		{path: "/favicon.svg", want: "<svg/>"},
	} {
		req := httptest.NewRequest(http.MethodGet, "http://officecli.io"+tc.path, nil)
		req.Host = "officecli.io"
		rec := httptest.NewRecorder()

		engine.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, body = %s", tc.path, rec.Code, rec.Body.String())
		}
		if body := rec.Body.String(); body != tc.want {
			t.Fatalf("%s body = %q", tc.path, body)
		}
	}
}

func writeIndex(t *testing.T, dir, label string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", dir, err)
	}
	indexPath := filepath.Join(dir, "index.html")
	if err := os.WriteFile(indexPath, []byte(label), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", indexPath, err)
	}
	return dir
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", name, err)
	}
}
