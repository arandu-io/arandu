package config

import (
	"fmt"
	"strings"
	"time"
)

// SessionDriver is the cache store session state is kept in.
//
// It names a store rather than inheriting the cache's, so a deployment can
// share its sessions while caching inside each process. The two settings are
// independent on purpose: what the cache loses to a restart is work, and what
// the sessions lose is everybody who was signed in.
type SessionDriver string

// The supported drivers. Same contract, same code path: swapping them is one
// line in bootstrap and no change anywhere else.
const (
	// SessionMemory keeps sessions in the process. Right for one instance and
	// wrong for two: behind a load balancer, half the requests land on the
	// replica that never saw the login.
	SessionMemory SessionDriver = "memory"
	// SessionRedis keeps sessions over RESP, shared by every replica.
	// It names the Redis store independently of CACHE_STORE, and that store is
	// a connector the binary links by a blank import: without it the boot stops
	// naming the import to add.
	SessionRedis SessionDriver = "redis"
)

// Session is where session state is kept and how long it lasts.
//
// Whether the cookie is HTTPS-only is not here. The framework's loader reads
// SESSION_SECURE_COOKIE into Config.Framework.Session.Secure, and that one value
// is what the session store, the CSRF guest cookie and the flash cookie are all
// built with: three cookies that disagreed about Secure would be a session that
// works and a form that answers 419, or the other way round. The name of the
// cookie is not here either, because it is not configurable: the CSRF token is
// bound to the session the cookie of that name carries.
//
// Nor is the scope of the cookie. The session store writes it for path /, for
// the host that answered, with SameSite=Lax, and takes none of the three, so a
// setting for any of them would be read and then ignored.
type Session struct {
	Driver SessionDriver

	// TTL is how long a session survives without activity.
	TTL time.Duration

	// CSRFTTL is how long a CSRF token stays valid. Shorter than the session on
	// purpose: a token that outlives the page it was rendered on is a token that
	// can be replayed.
	CSRFTTL time.Duration
}

// loadSession reads the session settings, against the cache stores that are
// already defined.
//
// It takes the cache rather than reading REDIS_URL a second time. The endpoint
// has one reader -- loadCache -- and a driver that names a store the cache
// configuration did not define is refused here, at the boot, rather than at the
// first request that finds no session where one was written.
func loadSession(cache Cache) (Session, error) {
	driver := SessionDriver(env("SESSION_DRIVER", string(SessionMemory)))
	switch driver {
	case SessionMemory:
	case SessionRedis:
		if cache.Address == "" {
			return Session{}, fmt.Errorf("SESSION_DRIVER %q requires REDIS_URL", driver)
		}
	default:
		return Session{}, fmt.Errorf("SESSION_DRIVER has unsupported value %q; expected memory or redis", driver)
	}
	// SESSION_SECURE is retired and refused rather than ignored, for the
	// reason a retired MAIL_ variable is: SESSION_SECURE=false written for a
	// deployment served over http would otherwise be dropped in silence, and
	// the first sign would be every session disappearing between two
	// requests. SESSION_COOKIE is not refused, though nothing reads it either:
	// every .env copied from an older .env.example carries
	// SESSION_COOKIE=arandu_session, a line that never changed anything.
	if env("SESSION_SECURE", "") != "" {
		return Session{}, fmt.Errorf("SESSION_SECURE is retired; remove it. " +
			"SESSION_SECURE_COOKIE decides the Secure attribute: set it to false only to serve over http outside APP_ENV=dev")
	}
	// The three variables that scoped the cookie are refused for the same
	// reason, when they ask for a cookie the store does not write: a
	// SESSION_DOMAIN written to share sessions across subdomains would be
	// dropped in silence, and every subdomain would sign in on its own. The
	// value that states what the store already writes asks for nothing and is
	// not refused -- every .env copied from an older .env.example carries
	// SESSION_PATH=/.
	for _, retired := range []struct{ name, written string }{
		{"SESSION_PATH", "/"},
		{"SESSION_DOMAIN", ""},
		{"SESSION_SAME_SITE", "lax"},
	} {
		if value := env(retired.name, ""); value != "" && !strings.EqualFold(value, retired.written) {
			return Session{}, fmt.Errorf("%s is retired; remove it. "+
				"The session cookie is written for path /, for the host that answered and with SameSite=Lax, "+
				"and nothing reads %s=%q", retired.name, retired.name, value)
		}
	}
	ttl, err := envSeconds("SESSION_TTL", 12*time.Hour)
	if err != nil {
		return Session{}, err
	}
	csrfTTL, err := envSeconds("CSRF_TTL", 2*time.Hour)
	if err != nil {
		return Session{}, err
	}
	return Session{
		Driver: driver,
		// The two lifetimes are read here, and here only. The session store and
		// the CSRF issuer are built in bootstrap/app.go, from this struct, and
		// they take a duration rather than reading one -- so whoever assembles
		// the application is who states it, and that is this package.
		TTL:     ttl,
		CSRFTTL: csrfTTL,
	}, nil
}
