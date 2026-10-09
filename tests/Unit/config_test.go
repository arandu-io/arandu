package unit_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/auth"

	appconfig "github.com/arandu-io/arandu/config"
)

// The session cookie is the whole credential: whoever reads it is signed in.
// What this file checks is the three attributes that decide who can, read off
// the cookie rather than off the configuration that produced it.
//
// Secure has one reader, the framework's loader: SESSION_SECURE_COOKIE when it
// is written, and otherwise Secure in every environment except dev. APP_ENV
// left unset counts as dev there, as it does for APP_DEBUG and every other
// development surface, so a deployment fixes an unnamed environment by naming
// it rather than by a second variable.

// unstatedEnv puts one test in a directory with no .env and blanks the variables
// that decide the cookie, so what is asserted is a property of the code and not
// of the shell the suite was started from.
//
// It is not loadConfigurationWith, which the rest of this suite uses: that one
// states APP_ENV=dev, and an environment that states nothing is one of the
// cases here. Blank is how absent is spelled -- every reader of these
// variables falls back on an empty value -- and t.Setenv puts back whatever the
// caller had.
func unstatedEnv(t *testing.T) {
	t.Helper()

	t.Chdir(t.TempDir())
	t.Setenv("APP_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("DATABASE_URL", "sqlite://"+filepath.Join(t.TempDir(), "test.sqlite"))
	for _, key := range []string{
		"APP_ENV", "APP_DEBUG", "APP_URL", "GEO_ENABLED", "GEO_INDEXING_ENABLED", "GEO_SURFACES",
		"SESSION_SECURE", "SESSION_COOKIE", "SESSION_SECURE_COOKIE", "SESSION_DRIVER", "SESSION_TTL", "CSRF_TTL", "CACHE_STORE", "REDIS_URL",
		"SESSION_PATH", "SESSION_DOMAIN", "SESSION_SAME_SITE",
	} {
		t.Setenv(key, "")
	}
}

// sessionCookie returns the cookie a started session writes under one
// environment.
//
// The store is built the way bootstrap/app.go builds it -- the same
// constructor, the same three settings out of the same loaded configuration,
// Secure from the framework's half of it --
// and the backend is left to its in-process default, because what is read here
// is the cookie and no session outlives the call. Nothing is booted: this
// answers what the bytes on the wire say.
func sessionCookie(t *testing.T, values map[string]string) *http.Cookie {
	t.Helper()

	unstatedEnv(t)
	for key, value := range values {
		t.Setenv(key, value)
	}

	cfg, err := appconfig.Load()
	if err != nil {
		t.Fatalf("loading the configuration: %v", err)
	}
	store := security.NewSessionStore(cfg.Framework.App.Key, cfg.Session.TTL, cfg.Framework.Session.Secure, nil)

	// Rotate rather than Start, because it is the call a sign-in makes: keeping
	// the id somebody arrived holding is session fixation, and this is the seam
	// that replaces it.
	response := httptest.NewRecorder()
	subject := auth.Subject{
		ID:     "11111111-1111-4111-8111-111111111111",
		Tenant: "22222222-2222-4222-8222-222222222222",
	}
	if _, err := store.Rotate(context.Background(), response, "", subject); err != nil {
		t.Fatalf("starting a session: %v", err)
	}

	cookies := response.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("a started session wrote %d cookies, want exactly the session cookie", len(cookies))
	}
	return cookies[0]
}

// TestTheSessionCookieIsHTTPSOnlyWhereverItWasStatedToBe walks the ways an
// environment can answer the question and reads the answer off the cookie.
//
// The variable decides when it is written; the environment decides when it is
// not, and only dev drops the attribute. The address never decides.
func TestTheSessionCookieIsHTTPSOnlyWhereverItWasStatedToBe(t *testing.T) {
	for _, c := range []struct {
		name  string
		env   map[string]string
		want  bool
		about string
	}{
		{
			name:  "a stated development environment",
			env:   map[string]string{"APP_ENV": "dev"},
			want:  false,
			about: "a Secure cookie is not sent to http://localhost, so a developer would be handed a browser that discards every session",
		},
		{
			name:  "a stated staging environment",
			env:   map[string]string{"APP_ENV": "staging"},
			want:  true,
			about: "the session travels over the network as the plain credential it is",
		},
		{
			name:  "a stated production environment",
			env:   map[string]string{"APP_ENV": "prod"},
			want:  true,
			about: "the session travels over the network as the plain credential it is",
		},
		{
			name:  "no environment named, which counts as dev",
			env:   map[string]string{},
			want:  false,
			about: "APP_ENV left unset is dev for every development surface, and naming the environment is the fix",
		},
		{
			name:  "SESSION_SECURE_COOKIE alone, with no environment named",
			env:   map[string]string{"SESSION_SECURE_COOKIE": "true"},
			want:  true,
			about: "the variable answers the question by itself",
		},
		{
			name:  "SESSION_SECURE_COOKIE off, declared, in production",
			env:   map[string]string{"APP_ENV": "prod", "SESSION_SECURE_COOKIE": "false"},
			want:  false,
			about: "a declared no is how a deployment served over http outside dev says so",
		},
		{
			name:  "an https address in dev",
			env:   map[string]string{"APP_ENV": "dev", "APP_URL": "https://app.example.test"},
			want:  false,
			about: "the address takes no part: dev is where http://localhost has to keep working",
		},
		{
			name:  "a production environment on a plain address",
			env:   map[string]string{"APP_ENV": "prod", "APP_URL": "http://app.example.test"},
			want:  true,
			about: "behind a proxy that ends TLS the address is http and the browser's connection is not",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			cookie := sessionCookie(t, c.env)
			if cookie.Secure != c.want {
				t.Errorf("the session cookie is written with Secure=%v, want %v: %s",
					cookie.Secure, c.want, c.about)
			}
		})
	}
}

// TestTheSessionCookieIsUnreadableByScriptAndNotSentCrossSite pins the two
// attributes nothing configures.
//
// The store writes them, so no variable can turn them off and nothing else in
// this project would notice if a future store stopped writing them. HTTPOnly is
// what keeps a script that reaches the page from reading the credential;
// SameSite=Lax is what keeps a cross-site form post from arriving authenticated.
func TestTheSessionCookieIsUnreadableByScriptAndNotSentCrossSite(t *testing.T) {
	cookie := sessionCookie(t, map[string]string{"APP_ENV": "prod"})

	if !cookie.HttpOnly {
		t.Error("the session cookie is readable from JavaScript: any script that reaches the page reads the credential")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite = %v, want Lax: the session is sent with cross-site requests, which is the "+
			"request CSRF protection exists to refuse", cookie.SameSite)
	}
}

// TestTheSessionCookieIsScopedToTheWholeHost pins the scope the boot holds a
// deployment to. SESSION_PATH, SESSION_DOMAIN and SESSION_SAME_SITE are refused
// at boot, and the refusal says the cookie is written for path / and for the
// host that answered; a store that started writing another scope would make
// that sentence false, and this is what notices.
func TestTheSessionCookieIsScopedToTheWholeHost(t *testing.T) {
	cookie := sessionCookie(t, map[string]string{"APP_ENV": "prod"})

	if cookie.Path != "/" {
		t.Errorf("the session cookie is written for path %q, want /: the refusal of SESSION_PATH says otherwise", cookie.Path)
	}
	if cookie.Domain != "" {
		t.Errorf("the session cookie is written for domain %q, want none (host-only): the refusal of SESSION_DOMAIN says otherwise", cookie.Domain)
	}
}
