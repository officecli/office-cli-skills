package publish

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/officecli/officecli/platform/internal/model"
)

type fakeAPIKeyStore struct {
	key       *model.APIKey
	touched   bool
	touchedID uint64
}

func (f *fakeAPIKeyStore) FindByHash(_ context.Context, _ string) (*model.APIKey, error) {
	if f.key == nil {
		return nil, nil
	}
	cloned := *f.key
	return &cloned, nil
}

func (f *fakeAPIKeyStore) TouchLastUsedAt(_ context.Context, id uint64, _ time.Time) error {
	f.touched = true
	f.touchedID = id
	return nil
}

func TestAuthorizeRejectsZeroEntitlementKey(t *testing.T) {
	svc := NewService(&fakeAPIKeyStore{
		key: &model.APIKey{
			ID:            1,
			Status:        model.APIKeyStatusActive,
			PlanName:      "Starter",
			KeyPrefix:     "cop_test",
			AllowedModes:  "external_only",
			HostedEnabled: false,
		},
	}, Config{HashSalt: "salt"})

	key, err := svc.authorize(context.Background(), "Bearer demo")
	require.Error(t, err)
	require.Nil(t, key)
	require.Contains(t, err.Error(), "publish entitlement is required")
}

func TestAuthorizeAcceptsPaidQuotaKey(t *testing.T) {
	quotaTotal := 100
	svc := NewService(&fakeAPIKeyStore{
		key: &model.APIKey{
			ID:            1,
			Status:        model.APIKeyStatusActive,
			PlanName:      "Growth",
			KeyPrefix:     "cop_paid",
			AllowedModes:  "external_only",
			HostedEnabled: false,
			QuotaTotal:    &quotaTotal,
		},
	}, Config{HashSalt: "salt"})

	key, err := svc.authorize(context.Background(), "Bearer demo")
	require.NoError(t, err)
	require.NotNil(t, key)
}

func TestAuthorizeAcceptsHostedCreditKey(t *testing.T) {
	svc := NewService(&fakeAPIKeyStore{
		key: &model.APIKey{
			ID:            2,
			Status:        model.APIKeyStatusActive,
			PlanName:      "Hosted",
			KeyPrefix:     "cop_hosted",
			AllowedModes:  "hybrid",
			HostedEnabled: true,
			CreditBalance: 200,
		},
	}, Config{HashSalt: "salt"})

	key, err := svc.authorize(context.Background(), "Bearer demo")
	require.NoError(t, err)
	require.NotNil(t, key)
}

func TestUploadAttachmentUsesDynamicSignatureHeaders(t *testing.T) {
	store := &fakeAPIKeyStore{key: &model.APIKey{ID: 1, Status: model.APIKeyStatusActive, QuotaTotal: intPtr(1)}}
	var gotKeyID string
	var gotTimestamp string
	var gotSignature string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKeyID = r.Header.Get("X-Auth-Key-Id")
		gotTimestamp = r.Header.Get("X-Auth-Timestamp")
		gotSignature = r.Header.Get("X-Auth-Signature")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
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
	_, err := svc.uploadAttachment(context.Background(), Request{
		FileName:     "demo.docx",
		DocumentType: "docx",
		DocumentName: "demo.docx",
		ContentType:  "application/octet-stream",
		Reader:       strings.NewReader("hello"),
	})
	require.NoError(t, err)
	require.Equal(t, "platform-prod", gotKeyID)
	require.NotEmpty(t, gotTimestamp)
	require.NotEmpty(t, gotSignature)
}

func TestUploadAttachmentFallsBackToLegacyAuthKey(t *testing.T) {
	store := &fakeAPIKeyStore{key: &model.APIKey{ID: 1, Status: model.APIKeyStatusActive, QuotaTotal: intPtr(1)}}
	var gotLegacy string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotLegacy = r.Header.Get("X-Auth-Key")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
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
	_, err := svc.uploadAttachment(context.Background(), Request{
		FileName:     "demo.docx",
		DocumentType: "docx",
		DocumentName: "demo.docx",
		ContentType:  "application/octet-stream",
		Reader:       strings.NewReader("hello"),
	})
	require.NoError(t, err)
	require.Equal(t, "legacy-key", gotLegacy)
}

func TestPublishUsesServerSideAttachmentUpload(t *testing.T) {
	quotaTotal := 100
	var uploaded bytes.Buffer
	var uploadAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/attachment/upload":
			uploadAuth = r.Header.Get("X-Auth-Key")
			if !strings.Contains(r.Header.Get("Content-Type"), "multipart/form-data") {
				http.Error(w, "bad content type", http.StatusBadRequest)
				return
			}
			if err := r.ParseMultipartForm(2 << 20); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			file, _, err := r.FormFile("file")
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			defer file.Close()
			if _, err := io.Copy(&uploaded, file); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]string{
					"storage_key": "attachments/20260408/demo.pptx",
				},
			})
		case "/api/preview-shares":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_url": "https://claudeoffice.com/preview/s/share-1",
				"password":   "123456",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	svc := NewService(&fakeAPIKeyStore{
		key: &model.APIKey{
			ID:            1,
			Status:        model.APIKeyStatusActive,
			PlanName:      "Growth",
			KeyPrefix:     "cop_paid",
			AllowedModes:  "external_only",
			HostedEnabled: false,
			QuotaTotal:    &quotaTotal,
		},
	}, Config{BaseURL: server.URL, AuthKey: "claudeoffice-auth", HashSalt: "salt"})

	result, err := svc.Publish(context.Background(), "Bearer demo", Request{
		FileName:     "demo.pptx",
		DocumentType: "pptx",
		DocumentName: "demo.pptx",
		ContentType:  "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		Reader:       bytes.NewReader([]byte("pptx-bytes")),
	})
	require.NoError(t, err)
	require.Equal(t, "https://claudeoffice.com/preview/s/share-1", result.AccessURL)
	require.Equal(t, "123456", result.Password)
	require.Equal(t, "claudeoffice-auth", uploadAuth)
	require.Equal(t, "pptx-bytes", uploaded.String())
}

func TestPublishSignsPreviewRequests(t *testing.T) {
	store := &fakeAPIKeyStore{key: &model.APIKey{ID: 7, Status: model.APIKeyStatusActive, QuotaTotal: intPtr(1)}}
	var seenUpload bool
	var seenPreviewShare bool
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/attachment/upload":
			seenUpload = r.Header.Get("X-Auth-Signature") != ""
			if err := r.ParseMultipartForm(2 << 20); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"storage_key": "attachments/demo.docx",
				},
			})
		case "/api/preview-shares":
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
	require.NoError(t, err)
	require.True(t, seenUpload)
	require.True(t, seenPreviewShare)
	require.True(t, store.touched)
	require.Equal(t, uint64(7), store.touchedID)
}

func intPtr(v int) *int { return &v }
