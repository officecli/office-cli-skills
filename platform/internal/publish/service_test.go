package publish

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/officecli/officecli/platform/internal/model"
	"github.com/officecli/officecli/platform/internal/officesdk"
	"github.com/officecli/officecli/platform/internal/previewshare"
)

type fakeAPIKeyStore struct {
	key              *model.APIKey
	session          *model.CLISession
	touched          bool
	touchedID        uint64
	sessionTouched   bool
	sessionTouchedID uint64
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

func (f *fakeAPIKeyStore) FindCLISessionByTokenHash(_ context.Context, _ string) (*model.CLISession, error) {
	if f.session == nil {
		return nil, nil
	}
	cloned := *f.session
	return &cloned, nil
}

func (f *fakeAPIKeyStore) TouchCLISession(_ context.Context, id uint64, _ time.Time) error {
	f.sessionTouched = true
	f.sessionTouchedID = id
	return nil
}

type fakeObjectStore struct {
	putKey         string
	putContentType string
	putData        []byte
	deletedKey     string
}

func (f *fakeObjectStore) PutObject(_ context.Context, key string, reader io.Reader, _ int64, contentType string) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	f.putKey = key
	f.putContentType = contentType
	f.putData = data
	return nil
}

func (f *fakeObjectStore) DeleteObject(_ context.Context, key string) error {
	f.deletedKey = key
	return nil
}

type fakeFileMetaStore struct {
	meta      *officesdk.FileMeta
	deletedID string
}

func (f *fakeFileMetaStore) SetFileMeta(_ context.Context, meta *officesdk.FileMeta) error {
	cloned := *meta
	f.meta = &cloned
	return nil
}

func (f *fakeFileMetaStore) DeleteFileMeta(_ context.Context, fileID string) error {
	f.deletedID = fileID
	return nil
}

type fakePreviewShareStore struct {
	share    *previewshare.PreviewShare
	password string
	got      previewshare.CreateParams
}

func (f *fakePreviewShareStore) CreateWithPassword(_ context.Context, params previewshare.CreateParams) (*previewshare.CreateResult, error) {
	f.got = params
	password := f.password
	if password == "" {
		password = "123456"
	}
	if f.share != nil {
		return &previewshare.CreateResult{Share: f.share, Password: password}, nil
	}
	return &previewshare.CreateResult{Share: &previewshare.PreviewShare{
		ShareToken: "share-1",
		FileID:     params.FileID,
		StorageKey: params.StorageKey,
		FileName:   params.FileName,
		FileType:   params.FileType,
		ExpiresAt:  params.ExpiresAt,
		Status:     previewshare.StatusActive,
	}, Password: password}, nil
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
	}, &fakeObjectStore{}, &fakeFileMetaStore{}, &fakePreviewShareStore{}, Config{HashSalt: "salt"})

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
	}, &fakeObjectStore{}, &fakeFileMetaStore{}, &fakePreviewShareStore{}, Config{HashSalt: "salt"})

	key, err := svc.authorize(context.Background(), "Bearer demo")
	require.NoError(t, err)
	require.NotNil(t, key)
}

func TestAuthorizeRejectsDisabledKey(t *testing.T) {
	quotaTotal := 100
	svc := NewService(&fakeAPIKeyStore{
		key: &model.APIKey{
			ID:            1,
			Status:        model.APIKeyStatusDisabled,
			PlanName:      "Growth",
			KeyPrefix:     "cop_paid",
			AllowedModes:  "external_only",
			HostedEnabled: false,
			QuotaTotal:    &quotaTotal,
		},
	}, &fakeObjectStore{}, &fakeFileMetaStore{}, &fakePreviewShareStore{}, Config{HashSalt: "salt"})

	key, err := svc.authorize(context.Background(), "Bearer demo")
	require.Error(t, err)
	require.Nil(t, key)
	require.Contains(t, err.Error(), "disabled")
}

