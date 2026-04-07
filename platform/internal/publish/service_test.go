package publish

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/officecli/officecli/platform/internal/model"
)

type fakeAPIKeyStore struct {
	key      *model.APIKey
	touched  bool
	touchedID uint64
}

func (f *fakeAPIKeyStore) FindByHash(_ context.Context, _ string) (*model.APIKey, error) {
	return f.key, nil
}

func (f *fakeAPIKeyStore) TouchLastUsedAt(_ context.Context, id uint64, _ time.Time) error {
	f.touched = true
	f.touchedID = id
	return nil
}

func TestServicePostJSONUsesDynamicSignatureHeaders(t *testing.T) {
	store := &fakeAPIKeyStore{key: &model.APIKey{ID: 1, Status: model.APIKeyStatusActive, QuotaTotal: intPtr(1)}}
	var gotKeyID string
	var gotTimestamp string
	var gotSignature string
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKeyID = r.Header.Get("X-Auth-Key-Id")
		gotTimestamp = r.Header.Get("X-Auth-Timestamp")
		gotSignature = r.Header.Get("X-Auth-Signature")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"upload_url":  server.URL + "/upload",
				"storage_key": "attachments/demo.docx",
			},
		})
	}))
	defer server.Close()

	svc := NewService(store, Config{
		BaseURL:          server.URL,
		AuthKeyID:        "platform-prod",
		AuthSharedSecret: "shared-secret",
		HashSalt:         "salt",
	})
	if _, err := svc.requestUploadToken(context.Background(), "demo.docx"); err != nil {
		t.Fatalf("requestUploadToken() error = %v", err)
	}
	if gotKeyID != "platform-prod" || gotTimestamp == "" || gotSignature == "" {
		t.Fatalf("dynamic auth headers missing: keyID=%q ts=%q sig=%q", gotKeyID, gotTimestamp, gotSignature)
	}
}

func TestServicePostJSONFallsBackToLegacyAuthKey(t *testing.T) {
	store := &fakeAPIKeyStore{key: &model.APIKey{ID: 1, Status: model.APIKeyStatusActive, QuotaTotal: intPtr(1)}}
	var gotLegacy string
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotLegacy = r.Header.Get("X-Auth-Key")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"upload_url":  server.URL + "/upload",
				"storage_key": "attachments/demo.docx",
			},
		})
	}))
	defer server.Close()

	svc := NewService(store, Config{
		BaseURL:  server.URL,
		AuthKey:  "legacy-key",
		HashSalt: "salt",
	})
	if _, err := svc.requestUploadToken(context.Background(), "demo.docx"); err != nil {
		t.Fatalf("requestUploadToken() error = %v", err)
	}
	if gotLegacy != "legacy-key" {
		t.Fatalf("legacy auth header = %q", gotLegacy)
	}
}

func TestServicePublishSignsPreviewRequests(t *testing.T) {
	store := &fakeAPIKeyStore{key: &model.APIKey{ID: 7, Status: model.APIKeyStatusActive, QuotaTotal: intPtr(1)}}
	var seenUploadToken bool
	var seenPreviewShare bool
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/attachment/upload/token":
			seenUploadToken = r.Header.Get("X-Auth-Signature") != ""
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"upload_url":  server.URL + "/upload",
					"storage_key": "attachments/demo.docx",
				},
			})
		case r.URL.Path == "/upload":
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/api/preview-shares":
			seenPreviewShare = r.Header.Get("X-Auth-Signature") != ""
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_url": server.URL + "/preview/s/1",
				"password":   "123456",
				"file_id":    "file-1",
				"expires_at": time.Now().UTC(),
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	svc := NewService(store, Config{
		BaseURL:              server.URL,
		AuthKeyID:            "platform-prod",
		AuthSharedSecret:     "shared-secret",
		HashSalt:             "salt",
		DefaultExpireSeconds: 60,
	})
	_, err := svc.Publish(context.Background(), "Bearer user-key", Request{
		FileName:     "demo.docx",
		DocumentType: "docx",
		DocumentName: "demo.docx",
		ContentType:  "application/octet-stream",
		Reader:       strings.NewReader("hello"),
	})
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if !seenUploadToken || !seenPreviewShare {
		t.Fatalf("dynamic signing missing: upload=%v preview=%v", seenUploadToken, seenPreviewShare)
	}
	if !store.touched || store.touchedID != 7 {
		t.Fatalf("TouchLastUsedAt not called: touched=%v id=%d", store.touched, store.touchedID)
	}
}

func intPtr(v int) *int { return &v }
