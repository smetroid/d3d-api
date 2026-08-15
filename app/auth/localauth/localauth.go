package localauth

import (
	"errors"
	"time"

	tk "github.com/smetroid/d3d-api/app/auth/token"
	"github.com/smetroid/d3d-api/app/db/postgres"
	"golang.org/x/crypto/bcrypt"
)

// LocalAuthProvider authenticates users against a bcrypt-hashed password
// stored in the Postgres "users" table. Suitable for local development
// and self-hosted deployments that don't have an LDAP/OAuth server.
type LocalAuthProvider struct {
	signingKey    string
	TokenDuration string `toml:"token_duration"`
	DB            *postgres.Postgres
}

func (p *LocalAuthProvider) SetSigningKey(key string) {
	p.signingKey = key
}

func (p *LocalAuthProvider) Connect() error {
	if p.DB == nil {
		return errors.New("localauth: DB not set")
	}
	return p.DB.InitUsersTable()
}

func (p *LocalAuthProvider) Close() {}

func (p *LocalAuthProvider) Authenticate(username, password string) (bool, string, error) {
	user, err := p.DB.GetUser(username)
	if err != nil || user.Username == "" {
		return false, "", nil
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return false, "", nil
	}

	dur := 24 * time.Hour
	token := tk.CreateExpiringToken(username, p.signingKey, dur, "localauth")
	return true, token, nil
}
