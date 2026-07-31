package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/observability"
	"github.com/arandu-io/framework/security"
)

// The console, through the real pipeline. Everything below goes through
// k.Handler(), which is the same handler `aru serve` binds to a port -- a test
// that drove the console directly would prove the console and not the wiring,
// and the wiring is where a Collector goes missing.

func bootedApp(t *testing.T) (http.Handler, *data.DB) {
	t.Helper()
	sqliteEnv(t)

	if err := dispatch("migrate", nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cfg, db, _ := openForTest(t)
	k, _ := build(cfg, db)
	if err := k.Boot(context.Background()); err != nil {
		t.Fatalf("boot: %v", err)
	}
	t.Cleanup(func() { _ = k.Shutdown() })
	return k.Handler(), db
}

// TestTheConsoleRecordsARealRequest is the shape of a debugging session: make a
// request, open the console, find it.
func TestTheConsoleRecordsARealRequest(t *testing.T) {
	handler, _ := bootedApp(t)

	first := httptest.NewRecorder()
	handler.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/auth/login", nil))
	id := first.Header().Get("X-Request-ID")
	if id == "" {
		t.Fatal("the response carries no request id")
	}

	list := httptest.NewRecorder()
	handler.ServeHTTP(list, httptest.NewRequest(http.MethodGet, observability.ConsolePath, nil))
	if list.Code != http.StatusOK {
		t.Fatalf("the console answered %d", list.Code)
	}
	if !strings.Contains(list.Body.String(), "/auth/login") {
		t.Errorf("the request is not in the console:\n%s", list.Body.String())
	}

	detail := httptest.NewRecorder()
	handler.ServeHTTP(detail, httptest.NewRequest(http.MethodGet, observability.ConsolePath+"/"+id, nil))
	if detail.Code != http.StatusOK {
		t.Fatalf("the detail page answered %d", detail.Code)
	}
	for _, want := range []string{id, "Timeline", "/auth/login"} {
		if !strings.Contains(detail.Body.String(), want) {
			t.Errorf("the detail page does not show %q", want)
		}
	}
}

// TestTheConsoleSeesTheQueriesOfTheRequest: the Collector reaching the recorder
// through the whole pipeline is the thing that breaks silently, and when it
// breaks the console shows a request with no queries -- which reads like the
// application not touching the database.
func TestTheConsoleSeesTheQueriesOfTheRequest(t *testing.T) {
	handler, _ := bootedApp(t)

	// A request that queries: the login form issues none, so this drives one
	// through a route that does.
	// The form first, for the CSRF token: without it the POST is refused by the
	// middleware and never reaches the database, which would make this test
	// pass or fail for the wrong reason.
	form := httptest.NewRecorder()
	handler.ServeHTTP(form, httptest.NewRequest(http.MethodGet, "/auth/login", nil))

	rec := post(handler, csrfToken(t, form.Body.String()), "nobody@example.test", "a-long-enough-password")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("the login was not attempted: status %d", rec.Code)
	}
	id := rec.Header().Get("X-Request-ID")

	detail := httptest.NewRecorder()
	handler.ServeHTTP(detail, httptest.NewRequest(http.MethodGet, observability.ConsolePath+"/"+id+"?format=json", nil))

	body := detail.Body.String()
	if !strings.Contains(body, "FROM users") && !strings.Contains(body, "from users") {
		t.Errorf("the console shows no query for a login attempt:\n%s", body)
	}
	// The origin is what saves the time: it has to name the repository file.
	if !strings.Contains(body, "user.repo.go") {
		t.Errorf("the query has no origin pointing at the repository:\n%s", body)
	}
}

func TestTheGrantStillComesFromTheSession(t *testing.T) {
	g := security.SystemGrant("user.view", tenantID())
	if data.Tenant(g) != tenantID() {
		t.Fatal("the tenant no longer comes from the Grant")
	}
}
