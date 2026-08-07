package main

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arandu-io/framework/config"
	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/httpx"
	"github.com/arandu-io/framework/httpx/middleware"
	"github.com/arandu-io/framework/kernel"

	controllers "github.com/arandu-io/arandu/app/Http/Controllers"
	"github.com/arandu-io/arandu/bootstrap"
	appconfig "github.com/arandu-io/arandu/config"
	"github.com/arandu-io/arandu/routes"
)

// These tests need no database. database/sql connects lazily, so the wiring, the
// pipeline and every route can be exercised without a server running -- which is
// what makes this a useful smoke test to keep in a project skeleton.
func testKernel(t *testing.T, env config.Env) *kernel.Kernel {
	t.Helper()

	cfg := config.Config{
		AppName:  "test",
		Env:      env,
		HTTPAddr: ":0",
		AppKey:   []byte("0123456789abcdef0123456789abcdef"),
		Database: config.DatabaseConfig{
			Connection: data.DialectPostgres,
			Host:       "127.0.0.1",
			Port:       "1",
			Database:   "does-not-exist",
			Username:   "user",
			Password:   "pass",
		},
		SessionTTL: time.Hour,
		CSRFTTL:    time.Hour,
		LogLevel:   slog.LevelError,
		Editor:     "vscode",
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("the test configuration is not valid: %v", err)
	}

	sqldb, err := sql.Open(cfg.Database.Connection.Driver(), cfg.Database.DSN())
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = sqldb.Close() })

	k := bootstrap.Build(appconfig.From(cfg), data.Wrap(sqldb, cfg.Database.Connection)).Kernel
	if err := k.Boot(context.Background()); err != nil {
		t.Fatalf("Boot: %v", err)
	}
	return k
}

// TestTheLandingPageRenders walks the whole view path in one request: the route
// from routes/web.go, the controller from app/Http/Controllers, the typed data,
// the kyse page and the layout it extends.
//
// It fails if the generated views were not committed, which is the mistake a
// skeleton invites: everything compiles, and every page answers with "no view
// named home".
func TestTheLandingPageRenders(t *testing.T) {
	k := testKernel(t, config.EnvDev)

	rec := httptest.NewRecorder()
	k.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200. Body:\n%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	// The layout ran: a page that rendered its sections without the layout would
	// answer 200 with a fragment and no <html>.
	if !strings.Contains(body, "<!doctype html>") {
		t.Error("the layout did not render around the page")
	}
	if !strings.Contains(body, "Hello world") {
		t.Error("the data the controller passed did not reach the page")
	}
	// The stylesheet and the scripts are embedded and content-addressed. A page
	// that asks for them by a plain name gets a 404 and no styling.
	if !strings.Contains(body, "/_arandu/assets/") {
		t.Error("the page does not reference the embedded assets")
	}
}

// TestTheRootRouteDoesNotSwallowEveryPath is the one place Go's router does not
// behave like Laravel's: "GET /" matches every path below it, so the landing
// page would answer for unknown URLs -- with 200, hiding the 404 and shadowing
// any route that is not mounted in this environment.
func TestTheRootRouteDoesNotSwallowEveryPath(t *testing.T) {
	k := testKernel(t, config.EnvDev)

	rec := httptest.NewRecorder()
	k.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/there-is-no-such-page", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("an unknown path answered %d, want 404: the root route is registered as \"/\" instead of \"/{$}\"", rec.Code)
	}
}

// TestTheHomeRouteIsAddressableByName: a link built from the route table
// survives the path changing, and a hardcoded "/" does not.
//
// It also pins the anchored pattern to a readable URL. "/{$}" is what Go's
// router needs and not what a href should contain, and a table that returned it
// verbatim would put a literal {$} in every link to the landing page.
func TestTheHomeRouteIsAddressableByName(t *testing.T) {
	r := httpx.NewRouter()
	routes.Web(r, routes.Deps{Home: controllers.NewHomeController("test")})

	got, err := r.Table().URL("home")
	if err != nil {
		t.Fatalf("URL(\"home\"): %v", err)
	}
	if got != "/" {
		t.Errorf("URL(\"home\") = %q, want \"/\"", got)
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
	t.Setenv("DB_CONNECTION", "sqlite")
	t.Setenv("DB_DATABASE", filepath.Join(t.TempDir(), "test.sqlite"))

	err := dispatch("migrat", nil)

	if err == nil {
		t.Fatal("an unknown command was accepted")
	}
	if !strings.Contains(err.Error(), "migrate:rollback") {
		t.Errorf("the error must list the valid commands, got: %v", err)
	}
}
