package main

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arandu-io/framework/config"
	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/httpx/middleware"
	"github.com/arandu-io/framework/kernel"
)

// These tests need no database. database/sql connects lazily, so the wiring, the
// pipeline and every route can be exercised without a server running -- which is
// what makes this a useful smoke test to keep in a project skeleton.
func testKernel(t *testing.T, env config.Env) *kernel.Kernel {
	t.Helper()

	cfg := config.Config{
		AppName:     "test",
		Env:         env,
		HTTPAddr:    ":0",
		AppKey:      []byte("0123456789abcdef0123456789abcdef"),
		DatabaseURL: "postgres://user:pass@127.0.0.1:1/does-not-exist",
		SessionTTL:  time.Hour,
		CSRFTTL:     time.Hour,
		LogLevel:    slog.LevelError,
		Editor:      "vscode",
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("the test configuration is not valid: %v", err)
	}

	sqldb, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = sqldb.Close() })

	k, _ := build(cfg, data.Wrap(sqldb))
	if err := k.Boot(context.Background()); err != nil {
		t.Fatalf("Boot: %v", err)
	}
	return k
}

// TestSeedAdminRefusesWithoutCredentials: the command reads them from the
// environment rather than from flags, because a password passed as an argument
// lands in the shell history and in the process list.
func TestSeedAdminRefusesWithoutCredentials(t *testing.T) {
	t.Setenv("ARANDU_ADMIN_EMAIL", "")
	t.Setenv("ARANDU_ADMIN_PASSWORD", "")

	err := seedAdmin(context.Background(), nil)

	if err == nil {
		t.Fatal("seed:admin ran without credentials")
	}
	if !strings.Contains(err.Error(), "ARANDU_ADMIN_EMAIL") {
		t.Errorf("the error must name what is missing, got: %v", err)
	}
}

// TestNewUUIDIsWellFormed: the generated tenant id goes into a uuid column, so a
// malformed value fails at insert time with a message about syntax rather than
// about what actually went wrong.
func TestNewUUIDIsWellFormed(t *testing.T) {
	id, err := newUUID()
	if err != nil {
		t.Fatalf("newUUID: %v", err)
	}

	if len(id) != 36 {
		t.Fatalf("uuid = %q, want 36 characters", id)
	}
	if id[14] != '4' {
		t.Errorf("uuid %q is not version 4", id)
	}
	if v := id[19]; v != '8' && v != '9' && v != 'a' && v != 'b' {
		t.Errorf("uuid %q has the wrong variant", id)
	}

	other, err := newUUID()
	if err != nil {
		t.Fatalf("newUUID: %v", err)
	}
	if id == other {
		t.Fatal("two generated uuids are identical")
	}
}

// TestLoginFormIsServedWithACSRFToken is the phase 1 claim in one request: the
// application boots, routes, and hands the browser a token bound to its session.
func TestLoginFormIsServedWithACSRFToken(t *testing.T) {
	k := testKernel(t, config.EnvDev)

	rec := httptest.NewRecorder()
	k.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/auth/login", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `name="_csrf"`) {
		t.Error("the form carries no CSRF field")
	}
	// The attribute below is the single most common mistake in this stack: without
	// it every HTMX request that changes state fails the CSRF check.
	if !strings.Contains(body, "hx-headers") || !strings.Contains(body, "X-CSRF-Token") {
		t.Error("the page is missing hx-headers with X-CSRF-Token")
	}
}

// TestWriteWithoutCSRFIsRejected proves the middleware is actually in the
// pipeline, which a wiring file can silently get wrong.
func TestWriteWithoutCSRFIsRejected(t *testing.T) {
	k := testKernel(t, config.EnvDev)

	rec := httptest.NewRecorder()
	k.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/auth/login", nil))

	if rec.Code != middleware.StatusCSRFExpired {
		t.Fatalf("status = %d, want %d", rec.Code, middleware.StatusCSRFExpired)
	}
}

// TestHealthFailsWithoutTheDatabase: the probe has to depend on the database, or
// a pod with no connection keeps receiving traffic.
func TestHealthFailsWithoutTheDatabase(t *testing.T) {
	k := testKernel(t, config.EnvProd)

	rec := httptest.NewRecorder()
	k.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/_arandu/health", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "auth") {
		t.Errorf("the body must name the failing module, got %q", rec.Body.String())
	}
}

func TestSecurityHeadersAreApplied(t *testing.T) {
	k := testKernel(t, config.EnvProd)

	rec := httptest.NewRecorder()
	k.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/auth/login", nil))

	if got := rec.Header().Get("Content-Security-Policy"); !strings.Contains(got, "default-src 'self'") {
		t.Errorf("CSP = %q", got)
	}
	if got := rec.Header().Get("X-Request-ID"); got == "" {
		t.Error("every response must carry a request id")
	}
}

// TestDebugConsoleIsDevelopmentOnly is the absolute rule of the observability
// package, checked here because the skeleton is what decides Env.
func TestDebugConsoleIsDevelopmentOnly(t *testing.T) {
	for env, want := range map[config.Env]int{
		config.EnvDev:  http.StatusOK,
		config.EnvProd: http.StatusNotFound,
	} {
		k := testKernel(t, env)
		rec := httptest.NewRecorder()
		k.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/_arandu/debug", nil))
		if rec.Code != want {
			t.Errorf("/_arandu/debug in %s = %d, want %d", env, rec.Code, want)
		}
	}
}

func TestRoutesAreListedByModule(t *testing.T) {
	k := testKernel(t, config.EnvDev)

	out := kernel.FormatRoutes(k.Routes())

	for _, want := range []string{"auth", "/auth/login", "/_arandu/health"} {
		if !strings.Contains(out, want) {
			t.Errorf("the route table does not mention %q:\n%s", want, out)
		}
	}
}

func TestUnknownCommandIsRejected(t *testing.T) {
	t.Setenv("APP_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("DATABASE_URL", "postgres://user:pass@127.0.0.1:1/does-not-exist")

	err := dispatch("migrat")

	if err == nil {
		t.Fatal("an unknown command was accepted")
	}
	if !strings.Contains(err.Error(), "serve, migrate, routes or seed:admin") {
		t.Errorf("the error must list the valid commands, got: %v", err)
	}
}
