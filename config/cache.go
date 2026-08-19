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
type Cache struct {
	Store CacheStore

	// Address is host:port, parsed out of the URL. It is empty for CacheMemory,
	// which dials nothing.
	Address string

	// Password authenticates the connection, and it is the password half of the
	// URL's userinfo: redis://:password@host.
	Password string

	// Database is the numbered database the URL's path names. Zero is right for
	// almost everything; separating environments belongs to separate instances,
	// not to db 1.
	Database int

	// Prefix is prepended to every key. It carries the application name so two
	// deployments can share one server without reading each other's entries.
	//
	// It is not the tenant: the tenant is prepended per entry, from the Grant,
	// and never from configuration.
	Prefix string

	// TTL is how long an entry lives when the caller states no lifetime.
	TTL time.Duration
}

func loadCache(base bootstrap.Configuration) Cache {
	store := CacheStore(env("CACHE_STORE", string(CacheMemory)))
	raw := env("REDIS_URL", "")
	if store == CacheRedis && raw == "" {
		// Falling back rather than failing: a cache that silently answers nothing
		// is worse than one that says which variable is missing, and the kernel
		// reports the store it ended up with at boot.
		store = CacheMemory
	}

	cfg := Cache{
		Store:  store,
		Prefix: env("CACHE_PREFIX", base.App.Name+":cache:"),
		TTL:    envSeconds("CACHE_TTL", 10*time.Minute),
	}
	if store == CacheRedis {
		endpoint := parseCacheURL(raw)
		cfg.Address = endpoint.Address
		cfg.Password = endpoint.Password
		cfg.Database = endpoint.Database
	}
	return cfg
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
}

// parseCacheURL reads REDIS_URL into the endpoint the client dials.
//
// A bad value panics rather than falling back to the in-process store. Falling
// back is the failure nobody sees: the application boots, every replica caches
// into memory of its own, and the first person to notice is the one served a
// value another replica already forgot.
func parseCacheURL(raw string) cacheEndpoint {
	raw = strings.TrimSpace(raw)

	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		panic(fmt.Sprintf(`REDIS_URL is not a URL: %q

    REDIS_URL=redis://127.0.0.1:6379
    REDIS_URL=redis://:password@cache.example.com:6379/1`, raw))
	}

	if !strings.EqualFold(u.Scheme, "redis") {
		panic(fmt.Sprintf("REDIS_URL uses the scheme %q, and this application speaks redis: %q", u.Scheme, raw))
	}

	host := u.Hostname()
	if host == "" {
		panic(fmt.Sprintf("REDIS_URL names no host: %q\n\n    REDIS_URL=redis://127.0.0.1:6379", raw))
	}
	port := u.Port()
	if port == "" {
		port = defaultCachePort
	}

	out := cacheEndpoint{Address: net.JoinHostPort(host, port)}

	if u.User != nil {
		out.Password, _ = u.User.Password()
		// Only the password is carried. RESP has named users, the client speaks
		// to the implicit one, and a URL naming another would authenticate as
		// somebody the connection cannot become -- which is a permission denied
		// on the first command rather than at boot.
		if name := u.User.Username(); name != "" && name != "default" {
			panic(fmt.Sprintf(`REDIS_URL names the user %q, and this application connects as the default one: %q

    REDIS_URL=redis://:password@cache.example.com:6379`, name, raw))
		}
	}

	if name := strings.TrimPrefix(u.Path, "/"); name != "" {
		number, err := strconv.Atoi(name)
		if err != nil || number < 0 {
			panic(fmt.Sprintf("REDIS_URL ends in %q, and the path of a RESP URL is the numbered database: %q", u.Path, raw))
		}
		out.Database = number
	}

	return out
}
