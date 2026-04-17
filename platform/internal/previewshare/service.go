package previewshare

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
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
	PasswordHash string     `gorm:"column:password_hash;size:128;not null"`
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

type CreateResult struct {
	Share    *PreviewShare
	Password string
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
	result, err := s.CreateWithPassword(ctx, params)
	if err != nil {
		return nil, err
	}
	return result.Share, nil
}

func (s *Service) CreateWithPassword(ctx context.Context, params CreateParams) (*CreateResult, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("preview share service unavailable")
	}
	token, err := randomString(24)
	if err != nil {
		return nil, err
	}
	password, err := randomDigits(6)
	if err != nil {
		return nil, err
	}
	share := &PreviewShare{
		ShareToken:   token,
		FileID:       strings.TrimSpace(params.FileID),
		StorageKey:   strings.TrimSpace(params.StorageKey),
		FileName:     strings.TrimSpace(params.FileName),
		FileType:     strings.ToLower(strings.TrimSpace(params.FileType)),
		PasswordHash: s.hashPassword(password),
		ExpiresAt:    params.ExpiresAt.UTC(),
		Status:       StatusActive,
	}
	if err := s.db.WithContext(ctx).Create(share).Error; err != nil {
		return nil, err
	}
	return &CreateResult{Share: share, Password: password}, nil
}

func (s *Service) GetByToken(ctx context.Context, token string) (*PreviewShare, error) {
	return s.getOne(ctx, "share_token = ?", strings.TrimSpace(token))
}

func (s *Service) GetByFileID(ctx context.Context, fileID string) (*PreviewShare, error) {
	return s.getOne(ctx, "file_id = ?", strings.TrimSpace(fileID))
}

func (s *Service) VerifyPassword(share *PreviewShare, password string) bool {
	if share == nil || strings.TrimSpace(share.PasswordHash) == "" {
		return false
	}
	got := []byte(s.hashPassword(password))
	want := []byte(strings.TrimSpace(share.PasswordHash))
	return len(got) == len(want) && subtle.ConstantTimeCompare(got, want) == 1
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

func (s *Service) HasAccessCookie(c *gin.Context, share *PreviewShare) bool {
	return s.hasValidAccessCookie(c, share)
}

func (s *Service) RequireShareAccess(c *gin.Context, fileID string) (*PreviewShare, int, error) {
	if s == nil || s.db == nil {
		return nil, 0, nil
	}
	share, status, err := s.getAuthorizedShare(c, fileID)
	if err != nil {
		return share, status, err
	}
	s.touchLastAccess(c.Request.Context(), share.ID)
	return share, 0, nil
}

func (s *Service) RequireShareDownloadAccess(c *gin.Context, fileID, storageKey string) (*PreviewShare, int, error) {
	if s == nil || s.db == nil {
		return nil, 0, nil
	}
	share, status, err := s.getActiveShare(c.Request.Context(), fileID)
	if err != nil {
		return share, status, err
	}
	if s.isAllowedInternalRequest(c) || s.hasValidAccessCookie(c, share) {
		s.touchLastAccess(c.Request.Context(), share.ID)
		return share, 0, nil
	}
	rawToken := strings.TrimSpace(c.Query("access_token"))
	if rawToken != "" && s.validateDownloadToken(rawToken, share.ShareToken, share.ExpiresAt.UTC().Unix(), storageKey) {
		s.touchLastAccess(c.Request.Context(), share.ID)
		return share, 0, nil
	}
	return share, http.StatusUnauthorized, fmt.Errorf("preview password required")
}

func (s *Service) IssueDownloadToken(ctx context.Context, fileID, storageKey string) (string, error) {
	share, _, err := s.getActiveShare(ctx, fileID)
	if err != nil {
		return "", err
	}
	return s.signDownloadToken(share.ShareToken, share.ExpiresAt.UTC().Unix(), storageKey), nil
}

func (s *Service) getAuthorizedShare(c *gin.Context, fileID string) (*PreviewShare, int, error) {
	share, status, err := s.getActiveShare(c.Request.Context(), fileID)
	if err != nil {
		return share, status, err
	}
	if s.isAllowedInternalRequest(c) || s.hasValidAccessCookie(c, share) {
		return share, 0, nil
	}
	return share, http.StatusUnauthorized, fmt.Errorf("preview password required")
}

func (s *Service) getActiveShare(ctx context.Context, fileID string) (*PreviewShare, int, error) {
	share, err := s.GetByFileID(ctx, fileID)
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

func (s *Service) hasValidAccessCookie(c *gin.Context, share *PreviewShare) bool {
	if c == nil || share == nil {
		return false
	}
	raw, err := c.Cookie(AccessCookieName)
	if err != nil {
		return false
	}
	return s.validateAccessCookie(raw, share.ShareToken, share.ExpiresAt.UTC().Unix())
}

func (s *Service) touchLastAccess(ctx context.Context, shareID uint64) {
	if s == nil || s.db == nil || shareID == 0 {
		return
	}
	now := s.now().UTC()
	_ = s.db.WithContext(ctx).Model(&PreviewShare{}).Where("id = ?", shareID).Update("last_access_at", now).Error
}

func (s *Service) isAllowedInternalRequest(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	if !isPrivateOrLoopbackIP(c.ClientIP(), c.Request.RemoteAddr) {
		return false
	}
	if isInternalRequestHost(c.Request.Host, c.GetHeader("X-Forwarded-Host")) {
		return true
	}
	return s.isTrustedPublicHost(c.Request.Host, c.GetHeader("X-Forwarded-Host"))
}

func isInternalRequestHost(hosts ...string) bool {
	for _, host := range hosts {
		host = strings.TrimSpace(host)
		if host == "" {
			continue
		}
		hostName := host
		if parsedHost, _, err := net.SplitHostPort(host); err == nil {
			hostName = parsedHost
		} else {
			hostName = strings.Trim(host, "[]")
		}
		hostName = strings.ToLower(strings.TrimSpace(hostName))
		switch hostName {
		case "localhost", "host.docker.internal":
			return true
		}
		if ip := net.ParseIP(hostName); ip != nil && (ip.IsLoopback() || ip.IsPrivate()) {
			return true
		}
	}
	return false
}

func (s *Service) isTrustedPublicHost(hosts ...string) bool {
	if s == nil {
		return false
	}
	cookieDomain := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(s.cookieDomain, ".")))
	if cookieDomain == "" {
		return false
	}
	for _, host := range hosts {
		hostName := canonicalHostName(host)
		if hostName == "" {
			continue
		}
		if hostName == cookieDomain || strings.HasSuffix(hostName, "."+cookieDomain) {
			return true
		}
	}
	return false
}

