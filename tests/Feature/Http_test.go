package feature_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arandu-io/arandu/tests"

	fhttp "github.com/arandu-io/framework/http"
	"github.com/arandu-io/framework/http/middleware"
	"github.com/arandu-io/framework/kernel"
	"github.com/arandu-io/hesape/config"

	controllers "github.com/arandu-io/arandu/app/Http/Controllers"
	"github.com/arandu-io/arandu/bootstrap"
	"github.com/arandu-io/arandu/routes"
)

func TestTheLandingPageRenders(t *testing.T) {
	k := tests.Kernel(t, config.EnvDev)

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
	// The application name, and not a literal from the page.
	//
	// Asserting a phrase of the skeleton's own landing page would fail in every
	// project that installed the starter kit, which replaces that page along
	// with the layout and the controller -- on its first push, for something the
	// person did not break.
	//
	// The app name survives the swap because both controllers pass it, and it
	// still proves what this test is for: a value the controller was given
	// reached the rendered page. A weaker assertion -- that the body is not
	// empty -- would pass with the error page.
	if !strings.Contains(body, "test") {
		t.Error("the application name the controller was given did not reach the page")
	}
	// The stylesheet and the scripts are embedded and content-addressed. A page
	// that asks for them by a plain name gets a 404 and no styling.
	if !strings.Contains(body, "/_arandu/assets/") {
		t.Error("the page does not reference the embedded assets")
	}
}

// TestTheRootRouteDoesNotSwallowEveryPath guards a property of Go's router:
// "GET /" matches every path below it, so an unguarded landing page would answer
// for unknown URLs -- with 200, hiding the 404 and shadowing any route that is
// not mounted in this environment.
func TestTheRootRouteDoesNotSwallowEveryPath(t *testing.T) {
	k := tests.Kernel(t, config.EnvDev)

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
	r := fhttp.NewRouter()
	routes.Web(r, routes.Deps{Home: controllers.NewHomeController("test", nil, nil, nil, "")})

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
	k := tests.Kernel(t, config.EnvDev)
	rec := publishedAuthLogin(t, k.Handler())
	body := rec.Body.String()
	if !strings.Contains(body, `name="_token"`) {
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
	k := tests.Kernel(t, config.EnvDev)
	publishedAuthLogin(t, k.Handler())

	rec := httptest.NewRecorder()
	k.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/auth/login", nil))

	if rec.Code != middleware.StatusCSRFExpired {
		t.Fatalf("status = %d, want %d", rec.Code, middleware.StatusCSRFExpired)
	}
}

// TestHealthFailsWithoutTheDatabase: the probe has to depend on the database, or
// a pod with no connection keeps receiving traffic.
func TestHealthFailsWithoutTheDatabase(t *testing.T) {
	k := tests.Kernel(t, config.EnvProd)

	rec := httptest.NewRecorder()
	k.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/_arandu/health", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "app") && !strings.Contains(body, "events") {
		t.Errorf("the body must name a database-backed module, got %q", body)
	}
}

func TestSecurityHeadersAreApplied(t *testing.T) {
	k := tests.Kernel(t, config.EnvProd)

	rec := httptest.NewRecorder()
	k.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

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
		k := tests.Kernel(t, env)
		rec := httptest.NewRecorder()
		k.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/_arandu/debug", nil))
		if rec.Code != want {
			t.Errorf("/_arandu/debug in %s = %d, want %d", env, rec.Code, want)
		}
	}
}

func TestRoutesAreListedByModule(t *testing.T) {
	k := tests.Kernel(t, config.EnvDev)

	out := kernel.FormatRoutes(k.Routes())

	for _, want := range []string{"app", "/{$}", "/_arandu/health"} {
		if !strings.Contains(out, want) {
			t.Errorf("the route table does not mention %q:\n%s", want, out)
		}
	}
}

func publishedAuthLogin(t *testing.T, handler http.Handler) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/auth/login", nil))
	if recorder.Code == http.StatusNotFound {
		t.Skip("the authentication UI is published into generated applications")
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /auth/login = %d, want 200", recorder.Code)
	}
	return recorder
}

func TestUnknownCommandIsRejected(t *testing.T) {
	t.Setenv("APP_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("DATABASE_URL", "sqlite://"+filepath.Join(t.TempDir(), "test.sqlite"))

	err := bootstrap.Dispatch("migrat", nil)

	if err == nil {
		t.Fatal("an unknown command was accepted")
	}
	if !strings.Contains(err.Error(), "migrate:rollback") {
		t.Errorf("the error must list the valid commands, got: %v", err)
	}
}
