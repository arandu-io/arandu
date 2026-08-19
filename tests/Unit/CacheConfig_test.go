package unit_test

import (
	"path/filepath"
	"testing"

	appconfig "github.com/arandu-io/arandu/config"
)

// One URL says where the store is, and everything the client dials comes out of
// it. What is checked here is that nothing is dropped on the way -- a password
// that does not arrive is an authentication failure on the first command, and a
// database number that does not arrive is a process reading somebody else's
// keys.

// loadCacheConfig reads the configuration with the two cache variables set.
func loadCacheConfig(t *testing.T, store, redisURL string) appconfig.Cache {
	t.Helper()

	t.Setenv("APP_ENV", "dev")
	t.Setenv("APP_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("DATABASE_URL", "sqlite://"+filepath.Join(t.TempDir(), "test.sqlite"))
	t.Setenv("CACHE_STORE", store)
	t.Setenv("REDIS_URL", redisURL)

	cfg, err := appconfig.Load()
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	return cfg.Cache
}

func TestTheCacheURLBecomesTheEndpointTheClientDials(t *testing.T) {
	for _, c := range []struct {
		name     string
		url      string
		address  string
		password string
		database int
	}{
		{
			name:    "the port is the RESP one when the URL names none",
			url:     "redis://cache.example.test",
			address: "cache.example.test:6379",
		},
		{
			name:    "a named port wins",
			url:     "redis://cache.example.test:6380",
			address: "cache.example.test:6380",
		},
		{
			// The shape providers hand out: no user, a password, a numbered
			// database.
			name:     "the password and the database travel with the host",
			url:      "redis://:secret@cache.example.test:6380/2",
			address:  "cache.example.test:6380",
			password: "secret",
			database: 2,
		},
		{
			// The implicit user, spelled out. It is what several providers put
			// in the URL and it names nobody else, so it is not a refusal.
			name:     "the default user is the one the client already connects as",
			url:      "redis://default:secret@cache.example.test:6379",
			address:  "cache.example.test:6379",
			password: "secret",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			cfg := loadCacheConfig(t, "redis", c.url)

			if cfg.Store != appconfig.CacheRedis {
				t.Fatalf("Store = %q, want redis", cfg.Store)
			}
			if cfg.Address != c.address {
				t.Errorf("Address = %q, want %q", cfg.Address, c.address)
			}
			if cfg.Password != c.password {
				t.Errorf("Password = %q, want %q", cfg.Password, c.password)
			}
			if cfg.Database != c.database {
				t.Errorf("Database = %d, want %d", cfg.Database, c.database)
			}
		})
	}
}

// TestTheSchemeIsTheOnlyThingThatAsksForEncryption.
//
// There is no variable beside the URL saying the same thing, because a switch
// next to an address is a second place to state what the address already
// states, and on the day they disagree the address is not the one that wins.
func TestTheSchemeIsTheOnlyThingThatAsksForEncryption(t *testing.T) {
	for _, c := range []struct {
		url string
		on  bool
	}{
		{"rediss://cache.example.test:6380", true},
		{"redis://cache.example.test:6379", false},
		// The scheme is compared without regard to case, like every other
		// scheme, so a URL written in capitals is not a connection in the clear.
		{"REDISS://cache.example.test:6380", true},
	} {
		t.Run(c.url, func(t *testing.T) {
			if cfg := loadCacheConfig(t, "redis", c.url); cfg.TLS != c.on {
				t.Errorf("TLS = %v, want %v", cfg.TLS, c.on)
			}
		})
	}
}

// TestTheCertificatesAreFilePathsTheConfigurationCarries.
//
// An environment variable cannot carry a parsed certificate, so it carries
// where one is and the client is built from that -- once, at the edge.
func TestTheCertificatesAreFilePathsTheConfigurationCarries(t *testing.T) {
	t.Setenv("REDIS_CA_FILE", "/etc/cache/ca.pem")
	t.Setenv("REDIS_CERT_FILE", "/etc/cache/client.pem")
	t.Setenv("REDIS_KEY_FILE", "/etc/cache/client-key.pem")
	t.Setenv("REDIS_TLS_SERVER_NAME", "cache.internal")

	cfg := loadCacheConfig(t, "redis", "rediss://10.0.0.7:6380")

	for _, c := range []struct{ field, got, want string }{
		{"TLSCAFile", cfg.TLSCAFile, "/etc/cache/ca.pem"},
		{"TLSCertFile", cfg.TLSCertFile, "/etc/cache/client.pem"},
		{"TLSKeyFile", cfg.TLSKeyFile, "/etc/cache/client-key.pem"},
		{"TLSServerName", cfg.TLSServerName, "cache.internal"},
	} {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.field, c.got, c.want)
		}
	}
}

// TestTheInProcessStoreDialsNothing.
//
// REDIS_URL is read by the session configuration too, so it is set in
// deployments whose cache is the in-process one. Parsing it into an endpoint
// anyway would be an address nothing asked for.
func TestTheInProcessStoreDialsNothing(t *testing.T) {
	cfg := loadCacheConfig(t, "memory", "redis://cache.example.test:6380")

	if cfg.Store != appconfig.CacheMemory {
		t.Fatalf("Store = %q, want memory", cfg.Store)
	}
	if cfg.Address != "" {
		t.Errorf("Address = %q, and the in-process store dials nothing", cfg.Address)
	}
}

// TestAStoreThatCannotBeReachedIsRefusedAtBoot.
//
// The alternative to refusing is a process that starts and caches somewhere
// nobody chose. Every case here is a URL that looks like it configures the
// store and does not.
func TestAStoreThatCannotBeReachedIsRefusedAtBoot(t *testing.T) {
	for _, c := range []struct {
		name string
		url  string
	}{
		{"a scheme this application does not speak", "memcached://cache.example.test:11211"},
		{"no scheme at all", "cache.example.test:6379"},
		{"no host", "redis://"},
		{"a named user the connection cannot become", "redis://reporting:secret@cache.example.test:6379"},
		{"a database that is not a number", "redis://cache.example.test:6379/production"},
	} {
		t.Run(c.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Errorf("REDIS_URL=%q was accepted, and it configures no store", c.url)
				}
			}()
			loadCacheConfig(t, "redis", c.url)
		})
	}
}
