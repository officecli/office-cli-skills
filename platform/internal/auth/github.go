package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"golang.org/x/oauth2"
	githubendpoint "golang.org/x/oauth2/github"
)

const (
	defaultGitHubUserURL   = "https://api.github.com/user"
	defaultGitHubEmailsURL = "https://api.github.com/user/emails"
)

var (
	githubUserURL   = defaultGitHubUserURL
	githubEmailsURL = defaultGitHubEmailsURL
)

type GitHubOAuthProvider struct {
	config *oauth2.Config
}

func NewGitHubOAuthProvider(clientID, clientSecret, redirectURL string) *GitHubOAuthProvider {
	return &GitHubOAuthProvider{config: &oauth2.Config{
		ClientID:     strings.TrimSpace(clientID),
		ClientSecret: strings.TrimSpace(clientSecret),
		RedirectURL:  strings.TrimSpace(redirectURL),
		Scopes:       []string{"read:user", "user:email"},
		Endpoint:     githubendpoint.Endpoint,
	}}
}

func (g *GitHubOAuthProvider) AuthCodeURL(state string) string {
	return g.config.AuthCodeURL(state, oauth2.AccessTypeOnline)
}

func (g *GitHubOAuthProvider) Exchange(ctx context.Context, code string) (*GoogleUser, error) {
	token, err := g.config.Exchange(ctx, code)
	if err != nil {
		return nil, err
	}
	client := g.config.Client(ctx, token)

	profile, err := fetchGitHubProfile(ctx, client)
	if err != nil {
		return nil, err
	}

	email := strings.TrimSpace(profile.Email)
	if email == "" {
		email, err = fetchGitHubPrimaryVerifiedEmail(ctx, client)
		if err != nil {
			return nil, err
		}
	}
	if email == "" {
		return nil, fmt.Errorf("github email not available; please make a primary verified email accessible")
	}

	subject := strconv.FormatInt(profile.ID, 10)
	if subject == "0" {
		return nil, fmt.Errorf("github user id is missing")
	}

	name := strings.TrimSpace(profile.Name)
	if name == "" {
		name = strings.TrimSpace(profile.Login)
	}
	if name == "" {
		name = email
	}

	user := &GoogleUser{
		Subject: subject,
		Email:   email,
		Name:    name,
	}
	if avatar := strings.TrimSpace(profile.AvatarURL); avatar != "" {
		user.AvatarURL = &avatar
	}
	return user, nil
}

type githubProfile struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

type githubEmail struct {
	Email    string `json:"email"`
	Primary  bool   `json:"primary"`
	Verified bool   `json:"verified"`
}

func fetchGitHubProfile(ctx context.Context, client *http.Client) (*githubProfile, error) {
	resp, err := githubGet(ctx, client, githubUserURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("github /user failed with status %d", resp.StatusCode)
	}
	var profile githubProfile
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return nil, fmt.Errorf("decode github user: %w", err)
	}
	return &profile, nil
}

func fetchGitHubPrimaryVerifiedEmail(ctx context.Context, client *http.Client) (string, error) {
	resp, err := githubGet(ctx, client, githubEmailsURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("github /user/emails failed with status %d", resp.StatusCode)
	}
	var emails []githubEmail
	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return "", fmt.Errorf("decode github emails: %w", err)
	}
	for _, e := range emails {
		if e.Primary && e.Verified {
			return strings.TrimSpace(e.Email), nil
		}
	}
	for _, e := range emails {
		if e.Verified {
			return strings.TrimSpace(e.Email), nil
		}
	}
	return "", nil
}

func githubGet(ctx context.Context, client *http.Client, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	return client.Do(req)
}
