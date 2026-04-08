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
	key *model.APIKey
}

func (f *fakeAPIKeyStore) FindByHash(_ context.Context, _ string) (*model.APIKey, error) {
	if f.key == nil {
		return nil, nil
	}
	cloned := *f.key
	return &cloned, nil
}

func (f *fakeAPIKeyStore) TouchLastUsedAt(_ context.Context, _ uint64, _ time.Time) error {
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
