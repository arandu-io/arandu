package config

import (
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

// Cache is which backend the cache runs on, where it is, and how long an entry
// lives when the caller states no lifetime.
type Cache struct {
	Store CacheStore

	// URL is the RESP endpoint, used only by CacheRedis.
	URL string

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
	url := env("REDIS_URL", "")
	if store == CacheRedis && url == "" {
		// Falling back rather than failing: a cache that silently answers nothing
		// is worse than one that says which variable is missing, and the kernel
		// reports the store it ended up with at boot.
		store = CacheMemory
	}
	return Cache{
		Store:  store,
		URL:    url,
		Prefix: env("CACHE_PREFIX", base.App.Name+":cache:"),
		TTL:    envSeconds("CACHE_TTL", 10*time.Minute),
	}
}
