package config

import (
	"time"

	framework "github.com/arandu-io/framework/config"
)

// CacheStore is which backend the cache runs on.
type CacheStore string

// The supported stores. Two, and they are not two ways to do one thing: memory
// is right for one process and wrong for two, and the interface is the same
// (RULE 11 -- an adapter behind an interface is not a mode).
const (
	// CacheMemory keeps entries in the process. Right for development and for a
	// single replica; behind a load balancer, half the requests miss.
	CacheMemory CacheStore = "memory"
	// CacheRedis speaks RESP, which is Dragonfly, Redis, Valkey or KeyDB.
	CacheRedis CacheStore = "redis"
)

// Cache is what config/cache.php holds.
type Cache struct {
	Store CacheStore

	// URL is the RESP endpoint, used only by CacheRedis.
	URL string

	// Prefix is prepended to every key. It carries the application name so two
	// deployments can share one server without reading each other's entries.
	//
	// It is not the tenant: the tenant is prepended per entry, from the Grant,
	// and never from configuration (RULE 14).
	Prefix string

	// TTL is how long an entry lives when the caller states no lifetime.
	TTL time.Duration
}

func loadCache(base framework.Config) Cache {
	store := CacheStore(env("CACHE_STORE", string(CacheMemory)))
	url := env("REDIS_URL", base.RedisURL)
	if store == CacheRedis && url == "" {
		// Falling back rather than failing: a cache that silently answers nothing
		// is worse than one that says which variable is missing, and the kernel
		// reports the store it ended up with at boot.
		store = CacheMemory
	}
	return Cache{
		Store:  store,
		URL:    url,
		Prefix: env("CACHE_PREFIX", base.AppName+":cache:"),
		TTL:    envSeconds("CACHE_TTL", 10*time.Minute),
	}
}
