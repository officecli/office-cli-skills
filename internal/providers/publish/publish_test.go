package publish

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewPublisherUsesPlatformPublishEndpoint(t *testing.T) {
	originalBaseURL := EmbeddedPublishBaseURL
	defer func() { EmbeddedPublishBaseURL = originalBaseURL }()

	var authHeader string
	var requestPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		authHeader = r.Header.Get("Authorization")
		_, _ = io.WriteString(w, `{"access_url":"https://officecli.io/p/share-1","password":"","file_id":"file-1"}`)
	}))
	defer server.Close()

	EmbeddedPublishBaseURL = server.URL
	publisher, err := NewPublisher(Config{Enabled: true, APIKey: "publish-key"})
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "deck.pptx")
	if err := os.WriteFile(filePath, []byte("pptx"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	result, err := publisher.Publish(context.Background(), PublishRequest{
		LocalFilePath: filePath,
		DocumentType:  "pptx",
		DocumentName:  "Shanghai and Beijing",
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if requestPath != "/api/publish" {
		t.Fatalf("request path = %q", requestPath)
	}
	if authHeader != "Bearer publish-key" {
		t.Fatalf("authorization = %q", authHeader)
	}
	if result.AccessURL != "https://officecli.io/p/share-1" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestValidateConfigRequiresAPIKey(t *testing.T) {
	err := ValidateConfig(Config{Enabled: true, BaseURL: "https://platform.officecli.io"})
	if err == nil || !strings.Contains(err.Error(), "online preview publishing credential") {
		t.Fatalf("expected API key validation error, got %v", err)
	}
}
