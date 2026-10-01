package unit_test

import (
	"path/filepath"
	"strings"
	"testing"

	appconfig "github.com/arandu-io/arandu/config"
	"github.com/arandu-io/framework/foundation/bootstrap"
)

// httpConfig loads the application configuration with the HTTP variables set as
// given and every other one cleared, so a value exported in a shell cannot
// answer for one a case did not set.
func httpConfig(t *testing.T, vars map[string]string) (appconfig.Config, error) {
	t.Helper()

	t.Setenv("APP_ENV", "dev")
	t.Setenv("APP_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("DATABASE_URL", "sqlite://"+filepath.Join(t.TempDir(), "test.sqlite"))
	for _, key := range []string{"HTTP_MAX_BODY_BYTES"} {
		t.Setenv(key, "")
	}
	for key, value := range vars {
		t.Setenv(key, value)
	}

	base, err := bootstrap.LoadConfiguration()
	if err != nil {
		t.Fatalf("loading the framework configuration: %v", err)
	}
	return appconfig.From(base)
}

func TestTheBodyLimitDefaultsToAFormSizedCeiling(t *testing.T) {
	cfg, err := httpConfig(t, nil)
	if err != nil {
		t.Fatalf("From: %v", err)
	}
	if cfg.HTTP.MaxBodyBytes != appconfig.DefaultMaxBodyBytes || appconfig.DefaultMaxBodyBytes != 4<<20 {
		t.Fatalf("MaxBodyBytes = %d, want the 4 MiB default", cfg.HTTP.MaxBodyBytes)
	}

	cfg, err = httpConfig(t, map[string]string{"HTTP_MAX_BODY_BYTES": "52428800"})
	if err != nil {
		t.Fatalf("From: %v", err)
	}
	if cfg.HTTP.MaxBodyBytes != 50<<20 {
		t.Fatalf("MaxBodyBytes = %d, want the 50 MiB that was written", cfg.HTTP.MaxBodyBytes)
	}
}

// TestABodyLimitThatCannotBeUsedIsReportedAtBoot. Zero is refused rather than
// read as "no limit": there is no unbounded body to ask for.
func TestABodyLimitThatCannotBeUsedIsReportedAtBoot(t *testing.T) {
	for _, value := range []string{"0", "-1", "4MB", "lots"} {
		t.Run(value, func(t *testing.T) {
			_, err := httpConfig(t, map[string]string{"HTTP_MAX_BODY_BYTES": value})
			if err == nil {
				t.Fatalf("From accepted HTTP_MAX_BODY_BYTES=%q", value)
			}
			if !strings.Contains(err.Error(), "HTTP_MAX_BODY_BYTES") || !strings.Contains(err.Error(), `"`+value+`"`) {
				t.Errorf("error = %q, want it to name the variable and quote %q", err, value)
			}
		})
	}
}
