package socialauth

import (
	"context"
	"fmt"
	"strconv"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
)

var (
	githubUserURL   = "https://api.github.com/user"
	githubEmailsURL = "https://api.github.com/user/emails"
)

func NewGitHubConfig(clientID, secret, redirectURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: secret,
		RedirectURL:  redirectURL,
		Scopes:       []string{"read:user", "user:email"},
		Endpoint:     github.Endpoint,
	}
}

// FetchGitHubProfile exchanges an authorization code and reads the profile.
// A missing email is not an error: GitHub accounts routinely expose none, and
// the column is nullable by design.
func FetchGitHubProfile(ctx context.Context, cfg *oauth2.Config, code string) (SocialUserProfile, error) {
	tok, err := cfg.Exchange(ctx, code)
	if err != nil {
		return SocialUserProfile{}, fmt.Errorf("github code exchange: %w", err)
	}
	client := cfg.Client(ctx, tok)

	var user struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Name  string `json:"name"`
	}
	if err := getJSON(ctx, client, githubUserURL, &user); err != nil {
		return SocialUserProfile{}, fmt.Errorf("github user: %w", err)
	}

	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	// A failure here is tolerated: the profile is still usable without it.
	_ = getJSON(ctx, client, githubEmailsURL, &emails)

	var email string
	for _, e := range emails {
		if e.Primary && e.Verified {
			email = e.Email
			break
		}
	}

	displayName := user.Name
	if displayName == "" {
		displayName = user.Login
	}

	return SocialUserProfile{
		Provider:    "github",
		ProviderID:  strconv.FormatInt(user.ID, 10),
		Email:       email,
		DisplayName: displayName,
		Username:    user.Login,
	}, nil
}
