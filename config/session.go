package config

import (
	"net/http"
	"time"

	framework "github.com/arandu-io/framework/config"
)

// SessionDriver is where session state is kept.
type SessionDriver string

// The supported drivers. Same contract, same code path: swapping them is one
// line in bootstrap and no change anywhere else (RULE 11).
const (
	// SessionMemory keeps sessions in the process. Right for one instance and
	// wrong for two: behind a load balancer, half the requests land on the
	// replica that never saw the login.
	SessionMemory SessionDriver = "memory"
	// SessionKV keeps them over RESP, shared by every replica.
	SessionKV SessionDriver = "kv"
)

// Session is what config/session.php holds.
type Session struct {
	Driver SessionDriver

	// URL is the RESP endpoint, used only by SessionKV.
	URL string

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

	// Secure marks the cookie HTTPS-only. It follows the environment rather than
	// a variable: a cookie that is Secure in development never reaches a browser
	// on http://localhost, and one that is not in production is one network away
	// from being read.
	Secure bool

	// SameSite is Lax by default, which keeps the session out of cross-site
	// form posts while leaving ordinary navigation working.
	SameSite http.SameSite
}

func loadSession(base framework.Config) Session {
	driver := SessionDriver(env("SESSION_DRIVER", string(SessionMemory)))
	url := env("REDIS_URL", base.RedisURL)
	if driver == SessionKV && url == "" {
		driver = SessionMemory
	}
	return Session{
		Driver:   driver,
		URL:      url,
		TTL:      base.SessionTTL,
		CSRFTTL:  base.CSRFTTL,
		Cookie:   env("SESSION_COOKIE", "arandu_session"),
		Path:     env("SESSION_PATH", "/"),
		Domain:   env("SESSION_DOMAIN", ""),
		Secure:   envBool("SESSION_SECURE", base.Env != framework.EnvDev),
		SameSite: http.SameSiteLaxMode,
	}
}
