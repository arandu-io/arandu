package feature_test

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/arandu-io/hesape/auth"

	"github.com/arandu-io/arandu/bootstrap"
	appconfig "github.com/arandu-io/arandu/config"
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
			sqliteEnv(t)
			t.Setenv("SESSION_LIFETIME", c.lifetime)

			cfg, err := appconfig.Load()
			if err != nil {
				t.Fatalf("loading the configuration: %v", err)
			}
			db, closeDB, err := bootstrap.Open(cfg)
			if err != nil {
				t.Fatalf("opening the database: %v", err)
			}
			t.Cleanup(closeDB)
			app, err := bootstrap.Build(cfg, db)
			if err != nil {
				t.Fatalf("wiring the application: %v", err)
			}

			response := httptest.NewRecorder()
			subject := auth.Subject{
				ID:     "11111111-1111-4111-8111-111111111111",
				Tenant: "22222222-2222-4222-8222-222222222222",
			}
			if _, err := app.Sessions.Start(context.Background(), response, subject); err != nil {
				t.Fatalf("starting a session: %v", err)
			}
			cookies := response.Result().Cookies()
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
