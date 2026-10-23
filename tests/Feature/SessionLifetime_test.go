package feature_test

import (
	"testing"
	"time"

	"github.com/arandu-io/framework/test"

	"github.com/arandu-io/arandu/tests"
)

// TestTheSessionLifetimeReachesTheCookie.
//
// SESSION_LIFETIME has one reader, the framework's loader, and it is a count of
// minutes. What is asserted is the end of the chain rather than the field: the
// configuration a boot loads, handed to the wiring a boot runs, starting the
// session a sign-in starts, and the Max-Age the browser receives. A store built
// from any other duration -- a lifetime of this project's own, a constant, the
// value read as seconds -- answers another number here.
func TestTheSessionLifetimeReachesTheCookie(t *testing.T) {
	for _, c := range []struct {
		name     string
		lifetime string
		want     time.Duration
	}{
		{name: "written, in minutes", lifetime: "30", want: 30 * time.Minute},
		{name: "unset, the framework's default", lifetime: "", want: 2 * time.Hour},
	} {
		t.Run(c.name, func(t *testing.T) {
			server := tests.Serve(t, test.Setenv("SESSION_LIFETIME", c.lifetime))
			user := tests.Register(t, server.App, "Ana Lima", "ana@example.test", "a-long-enough-password")

			cookies := tests.SignIn(t, server, user).Cookies()
			if len(cookies) != 1 {
				t.Fatalf("a started session wrote %d cookies, want exactly the session cookie", len(cookies))
			}
			if got, want := cookies[0].MaxAge, int(c.want.Seconds()); got != want {
				t.Errorf("SESSION_LIFETIME=%q: the session cookie has Max-Age=%d, want %d (%s)",
					c.lifetime, got, want, c.want)
			}
		})
	}
}
