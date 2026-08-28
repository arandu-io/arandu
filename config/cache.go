package config

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/arandu-io/framework/foundation/bootstrap"
)

// CacheStore is which backend the cache runs on.
type CacheStore string

// The supported stores. Two, and they are not two ways to do one thing: memory
// is right for one process and wrong for two, and the interface is the same --
// an adapter behind an interface is not a mode.
const (
	// CacheMemory keeps entries in the process. Right for development and for a
	// single replica; behind a load balancer, half the requests miss.
	CacheMemory CacheStore = "memory"
	// CacheRedis speaks RESP, which is Dragonfly, Redis, Valkey or KeyDB.
	CacheRedis CacheStore = "redis"
)

// defaultCachePort is the port RESP listens on when the URL names none.
const defaultCachePort = "6379"

// Cache is which backend the cache runs on, where it is, and how long an entry
// lives when the caller states no lifetime.
//
// The endpoint comes from one URL, for the same reason the mailer's does: the
// scheme is the transport, so a configuration cannot ask for one thing and
// carry the settings of another, and a credential cannot be left behind in a
// variable the new endpoint does not read.
//
//	REDIS_URL=redis://127.0.0.1:6379
//	REDIS_URL=redis://:password@cache.example.com:6379/1
//	REDIS_URL=rediss://:password@cache.example.com:6380    encrypted
type Cache struct {
	Store CacheStore

	// Address is host:port, parsed out of the URL. It is empty when REDIS_URL
	// names no endpoint, and only then.
	//
	// It is parsed whether or not Store is the RESP one, because the cache is
	// not the only thing that names that store: a deployment whose sessions are
	// shared and whose cache stays in the process is a coherent one, and a
	// store that existed only while the cache happened to default to it could
	// not be named by anything else.
	//
	// One reader, here. Every feature that needs the endpoint reads this field
	// rather than REDIS_URL, because two parsers of one variable are two
	// answers the day one of them grows a default the other has not.
	Address string

	// Password authenticates the connection, and it is the password half of the
	// URL's userinfo: redis://:password@host.
	Password string

	// Database is the numbered database the URL's path names. Zero is right for
	// almost everything; separating environments belongs to separate instances,
	// not to db 1.
	Database int

	// TLS says the connection is encrypted, and it is the scheme of the URL:
	// rediss:// asks for it and redis:// does not.
	//
	// The scheme rather than a variable beside the URL, for the reason the
	// mailer's encryption is smtps:// rather than MAIL_ENCRYPTION: a switch next
	// to an address is a second place saying what the address already says, and
	// the day they disagree the address is not the one that wins.
	//
	// It is off by default because a client that demanded it would refuse every
	// server that does not offer it. On any network this process does not own,
	// turning it on is the only correct setting: without it the password, the
	// session ids and every cached value cross the wire in the clear.
	TLS bool

	// TLSCAFile is the authority that signed the server's certificate, when it
	// is not one the system already trusts.
	//
	// These four are file paths rather than the certificates themselves,
	// because an environment variable carries a string and a connection needs
	// parsed keys. They are read once, where the client is built.
	//
	// A managed endpoint needs none of them: its certificate is signed by a
	// public authority and the host half of the URL is the name that
	// certificate has to carry. A self-hosted server needs all of them, and
	// that is the case they exist for -- it presents a certificate from an
	// authority of its own and, by default, asks for one back.
	TLSCAFile string

	// TLSCertFile and TLSKeyFile are this client's certificate and its private
	// key, for a server that asks the client to prove who it is. Naming one
	// without the other is refused: half a pair authenticates nobody.
	TLSCertFile string
	TLSKeyFile  string

	// TLSServerName is the name the server's certificate must carry, when it is
	// not the host of the URL. It is what an address reached through a tunnel or
	// by IP needs, and setting it wrong is the one way to weaken the check
	// without turning it off.
	TLSServerName string

	// Prefix is prepended to every key. It carries the application name so two
	// deployments can share one server without reading each other's entries.
	//
	// It is not the tenant: the tenant is prepended per entry, from the Grant,
	// and never from configuration.
	Prefix string

	// TTL is how long an entry lives when the caller states no lifetime.
	TTL time.Duration
}

