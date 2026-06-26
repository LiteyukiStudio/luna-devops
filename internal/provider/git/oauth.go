package gitprovider

import (
	"fmt"
	"strings"

	"github.com/LiteyukiStudio/devops/internal/model"
	"golang.org/x/oauth2"
)

func OAuthConfig(provider model.GitProvider, redirectURL string, clientSecret string) (*oauth2.Config, error) {
	endpoint, err := OAuthEndpoint(provider)
	if err != nil {
		return nil, err
	}
	scopes := []string{"repo", "read:user"}
	if provider.Type == "gitea" {
		scopes = []string{"read:repository", "write:repository", "read:user"}
	}
	return &oauth2.Config{
		ClientID:     provider.ClientID,
		ClientSecret: strings.TrimSpace(clientSecret),
		Endpoint:     endpoint,
		RedirectURL:  redirectURL,
		Scopes:       scopes,
	}, nil
}

func OAuthEndpoint(provider model.GitProvider) (oauth2.Endpoint, error) {
	base := strings.TrimRight(provider.BaseURL, "/")
	switch provider.Type {
	case "github":
		if base == "" {
			base = "https://github.com"
		}
		return oauth2.Endpoint{
			AuthURL:  base + "/login/oauth/authorize",
			TokenURL: base + "/login/oauth/access_token",
		}, nil
	case "gitea":
		return oauth2.Endpoint{
			AuthURL:  base + "/login/oauth/authorize",
			TokenURL: base + "/login/oauth/access_token",
		}, nil
	default:
		return oauth2.Endpoint{}, fmt.Errorf("git provider type %q is not supported", provider.Type)
	}
}
