// Package socialauth implements the OAuth 2.0 authorization-code flow for
// Google and GitHub sign-in.
//
// It is unrelated to app/auth/oauth, which is a password-grant provider for a
// different backend despite the similar name.
package socialauth

import (
	"errors"
	"fmt"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/google/uuid"
	"github.com/smetroid/d3d-api/app/auth/token"
)

// StateTTL bounds how long an in-flight OAuth handshake may take.
const StateTTL = 10 * time.Minute

// stateIssuer scopes the state JWT to this purpose so a session token can
// never be replayed as a state parameter, or vice versa.
const stateIssuer = "d3d-social-state"

// SocialUserProfile is the provider-agnostic shape both fetchers return.
// Username is the raw provider handle; the namespaced `provider:handle`
// account name is built later, in the database layer.
type SocialUserProfile struct {
	Provider    string
	ProviderID  string
	Email       string
	DisplayName string
	Username    string
}

// GenerateState mints the signed, expiring OAuth state parameter. Signing it
// gives stateless CSRF protection: no server-side store is needed because the
// signature and expiry alone prove we issued it recently.
func GenerateState(signingKey string) (string, error) {
	return token.CreateToken(signingKey, jwt.MapClaims{
		"iss": stateIssuer,
		"exp": time.Now().Add(StateTTL).Unix(),
		"jti": uuid.New().String(),
	})
}

// ValidateState reports whether state is one we issued and has not expired.
func ValidateState(state, signingKey string) error {
	parsed, err := jwt.Parse(state, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method %v", t.Header["alg"])
		}
		return []byte(signingKey), nil
	})
	if err != nil {
		return fmt.Errorf("invalid oauth state: %w", err)
	}
	if !parsed.Valid {
		return errors.New("invalid oauth state")
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return errors.New("invalid oauth state claims")
	}
	if iss, _ := claims["iss"].(string); iss != stateIssuer {
		return errors.New("oauth state was issued for another purpose")
	}
	return nil
}