func TestPublishCreatesLocalPreviewShare(t *testing.T) {
	quotaTotal := 100
	apiKeys := &fakeAPIKeyStore{
		key: &model.APIKey{
			ID:            7,
			Status:        model.APIKeyStatusActive,
			PlanName:      "Growth",
			KeyPrefix:     "cop_paid",
			AllowedModes:  "external_only",
			HostedEnabled: false,
			QuotaTotal:    &quotaTotal,
		},
	}
	objects := &fakeObjectStore{}
	files := &fakeFileMetaStore{}
	shares := &fakePreviewShareStore{}
	svc := NewService(apiKeys, objects, files, shares, Config{
		SiteBaseURL:          "https://officecli.io",
		HashSalt:             "salt",
		DefaultExpireSeconds: 3600,
	})

	result, err := svc.Publish(context.Background(), "Bearer demo", Request{
		FileName:     "demo",
		DocumentType: "pptx",
		DocumentName: "Quarterly deck",
		ContentType:  "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		Reader:       bytes.NewReader([]byte("pptx-bytes")),
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "https://officecli.io/p/share-1", result.AccessURL)
	require.Equal(t, "123456", result.Password)
	require.NotEmpty(t, result.FileID)
	require.NotNil(t, result.ExpiresAt)
	require.True(t, apiKeys.touched)
	require.Equal(t, uint64(7), apiKeys.touchedID)

	require.Contains(t, objects.putKey, "preview/")
	require.Equal(t, "pptx-bytes", string(objects.putData))
	require.Equal(t, "application/vnd.openxmlformats-officedocument.presentationml.presentation", objects.putContentType)

	require.NotNil(t, files.meta)
	require.Equal(t, result.FileID, files.meta.ID)
	require.Equal(t, "Quarterly deck.pptx", files.meta.Name)
	require.Equal(t, objects.putKey, files.meta.StorageKey)
	require.False(t, files.meta.FromSDK)

	require.Equal(t, result.FileID, shares.got.FileID)
	require.Equal(t, objects.putKey, shares.got.StorageKey)
	require.Equal(t, "Quarterly deck.pptx", shares.got.FileName)
	require.Equal(t, "pptx", shares.got.FileType)
}

func TestPublishAcceptsActiveCLISession(t *testing.T) {
	apiKeys := &fakeAPIKeyStore{
		session: &model.CLISession{
			ID:        17,
			UserID:    42,
			ExpiresAt: time.Now().UTC().Add(time.Hour),
		},
	}
	objects := &fakeObjectStore{}
	files := &fakeFileMetaStore{}
	shares := &fakePreviewShareStore{}
	svc := NewService(apiKeys, objects, files, shares, Config{
		SiteBaseURL:          "https://officecli.io",
		HashSalt:             "salt",
		DefaultExpireSeconds: 3600,
	})

	result, err := svc.Publish(context.Background(), "Bearer ocli_sess_current", Request{
		FileName:     "demo.png",
		DocumentType: "img",
		DocumentName: "Tea cup",
		ContentType:  "image/png",
		Reader:       bytes.NewReader([]byte("png-bytes")),
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "https://officecli.io/p/share-1", result.AccessURL)
	require.True(t, apiKeys.sessionTouched)
	require.Equal(t, uint64(17), apiKeys.sessionTouchedID)
	require.False(t, apiKeys.touched)
	require.NotNil(t, files.meta)
	require.Equal(t, "publish-user-42", files.meta.CreatorID)
	require.Equal(t, "publish-user-42", files.meta.ModifierID)
}

func TestPublishAcceptsReportHTML(t *testing.T) {
	quotaTotal := 100
	apiKeys := &fakeAPIKeyStore{
		key: &model.APIKey{
			ID:         8,
			Status:     model.APIKeyStatusActive,
			PlanName:   "Growth",
			KeyPrefix:  "cop_paid",
			QuotaTotal: &quotaTotal,
		},
	}
	objects := &fakeObjectStore{}
	files := &fakeFileMetaStore{}
	shares := &fakePreviewShareStore{}
	svc := NewService(apiKeys, objects, files, shares, Config{
		SiteBaseURL:          "https://officecli.io",
		HashSalt:             "salt",
		DefaultExpireSeconds: 3600,
	})

	result, err := svc.Publish(context.Background(), "Bearer demo", Request{
		FileName:     "q2-business-review.html",
		DocumentType: "report",
		DocumentName: "Q2 Business Review",
		ContentType:  "text/html; charset=utf-8",
		Reader:       bytes.NewReader([]byte("<html><body>report</body></html>")),
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "https://officecli.io/p/share-1", result.AccessURL)
	require.Equal(t, "Q2 Business Review.html", files.meta.Name)
	require.Equal(t, "report", shares.got.FileType)
	require.Equal(t, "text/html; charset=utf-8", objects.putContentType)
	require.Equal(t, "<html><body>report</body></html>", string(objects.putData))
}

func TestPublishAcceptsStandaloneImage(t *testing.T) {
	quotaTotal := 100
	apiKeys := &fakeAPIKeyStore{
		key: &model.APIKey{
			ID:         9,
			Status:     model.APIKeyStatusActive,
			PlanName:   "Growth",
			KeyPrefix:  "cop_paid",
			QuotaTotal: &quotaTotal,
		},
	}
	objects := &fakeObjectStore{}
	files := &fakeFileMetaStore{}
	shares := &fakePreviewShareStore{}
	svc := NewService(apiKeys, objects, files, shares, Config{
		SiteBaseURL:          "https://officecli.io",
		HashSalt:             "salt",
		DefaultExpireSeconds: 3600,
	})

	result, err := svc.Publish(context.Background(), "Bearer demo", Request{
		FileName:     "launch-visual",
		DocumentType: "img",
		DocumentName: "Launch Visual",
		ContentType:  "image/png",
		Reader:       bytes.NewReader([]byte("png-bytes")),
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "https://officecli.io/p/share-1", result.AccessURL)
	require.Equal(t, "Launch Visual.png", files.meta.Name)
	require.Equal(t, "img", shares.got.FileType)
	require.Contains(t, objects.putKey, "preview/")
	require.Contains(t, objects.putKey, "/original/Launch Visual.png")
	require.Equal(t, "image/png", objects.putContentType)
	require.Equal(t, "png-bytes", string(objects.putData))
}

func TestPublishRejectsMissingFileBytes(t *testing.T) {
	quotaTotal := 1
	svc := NewService(&fakeAPIKeyStore{
		key: &model.APIKey{ID: 1, Status: model.APIKeyStatusActive, QuotaTotal: &quotaTotal},
	}, &fakeObjectStore{}, &fakeFileMetaStore{}, &fakePreviewShareStore{}, Config{HashSalt: "salt"})

	_, err := svc.Publish(context.Background(), "Bearer demo", Request{
		FileName:     "demo.docx",
		DocumentType: "docx",
		DocumentName: "demo.docx",
		Reader:       bytes.NewReader(nil),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "file is required")
}

func intPtr(v int) *int { return &v }