func loadCache(base bootstrap.Configuration) (Cache, error) {
	store := CacheStore(env("CACHE_STORE", string(CacheMemory)))
	raw := env("REDIS_URL", "")
	switch store {
	case CacheMemory:
	case CacheRedis:
		if raw == "" {
			return Cache{}, fmt.Errorf("CACHE_STORE %q requires REDIS_URL", store)
		}
	default:
		return Cache{}, fmt.Errorf("CACHE_STORE has unsupported value %q; expected memory or redis", store)
	}

	ttl, err := envSeconds("CACHE_TTL", 10*time.Minute)
	if err != nil {
		return Cache{}, err
	}
	cfg := Cache{
		Store:         store,
		Prefix:        env("CACHE_PREFIX", base.App.Name+":cache:"),
		TTL:           ttl,
		TLSCAFile:     env("REDIS_CA_FILE", ""),
		TLSCertFile:   env("REDIS_CERT_FILE", ""),
		TLSKeyFile:    env("REDIS_KEY_FILE", ""),
		TLSServerName: env("REDIS_TLS_SERVER_NAME", ""),
	}
	// Parsed whenever it is set, and not only when the cache uses it. A URL that
	// is set and cannot be read is a configuration error wherever it was meant
	// to be used, and the store it defines is named by more than the cache --
	// see Address.
	if raw != "" {
		endpoint, err := parseCacheURL(raw)
		if err != nil {
			return Cache{}, err
		}
		cfg.Address = endpoint.Address
		cfg.Password = endpoint.Password
		cfg.Database = endpoint.Database
		cfg.TLS = endpoint.TLS
	}
	return cfg, nil
}

// cacheEndpoint is what one URL says about where the store is.
//
// It is separate from Cache so that parsing answers only what it parsed: the
// prefix and the lifetime are read from their own variables, and a parser that
// returned a whole Cache would be a parser that could quietly blank them.
type cacheEndpoint struct {
	Address  string
	Password string
	Database int
	TLS      bool
}

// parseCacheURL reads REDIS_URL into the endpoint the client dials.
//
// A bad value is returned to the boot boundary rather than falling back to the
// in-process store. Falling back would let every replica cache into memory of
// its own while reporting a successful boot.
func parseCacheURL(raw string) (cacheEndpoint, error) {
	raw = strings.TrimSpace(raw)

	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		return cacheEndpoint{}, fmt.Errorf("REDIS_URL has invalid URL value %q", raw)
	}

	// rediss is the encrypted connection and redis is the one in the clear, the
	// same way smtps and smtp name the two SMTP sessions. Naming the difference
	// in the scheme is what stops a configuration from carrying an address and a
	// switch beside it that say different things.
	encrypted := strings.EqualFold(u.Scheme, "rediss")
	if !encrypted && !strings.EqualFold(u.Scheme, "redis") {
		return cacheEndpoint{}, fmt.Errorf("REDIS_URL uses unsupported scheme %q; this application supports redis and rediss", u.Scheme)
	}

	host := u.Hostname()
	if host == "" {
		return cacheEndpoint{}, fmt.Errorf("REDIS_URL names no host")
	}
	port := u.Port()
	if port == "" {
		port = defaultCachePort
	}

	out := cacheEndpoint{Address: net.JoinHostPort(host, port), TLS: encrypted}

	if u.User != nil {
		out.Password, _ = u.User.Password()
		// Only the password is carried. RESP has named users, the client speaks
		// to the implicit one, and a URL naming another would authenticate as
		// somebody the connection cannot become -- which is a permission denied
		// on the first command rather than at boot.
		if name := u.User.Username(); name != "" && name != "default" {
			return cacheEndpoint{}, fmt.Errorf("REDIS_URL names unsupported user %q; expected the default user", name)
		}
	}

	if name := strings.TrimPrefix(u.Path, "/"); name != "" {
		number, err := strconv.Atoi(name)
		if err != nil || number < 0 {
			return cacheEndpoint{}, fmt.Errorf("REDIS_URL has invalid database path %q; expected a non-negative integer", u.Path)
		}
		out.Database = number
	}

	return out, nil
}

// validateRedisURL applies the connection constraints shared by every feature
// that selects the RESP endpoint without needing its parsed cache fields.
func validateRedisURL(raw string) error {
	_, err := parseCacheURL(raw)
	return err
}
