package feature_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/arandu-io/hesape/auth"

	"github.com/arandu-io/arandu/bootstrap"
)

// TestEveryCookieTheApplicationWritesFollowsTheOneSecureDecision boots the
// application the way `aru serve` does, under the three ways a deployment
// answers whether its cookies are HTTPS-only, and reads the answer off the
// cookies the wired application writes.
//
// Two of them are built in bootstrap/app.go -- the CSRF guest cookie a page
// sets for a visitor with no session, and the session cookie a sign-in sets --
// and both must carry what the framework's loader decided: a guest cookie that
// disagreed with the session cookie is a site where signing in works and every
// guest form answers 419, or the other way round. The flash cookie is the
// kernel's own and follows the same value there.
func TestEveryCookieTheApplicationWritesFollowsTheOneSecureDecision(t *testing.T) {
	for _, c := range []struct {
		name   string
		appEnv string
		secure string
		want   bool
	}{
		{name: "dev", appEnv: "dev", want: false},
		{name: "prod, nothing declared", appEnv: "prod", want: true},
		{name: "prod, declared false to serve over http", appEnv: "prod", secure: "false", want: false},
	} {
		t.Run(c.name, func(t *testing.T) {
			sqliteEnv(t)
			t.Setenv("APP_ENV", c.appEnv)
			t.Setenv("APP_DEBUG", "")
			t.Setenv("APP_URL", "http://app.internal:8080")
			t.Setenv("SESSION_SECURE_COOKIE", c.secure)
			app := bootedInstance(t)

			page := httptest.NewRecorder()
			app.Kernel.Handler().ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
			guest := page.Result().Cookies()
			if len(guest) == 0 {
				t.Fatalf("GET / answered %d and set no cookie, want the CSRF guest cookie", page.Code)
			}
			for _, cookie := range guest {
				if cookie.Secure != c.want {
					t.Errorf("the guest cookie %s is written with Secure=%t, want %t", cookie.Name, cookie.Secure, c.want)
				}
			}

			signIn := httptest.NewRecorder()
			subject := auth.Subject{ID: "33333333-3333-4333-8333-333333333333", Tenant: bootstrap.Tenant()}
			if _, err := app.Sessions.Start(context.Background(), signIn, subject); err != nil {
				t.Fatalf("starting a session: %v", err)
			}
			session := signIn.Result().Cookies()
			if len(session) != 1 {
				t.Fatalf("a started session wrote %d cookies, want exactly the session cookie", len(session))
			}
			for _, cookie := range session {
				if cookie.Secure != c.want {
					t.Errorf("the session cookie %s is written with Secure=%t, want %t", cookie.Name, cookie.Secure, c.want)
				}
			}
		})
	}
}