func canonicalHostName(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	hostName := host
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		hostName = parsedHost
	} else {
		hostName = strings.Trim(host, "[]")
	}
	return strings.ToLower(strings.TrimSpace(hostName))
}

func isPrivateOrLoopbackIP(values ...string) bool {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		host := value
		if parsedHost, _, err := net.SplitHostPort(value); err == nil {
			host = parsedHost
		}
		ip := net.ParseIP(strings.Trim(host, "[]"))
		if ip == nil {
			continue
		}
		if ip.IsLoopback() || ip.IsPrivate() {
			return true
		}
	}
	return false
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

func (s *Service) hashPassword(password string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(s.cookieSecret) + ":" + strings.TrimSpace(password)))
	return hex.EncodeToString(sum[:])
}

func (s *Service) signAccessCookie(shareToken string, expiresUnix int64) string {
	payload := fmt.Sprintf("%s:%d", shareToken, expiresUnix)
	mac := hmac.New(sha256.New, []byte(s.cookieSecret))
	_, _ = mac.Write([]byte(payload))
	value := payload + ":" + hex.EncodeToString(mac.Sum(nil))
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func (s *Service) signDownloadToken(shareToken string, expiresUnix int64, storageKey string) string {
	encodedStorageKey := base64.RawURLEncoding.EncodeToString([]byte(strings.TrimSpace(storageKey)))
	payload := fmt.Sprintf("%s:%d:%s", shareToken, expiresUnix, encodedStorageKey)
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

func (s *Service) validateDownloadToken(raw, shareToken string, expiresUnix int64, storageKey string) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return false
	}
	parts := strings.Split(string(decoded), ":")
	if len(parts) != 4 {
		return false
	}
	expectedStorageKey := base64.RawURLEncoding.EncodeToString([]byte(strings.TrimSpace(storageKey)))
	if parts[0] != shareToken || parts[1] != fmt.Sprintf("%d", expiresUnix) || parts[2] != expectedStorageKey {
		return false
	}
	mac := hmac.New(sha256.New, []byte(s.cookieSecret))
	_, _ = mac.Write([]byte(parts[0] + ":" + parts[1] + ":" + parts[2]))
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(parts[3]))
}

func randomString(length int) (string, error) {
	raw := make([]byte, length)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw)[:length], nil
}

func randomDigits(length int) (string, error) {
	raw := make([]byte, length)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	out := make([]byte, length)
	for i, b := range raw {
		out[i] = '0' + (b % 10)
	}
	return string(out), nil
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
