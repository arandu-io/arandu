package feature_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arandu-io/framework/data"
	fhttp "github.com/arandu-io/framework/http"
	"github.com/arandu-io/framework/observability"
	"github.com/arandu-io/framework/security"

	"github.com/arandu-io/arandu/bootstrap"
)

// The console, through the real pipeline. Everything below goes through
// k.Handler(), which is the same handler `aru serve` binds to a port -- a test
// that drove the console directly would prove the console and not the wiring,
// and the wiring is where a Collector goes missing.

// bootedApp boots the whole application over a migrated database and hands back
// what the wiring produced.
//
// The App and not only its handler, because two of the things this application
// wires are reachable from no route: the relay that empties the outbox is one,
// and a test that built one of its own would pass over an application that wires
// none.
func bootedApp(t *testing.T) (bootstrap.App, *data.DB) {
	t.Helper()
	app, db := builtApp(t)
	bootApp(t, app)
	return app, db
}

func builtApp(t *testing.T) (bootstrap.App, *data.DB) {
	t.Helper()
	sqliteEnv(t)

	if err := bootstrap.Dispatch("migrate", nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cfg, db, _ := openForTest(t)
	app, err := bootstrap.Build(cfg, db)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return app, db
}

func bootApp(t *testing.T, app bootstrap.App) {
	t.Helper()
	if err := app.Kernel.Boot(context.Background()); err != nil {
		t.Fatalf("boot: %v", err)
	}
	t.Cleanup(func() { _ = app.Kernel.Shutdown() })
}

// TestTheConsoleRecordsARealRequest is the shape of a debugging session: make a
// request, open the console, find it.
func TestTheConsoleRecordsARealRequest(t *testing.T) {
	app, _ := bootedApp(t)
	handler := app.Kernel.Handler()
	const requested = "/robots.txt"

	first := httptest.NewRecorder()
	handler.ServeHTTP(first, httptest.NewRequest(http.MethodGet, requested, nil))
	id := first.Header().Get("X-Request-ID")
	if id == "" {
		t.Fatal("the response carries no request id")
	}

	list := httptest.NewRecorder()
	handler.ServeHTTP(list, httptest.NewRequest(http.MethodGet, observability.ConsolePath, nil))
	if list.Code != http.StatusOK {
		t.Fatalf("the console answered %d", list.Code)
	}
	if !strings.Contains(list.Body.String(), requested) {
		t.Errorf("the request is not in the console:\n%s", list.Body.String())
	}

	detail := httptest.NewRecorder()
	handler.ServeHTTP(detail, httptest.NewRequest(http.MethodGet, observability.ConsolePath+"/"+id, nil))
	if detail.Code != http.StatusOK {
		t.Fatalf("the detail page answered %d", detail.Code)
	}
	for _, want := range []string{id, "Timeline", requested} {
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
	app, _ := builtApp(t)

	user, err := app.Users.Register(
		context.Background(), bootstrap.Tenant(), "Ana", "ana@example.test", "a-long-enough-password",
	)
	if err != nil {
		t.Fatalf("registering the user read by the request: %v", err)
	}
	app.Kernel.Register(queryProbe{query: func(ctx context.Context) error {
		_, err := app.Users.FindForAuthentication(ctx, user.TenantID, user.ID)
		return err
	}})
	bootApp(t, app)
	handler := app.Kernel.Handler()

	request := httptest.NewRequest(http.MethodGet, "/console-query-probe", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, request)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("the application-owned query probe answered %d", rec.Code)
	}
	id := rec.Header().Get("X-Request-ID")
	if id == "" {
		t.Fatal("the queried response carries no request id")
	}

	detail := httptest.NewRecorder()
	handler.ServeHTTP(detail, httptest.NewRequest(http.MethodGet, observability.ConsolePath+"/"+id+"?format=json", nil))

	body := detail.Body.String()
	if !strings.Contains(strings.ToLower(body), `from \"users\"`) {
		t.Errorf("the console shows no application user query:\n%s", body)
	}
	// The origin is what saves the time: Model queries cross Hesape's native
	// database seam, and the recorder has to name that exact source rather than
	// an obsolete community-module repository.
	if !strings.Contains(body, "database/dbmodel.go") {
		t.Errorf("the query has no origin pointing at the native model database seam:\n%s", body)
	}
}

type queryProbe struct {
	query func(context.Context) error
}

func (queryProbe) Name() string { return "query-probe" }

func (p queryProbe) Routes(router *fhttp.Router) {
	router.Get("/console-query-probe", func(writer http.ResponseWriter, request *http.Request) {
		if err := p.query(request.Context()); err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	})
}

func TestTheGrantStillComesFromTheSession(t *testing.T) {
	g := security.SystemGrant("user.view", bootstrap.Tenant())
	if data.Tenant(g) != bootstrap.Tenant() {
		t.Fatal("the tenant no longer comes from the Grant")
	}
}
