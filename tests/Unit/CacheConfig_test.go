package unit_test

import (
	"path/filepath"
	"strings"
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
	cfg, err := loadApplicationConfig(t, store, redisURL)
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	return cfg.Cache
}

func loadApplicationConfig(t *testing.T, store, redisURL string) (appconfig.Config, error) {
	t.Helper()

	t.Setenv("APP_ENV", "dev")
	t.Setenv("APP_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("DATABASE_URL", "sqlite://"+filepath.Join(t.TempDir(), "test.sqlite"))
	t.Setenv("CACHE_STORE", store)
	t.Setenv("REDIS_URL", redisURL)

	return appconfig.Load()
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

// TestTheRESPStoreIsDefinedWhoeverNamesIt.
//
// The endpoint is parsed whether or not the cache defaults to it, because the
// cache is not the only thing that names that store: SESSION_DRIVER=kv beside
// CACHE_STORE=memory is a deployment that shares its sessions and caches inside
// each process, and a store that existed only while the cache happened to
// default to it could not be named by anything else.
//
// Defining a store is not dialling one. Nothing here opens a connection, and
// what does open one is the wiring, when something resolves the store -- see
// TestTheInProcessStoreOpensNoConnection in tests/Feature/Cache_test.go, where
// this same pair of settings produces no connection at all.
func TestTheRESPStoreIsDefinedWhoeverNamesIt(t *testing.T) {
	cfg := loadCacheConfig(t, "memory", "redis://cache.example.test:6380")

	if cfg.Store != appconfig.CacheMemory {
		t.Fatalf("Store = %q, want memory", cfg.Store)
	}
	if cfg.Address != "cache.example.test:6380" {
		t.Errorf("Address = %q, and a session that names this store would have nothing to dial", cfg.Address)
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
			_, err := loadApplicationConfig(t, "redis", c.url)
			if err == nil {
				t.Fatalf("REDIS_URL=%q was accepted, and it configures no store", c.url)
			}
			if !strings.Contains(err.Error(), "REDIS_URL") {
				t.Errorf("error = %q, want it to name REDIS_URL", err)
			}
		})
	}
}
