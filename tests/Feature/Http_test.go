package feature_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/arandu-io/arandu/tests"

	"github.com/arandu-io/framework/arandutest"
	fhttp "github.com/arandu-io/framework/http"
	"github.com/arandu-io/framework/http/middleware"
	"github.com/arandu-io/hesape/auth"
	"github.com/arandu-io/hesape/cache"
	"github.com/arandu-io/hesape/config"
	hhttp "github.com/arandu-io/hesape/http"
	"github.com/arandu-io/hesape/routing"

	controllers "github.com/arandu-io/arandu/app/Http/Controllers"
	appmiddleware "github.com/arandu-io/arandu/app/Http/Middleware"
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

// TestTheLandingPageGreetsWhoIsSignedIn: the page is public, and its route
// carries the subject of a live session, so the controller greets by name
// without loading the session itself.
func TestTheLandingPageGreetsWhoIsSignedIn(t *testing.T) {
	app := tests.Booted(t)
	user, err := app.Users.Register(context.Background(), bootstrap.Tenant(), "Ana Lima", "ana@example.test", "a-long-enough-password")
	if err != nil {
		t.Fatalf("registering the account: %v", err)
	}
	tests.SignedIn(t, app, user.Subject()).Get("/").AssertOk().AssertSee("Ana Lima")
	arandutest.NewClient(t, app.Kernel.Handler()).Get("/").OK().DontSee("Ana Lima")
}

// TestTheLandingPageOffersOnlyAddressesThatAnswer: every link and every form
// the landing page draws, for a guest and for somebody signed in, leads to an
// address the application answers.
//
// The navigation is built from the route table, and the authentication UI is
// what registers the sign-in and sign-out routes. A project without it has
// neither, so the header has nothing to offer there -- and an empty href, or
// one written as a literal path, is a control that leads to the page it is on
// or to a 404. With the UI published the same assertions hold, because then
// the routes exist and every address answers.
//
// It asserts on addresses and statuses only. The words on the controls belong
// to whichever layout the project has.
func TestTheLandingPageOffersOnlyAddressesThatAnswer(t *testing.T) {
	app := tests.Booted(t)
	user, err := app.Users.Register(context.Background(), bootstrap.Tenant(), "Ana Lima", "ana@example.test", "a-long-enough-password")
	if err != nil {
		t.Fatalf("registering the account: %v", err)
	}
	signedIn := user.Subject()

	for _, c := range []struct {
		who     string
		subject *auth.Subject
	}{
		{"a guest", nil},
		{"somebody signed in", &signedIn},
	} {
		b := newBrowser(t, app, c.subject)
		page := b.do(http.MethodGet, "/", nil)
		if page.Code != http.StatusOK {
			t.Fatalf("%s: GET / = %d, want 200", c.who, page.Code)
		}
		body := page.Body.String()

		for _, empty := range emptyAddress.FindAllString(body, -1) {
			t.Errorf("%s: the landing page draws %s, a control with nowhere to go", c.who, empty)
		}

		links := 0
		for _, m := range linkAddress.FindAllStringSubmatch(body, -1) {
			links++
			if code := b.do(http.MethodGet, m[1], nil).Code; code == http.StatusNotFound || code == http.StatusMethodNotAllowed {
				t.Errorf("%s: the landing page links %s, which answers %d", c.who, m[1], code)
			}
		}
		if links == 0 {
			t.Errorf("%s: the landing page links no address of this application, so nothing was checked", c.who)
		}

		// The forms last: with the authentication UI published, the sign-out
		// form ends the session it is posted from.
		token := csrfTokenFromPage(t, body)
		for _, m := range formAddress.FindAllStringSubmatch(body, -1) {
			code := b.do(http.MethodPost, m[1], url.Values{"_token": {token}}).Code
			if code == http.StatusNotFound || code == http.StatusMethodNotAllowed {
				t.Errorf("%s: the landing page posts a form to %s, which answers %d", c.who, m[1], code)
			}
		}
	}
}

var (
	// emptyAddress is a link or a form with an empty address.
	emptyAddress = regexp.MustCompile(`(?:href|action)=""`)
	// linkAddress is a link to an address of this application.
	linkAddress = regexp.MustCompile(`href="(/[^"]*)"`)
	// formAddress is a form posted to an address of this application.
	formAddress = regexp.MustCompile(`<form[^>]*\saction="(/[^"]*)"`)
)

// browser sends requests straight to the application's handler and keeps the
// cookies it is given, as a browser does, while answering the status of each
// response -- which is the one thing this file asks and the shared client does
// not hand back.
type browser struct {
	t       *testing.T
	handler http.Handler
	cookies map[string]string
}

// newBrowser returns a browser for app, holding a session for subject when one
// is given. The session is started the way tests.SignedIn starts one: through
// the store the route guards read, on an address only this handler has.
func newBrowser(t *testing.T, app bootstrap.App, subject *auth.Subject) *browser {
	t.Helper()
	const signInHere = "/_tests/sign-in"
	inner := app.Kernel.Handler()
	b := &browser{t: t, cookies: map[string]string{}}
	b.handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != signInHere || subject == nil {
			inner.ServeHTTP(w, r)
			return
		}
		if _, err := app.Sessions.Start(r.Context(), w, *subject); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	if subject != nil {
		if code := b.do(http.MethodGet, signInHere, nil).Code; code != http.StatusNoContent {
			t.Fatalf("starting the session answered %d", code)
		}
	}
	return b
}

