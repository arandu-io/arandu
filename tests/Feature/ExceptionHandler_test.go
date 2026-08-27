package feature_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	fhttp "github.com/arandu-io/framework/http"
	"github.com/arandu-io/hesape/config"

	"github.com/arandu-io/arandu/tests"
)

// What this application answers when a request fails, driven through its own
// pipeline.
//
// The handler is built by the bootstrapper and installed as the outermost
// middleware, and the only way to see that from outside is to make something
// panic behind it. A test that called the handler directly would prove the
// handler, which the collection already proves; what breaks here is the wiring
// -- and a pipeline with no panic handler looks exactly like one with a working
// handler until the day something panics.

// panicText is what the route below raises. It reads like a real defect: a
// sentence with a secret in it, because "leaks nothing" is the claim the
// production assertion makes and an assertion needs something to look for.
const panicText = "the payment gateway returned nothing, token=secret-token"

// panicRoute is a module whose one route fails the way a defect fails.
//
// A module rather than a handler wrapped by the test, because the pipeline is
// what is under test: registered here, the request goes through Recover,
// Observe, the security headers, the limiter and the CSRF check, in the order
// bootstrap/app.go put them in.
type panicRoute struct{}

// Name is the slug the route table and any diagnosis attribute this to.
func (panicRoute) Name() string { return "panicroute" }

// Routes registers the one route.
func (panicRoute) Routes(r *fhttp.Router) {
	r.Get("/tests/panic", func(http.ResponseWriter, *http.Request) { panic(panicText) })
}

// TestAPanicIsAnsweredRatherThanEscaping is the baseline, and it is about the
// pipeline rather than about the page: a panic that escaped would take the
// connection down with no status at all.
func TestAPanicIsAnsweredRatherThanEscaping(t *testing.T) {
	k := tests.Kernel(t, config.EnvProd, panicRoute{})

	rec := httptest.NewRecorder()
	k.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tests/panic", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

// TestTheProductionAnswerLeaksNothingAndCarriesTheRequestID.
//
// The status page is what the exception handler draws outside development, and
// the request id on it is the whole of what a person has to go on: it is the
// thread from the page somebody is looking at to the log line for that request.
// The panic text is not on it, and that is the reason the page exists rather
// than the error being rendered.
func TestTheProductionAnswerLeaksNothingAndCarriesTheRequestID(t *testing.T) {
	k := tests.Kernel(t, config.EnvProd, panicRoute{})

	rec := httptest.NewRecorder()
	k.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tests/panic", nil))

	body := rec.Body.String()
	if strings.Contains(body, panicText) || strings.Contains(body, "secret-token") {
		t.Errorf("the production response repeats the panic:\n%s", body)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "text/html") {
		t.Errorf("Content-Type = %q, want the status page", got)
	}
	if !strings.Contains(body, "Internal Server Error") {
		t.Errorf("the page does not name the status:\n%s", body)
	}

	id := rec.Header().Get("X-Request-ID")
	if id == "" {
		t.Fatal("the response carries no request id, so nothing ties the page to the log")
	}
	if !strings.Contains(body, id) {
		t.Errorf("the page does not show the request id %q:\n%s", id, body)
	}
}

// TestTheDebugPageShowsTheFailureInDevelopment.
//
// Development is the only place the inside of the process may be visible, and
// it is decided by App.Debug -- which follows the environment and is refused in
// production. The frames of this application are told from the framework's by
// module path prefix, which is why the page is asked for the module rather than
// only for the panic: a page that classified every frame as somebody else's
// would be wrong on the one screen where it costs the most.
func TestTheDebugPageShowsTheFailureInDevelopment(t *testing.T) {
	k := tests.Kernel(t, config.EnvDev, panicRoute{})

	rec := httptest.NewRecorder()
	k.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tests/panic", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, panicText) {
		t.Errorf("the debug page does not show the panic:\n%s", body)
	}
	if !strings.Contains(body, "ExceptionHandler_test.go") {
		t.Errorf("the debug page does not show the frame that panicked:\n%s", body)
	}
}
