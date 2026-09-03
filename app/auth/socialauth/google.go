package socialauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// googleUserInfoURL is a variable rather than a constant so tests can point it
// at a local server.
var googleUserInfoURL = "https://www.googleapis.com/oauth2/v2/userinfo"

func NewGoogleConfig(clientID, secret, redirectURL string) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: secret,
		RedirectURL:  redirectURL,
		Scopes:       []string{"openid", "email", "profile"},
		Endpoint:     google.Endpoint,
	}
}

// FetchGoogleProfile exchanges an authorization code and reads the profile.
func FetchGoogleProfile(ctx context.Context, cfg *oauth2.Config, code string) (SocialUserProfile, error) {
	tok, err := cfg.Exchange(ctx, code)
	if err != nil {
		return SocialUserProfile{}, fmt.Errorf("google code exchange: %w", err)
	}

	var body struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := getJSON(ctx, cfg.Client(ctx, tok), googleUserInfoURL, &body); err != nil {
		return SocialUserProfile{}, fmt.Errorf("google userinfo: %w", err)
	}

	// Username is the account's stable id (its OpenID "sub"), not the email
	// local part: an email local part is not unique across domains (ada@gmail.com
	// vs ada@work.com would collide), and emails can change on a Google account
	// while the id cannot. Using the id keeps UNIQUE(username) safe by construction.
	return SocialUserProfile{
		Provider:    "google",
		ProviderID:  body.ID,
		Email:       body.Email,
		DisplayName: body.Name,
		Username:    body.ID,
	}, nil
}

// getJSON performs a GET with the OAuth-authenticated client and decodes the
// response into out.
func getJSON(ctx context.Context, client *http.Client, url string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned %s", url, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
