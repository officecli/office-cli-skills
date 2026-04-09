package publish

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewPublisherUsesEmbeddedDynamicAuthWithoutAPIKey(t *testing.T) {
	originalBaseURL := EmbeddedPublishBaseURL
	originalKeyID := EmbeddedPublishAuthKeyID
	originalAuthKey := EmbeddedPublishAuthKey
	defer func() {
		EmbeddedPublishBaseURL = originalBaseURL
		EmbeddedPublishAuthKeyID = originalKeyID
		EmbeddedPublishAuthKey = originalAuthKey
	}()

	var sawUploadAuth bool
	var previewPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/attachment/upload":
			if r.Header.Get("X-Auth-Key-Id") == "embedded-key-id" &&
				r.Header.Get("X-Auth-Timestamp") != "" &&
				r.Header.Get("X-Auth-Nonce") != "" &&
				r.Header.Get("X-Auth-Signature") != "" &&
				r.Header.Get("Authorization") == "" {
				sawUploadAuth = true
			}
			_, _ = io.WriteString(w, `{"data":{"storage_key":"store-1"}}`)
		case "/api/preview-shares":
			if err := json.NewDecoder(r.Body).Decode(&previewPayload); err != nil {
				t.Fatalf("decode preview payload: %v", err)
			}
			_, _ = io.WriteString(w, `{"access_url":"https://claudeoffice.com/preview/t/1","password":"123456"}`)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	EmbeddedPublishBaseURL = server.URL
	EmbeddedPublishAuthKeyID = "embedded-key-id"
	EmbeddedPublishAuthKey = "embedded-secret"

	publisher, err := NewPublisher(Config{Enabled: true})
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
		DocumentName:  "上海和北京",
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if !sawUploadAuth {
		t.Fatal("expected dynamic auth headers on upload request")
	}
	if strings.TrimSpace(previewPayload["expires_at"].(string)) == "" {
		t.Fatalf("expected expires_at in preview payload: %#v", previewPayload)
	}
	if result.AccessURL == "" || result.Password == "" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestValidateConfigAllowsEmbeddedDynamicAuthWithoutAPIKey(t *testing.T) {
	originalBaseURL := EmbeddedPublishBaseURL
	originalAuthKey := EmbeddedPublishAuthKey
	defer func() {
		EmbeddedPublishBaseURL = originalBaseURL
		EmbeddedPublishAuthKey = originalAuthKey
	}()

	EmbeddedPublishBaseURL = "https://claudeoffice.com"
	EmbeddedPublishAuthKey = "embedded-secret"

	if err := ValidateConfig(Config{Enabled: true}); err != nil {
		t.Fatalf("ValidateConfig: %v", err)
	}
}

func TestValidateConfigStillRequiresAPIKeyWithoutEmbeddedDynamicAuth(t *testing.T) {
	originalAuthKey := EmbeddedPublishAuthKey
	defer func() { EmbeddedPublishAuthKey = originalAuthKey }()
	EmbeddedPublishAuthKey = ""

	err := ValidateConfig(Config{Enabled: true, BaseURL: "https://publish.example.com"})
	if err == nil || !strings.Contains(err.Error(), "在线预览发布访问凭证") {
		t.Fatalf("expected API key validation error, got %v", err)
	}
}
