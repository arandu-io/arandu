package unit_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/arandu-io/framework/security"

	appconfig "github.com/arandu-io/arandu/config"
)

// The session cookie is the whole credential: whoever reads it is signed in.
// What this file checks is the three attributes that decide who can, read off
// the cookie rather than off the configuration that produced it.
//
// Secure used to be derived from APP_ENV alone, and APP_ENV has a default of
// development -- the one environment where the cookie is allowed to travel over
// http. A deployment that set neither APP_ENV nor SESSION_SECURE therefore sent
// its sessions unprotected, and nothing said so at the boot or afterwards.

// unstatedEnv puts one test in a directory with no .env and blanks the variables
// that decide the cookie, so what is asserted is a property of the code and not
// of the shell the suite was started from.
//
// It is not loadConfigurationWith, which the rest of this suite uses: that one
// states APP_ENV=dev, and an environment that states nothing is exactly the
// subject here. Blank is how absent is spelled -- every reader of these
// variables falls back on an empty value -- and t.Setenv puts back whatever the
// caller had.
func unstatedEnv(t *testing.T) {
	t.Helper()

	t.Chdir(t.TempDir())
	t.Setenv("APP_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("DATABASE_URL", "sqlite://"+filepath.Join(t.TempDir(), "test.sqlite"))
	for _, key := range []string{
		"APP_ENV", "APP_DEBUG", "APP_URL", "SESSION_SECURE", "SESSION_SECURE_COOKIE",
		"SESSION_DRIVER", "SESSION_TTL", "CSRF_TTL", "CACHE_STORE", "REDIS_URL",
	} {
		t.Setenv(key, "")
	}
}

// TestAnUnnamedEnvironmentDoesNotUndoAnHTTPSDeployment is the regression.
//
// APP_ENV was the whole of what decided this, and it falls back to development
// -- the one environment where the cookie may travel over http. So a deployment
// that named its address and not its environment served every session in the
// clear, and nothing said so at the boot or on any request afterwards.
func TestAnUnnamedEnvironmentDoesNotUndoAnHTTPSDeployment(t *testing.T) {
	cookie := sessionCookie(t, map[string]string{"APP_URL": "https://app.example.test"})

	if !cookie.Secure {
		t.Error("an application serving on https writes its session cookie without Secure when APP_ENV is " +
			"unset: the environment falls back to development, and the credential goes out over the network")
	}
}

// sessionCookie returns the cookie a started session writes under one
// environment.
//
// The store is built the way bootstrap/app.go builds it -- the same
// constructor, the same three settings out of the same loaded configuration --
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
	store := security.NewSessionStore(cfg.Framework.App.Key, cfg.Session.TTL, cfg.Session.Secure, nil)

	// Rotate rather than Start, because it is the call a sign-in makes: keeping
	// the id somebody arrived holding is session fixation, and this is the seam
	// that replaces it.
	response := httptest.NewRecorder()
	subject := security.Subject{
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
// Both variables are here because either is enough on its own: a deployment that
// says what it wants of the cookie does not also have to say what environment it
// is, and one that says what environment it is does not have to spell the cookie
// out.
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
			name:  "SESSION_SECURE alone, with no environment named",
			env:   map[string]string{"SESSION_SECURE": "true"},
			want:  true,
			about: "the variable answers the question by itself",
		},
		{
			name:  "SESSION_SECURE off, deliberately, with no environment named",
			env:   map[string]string{"SESSION_SECURE": "false"},
			want:  false,
			about: "a stated no is a decision, and it is the silent no the refusal was written against",
		},
		{
			name:  "SESSION_SECURE overriding a stated environment",
			env:   map[string]string{"APP_ENV": "prod", "SESSION_SECURE": "false"},
			want:  false,
			about: "the variable is what a deployment reaches for when the environment name is not the whole story",
		},
		{
			name:  "an https address with no environment named",
			env:   map[string]string{"APP_URL": "https://app.example.test"},
			want:  true,
			about: "the address is stated by every deployment, because its absolute links are built from it",
		},
		{
			name:  "a production environment on a plain address",
			env:   map[string]string{"APP_ENV": "prod", "APP_URL": "http://app.example.test"},
			want:  true,
			about: "the environment kept the attribute before the address was consulted, and consulting it must not take the attribute away",
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
