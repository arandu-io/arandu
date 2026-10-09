package unit_test

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appconfig "github.com/arandu-io/arandu/config"
	"github.com/arandu-io/arandu/tests"
)

// TestSessionTTLStopsTheBootNamingSessionLifetime: SESSION_TTL was this
// project's own reading of the lifetime, in seconds, and nothing reads it now.
// A deployment that still writes it is stopped by the framework's loader with
// the line to write instead, rather than having its lifetime replaced by the
// default without a word.
func TestSessionTTLStopsTheBootNamingSessionLifetime(t *testing.T) {
	_, err := loadConfigurationWith(t, map[string]string{"SESSION_TTL": "1800"})
	if err == nil {
		t.Fatal("the boot accepted SESSION_TTL=1800, a variable nothing reads")
	}
	for _, want := range []string{`SESSION_TTL is "1800"`, "SESSION_LIFETIME=30"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to contain %q", err, want)
		}
	}
}

// TestTheEnvExampleBootsWithTheLifetimeItStates loads .env.example as a new
// project's .env, unchanged but for the key `aru key:generate` writes, and
// reads the lifetime the store is built with.
//
// It is the file every project starts from, so a variable in it that the boot
// refuses is a project that does not start the first time it is run. And the
// value it states is twelve hours, what it stated in seconds before the unit
// changed: a .env copied from it keeps the session it had.
func TestTheEnvExampleBootsWithTheLifetimeItStates(t *testing.T) {
	example, err := os.ReadFile(filepath.Join(tests.Root(t), ".env.example"))
	if err != nil {
		t.Fatalf("reading .env.example: %v", err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), example, 0o600); err != nil {
		t.Fatalf("writing .env: %v", err)
	}
	t.Chdir(dir)

	// A variable already in the environment wins over the file, so every one the
	// file names is unset -- through t.Setenv first, which puts back whatever
	// the shell had -- and the file is what answers.
	lines := bufio.NewScanner(bytes.NewReader(example))
	for lines.Scan() {
		line := strings.TrimSpace(lines.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, _, _ := strings.Cut(line, "=")
		t.Setenv(name, "")
		if err := os.Unsetenv(name); err != nil {
			t.Fatalf("unsetting %s: %v", name, err)
		}
	}
	for _, name := range []string{"SESSION_LIFETIME", "SESSION_TTL", "SESSION_PATH", "SESSION_DOMAIN", "SESSION_SAME_SITE"} {
		t.Setenv(name, "")
		if err := os.Unsetenv(name); err != nil {
			t.Fatalf("unsetting %s: %v", name, err)
		}
	}
	t.Setenv("APP_KEY", "0123456789abcdef0123456789abcdef")

	cfg, err := appconfig.Load()
	if err != nil {
		t.Fatalf(".env.example does not boot: %v", err)
	}
	if got, want := cfg.Framework.Session.Lifetime, 12*time.Hour; got != want {
		t.Errorf(".env.example gives a session of %s, want %s: SESSION_LIFETIME=720 is the 43200 seconds it wrote before", got, want)
	}
}
