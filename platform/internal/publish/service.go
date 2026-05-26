package publish

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/officecli/officecli/platform/internal/clisession"
	"github.com/officecli/officecli/platform/internal/model"
	"github.com/officecli/officecli/platform/internal/officesdk"
	"github.com/officecli/officecli/platform/internal/previewshare"
)

type APIKeyStore interface {
	FindByHash(ctx context.Context, hash string) (*model.APIKey, error)
	TouchLastUsedAt(ctx context.Context, id uint64, usedAt time.Time) error
	FindCLISessionByTokenHash(ctx context.Context, tokenHash string) (*model.CLISession, error)
	TouchCLISession(ctx context.Context, id uint64, usedAt time.Time) error
}

type ObjectStore interface {
	PutObject(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error
	DeleteObject(ctx context.Context, key string) error
}

type FileMetaStore interface {
	SetFileMeta(ctx context.Context, meta *officesdk.FileMeta) error
	DeleteFileMeta(ctx context.Context, fileID string) error
}

type PreviewShareStore interface {
	CreateWithPassword(ctx context.Context, params previewshare.CreateParams) (*previewshare.CreateResult, error)
}

type Config struct {
	SiteBaseURL          string
	HashSalt             string
	DefaultExpireSeconds int
}

type Service struct {
	store   APIKeyStore
	objects ObjectStore
	files   FileMetaStore
	shares  PreviewShareStore
	cfg     Config
	now     func() time.Time
}

type Request struct {
	FileName         string
	DocumentType     string
	DocumentName     string
	ExpiresInSeconds int
	ContentType      string
	Reader           io.Reader
}

type Result struct {
	AccessURL string     `json:"access_url"`
	Password  string     `json:"password"`
	FileID    string     `json:"file_id,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type principal struct {
	apiKey  *model.APIKey
	session *model.CLISession
}

func NewService(store APIKeyStore, objects ObjectStore, files FileMetaStore, shares PreviewShareStore, cfg Config) *Service {
	return &Service{
		store:   store,
		objects: objects,
		files:   files,
		shares:  shares,
		cfg:     cfg,
		now:     time.Now,
	}
}

func (s *Service) Publish(ctx context.Context, bearer string, req Request) (*Result, error) {
	if s == nil || s.store == nil || s.objects == nil || s.files == nil || s.shares == nil {
		return nil, fmt.Errorf("publish service unavailable")
	}
	actor, err := s.authorize(ctx, bearer)
	if err != nil {
		return nil, err
	}
	if err := validateRequest(req); err != nil {
		return nil, err
	}

	fileData, err := io.ReadAll(req.Reader)
	if err != nil {
		return nil, err
	}
	if len(fileData) == 0 {
		return nil, fmt.Errorf("file is required")
	}

	fileID := newFileID()
	documentName := normalizeDocumentName(req.DocumentName, req.FileName, req.DocumentType)
	contentType := normalizeContentType(req.ContentType, documentName)
	storageKey := fmt.Sprintf("preview/%s/original/%s", fileID, filepath.Base(documentName))
	now := s.now().UTC()

	if err := s.objects.PutObject(ctx, storageKey, bytes.NewReader(fileData), int64(len(fileData)), contentType); err != nil {
		return nil, err
	}

	meta := &officesdk.FileMeta{
		ID:         fileID,
		Name:       documentName,
		StorageKey: storageKey,
		Version:    1,
		FromSDK:    false,
		CreateTime: now.Unix(),
		ModifyTime: now.Unix(),
		CreatorID:  actor.creatorID(),
		ModifierID: actor.creatorID(),
	}
	if err := s.files.SetFileMeta(ctx, meta); err != nil {
		_ = s.objects.DeleteObject(ctx, storageKey)
		return nil, err
	}

	expiresAt := now.Add(time.Duration(s.expireSeconds(req.ExpiresInSeconds)) * time.Second)
	result, err := s.shares.CreateWithPassword(ctx, previewshare.CreateParams{
		FileID:     fileID,
		StorageKey: storageKey,
		FileName:   documentName,
		FileType:   strings.ToLower(strings.TrimSpace(req.DocumentType)),
		ExpiresAt:  expiresAt,
	})
	if err != nil {
		_ = s.files.DeleteFileMeta(ctx, fileID)
		_ = s.objects.DeleteObject(ctx, storageKey)
		return nil, err
	}
	share := result.Share

	actor.touch(ctx, s.store, now)
	return &Result{
		AccessURL: strings.TrimRight(defaultSiteBaseURL(s.cfg.SiteBaseURL), "/") + "/p/" + share.ShareToken,
		Password:  result.Password,
		FileID:    fileID,
		ExpiresAt: &share.ExpiresAt,
	}, nil
}

func (s *Service) authorize(ctx context.Context, bearer string) (*principal, error) {
	keyValue := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(bearer), "Bearer "))
	if keyValue == "" {
		return nil, fmt.Errorf("missing api key")
	}
	hash := hashAPIKey(keyValue, s.cfg.HashSalt)
	key, err := s.store.FindByHash(ctx, hash)
	if err != nil {
		return nil, err
	}
	switch {
	case key == nil:
		return s.authorizeSession(ctx, keyValue)
	case key.Status != model.APIKeyStatusActive:
		return nil, fmt.Errorf("api key is disabled")
	case key.ExpiresAt != nil && key.ExpiresAt.Before(time.Now().UTC()):
		return nil, fmt.Errorf("api key is expired")
	case !hasPublishEntitlement(key):
		return nil, fmt.Errorf("publish entitlement is required")
	default:
		return &principal{apiKey: key}, nil
	}
}

func (s *Service) authorizeSession(ctx context.Context, token string) (*principal, error) {
	session, err := s.store.FindCLISessionByTokenHash(ctx, clisession.HashToken(token))
	if err != nil {
		return nil, err
	}
	switch {
	case session == nil:
		return nil, fmt.Errorf("invalid api key")
	case session.RevokedAt != nil:
		return nil, fmt.Errorf("cli session is revoked")
	case session.ExpiresAt.Before(time.Now().UTC()):
		return nil, fmt.Errorf("cli session is expired")
	default:
		return &principal{session: session}, nil
	}
}

func (p *principal) creatorID() string {
	if p != nil && p.session != nil {
		return fmt.Sprintf("publish-user-%d", p.session.UserID)
	}
	return "publish"
}

func (p *principal) touch(ctx context.Context, store APIKeyStore, now time.Time) {
	if p == nil || store == nil {
		return
	}
	if p.apiKey != nil {
		_ = store.TouchLastUsedAt(ctx, p.apiKey.ID, now)
		return
	}
	if p.session != nil {
		_ = store.TouchCLISession(ctx, p.session.ID, now)
	}
}

func hasPublishEntitlement(key *model.APIKey) bool {
	if key == nil {
		return false
	}
	if key.QuotaTotal != nil && *key.QuotaTotal > 0 {
		return true
	}
	if key.QuotaUsed > 0 {
		return true
	}
	if key.CreditBalance > 0 {
		return true
	}
	return false
}

func validateRequest(req Request) error {
	fileName := strings.TrimSpace(req.FileName)
	docName := strings.TrimSpace(req.DocumentName)
	docType := strings.ToLower(strings.TrimSpace(req.DocumentType))
	if fileName == "" || docName == "" || req.Reader == nil {
		return fmt.Errorf("file, document_type and document_name are required")
	}
	switch docType {
	case "pptx", "docx", "xlsx", "report", "img":
		return nil
	default:
		return fmt.Errorf("unsupported document_type")
	}
}

func (s *Service) expireSeconds(requested int) int {
	if requested > 0 {
		return requested
	}
	if s.cfg.DefaultExpireSeconds > 0 {
		return s.cfg.DefaultExpireSeconds
	}
	return 30 * 24 * 60 * 60
}

func defaultSiteBaseURL(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "https://officecli.io"
	}
	return value
}

func normalizeDocumentName(documentName, fileName, documentType string) string {
	name := strings.TrimSpace(documentName)
	if name == "" {
		name = strings.TrimSpace(fileName)
	}
	name = filepath.Base(name)
	if strings.EqualFold(strings.TrimSpace(documentType), "img") {
		ext := strings.ToLower(filepath.Ext(name))
		switch ext {
		case ".png", ".jpg", ".jpeg", ".webp":
			return name
		default:
			return name + ".png"
		}
	}
	ext := normalizedDocumentExt(documentType)
	if !strings.EqualFold(filepath.Ext(name), ext) {
		name += ext
	}
	return name
}

func normalizedDocumentExt(documentType string) string {
	switch strings.ToLower(strings.TrimSpace(documentType)) {
	case "report":
		return ".html"
	default:
		return "." + strings.ToLower(strings.TrimSpace(documentType))
	}
}

func normalizeContentType(contentType, fileName string) string {
	value := strings.TrimSpace(contentType)
	if value != "" {
		return value
	}
	if guessed := mime.TypeByExtension(strings.ToLower(filepath.Ext(fileName))); guessed != "" {
		return guessed
	}
	return "application/octet-stream"
}

func newFileID() string {
	return strings.ReplaceAll(uuid.NewString(), "-", "")
}

func hashAPIKey(apiKey, salt string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(salt) + ":" + strings.TrimSpace(apiKey)))
	return hex.EncodeToString(sum[:])
}
