package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func withGitHubAPIBase(t *testing.T, base string) {
	t.Helper()
	prevUser, prevEmails := githubUserURL, githubEmailsURL
	githubUserURL = base + "/user"
	githubEmailsURL = base + "/user/emails"
	t.Cleanup(func() {
		githubUserURL = prevUser
		githubEmailsURL = prevEmails
	})
}

func newGitHubTestServer(t *testing.T, userBody, emailsBody string, userStatus, emailsStatus int) (*httptest.Server, *GitHubOAuthProvider) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/login/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"test-token","token_type":"bearer","scope":"read:user user:email"}`))
	})
	mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(userStatus)
		_, _ = w.Write([]byte(userBody))
	})
	mux.HandleFunc("/user/emails", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(emailsStatus)
		_, _ = w.Write([]byte(emailsBody))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	provider := NewGitHubOAuthProvider("client-id", "client-secret", "https://example.com/cb")
	provider.config.Endpoint.TokenURL = srv.URL + "/login/oauth/access_token"
	// override the package-level URLs by swapping the Exchange path via a wrapper isn't trivial,
	// so we point the HTTP calls at the test server by rewriting the URLs through DefaultTransport.
	// Instead, we patch the package URLs via test seam:
	return srv, provider
}

func TestGitHubProviderExchangeUsesEmailFromProfile(t *testing.T) {
	srv, provider := newGitHubTestServer(t,
		`{"id":42,"login":"octocat","name":"The Octocat","email":"oct@example.com","avatar_url":"https://avatars/oct.png"}`,
		`[]`, http.StatusOK, http.StatusOK)
	withGitHubAPIBase(t, srv.URL)

	user, err := provider.Exchange(context.Background(), "code")
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if user.Subject != "42" {
		t.Fatalf("subject = %q", user.Subject)
	}
	if user.Email != "oct@example.com" {
		t.Fatalf("email = %q", user.Email)
	}
	if user.Name != "The Octocat" {
		t.Fatalf("name = %q", user.Name)
	}
	if user.AvatarURL == nil || *user.AvatarURL == "" {
		t.Fatal("avatar missing")
	}
}

func TestGitHubProviderExchangeFallsBackToEmailsEndpoint(t *testing.T) {
	srv, provider := newGitHubTestServer(t,
		`{"id":7,"login":"ghost","name":"","email":null,"avatar_url":""}`,
		`[{"email":"public@example.com","primary":false,"verified":true},{"email":"primary@example.com","primary":true,"verified":true}]`,
		http.StatusOK, http.StatusOK)
	withGitHubAPIBase(t, srv.URL)

	user, err := provider.Exchange(context.Background(), "code")
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if user.Email != "primary@example.com" {
		t.Fatalf("expected primary verified email, got %q", user.Email)
	}
	if user.Name != "ghost" {
		t.Fatalf("expected fallback to login, got %q", user.Name)
	}
}

func TestGitHubProviderExchangeErrorsWhenNoUsableEmail(t *testing.T) {
	srv, provider := newGitHubTestServer(t,
		`{"id":99,"login":"noemail","name":"No Email","email":null}`,
		`[{"email":"unverified@example.com","primary":true,"verified":false}]`,
		http.StatusOK, http.StatusOK)
	withGitHubAPIBase(t, srv.URL)

	_, err := provider.Exchange(context.Background(), "code")
	if err == nil || !strings.Contains(err.Error(), "github email not available") {
		t.Fatalf("expected email-missing error, got %v", err)
	}
}
