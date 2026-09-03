package socialauth

import (
	"testing"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/smetroid/d3d-api/app/auth/token"
)

const testKey = "test-signing-key"

func TestStateRoundTrip(t *testing.T) {
	state, err := GenerateState(testKey)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if err := ValidateState(state, testKey); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestValidateStateRejectsWrongKey(t *testing.T) {
	state, err := GenerateState(testKey)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if err := ValidateState(state, "other-key"); err == nil {
		t.Fatal("expected error for a state signed with a different key")
	}
}

func TestValidateStateRejectsExpired(t *testing.T) {
	expired, err := token.CreateToken(testKey, jwt.MapClaims{
		"iss": stateIssuer,
		"exp": time.Now().Add(-time.Minute).Unix(),
		"jti": "expired",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := ValidateState(expired, testKey); err == nil {
		t.Fatal("expected error for an expired state")
	}
}

func TestValidateStateRejectsForeignIssuer(t *testing.T) {
	foreign, err := token.CreateToken(testKey, jwt.MapClaims{
		"iss": "some-other-issuer",
		"exp": time.Now().Add(time.Minute).Unix(),
		"jti": "foreign",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := ValidateState(foreign, testKey); err == nil {
		t.Fatal("expected error for a token issued for another purpose")
	}
}

func TestValidateStateRejectsGarbage(t *testing.T) {
	if err := ValidateState("not-a-jwt", testKey); err == nil {
		t.Fatal("expected error for a malformed state")
	}
}
