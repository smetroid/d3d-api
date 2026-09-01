package app

import (
	"os"
	"os/exec"
	"testing"

	"github.com/smetroid/d3d-api/app/config"
	"github.com/smetroid/d3d-api/app/db/postgres"
)

// TestBuildAppFatalsOnEmptyFrontendOrigin is the regression test for I3: an
// empty FrontendOrigin must not silently produce an AllowOrigins list that
// matches no browser origin. BuildApp must log.Fatal at boot instead.
//
// log.Fatal calls os.Exit, so the assertion runs in a subprocess (the
// standard library's own pattern for testing os.Exit paths): the child
// re-invokes this test with BE_CRASHER=1, which drives straight into
// BuildApp with a blank FrontendOrigin; the parent asserts the child exited
// non-zero.
//
// Postgres must be reachable (via TEST_DATABASE_URL) so that the *only*
// possible source of a non-zero exit is the FrontendOrigin guard, not an
// unrelated DB connection failure — otherwise this test would pass whether
// or not the guard exists.
func TestBuildAppFatalsOnEmptyFrontendOrigin(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping")
	}

	if os.Getenv("BE_CRASHER") == "1" {
		// Everything BuildApp needs to run clean to completion is supplied
		// except FrontendOrigin, so a non-zero exit can only come from the
		// guard under test — not a DB failure, not the (later) SigningKey
		// guard, not a nil AuthProvider.
		cfg := config.SamusConfig{Postgres: postgres.Postgres{DSN: dsn}}
		cfg.Samus.AuthProvider = "local"
		cfg.Samus.SigningKey = "test-signing-key"
		BuildApp(cfg)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestBuildAppFatalsOnEmptyFrontendOrigin")
	cmd.Env = append(os.Environ(), "BE_CRASHER=1", "TEST_DATABASE_URL="+dsn)
	err := cmd.Run()

	if exitErr, ok := err.(*exec.ExitError); ok && !exitErr.Success() {
		return
	}
	t.Fatalf("BuildApp with an empty FrontendOrigin (and a reachable DB) did not exit non-zero (err=%v); want log.Fatal", err)
}