// do sends one request, with a form body when form is not nil, and keeps the
// cookies of the response.
func (b *browser) do(method, path string, form url.Values) *httptest.ResponseRecorder {
	b.t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req := httptest.NewRequest(method, path, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for name, value := range b.cookies {
		req.AddCookie(&http.Cookie{Name: name, Value: value})
	}
	rec := httptest.NewRecorder()
	b.handler.ServeHTTP(rec, req)
	for _, c := range rec.Result().Cookies() {
		if c.MaxAge < 0 {
			delete(b.cookies, c.Name)
			continue
		}
		b.cookies[c.Name] = c.Value
	}
	return rec
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
	// The token resolver and the idempotency store are guards' arguments, and a
	// guard refuses nil at registration; neither is asked anything here.
	routes.Web(r, routes.Deps{
		Home:        controllers.NewHomeController("test", nil, ""),
		Tokens:      appmiddleware.NewPersonalAccessTokens(nil),
		Idempotency: cache.NewArrayStore(),
	})

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

// TestSpoofedMethodReachesTheRouterAfterCSRF proves the browser-facing
// contract through the complete application pipeline. The browser can submit
// only POST, but the real router must observe DELETE after the request has
// passed the session-bound CSRF check. There is deliberately no DELETE probe
// route, so 405 is the public evidence; without the override, the POST probe
// handler answers instead.
func TestSpoofedMethodReachesTheRouterAfterCSRF(t *testing.T) {
	k := tests.Kernel(t, config.EnvDev, methodProbeModule{})

	form := httptest.NewRecorder()
	k.Handler().ServeHTTP(form, httptest.NewRequest(http.MethodGet, "/", nil))
	if form.Code != http.StatusOK {
		t.Fatalf("token page status = %d, want 200", form.Code)
	}
	body := url.Values{
		"_token":  {csrfTokenFromPage(t, form.Body.String())},
		"_method": {http.MethodDelete},
	}
	request := httptest.NewRequest(http.MethodPost, "/method-probe", strings.NewReader(body.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, cookie := range form.Result().Cookies() {
		request.AddCookie(cookie)
	}

	response := httptest.NewRecorder()
	k.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("spoofed DELETE status = %d, want 405; the router did not observe DELETE", response.Code)
	}
}

// TestAGuestTokenIsBoundToThatGuest is the landing page drawn for somebody with
// no session: the token on it comes from the middleware, through view.New, and
// it is bound to a cookie of that visitor's own. Their write passes; the same
// token without their cookie, or offered by another visitor, is refused.
func TestAGuestTokenIsBoundToThatGuest(t *testing.T) {
	k := tests.Kernel(t, config.EnvDev, methodProbeModule{})

	visit := func() (string, []*http.Cookie) {
		t.Helper()
		page := httptest.NewRecorder()
		k.Handler().ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
		if page.Code != http.StatusOK {
			t.Fatalf("anonymous GET / = %d, want 200. Body:\n%s", page.Code, page.Body.String())
		}
		return csrfTokenFromPage(t, page.Body.String()), page.Result().Cookies()
	}
	post := func(token string, cookies []*http.Cookie) int {
		t.Helper()
		body := url.Values{"_token": {token}}
		request := httptest.NewRequest(http.MethodPost, "/method-probe", strings.NewReader(body.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		for _, cookie := range cookies {
			request.AddCookie(cookie)
		}
		rec := httptest.NewRecorder()
		k.Handler().ServeHTTP(rec, request)
		return rec.Code
	}

	token, cookies := visit()
	if !hasCookie(cookies, "arandu_csrf_guest") {
		t.Fatalf("the guest's first page set no binding cookie; it set %v", cookies)
	}
	if got := post(token, cookies); got != http.StatusNoContent {
		t.Fatalf("a guest posting the token of its own page = %d, want 204", got)
	}
	if got := post(token, nil); got != middleware.StatusCSRFExpired {
		t.Fatalf("the token without the guest cookie = %d, want %d", got, middleware.StatusCSRFExpired)
	}
	_, other := visit()
	if got := post(token, other); got != middleware.StatusCSRFExpired {
		t.Fatalf("one guest's token under another guest's cookie = %d, want %d", got, middleware.StatusCSRFExpired)
	}
}

func hasCookie(cookies []*http.Cookie, name string) bool {
	for _, cookie := range cookies {
		if cookie.Name == name && cookie.Value != "" {
			return true
		}
	}
	return false
}

type methodProbeModule struct{}

func (methodProbeModule) Name() string { return "method-probe" }

func (methodProbeModule) Routes(router *fhttp.Router) {
	router.Action(http.MethodPost, "/method-probe", func(ctx *hhttp.Context) error {
		return ctx.Status(http.StatusNoContent)
	})
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

	out := routing.FormatRoutes(k.Routes())

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

func csrfTokenFromPage(t *testing.T, html string) string {
	t.Helper()
	for _, marker := range []string{`name="_token" value="`, `"X-CSRF-Token": "`} {
		start := strings.Index(html, marker)
		if start < 0 {
			continue
		}
		value := html[start+len(marker):]
		end := strings.IndexByte(value, '"')
		if end < 0 {
			t.Fatal("the CSRF token is not terminated")
		}
		return value[:end]
	}
	t.Fatal("the page carries no CSRF token")
	return ""
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
