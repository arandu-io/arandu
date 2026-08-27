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

// TestAPoolSettingThatCannotBeUsedIsReportedAtBoot.
//
// The three pool variables are parsed by the framework now, and the refusal
// travelled with them: a value that is present and cannot be used stops the
// boot naming the variable and quoting what it was given. That is checked from
// here because this is the application that boots, and a refusal nobody
// exercises is a refusal that goes quiet without anybody noticing.
//
// Zero and a negative are refused with the unparseable one, and that is the half
// worth having a case for. Zero has two plausible readings -- "give me the
// default" and "take the bound off" -- and there is no unbounded pool to ask
// for, so reading it as either would answer one person's question with the
// other's. Leaving the variable out is how the default is asked for, which the
// case below its own asserts.
//
// The assertions are substrings and not the whole sentence: a check against the
// full message fails on a rewording that changed nothing and passes over a
// message that stopped naming the variable, which is the wrong answer both ways
// round.
func TestAPoolSettingThatCannotBeUsedIsReportedAtBoot(t *testing.T) {
	for _, c := range []struct {
		name  string
		value string
		want  []string
	}{
		{
			name:  "a value that is not a number",
			value: "plenty",
			// "whole number of connections" rather than "integer": it says what
			// the number counts, which is what an operator reading it needs.
			want: []string{"DB_MAX_OPEN_CONNS", `"plenty"`, "whole number of connections"},
		},
		{
			name:  "zero, which is not how the default is asked for",
			value: "0",
			want:  []string{"DB_MAX_OPEN_CONNS", `"0"`, "greater than zero", "Leave it unset"},
		},
		{
			name:  "a negative pool",
			value: "-1",
			want:  []string{"DB_MAX_OPEN_CONNS", `"-1"`, "greater than zero"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := loadConfigurationWith(t, map[string]string{"DB_MAX_OPEN_CONNS": c.value})
			if err == nil {
				t.Fatalf("Load accepted DB_MAX_OPEN_CONNS=%q", c.value)
			}
			for _, want := range c.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error = %q, want it to contain %q", err, want)
				}
			}
			// The message must not hand back a number that looks like the answer.
			// The defaults belong to the package that applies them, and an error
			// printing one invites it to be read as the value to write down --
			// which is how a default comes to be pinned in every .env there is.
			if strings.Contains(err.Error(), "25") {
				t.Errorf("the message prints the default, which invites pinning it: %q", err)
			}
		})
	}
}

// TestThePoolIsLeftToTheAdapterWhenNothingAsks is the other half of the refusal
// above: absent is zero, and zero is how the adapter is asked for its own
// answer. Without this, "leave it unset" is advice nothing checks.
func TestThePoolIsLeftToTheAdapterWhenNothingAsks(t *testing.T) {
	cfg, err := loadConfigurationWith(t, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	conn := cfg.Database.Connection
	if conn.MaxOpenConns != 0 || conn.MaxIdleConns != 0 || conn.ConnMaxLifetime != 0 {
		t.Errorf("an unset pool arrived as %d/%d/%s, want zeroes: a number written here is one "+
			"the adapter no longer decides", conn.MaxOpenConns, conn.MaxIdleConns, conn.ConnMaxLifetime)
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

func TestClosedConfigurationDiscriminatorsFailAtBoot(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want []string
	}{
		{
			name: "cache Redis without its URL",
			env:  map[string]string{"CACHE_STORE": "redis"},
			want: []string{"CACHE_STORE", `"redis"`, "REDIS_URL"},
		},
		{
			name: "KV session without its URL",
			env:  map[string]string{"SESSION_DRIVER": "kv"},
			want: []string{"SESSION_DRIVER", `"kv"`, "REDIS_URL"},
		},
		{
			name: "Redis queue without its URL",
			env:  map[string]string{"QUEUE_CONNECTION": "redis"},
			want: []string{"QUEUE_CONNECTION", `"redis"`, "REDIS_URL"},
		},
		{
			name: "KV session with an unknown URL scheme",
			env: map[string]string{
				"SESSION_DRIVER": "kv",
				"REDIS_URL":      "memcached://cache.example.test:11211",
			},
			want: []string{"REDIS_URL", `"memcached"`, "redis and rediss"},
		},
		{
			name: "Redis queue with an unknown URL scheme",
			env: map[string]string{
				"QUEUE_CONNECTION": "redis",
				"REDIS_URL":        "memcached://cache.example.test:11211",
			},
			want: []string{"REDIS_URL", `"memcached"`, "redis and rediss"},
		},
		{
			name: "unknown cache store",
			env:  map[string]string{"CACHE_STORE": "memcached"},
			want: []string{"CACHE_STORE", `"memcached"`, "memory", "redis"},
		},
		{
			name: "unknown session driver",
			env:  map[string]string{"SESSION_DRIVER": "database"},
			want: []string{"SESSION_DRIVER", `"database"`, "memory", "kv"},
		},
		{
			name: "unknown queue connection",
			env:  map[string]string{"QUEUE_CONNECTION": "sqs"},
			want: []string{"QUEUE_CONNECTION", `"sqs"`, "database", "redis"},
		},
		{
			name: "unknown filesystem disk",
			env:  map[string]string{"FILESYSTEM_DISK": "gcs"},
			want: []string{"FILESYSTEM_DISK", `"gcs"`, "local", "r2", "s3"},
		},
		{
			name: "unknown log format",
			env:  map[string]string{"LOG_FORMAT": "yaml"},
			want: []string{"LOG_FORMAT", `"yaml"`, "json", "text"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := loadConfigurationWith(t, test.env)
			if err == nil {
				t.Fatal("Load accepted an invalid discriminator")
			}
			for _, want := range test.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error = %q, want it to contain %q", err, want)
				}
			}
		})
	}
}

