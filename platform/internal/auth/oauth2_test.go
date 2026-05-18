package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestOAuth2ProviderUsesConfiguredEndpointsAndUserinfo(t *testing.T) {
	var gotAuthHeader string
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		if r.Form.Get("code") != "code-123" {
			t.Fatalf("code = %q", r.Form.Get("code"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "token-123",
			"token_type":   "Bearer",
			"expires_in":   300,
		})
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		gotAuthHeader = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sub":     "company-user-1",
			"email":   "dev@example.com",
			"name":    "Dev User",
			"picture": "https://idp.example.com/avatar.png",
		})
	})

	provider := NewOAuth2Provider(OAuth2ProviderConfig{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		RedirectURL:  "https://officecli.shimodev.com/api/auth/oauth2/callback",
		AuthURL:      server.URL + "/authorize",
		TokenURL:     server.URL + "/token",
		UserinfoURL:  server.URL + "/userinfo",
		Scopes:       []string{"openid", "email", "profile"},
	})

	loginURL := provider.AuthCodeURL("state-123")
	parsed, err := url.Parse(loginURL)
	if err != nil {
		t.Fatalf("Parse loginURL: %v", err)
	}
	if parsed.Path != "/authorize" {
		t.Fatalf("auth path = %q", parsed.Path)
	}
	if parsed.Query().Get("client_id") != "client-id" {
		t.Fatalf("client_id = %q", parsed.Query().Get("client_id"))
	}
	if parsed.Query().Get("state") != "state-123" {
		t.Fatalf("state = %q", parsed.Query().Get("state"))
	}

	user, err := provider.Exchange(context.Background(), "code-123")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if user.Subject != "company-user-1" || user.Email != "dev@example.com" || user.Name != "Dev User" {
		t.Fatalf("user = %+v", user)
	}
	if user.AvatarURL == nil || *user.AvatarURL != "https://idp.example.com/avatar.png" {
		t.Fatalf("avatar = %#v", user.AvatarURL)
	}
	if !strings.HasPrefix(gotAuthHeader, "Bearer token-123") {
		t.Fatalf("Authorization = %q", gotAuthHeader)
	}
}

func TestOAuth2ProviderReadsWrappedUserinfoDataObject(t *testing.T) {
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "token-123",
			"token_type":   "Bearer",
		})
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"msg":  "ok",
			"data": map[string]any{
				"sub":     "oa-user-1",
				"email":   "oa@example.com",
				"name":    "OA User",
				"picture": "https://oa.shimodev.com/avatar.png",
			},
		})
	})

	provider := NewOAuth2Provider(OAuth2ProviderConfig{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		RedirectURL:  "https://officecli.shimodev.com/api/auth/oauth2/callback",
		AuthURL:      server.URL + "/authorize",
		TokenURL:     server.URL + "/token",
		UserinfoURL:  server.URL + "/userinfo",
		Scopes:       []string{"profile"},
	})

	user, err := provider.Exchange(context.Background(), "code-123")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if user.Subject != "oa-user-1" || user.Email != "oa@example.com" || user.Name != "OA User" {
		t.Fatalf("user = %+v", user)
	}
}

func TestOAuth2ProviderAcceptsInternalUserinfoWithoutEmail(t *testing.T) {
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "token-123",
			"token_type":   "Bearer",
		})
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"msg":  "ok",
			"data": map[string]any{
				"id":   12345,
				"name": "OA User",
			},
		})
	})

	provider := NewOAuth2Provider(OAuth2ProviderConfig{
		ClientID:     "client-id",
		ClientSecret: "client-secret",
		RedirectURL:  "https://officecli.shimodev.com/api/auth/oauth2/callback",
		AuthURL:      server.URL + "/authorize",
		TokenURL:     server.URL + "/token",
		UserinfoURL:  server.URL + "/userinfo",
		Scopes:       []string{"profile"},
	})

	user, err := provider.Exchange(context.Background(), "code-123")
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if user.Subject != "12345" || user.Email != "12345@oauth2.local" || user.Name != "OA User" {
		t.Fatalf("user = %+v", user)
	}
}
