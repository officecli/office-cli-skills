package auth

import (
	"context"
	"fmt"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	googleoauth "google.golang.org/api/oauth2/v2"
	"google.golang.org/api/option"
)

type GoogleOAuthProvider struct {
	config *oauth2.Config
}

func NewGoogleOAuthProvider(clientID, clientSecret, redirectURL string) *GoogleOAuthProvider {
	return &GoogleOAuthProvider{config: &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes:       []string{"openid", "profile", "email"},
		Endpoint:     google.Endpoint,
	}}
}

func (g *GoogleOAuthProvider) AuthCodeURL(state string) string {
	return g.config.AuthCodeURL(state, oauth2.AccessTypeOnline)
}

func (g *GoogleOAuthProvider) Exchange(ctx context.Context, code string) (*GoogleUser, error) {
	token, err := g.config.Exchange(ctx, code)
	if err != nil {
		return nil, err
	}
	client := g.config.Client(ctx, token)
	svc, err := googleoauth.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, err
	}
	info, err := svc.Userinfo.Get().Do()
	if err != nil {
		return nil, err
	}
	if info.Id == "" || info.Email == "" {
		return nil, fmt.Errorf("google user profile is incomplete")
	}
	var avatar *string
	if info.Picture != "" {
		avatar = &info.Picture
	}
	return &GoogleUser{Subject: info.Id, Email: info.Email, Name: info.Name, AvatarURL: avatar}, nil
}
