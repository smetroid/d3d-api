package socialauth

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/oauth2"
)

// oauthServer stands in for a provider: it answers the token exchange at
// /token and whatever profile paths the test registers.
func oauthServer(t *testing.T, routes map[string]string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"at-1","token_type":"Bearer"}`))
	})
	for path, body := range routes {
		body := body
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			if got := r.Header.Get("Authorization"); got != "Bearer at-1" {
				t.Errorf("Authorization = %q, want %q", got, "Bearer at-1")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		})
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func testConfig(srv *httptest.Server) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     "id",
		ClientSecret: "secret",
		RedirectURL:  "http://localhost:5173/auth/callback",
		Endpoint: oauth2.Endpoint{
			AuthURL:  srv.URL + "/auth",
			TokenURL: srv.URL + "/token",
		},
	}
}

func TestFetchGoogleProfile(t *testing.T) {
	srv := oauthServer(t, map[string]string{
		"/userinfo": `{"id":"g-123","email":"ada@example.com","name":"Ada Lovelace"}`,
	})
	googleUserInfoURL = srv.URL + "/userinfo"

	got, err := FetchGoogleProfile(context.Background(), testConfig(srv), "code-1")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	want := SocialUserProfile{
		Provider:    "google",
		ProviderID:  "g-123",
		Email:       "ada@example.com",
		DisplayName: "Ada Lovelace",
		Username:    "g-123",
	}
	if got != want {
		t.Errorf("profile = %+v, want %+v", got, want)
	}
}

func TestFetchGoogleProfileToleratesMissingEmail(t *testing.T) {
	srv := oauthServer(t, map[string]string{
		"/userinfo": `{"id":"g-456","name":"No Mail"}`,
	})
	googleUserInfoURL = srv.URL + "/userinfo"

	got, err := FetchGoogleProfile(context.Background(), testConfig(srv), "code-1")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if got.Username != "g-456" {
		t.Errorf("Username = %q, want the provider id as fallback", got.Username)
	}
	if got.Email != "" {
		t.Errorf("Email = %q, want empty", got.Email)
	}
}

// Two Google accounts sharing an email local part must not collide: the
// username is derived from the id, so UNIQUE(username) holds by construction.
func TestFetchGoogleProfileUsernamesAreUniquePerAccount(t *testing.T) {
	srvA := oauthServer(t, map[string]string{
		"/userinfo": `{"id":"g-111","email":"ada@gmail.com","name":"Ada One"}`,
	})
	googleUserInfoURL = srvA.URL + "/userinfo"
	a, err := FetchGoogleProfile(context.Background(), testConfig(srvA), "code-1")
	if err != nil {
		t.Fatalf("fetch a: %v", err)
	}

	srvB := oauthServer(t, map[string]string{
		"/userinfo": `{"id":"g-222","email":"ada@work.com","name":"Ada Two"}`,
	})
	googleUserInfoURL = srvB.URL + "/userinfo"
	b, err := FetchGoogleProfile(context.Background(), testConfig(srvB), "code-1")
	if err != nil {
		t.Fatalf("fetch b: %v", err)
	}

	if a.Username == b.Username {
		t.Fatalf("two accounts sharing an email local part collided on username %q", a.Username)
	}
}

func TestFetchGitHubProfileUsesPrimaryVerifiedEmail(t *testing.T) {
	srv := oauthServer(t, map[string]string{
		"/user": `{"id":583231,"login":"smetroid","name":"Enrique Carranco"}`,
		"/user/emails": `[{"email":"other@example.com","primary":false,"verified":true},
		                  {"email":"me@example.com","primary":true,"verified":true}]`,
	})
	githubUserURL = srv.URL + "/user"
	githubEmailsURL = srv.URL + "/user/emails"

	got, err := FetchGitHubProfile(context.Background(), testConfig(srv), "code-1")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	want := SocialUserProfile{
		Provider:    "github",
		ProviderID:  "583231",
		Email:       "me@example.com",
		DisplayName: "Enrique Carranco",
		Username:    "smetroid",
	}
	if got != want {
		t.Errorf("profile = %+v, want %+v", got, want)
	}
}

func TestFetchGitHubProfileToleratesNoEmail(t *testing.T) {
	srv := oauthServer(t, map[string]string{
		"/user":        `{"id":99,"login":"ghost","name":""}`,
		"/user/emails": `[]`,
	})
	githubUserURL = srv.URL + "/user"
	githubEmailsURL = srv.URL + "/user/emails"

	got, err := FetchGitHubProfile(context.Background(), testConfig(srv), "code-1")
	if err != nil {
		t.Fatalf("fetch must tolerate a missing email: %v", err)
	}
	if got.Email != "" {
		t.Errorf("Email = %q, want empty", got.Email)
	}
	if got.DisplayName != "ghost" {
		t.Errorf("DisplayName = %q, want the login as fallback", got.DisplayName)
	}
}

// TestFetchGitHubProfileLogsEmailsFetchFailure is the regression test for the
// "Minor" whole-branch finding: a failed /user/emails fetch (outage, 5xx,
// rate-limit) must not be indistinguishable from "this GitHub account has no
// public email". The profile is still allowed to come back with an empty
// Email — that part is correct and unchanged — but the failure must be
// logged so it is operationally visible.
func TestFetchGitHubProfileLogsEmailsFetchFailure(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"at-1","token_type":"Bearer"}`))
	})
	mux.HandleFunc("/user", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":42,"login":"flaky","name":"Flaky User"}`))
	})
	mux.HandleFunc("/user/emails", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	githubUserURL = srv.URL + "/user"
	githubEmailsURL = srv.URL + "/user/emails"

	var logBuf bytes.Buffer
	prevOutput := log.Writer()
	log.SetOutput(&logBuf)
	defer log.SetOutput(prevOutput)

	got, err := FetchGitHubProfile(context.Background(), testConfig(srv), "code-1")
	if err != nil {
		t.Fatalf("fetch must tolerate a failed emails call: %v", err)
	}
	if got.Email != "" {
		t.Errorf("Email = %q, want empty", got.Email)
	}
	if logBuf.Len() == 0 {
		t.Error("expected the emails fetch failure to be logged, got nothing")
	}
}
