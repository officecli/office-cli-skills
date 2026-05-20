package operations

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/officecli/officecli-internal/platform/internal/model"
)

const (
	VisitorCookieName = "ocli_visitor_id"
	VisitorCookieTTL  = 180 * 24 * time.Hour

	maxMetadataBytes = 8 << 10
	maxMetadataDepth = 4
)

type Store interface {
	CreateOperationalEvent(ctx context.Context, event *model.OperationalEvent) error
}

type Service struct {
	store Store
}

type TrackRequest struct {
	EventName       string          `json:"event_name"`
	Surface         string          `json:"surface"`
	VisitorID       string          `json:"visitor_id,omitempty"`
	FingerprintHash string          `json:"fingerprint_hash,omitempty"`
	APIKeyID        *uint64         `json:"api_key_id,omitempty"`
	OrderID         *uint64         `json:"order_id,omitempty"`
	Invite          string          `json:"invite,omitempty"`
	UTMSource       string          `json:"utm_source,omitempty"`
	UTMMedium       string          `json:"utm_medium,omitempty"`
	UTMCampaign     string          `json:"utm_campaign,omitempty"`
	UTMTerm         string          `json:"utm_term,omitempty"`
	UTMContent      string          `json:"utm_content,omitempty"`
	UTMID           string          `json:"utm_id,omitempty"`
	PagePath        string          `json:"page_path,omitempty"`
	Referrer        string          `json:"referrer,omitempty"`
	UserAgent       string          `json:"user_agent,omitempty"`
	Metadata        json.RawMessage `json:"metadata,omitempty"`
}

type TrackContext struct {
	UserID    *uint64
	Host      string
	Secure    bool
	UserAgent string
}

type TrackResult struct {
	VisitorCookie *http.Cookie
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) Track(ctx context.Context, req TrackRequest, trackCtx TrackContext) (*TrackResult, error) {
	event, err := req.toEvent(trackCtx)
	if err != nil {
		return nil, err
	}
	if err := s.store.CreateOperationalEvent(ctx, event); err != nil {
		return nil, err
	}
	result := &TrackResult{}
	if event.VisitorID != nil && *event.VisitorID != "" {
		result.VisitorCookie = VisitorCookie(*event.VisitorID, trackCtx.Host, trackCtx.Secure)
	}
	return result, nil
}

func (r TrackRequest) toEvent(trackCtx TrackContext) (*model.OperationalEvent, error) {
	eventName := strings.TrimSpace(r.EventName)
	if !allowedEvent(eventName) {
		return nil, fmt.Errorf("invalid event_name")
	}
	surface := strings.TrimSpace(r.Surface)
	if !allowedSurface(surface) {
		return nil, fmt.Errorf("invalid surface")
	}
	metadataJSON, err := normalizeMetadata(r.Metadata)
	if err != nil {
		return nil, err
	}
	userAgent := r.UserAgent
	if strings.TrimSpace(userAgent) == "" {
		userAgent = trackCtx.UserAgent
	}
	visitorID, err := optionalString(r.VisitorID, 96, "visitor_id")
	if err != nil {
		return nil, err
	}
	if visitorID != nil && !isVisitorIDValid(*visitorID) {
		return nil, fmt.Errorf("invalid visitor_id")
	}
	fingerprintHash, err := optionalString(r.FingerprintHash, 128, "fingerprint_hash")
	if err != nil {
		return nil, err
	}
	invite, err := optionalString(r.Invite, 191, "invite")
	if err != nil {
		return nil, err
	}
	utmSource, err := optionalString(r.UTMSource, 191, "utm_source")
	if err != nil {
		return nil, err
	}
	utmMedium, err := optionalString(r.UTMMedium, 191, "utm_medium")
	if err != nil {
		return nil, err
	}
	utmCampaign, err := optionalString(r.UTMCampaign, 191, "utm_campaign")
	if err != nil {
		return nil, err
	}
	utmTerm, err := optionalString(r.UTMTerm, 191, "utm_term")
	if err != nil {
		return nil, err
	}
	utmContent, err := optionalString(r.UTMContent, 191, "utm_content")
	if err != nil {
		return nil, err
	}
	utmID, err := optionalString(r.UTMID, 191, "utm_id")
	if err != nil {
		return nil, err
	}
	pagePath, err := optionalString(r.PagePath, 512, "page_path")
	if err != nil {
		return nil, err
	}
	referrer, err := optionalString(r.Referrer, 512, "referrer")
	if err != nil {
		return nil, err
	}
	userAgentValue, err := optionalString(userAgent, 512, "user_agent")
	if err != nil {
		return nil, err
	}
	event := &model.OperationalEvent{
		EventName:       eventName,
		Surface:         surface,
		VisitorID:       visitorID,
		FingerprintHash: fingerprintHash,
		UserID:          trackCtx.UserID,
		APIKeyID:        r.APIKeyID,
		OrderID:         r.OrderID,
		Invite:          invite,
		UTMSource:       utmSource,
		UTMMedium:       utmMedium,
		UTMCampaign:     utmCampaign,
		UTMTerm:         utmTerm,
		UTMContent:      utmContent,
		UTMID:           utmID,
		PagePath:        pagePath,
		Referrer:        referrer,
		UserAgent:       userAgentValue,
		MetadataJSON:    metadataJSON,
	}
	return event, nil
}

