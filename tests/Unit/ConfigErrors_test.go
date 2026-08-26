package unit_test

import (
	"path/filepath"
	"strings"
	"testing"

	appconfig "github.com/arandu-io/arandu/config"
	"github.com/arandu-io/framework/foundation/bootstrap"
)

func TestInvalidBooleanConfigurationIsReportedAtBoot(t *testing.T) {
	t.Setenv("APP_ENV", "dev")
	t.Setenv("APP_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("DATABASE_URL", "sqlite://"+filepath.Join(t.TempDir(), "test.sqlite"))
	t.Setenv("SESSION_SECURE", "sometimes")

	base, err := bootstrap.LoadConfiguration()
	if err != nil {
		t.Fatalf("loading the framework configuration: %v", err)
	}
	_, err = appconfig.From(base)
	if err == nil {
		t.Fatal("From accepted an invalid SESSION_SECURE value")
	}
	for _, want := range []string{"SESSION_SECURE", `"sometimes"`, "boolean"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to contain %q", err, want)
		}
	}
}

func TestInvalidIntegerConfigurationIsReportedAtBoot(t *testing.T) {
	t.Setenv("APP_ENV", "dev")
	t.Setenv("APP_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("DATABASE_URL", "sqlite://"+filepath.Join(t.TempDir(), "test.sqlite"))
	t.Setenv("DB_MAX_OPEN_CONNS", "plenty")

	_, err := appconfig.Load()
	if err == nil {
		t.Fatal("Load accepted an invalid DB_MAX_OPEN_CONNS value")
	}
	for _, want := range []string{"DB_MAX_OPEN_CONNS", `"plenty"`, "integer"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to contain %q", err, want)
		}
	}
}

func TestInvalidRedisURLIsReportedAtBoot(t *testing.T) {
	t.Setenv("APP_ENV", "dev")
	t.Setenv("APP_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("DATABASE_URL", "sqlite://"+filepath.Join(t.TempDir(), "test.sqlite"))
	t.Setenv("CACHE_STORE", "redis")
	t.Setenv("REDIS_URL", "memcached://cache.example.test:11211")

	_, err := appconfig.Load()
	if err == nil {
		t.Fatal("Load accepted an unsupported REDIS_URL scheme")
	}
	for _, want := range []string{"REDIS_URL", `"memcached"`, "redis and rediss"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to contain %q", err, want)
		}
	}
}

func TestInvalidMailURLIsReportedAtBoot(t *testing.T) {
	t.Setenv("APP_ENV", "dev")
	t.Setenv("APP_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("DATABASE_URL", "sqlite://"+filepath.Join(t.TempDir(), "test.sqlite"))
	t.Setenv("MAIL_URL", "ses://secret")

	_, err := appconfig.Load()
	if err == nil {
		t.Fatal("Load accepted an unsupported MAIL_URL scheme")
	}
	for _, want := range []string{"MAIL_URL", `"ses"`, "unsupported"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to contain %q", err, want)
		}
	}
}

func TestRetiredMailVariableIsReportedAtBoot(t *testing.T) {
	t.Setenv("APP_ENV", "dev")
	t.Setenv("APP_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("DATABASE_URL", "sqlite://"+filepath.Join(t.TempDir(), "test.sqlite"))
	t.Setenv("MAIL_HOST", "smtp.example.test")

	_, err := appconfig.Load()
	if err == nil {
		t.Fatal("Load accepted the retired MAIL_HOST variable")
	}
	for _, want := range []string{"MAIL_HOST", "retired", "MAIL_URL"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to contain %q", err, want)
		}
	}
}

func TestFromRejectsInvalidFrameworkConfigurationWithoutPanicking(t *testing.T) {
	_, err := appconfig.From(bootstrap.Configuration{})
	if err == nil {
		t.Fatal("From accepted an invalid framework configuration")
	}
	if !strings.Contains(err.Error(), "APP_ENV") {
		t.Errorf("error = %q, want it to identify APP_ENV", err)
	}
}

func TestLoadPreservesTheFrameworkBootstrapError(t *testing.T) {
	t.Setenv("APP_ENV", "dev")
	t.Setenv("APP_KEY", "short")
	t.Setenv("DATABASE_URL", "sqlite://"+filepath.Join(t.TempDir(), "test.sqlite"))
	t.Setenv("SESSION_SECURE", "sometimes")

	_, err := appconfig.Load()
	if err == nil {
		t.Fatal("Load accepted an invalid APP_KEY")
	}
	if !strings.Contains(err.Error(), "APP_KEY") {
		t.Errorf("error = %q, want it to preserve the APP_KEY bootstrap error", err)
	}
	if strings.Contains(err.Error(), "SESSION_SECURE") {
		t.Errorf("error = %q, and the later application error masked the bootstrap error", err)
	}
}
