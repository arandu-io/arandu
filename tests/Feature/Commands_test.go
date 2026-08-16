package feature_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"

	"github.com/arandu-io/arandu/bootstrap"
	appconfig "github.com/arandu-io/arandu/config"
)

// The tests below run the real thing against a real database, because SQLite is
// a file: migrate, seed, log in, log out. No server, nothing installed, and no
// reason for a project to ship without this level of proof.

func sqliteEnv(t *testing.T) {
	t.Helper()
	t.Setenv("APP_ENV", "dev")
	t.Setenv("APP_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("DATABASE_URL", "sqlite://"+filepath.Join(t.TempDir(), "test.sqlite"))
	t.Setenv("ARANDU_TENANT_ID", "11111111-1111-4111-8111-111111111111")
}

// TestMigrateAndRollbackOnSQLite walks the deploy commands in the order a person
// actually uses them.
func TestMigrateAndRollbackOnSQLite(t *testing.T) {
	sqliteEnv(t)

	if err := bootstrap.Dispatch("migrate", nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Twice, because a deploy pipeline runs it on every release.
	if err := bootstrap.Dispatch("migrate", nil); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	if err := bootstrap.Dispatch("migrate:status", nil); err != nil {
		t.Fatalf("migrate:status: %v", err)
	}
	if err := bootstrap.Dispatch("migrate:rollback", nil); err != nil {
		t.Fatalf("migrate:rollback: %v", err)
	}
	if err := bootstrap.Dispatch("migrate:fresh", nil); err != nil {
		t.Fatalf("migrate:fresh: %v", err)
	}
}

// TestLoginOnSQLite is the phase 1 promise end to end, on a database that needs
// no installation: migrate, seed the administrator, and sign in.
func TestLoginOnSQLite(t *testing.T) {
	sqliteEnv(t)
	t.Setenv("ARANDU_ADMIN_EMAIL", "admin@example.test")
	t.Setenv("ARANDU_ADMIN_PASSWORD", "a-long-enough-password")

	if err := bootstrap.Dispatch("migrate", nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := bootstrap.Dispatch("db:seed", nil); err != nil {
		t.Fatalf("db:seed: %v", err)
	}
	// Seeding has to be safe to run again, or it cannot be part of a deploy.
	if err := bootstrap.Dispatch("db:seed", nil); err != nil {
		t.Fatalf("second db:seed: %v", err)
	}

	cfg, db, _ := openForTest(t)
	k := bootstrap.Build(cfg, db).Kernel
	if err := k.Boot(context.Background()); err != nil {
		t.Fatalf("Boot: %v", err)
	}
	handler := k.Handler()

	// The login form issues the CSRF token the POST has to carry.
	form := httptest.NewRecorder()
	handler.ServeHTTP(form, httptest.NewRequest(http.MethodGet, "/auth/login", nil))
	token := csrfToken(t, form.Body.String())

	t.Run("wrong password is refused", func(t *testing.T) {
		rec := post(handler, token, "admin@example.test", "not-the-password")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
		if len(rec.Result().Cookies()) != 0 {
			t.Error("a failed login must not set a session cookie")
		}
	})

	t.Run("unknown user is refused the same way", func(t *testing.T) {
		rec := post(handler, token, "nobody@example.test", "a-long-enough-password")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
		// Same answer as a wrong password: otherwise the endpoint tells an
		// attacker which addresses have accounts.
		if strings.Contains(strings.ToLower(rec.Body.String()), "not found") {
			t.Error("the response distinguishes an unknown user from a wrong password")
		}
	})

	t.Run("correct password signs in", func(t *testing.T) {
		rec := post(handler, token, "ADMIN@example.test", "a-long-enough-password")

		// What matters is that the person is signed in and told where to go,
		// and this asserts that rather than one shape of it.
		//
		// The shape depends on the client: an HTMX request gets 204 with
		// HX-Redirect, a plain form post gets 303 with Location. It also
		// depends on WHICH sign-in module is wired -- the framework ships a
		// minimal one and the starter kit replaces it, at the same path.
		// Demanding one status and one header would turn this suite red for
		// anybody who installed the kit.
		if rec.Code != http.StatusOK && rec.Code != http.StatusNoContent && rec.Code != http.StatusSeeOther {
			t.Fatalf("status = %d, want 2xx or 303 (the email is matched case-insensitively)", rec.Code)
		}
		to := rec.Header().Get("HX-Redirect")
		if to == "" {
			to = rec.Header().Get("Location")
		}
		if to != "/" {
			t.Errorf("the client was sent to %q, want /", to)
		}

		cookies := rec.Result().Cookies()
		if len(cookies) != 1 || cookies[0].Name != security.SessionCookieName {
			t.Fatalf("cookies = %+v, want one session cookie", cookies)
		}
		if !cookies[0].HttpOnly {
			t.Error("the session cookie must be HttpOnly")
		}
		if strings.Contains(rec.Body.String(), "argon2") {
			t.Error("the response leaks the password hash")
		}
	})
}

// TestSeedRefusesAnUnknownName covers the typo in the seeder name, which is the
// main way to get this command wrong.
func TestSeedRefusesAnUnknownName(t *testing.T) {
	sqliteEnv(t)
	if err := bootstrap.Dispatch("migrate", nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	err := bootstrap.Dispatch("db:seed", []string{"NoSuchSeeder"})

	if err == nil {
		t.Fatal("an unknown seeder was accepted")
	}
	if !strings.Contains(err.Error(), "available") {
		t.Errorf("the error must list the seeders that exist, got: %v", err)
	}
}

// TestTheOldClassFlagSaysWhatToTypeInstead.
//
// The seeder name is positional, and a name that is sometimes a flag and
// sometimes a word is two spellings of one thing. What matters is that the
// refusal of `--class=X` contains the command to run -- an error that only says
// "unknown argument" sends somebody to the source of a CLI to find out what
// changed.
func TestTheOldClassFlagSaysWhatToTypeInstead(t *testing.T) {
	sqliteEnv(t)
	if err := bootstrap.Dispatch("migrate", nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	err := bootstrap.Dispatch("db:seed", []string{"--class=PostSeeder"})

	if err == nil {
		t.Fatal("--class= was accepted")
	}
	if !strings.Contains(err.Error(), "db:seed PostSeeder") {
		t.Errorf("the error does not say what to type instead: %v", err)
	}
}

// TestFreshRefusesOutsideDevelopment: migrate:fresh drops every table, and a
// confirmation prompt is not a guard anyone should trust at 3am.
func TestFreshRefusesOutsideDevelopment(t *testing.T) {
	sqliteEnv(t)
	t.Setenv("APP_ENV", "prod")

	err := bootstrap.Dispatch("migrate:fresh", nil)

	if err == nil {
		t.Fatal("migrate:fresh ran outside development")
	}
	if !strings.Contains(err.Error(), "APP_ENV=dev") {
		t.Errorf("error = %v", err)
	}
}

func post(handler http.Handler, token, email, password string) *httptest.ResponseRecorder {
	body := url.Values{
		"_csrf":    {token},
		"email":    {email},
		"password": {password},
	}
	r := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, r)
	return rec
}

func csrfToken(t *testing.T, html string) string {
	t.Helper()
	const marker = `name="_csrf" value="`
	i := strings.Index(html, marker)
	if i < 0 {
		t.Fatalf("no CSRF field in the form:\n%s", html)
	}
	rest := html[i+len(marker):]
	end := strings.IndexByte(rest, '"')
	if end < 0 {
		t.Fatal("the CSRF field is not terminated")
	}
	return rest[:end]
}

// openForTest builds the same configuration and handle the commands use, so the
// test exercises the real wiring rather than a parallel one.
func openForTest(t *testing.T) (appconfig.Config, *data.DB, func()) {
	t.Helper()

	cfg, err := appconfig.Load()
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	db, closeDB, err := bootstrap.Open(cfg)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(closeDB)
	return cfg, db, closeDB
}