func optionalString(value string, max int, field string) (*string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}
	if len(trimmed) > max {
		return nil, fmt.Errorf("%s is too long", field)
	}
	return &trimmed, nil
}

func allowedEvent(name string) bool {
	switch name {
	case "page_view", "pricing_view", "cta_click", "download_click", "console_open", "login_start", "checkout_start", "invite_param_hit", "invite_param_carried", "api_key_create_success", "growth_status_view":
		return true
	default:
		return false
	}
}

func allowedSurface(surface string) bool {
	switch surface {
	case "site", "app":
		return true
	default:
		return false
	}
}

func normalizeMetadata(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "{}", nil
	}
	if len(raw) > maxMetadataBytes {
		return "", fmt.Errorf("metadata is too large")
	}
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", fmt.Errorf("metadata must be valid JSON")
	}
	object, ok := payload.(map[string]any)
	if !ok {
		return "", fmt.Errorf("metadata must be an object")
	}
	if jsonDepth(object) > maxMetadataDepth {
		return "", fmt.Errorf("metadata is too deep")
	}
	normalized, err := json.Marshal(object)
	if err != nil {
		return "", err
	}
	if len(normalized) > maxMetadataBytes {
		return "", fmt.Errorf("metadata is too large")
	}
	return string(normalized), nil
}

func jsonDepth(value any) int {
	switch typed := value.(type) {
	case map[string]any:
		maxChild := 0
		for _, child := range typed {
			if depth := jsonDepth(child); depth > maxChild {
				maxChild = depth
			}
		}
		return maxChild + 1
	case []any:
		maxChild := 0
		for _, child := range typed {
			if depth := jsonDepth(child); depth > maxChild {
				maxChild = depth
			}
		}
		return maxChild + 1
	default:
		return 0
	}
}

func isVisitorIDValid(value string) bool {
	if value == "" || len(value) > 96 {
		return false
	}
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		switch r {
		case '-', '_', '.', ':':
			continue
		default:
			return false
		}
	}
	return true
}

func VisitorCookie(visitorID, host string, secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     VisitorCookieName,
		Value:    visitorID,
		Path:     "/",
		Domain:   visitorCookieDomain(host),
		MaxAge:   int(VisitorCookieTTL.Seconds()),
		Expires:  time.Now().UTC().Add(VisitorCookieTTL),
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		HttpOnly: false,
	}
}

func visitorCookieDomain(host string) string {
	hostOnly := host
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		hostOnly = parsedHost
	}
	hostOnly = strings.ToLower(strings.TrimSpace(hostOnly))
	if hostOnly == "localhost" || net.ParseIP(hostOnly) != nil {
		return ""
	}
	if hostOnly == "officecli.io" || strings.HasSuffix(hostOnly, ".officecli.io") {
		return ".officecli.io"
	}
	if hostOnly == "shimodev.com" || strings.HasSuffix(hostOnly, ".shimodev.com") {
		return ".shimodev.com"
	}
	return ""
}
