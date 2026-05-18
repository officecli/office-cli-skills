package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/oauth2"
)

type OAuth2ProviderConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	AuthURL      string
	TokenURL     string
	UserinfoURL  string
	Scopes       []string
	SubjectField string
	EmailField   string
	NameField    string
	AvatarField  string
}

type OAuth2Provider struct {
	config       *oauth2.Config
	userinfoURL  string
	subjectField string
	emailField   string
	nameField    string
	avatarField  string
}

func NewOAuth2Provider(cfg OAuth2ProviderConfig) *OAuth2Provider {
	scopes := cfg.Scopes
	if len(scopes) == 0 {
		scopes = []string{"openid", "profile", "email"}
	}
	return &OAuth2Provider{
		config: &oauth2.Config{
			ClientID:     strings.TrimSpace(cfg.ClientID),
			ClientSecret: strings.TrimSpace(cfg.ClientSecret),
			RedirectURL:  strings.TrimSpace(cfg.RedirectURL),
			Scopes:       scopes,
			Endpoint: oauth2.Endpoint{
				AuthURL:  strings.TrimSpace(cfg.AuthURL),
				TokenURL: strings.TrimSpace(cfg.TokenURL),
			},
		},
		userinfoURL:  strings.TrimSpace(cfg.UserinfoURL),
		subjectField: defaultFieldName(cfg.SubjectField, "sub"),
		emailField:   defaultFieldName(cfg.EmailField, "email"),
		nameField:    defaultFieldName(cfg.NameField, "name"),
		avatarField:  defaultFieldName(cfg.AvatarField, "picture"),
	}
}

func (p *OAuth2Provider) AuthCodeURL(state string) string {
	return p.config.AuthCodeURL(state, oauth2.AccessTypeOnline)
}

func (p *OAuth2Provider) Exchange(ctx context.Context, code string) (*GoogleUser, error) {
	if strings.TrimSpace(p.userinfoURL) == "" {
		return nil, fmt.Errorf("OAUTH2_USERINFO_URL is required")
	}
	token, err := p.config.Exchange(ctx, code)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.userinfoURL, nil)
	if err != nil {
		return nil, err
	}
	token.SetAuthHeader(req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("oauth2 userinfo failed with status %d", resp.StatusCode)
	}
	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	payload = userinfoProfilePayload(payload)
	subject := firstStringField(payload, p.subjectField, "sub", "id", "externalId", "external_id", "userId", "user_id", "uid", "openId", "open_id", "account", "username")
	email := firstStringField(payload, p.emailField, "email", "mail", "emailAddress", "email_address")
	if email == "" && subject != "" {
		email = syntheticOAuth2Email(subject)
	}
	name := firstStringField(payload, p.nameField, "name", "displayName", "display_name", "nickname", "nickName", "realName", "real_name", "username", "account", "email")
	user := &GoogleUser{
		Subject: subject,
		Email:   email,
		Name:    name,
	}
	if avatar := firstStringField(payload, p.avatarField, "picture", "avatar", "avatarURL", "avatarUrl", "avatar_url"); avatar != "" {
		user.AvatarURL = &avatar
	}
	if user.Subject == "" || user.Email == "" {
		return nil, fmt.Errorf("oauth2 user profile is incomplete")
	}
	return user, nil
}

func userinfoProfilePayload(payload map[string]any) map[string]any {
	data, ok := payload["data"]
	if !ok {
		return payload
	}
	nested, ok := data.(map[string]any)
	if !ok {
		return payload
	}
	return nested
}

func defaultFieldName(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func firstStringField(payload map[string]any, fields ...string) string {
	seen := map[string]struct{}{}
	for _, field := range fields {
		normalized := strings.TrimSpace(field)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		if value := stringField(payload, normalized); value != "" {
			return value
		}
	}
	return ""
}

func stringField(payload map[string]any, field string) string {
	value, ok := payload[strings.TrimSpace(field)]
	if !ok {
		return ""
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func syntheticOAuth2Email(subject string) string {
	normalized := strings.ToLower(strings.TrimSpace(subject))
	normalized = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-' {
			return r
		}
		return '-'
	}, normalized)
	normalized = strings.Trim(normalized, ".-_")
	if normalized == "" {
		normalized = "user"
	}
	return normalized + "@oauth2.local"
}
