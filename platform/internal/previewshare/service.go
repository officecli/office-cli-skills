package previewshare

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const AccessCookieName = "cop_preview_access"

const (
	StatusActive   = "active"
	StatusDisabled = "disabled"
)

var ErrNotFound = errors.New("preview share not found")

type PreviewShare struct {
	ID           uint64     `gorm:"primaryKey"`
	ShareToken   string     `gorm:"column:share_token;size:128;uniqueIndex;not null"`
	FileID       string     `gorm:"column:file_id;size:128;uniqueIndex;not null"`
	StorageKey   string     `gorm:"column:storage_key;size:512;index;not null"`
	FileName     string     `gorm:"column:file_name;size:255;not null"`
	FileType     string     `gorm:"column:file_type;size:16;not null"`
	ExpiresAt    time.Time  `gorm:"column:expires_at;index;not null"`
	Status       string     `gorm:"column:status;size:16;index;not null"`
	LastAccessAt *time.Time `gorm:"column:last_access_at"`
	CreatedAt    time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt    time.Time  `gorm:"column:updated_at;autoUpdateTime"`
}

func (PreviewShare) TableName() string { return "preview_shares" }

type objectCleaner interface {
	DeleteObject(ctx context.Context, key string) error
}

type fileMetaCleaner interface {
	DeleteFileMeta(ctx context.Context, fileID string) error
}

type Service struct {
	db           *gorm.DB
	cookieSecret string
	cookieDomain string
	files        fileMetaCleaner
	objects      objectCleaner
	now          func() time.Time
}

type CreateParams struct {
	FileID     string
	StorageKey string
	FileName   string
	FileType   string
	ExpiresAt  time.Time
}

func NewService(db *gorm.DB, cookieSecret, cookieDomain string, files fileMetaCleaner, objects objectCleaner) *Service {
	return &Service{
		db:           db,
		cookieSecret: strings.TrimSpace(cookieSecret),
		cookieDomain: strings.TrimSpace(cookieDomain),
		files:        files,
		objects:      objects,
		now:          time.Now,
	}
}

func (s *Service) Create(ctx context.Context, params CreateParams) (*PreviewShare, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("preview share service unavailable")
	}
	token, err := randomString(24)
	if err != nil {
		return nil, err
	}
	share := &PreviewShare{
		ShareToken: token,
		FileID:     strings.TrimSpace(params.FileID),
		StorageKey: strings.TrimSpace(params.StorageKey),
		FileName:   strings.TrimSpace(params.FileName),
		FileType:   strings.ToLower(strings.TrimSpace(params.FileType)),
		ExpiresAt:  params.ExpiresAt.UTC(),
		Status:     StatusActive,
	}
	if err := s.db.WithContext(ctx).Create(share).Error; err != nil {
		return nil, err
	}
	return share, nil
}

func (s *Service) GetByToken(ctx context.Context, token string) (*PreviewShare, error) {
	return s.getOne(ctx, "share_token = ?", strings.TrimSpace(token))
}

func (s *Service) GetByFileID(ctx context.Context, fileID string) (*PreviewShare, error) {
	return s.getOne(ctx, "file_id = ?", strings.TrimSpace(fileID))
}

func (s *Service) ValidateEntryRequest(ctx context.Context, token string) (*PreviewShare, int, error) {
	share, err := s.GetByToken(ctx, token)
	if err != nil {
		if err == ErrNotFound {
			return nil, http.StatusNotFound, err
		}
		return nil, http.StatusInternalServerError, err
	}
	if share.Status != StatusActive {
		return share, http.StatusForbidden, fmt.Errorf("preview share is disabled")
	}
	if share.ExpiresAt.UTC().Before(s.now().UTC()) {
		return share, http.StatusGone, fmt.Errorf("preview share expired")
	}
	return share, 0, nil
}