func TestClosedConfigurationDiscriminatorsAcceptDocumentedValues(t *testing.T) {
	t.Run("explicitly empty values keep the development defaults", func(t *testing.T) {
		cfg, err := loadConfigurationWith(t, nil)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.Cache.Store != appconfig.CacheMemory || cfg.Session.Driver != appconfig.SessionMemory ||
			cfg.Queue.Connection != appconfig.QueueDatabase || cfg.Filesystems.Default != appconfig.DiskLocal ||
			cfg.Logging.Format != "text" {
			t.Fatalf("defaults = cache %q, session %q, queue %q, disk %q, log %q",
				cfg.Cache.Store, cfg.Session.Driver, cfg.Queue.Connection, cfg.Filesystems.Default, cfg.Logging.Format)
		}
	})

	t.Run("shared drivers and the R2 disk are accepted", func(t *testing.T) {
		cfg, err := loadConfigurationWith(t, map[string]string{
			"CACHE_STORE":      "redis",
			"SESSION_DRIVER":   "kv",
			"QUEUE_CONNECTION": "redis",
			"FILESYSTEM_DISK":  "r2",
			"LOG_FORMAT":       "json",
			"REDIS_URL":        "rediss://cache.example.test:6380",
		})
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.Cache.Store != appconfig.CacheRedis || cfg.Session.Driver != appconfig.SessionKV ||
			cfg.Queue.Connection != appconfig.QueueRedis || cfg.Filesystems.Default != appconfig.DiskR2 ||
			cfg.Logging.Format != "json" {
			t.Fatalf("configured = cache %q, session %q, queue %q, disk %q, log %q",
				cfg.Cache.Store, cfg.Session.Driver, cfg.Queue.Connection, cfg.Filesystems.Default, cfg.Logging.Format)
		}
	})

	t.Run("the S3 disk remains supported", func(t *testing.T) {
		cfg, err := loadConfigurationWith(t, map[string]string{"FILESYSTEM_DISK": "s3"})
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if cfg.Filesystems.Default != appconfig.DiskS3 {
			t.Fatalf("Filesystems.Default = %q, want s3", cfg.Filesystems.Default)
		}
	})
}

func loadConfigurationWith(t *testing.T, values map[string]string) (appconfig.Config, error) {
	t.Helper()
	t.Chdir(t.TempDir())
	t.Setenv("APP_ENV", "dev")
	t.Setenv("APP_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("DATABASE_URL", "sqlite://"+filepath.Join(t.TempDir(), "test.sqlite"))
	for _, key := range []string{
		"CACHE_STORE", "SESSION_DRIVER", "QUEUE_CONNECTION", "FILESYSTEM_DISK",
		"LOG_FORMAT", "REDIS_URL",
		"SESSION_SECURE", "SESSION_TTL", "CSRF_TTL",
		"DB_MAX_OPEN_CONNS", "DB_MAX_IDLE_CONNS", "DB_CONN_MAX_LIFETIME",
		"CACHE_TTL", "QUEUE_WORKERS", "QUEUE_RETRY_AFTER", "QUEUE_MAX_ATTEMPTS",
		"AUTH_PASSWORD_MIN_LENGTH", "AUTH_PASSWORD_RESET_TTL",
		"MAIL_URL", "MAIL_MAILER", "MAIL_HOST", "MAIL_PORT", "MAIL_USERNAME",
		"MAIL_PASSWORD", "MAIL_ENCRYPTION", "MAIL_KEY",
	} {
		t.Setenv(key, "")
	}
	for key, value := range values {
		t.Setenv(key, value)
	}
	return appconfig.Load()
}
