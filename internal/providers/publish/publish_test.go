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
		DocumentName:  "Shanghai and Beijing",
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

func TestNewPublisherNormalizesEmbeddedBaseURLWithAPISuffix(t *testing.T) {
	originalBaseURL := EmbeddedPublishBaseURL
	originalKeyID := EmbeddedPublishAuthKeyID
	originalAuthKey := EmbeddedPublishAuthKey
	defer func() {
		EmbeddedPublishBaseURL = originalBaseURL
		EmbeddedPublishAuthKeyID = originalKeyID
		EmbeddedPublishAuthKey = originalAuthKey
	}()

	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/api/attachment/upload":
			_, _ = io.WriteString(w, `{"data":{"storage_key":"store-1"}}`)
		case "/api/preview-shares":
			_, _ = io.WriteString(w, `{"access_url":"https://claudeoffice.com/preview/t/1","password":"123456"}`)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	EmbeddedPublishBaseURL = "https://claudeoffice.com"
	EmbeddedPublishAuthKeyID = "embedded-key-id"
	EmbeddedPublishAuthKey = "embedded-secret"

	publisher, err := NewPublisher(Config{
		Enabled: true,
		BaseURL: server.URL + "/api",
	})
	if err != nil {
		t.Fatalf("NewPublisher: %v", err)
	}
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "deck.pptx")
	if err := os.WriteFile(filePath, []byte("pptx"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := publisher.Publish(context.Background(), PublishRequest{
		LocalFilePath: filePath,
		DocumentType:  "pptx",
		DocumentName:  "Shanghai and Beijing",
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	if len(paths) != 2 {
		t.Fatalf("expected 2 embedded publish requests, got %d (%v)", len(paths), paths)
	}
	for _, path := range paths {
		if strings.HasPrefix(path, "/api/api/") {
			t.Fatalf("expected embedded base URL normalization, got path %s", path)
		}
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
	if err == nil || !strings.Contains(err.Error(), "online preview publishing credential") {
		t.Fatalf("expected API key validation error, got %v", err)
	}
}
