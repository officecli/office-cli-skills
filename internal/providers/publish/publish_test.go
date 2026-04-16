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

func TestPublisherSendsReportDocumentType(t *testing.T) {
	originalBaseURL := EmbeddedPublishBaseURL
	defer func() { EmbeddedPublishBaseURL = originalBaseURL }()

	var gotDocumentType string
	var gotDocumentName string
	var gotFileBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("ParseMultipartForm: %v", err)
		}
		gotDocumentType = r.FormValue("document_type")
		gotDocumentName = r.FormValue("document_name")
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("FormFile: %v", err)
		}
		defer file.Close()
		data, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		gotFileBody = string(data)
		_, _ = io.WriteString(w, `{"access_url":"https://officecli.io/p/share-report","password":"654321","file_id":"file-report"}`)
	}))
	defer server.Close()

	EmbeddedPublishBaseURL = server.URL
	publisher, err := NewPublisher(Config{Enabled: true, APIKey: "publish-key"})
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "q2-business-review.html")
	if err := os.WriteFile(filePath, []byte("<html><body>report</body></html>"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	result, err := publisher.Publish(context.Background(), PublishRequest{
		LocalFilePath: filePath,
		DocumentType:  "report",
		DocumentName:  "Q2 Business Review.html",
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if gotDocumentType != "report" {
		t.Fatalf("document_type = %q", gotDocumentType)
	}
	if gotDocumentName != "Q2 Business Review.html" {
		t.Fatalf("document_name = %q", gotDocumentName)
	}
	if gotFileBody != "<html><body>report</body></html>" {
		t.Fatalf("file body = %q", gotFileBody)
	}
	if result.AccessURL != "https://officecli.io/p/share-report" {
		t.Fatalf("unexpected result: %+v", result)
	}
}
