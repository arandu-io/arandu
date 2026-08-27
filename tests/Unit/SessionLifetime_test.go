package unit_test

import (
	"path/filepath"
	"testing"
	"time"

	appconfig "github.com/arandu-io/arandu/config"
)

// TestTheSessionLifetimeHasOneReader.
//
// SESSION_TTL was read twice -- once into the session settings the store is
// built from, and once into an auth field nothing ever asked for. Two readers of
// one variable is two answers the day one of them grows a rule the other has
// not, and the copy that answered nothing was the one to go.
//
// What is asserted is that the reader that remains is the one the store is built
// from, and that it carries what the variable said. A second reader cannot be
// asserted against directly -- it would be a compile error to name one that is
// gone -- so this pins the half that has to keep being true.
func TestTheSessionLifetimeHasOneReader(t *testing.T) {
	t.Setenv("APP_ENV", "dev")
	t.Setenv("APP_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("DATABASE_URL", "sqlite://"+filepath.Join(t.TempDir(), "test.sqlite"))
	t.Setenv("SESSION_TTL", "1800")

	cfg, err := appconfig.Load()
	if err != nil {
		t.Fatalf("loading the configuration: %v", err)
	}

	if cfg.Session.TTL != 30*time.Minute {
		t.Errorf("the session lasts %s, want 30m: SESSION_TTL was read and dropped", cfg.Session.TTL)
	}
}
