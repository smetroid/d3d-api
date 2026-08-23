package token_test

import (
	"testing"
	"time"

	"github.com/dgrijalva/jwt-go"
	tk "github.com/smetroid/d3d-api/app/auth/token"
)

const testKey = "test-signing-key"

func parseToken(t *testing.T, tokenStr, key string) jwt.MapClaims {
	t.Helper()
	parsed, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			t.Fatalf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(key), nil
	})
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok || !parsed.Valid {
		t.Fatal("token not valid")
	}
	return claims
}

func TestCreateToken_RoundTrip(t *testing.T) {
	claims := jwt.MapClaims{"sub": "alice", "role": "admin"}
	tokenStr, err := tk.CreateToken(testKey, claims)
	if err != nil {
		t.Fatal(err)
	}
	got := parseToken(t, tokenStr, testKey)
	if got["sub"] != "alice" || got["role"] != "admin" {
		t.Errorf("claims mismatch: %v", got)
	}
}

func TestCreateToken_WrongKeyFails(t *testing.T) {
	claims := jwt.MapClaims{"sub": "alice"}
	tokenStr, err := tk.CreateToken(testKey, claims)
	if err != nil {
		t.Fatal(err)
	}
	_, err = jwt.Parse(tokenStr, func(_ *jwt.Token) (interface{}, error) {
		return []byte("wrong-key"), nil
	})
	if err == nil {
		t.Error("expected validation error with wrong key")
	}
}

func TestCreateExpiringToken_Claims(t *testing.T) {
	tokenStr := tk.CreateExpiringToken("alice", testKey, time.Hour, "localauth")
	claims := parseToken(t, tokenStr, testKey)

	for field, want := range map[string]string{
		"jti":  "alice",
		"iss":  "localauth",
		"name": "alice",
		"role": "admin",
	} {
		if claims[field] != want {
			t.Errorf("%s: got %v, want %s", field, claims[field], want)
		}
	}
	exp, ok := claims["exp"].(float64)
	if !ok || exp <= float64(time.Now().Unix()) {
		t.Errorf("exp not in the future: %v", exp)
	}
}

func TestCreateExpiringToken_Expired(t *testing.T) {
	tokenStr := tk.CreateExpiringToken("alice", testKey, -time.Second, "localauth")
	_, err := jwt.Parse(tokenStr, func(_ *jwt.Token) (interface{}, error) {
		return []byte(testKey), nil
	})
	if err == nil {
		t.Error("expected error for already-expired token")
	}
}

func TestCreateExpirationFreeAgentToken_Claims(t *testing.T) {
	tokenStr := tk.CreateExpirationFreeAgentToken("agent1", testKey)
	claims := parseToken(t, tokenStr, testKey)

	if claims["jti"] != "agent1" {
		t.Errorf("jti: got %v, want agent1", claims["jti"])
	}
	if claims["iss"] != "samus-token-tool" {
		t.Errorf("iss: got %v, want samus-token-tool", claims["iss"])
	}
	if _, hasExp := claims["exp"]; hasExp {
		t.Error("agent token should not have an exp claim")
	}
	if _, hasIat := claims["iat"]; !hasIat {
		t.Error("agent token should have an iat claim")
	}
}