func (s *Service) RequireShareAccess(c *gin.Context, fileID string) (*PreviewShare, int, error) {
	if s == nil || s.db == nil {
		return nil, 0, nil
	}
	share, err := s.GetByFileID(c.Request.Context(), fileID)
	if err != nil {
		if err == ErrNotFound {
			return nil, http.StatusNotFound, err
		}
		return nil, http.StatusInternalServerError, err
	}
	if share.Status != StatusActive {
		return share, http.StatusForbidden, fmt.Errorf("preview share is disabled")
	}
	if share.ExpiresAt.UTC().Before(s.now().UTC()) {
		return share, http.StatusGone, fmt.Errorf("preview share expired")
	}
	raw, err := c.Cookie(AccessCookieName)
	if err != nil || !s.validateAccessCookie(raw, share.ShareToken, share.ExpiresAt.UTC().Unix()) {
		return share, http.StatusUnauthorized, fmt.Errorf("preview login is required")
	}
	now := s.now().UTC()
	_ = s.db.WithContext(c.Request.Context()).Model(&PreviewShare{}).Where("id = ?", share.ID).Update("last_access_at", now).Error
	return share, 0, nil
}

func (s *Service) IssueAccessCookie(c *gin.Context, share *PreviewShare) {
	if c == nil || share == nil {
		return
	}
	expiresAt := share.ExpiresAt.UTC()
	ttl := time.Until(expiresAt)
	if ttl > 24*time.Hour {
		ttl = 24 * time.Hour
	}
	if ttl < 0 {
		ttl = 0
	}
	value := s.signAccessCookie(share.ShareToken, expiresAt.Unix())
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(AccessCookieName, value, int(ttl.Seconds()), "/", s.cookieDomain, shouldUseSecurePreviewCookie(c.Request), true)
}

func (s *Service) CleanupExpired(ctx context.Context) error {
	if s == nil || s.db == nil {
		return nil
	}
	var shares []PreviewShare
	if err := s.db.WithContext(ctx).Where("expires_at <= ?", s.now().UTC()).Find(&shares).Error; err != nil {
		return err
	}
	for _, share := range shares {
		if s.objects != nil && strings.TrimSpace(share.StorageKey) != "" {
			_ = s.objects.DeleteObject(ctx, share.StorageKey)
		}
		if s.files != nil && strings.TrimSpace(share.FileID) != "" {
			_ = s.files.DeleteFileMeta(ctx, share.FileID)
		}
		if err := s.db.WithContext(ctx).Delete(&PreviewShare{}, share.ID).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) getOne(ctx context.Context, query string, args ...any) (*PreviewShare, error) {
	if s == nil || s.db == nil {
		return nil, ErrNotFound
	}
	var share PreviewShare
	if err := s.db.WithContext(ctx).Where(query, args...).First(&share).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &share, nil
}

func (s *Service) signAccessCookie(shareToken string, expiresUnix int64) string {
	payload := fmt.Sprintf("%s:%d", shareToken, expiresUnix)
	mac := hmac.New(sha256.New, []byte(s.cookieSecret))
	_, _ = mac.Write([]byte(payload))
	value := payload + ":" + hex.EncodeToString(mac.Sum(nil))
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func (s *Service) validateAccessCookie(raw, shareToken string, expiresUnix int64) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return false
	}
	parts := strings.Split(string(decoded), ":")
	if len(parts) != 3 {
		return false
	}
	if parts[0] != shareToken || parts[1] != fmt.Sprintf("%d", expiresUnix) {
		return false
	}
	mac := hmac.New(sha256.New, []byte(s.cookieSecret))
	_, _ = mac.Write([]byte(parts[0] + ":" + parts[1]))
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(parts[2]))
}

func randomString(length int) (string, error) {
	raw := make([]byte, length)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw)[:length], nil
}

func shouldUseSecurePreviewCookie(r *http.Request) bool {
	if r == nil {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https") {
		return true
	}
	return r.TLS != nil
}
