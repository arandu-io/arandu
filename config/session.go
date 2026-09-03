package config

import (
	"fmt"
	"net/http"
	"time"

	hconfig "github.com/arandu-io/hesape/config"
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
	// SessionKV keeps them over RESP, shared by every replica. It is the store
	// CACHE_STORE spells redis; these two words name one store, and the
	// bootstrap is where that is written down once.
	SessionKV SessionDriver = "kv"
)

// Session is where session state is kept, how long it lasts, and how the cookie
// that carries its id is scoped.
type Session struct {
	Driver SessionDriver

	// TTL is how long a session survives without activity.
	TTL time.Duration

	// CSRFTTL is how long a CSRF token stays valid. Shorter than the session on
	// purpose: a token that outlives the page it was rendered on is a token that
	// can be replayed.
	CSRFTTL time.Duration

	// Cookie is the name of the cookie carrying the session id.
	Cookie string

	// Path and Domain scope the cookie.
	Path   string
	Domain string

	// Secure marks the cookie HTTPS-only. It is present unless the environment
	// names itself as development, rather than absent unless something names
	// production: an environment nobody named is the one running behind a proxy
	// that terminates TLS, and a cookie that travels in the clear because a
	// variable was never written is the failure that looks like nothing at all.
	Secure bool

	// SameSite is Lax by default, which keeps the session out of cross-site
	// form posts while leaving ordinary navigation working.
	SameSite http.SameSite
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
	case SessionKV:
		if cache.Address == "" {
			return Session{}, fmt.Errorf("SESSION_DRIVER %q requires REDIS_URL", driver)
		}
	default:
		return Session{}, fmt.Errorf("SESSION_DRIVER has unsupported value %q; expected memory or kv", driver)
	}
	secure, err := loadSessionSecure()
	if err != nil {
		return Session{}, err
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
		TTL:      ttl,
		CSRFTTL:  csrfTTL,
		Cookie:   env("SESSION_COOKIE", "arandu_session"),
		Path:     env("SESSION_PATH", "/"),
		Domain:   env("SESSION_DOMAIN", ""),
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}, nil
}

// loadSessionSecure answers whether the session cookie carries Secure.
//
// The attribute is present unless APP_ENV was written and names development.
// An environment that names nothing gets it, and that is the direction the
// default has to fail in: the deployment that names nothing is the ordinary one
// behind a proxy, where TLS ends at the proxy and this process listens on http
// inside the network. Neither the scheme this process sees nor a variable
// nobody wrote can report that the browser's connection is https, and guessing
// the permissive way puts the session id on the network in the clear on every
// request, with nothing said at boot.
//
// APP_ENV is read here rather than taken from the parsed environment because
// that value cannot answer the question. The parse turns an absent variable
// into development, so "nobody wrote it" and "somebody wrote dev" arrive as one
// word -- and only the second of the two may drop the attribute. Comparing the
// exact spelling is the whole test, because a value that is neither dev,
// staging nor prod never arrives: the boot refuses it before the session is
// built.
//
// SESSION_SECURE removes the attribute whatever the environment says, and
// development served over http needs one of the two -- a Secure cookie never
// reaches a browser on http://localhost, so the session disappears between
// requests. Naming APP_ENV=dev is the other, and it is what .env.example
// already sets.
func loadSessionSecure() (bool, error) {
	return envBool("SESSION_SECURE", env("APP_ENV", "") != string(hconfig.EnvDev))
}
