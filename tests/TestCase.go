// Package tests is the base every test in this project builds on.
//
// It is a package the suites import rather than a base class they extend. What
// belongs here is what more than one suite needs -- booting the application,
// opening a database, reading a file from the project root -- and nothing else.
// A helper used by one test belongs beside it.
//
// The layers:
//
//	tests/Unit/         one rule, with nothing running
//	tests/Feature/      the HTTP answer in process, the policy and the JSON,
//	                    over a server tests.Serve booted
//	tests/Conformance/  the framework's kits, one call each
//	tests/E2E/          the journey in a real browser, behind the e2e tag
//
// tests/Connectors is the one directory beside them: it links no connector,
// so it can ask what a binary without one answers.
package tests

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/arandu-io/framework/foundation"
	fwbootstrap "github.com/arandu-io/framework/foundation/bootstrap"
	"github.com/arandu-io/framework/test"
	"github.com/arandu-io/framework/test/browser"
	"github.com/arandu-io/framework/test/browser/cdp"
	"github.com/arandu-io/hesape/auth"
	"github.com/arandu-io/hesape/auth/account"
	"github.com/arandu-io/hesape/cache"
	"github.com/arandu-io/hesape/config"
	"github.com/arandu-io/hesape/database"
	hhttp "github.com/arandu-io/hesape/http"
	"github.com/arandu-io/hesape/queue"

	models "github.com/arandu-io/arandu/app/Models"
	providers "github.com/arandu-io/arandu/app/Providers"
	"github.com/arandu-io/arandu/bootstrap"
	appconfig "github.com/arandu-io/arandu/config"
)

// AppName is the application name every application booted here is
// configured with, as APP_NAME.
//
// It is a phrase no page uses as its title and no page writes, so a test that
// finds it in a page has found the configured name, not a heading or a title
// that happens to read the same.
const AppName = "Brandproof Ledger"

// Kernel boots the application for a test.
//
// It needs no database. database/sql connects lazily, so the wiring, the
// pipeline and every route can be exercised without a server running -- which is
// what makes this a useful smoke test to keep in a project skeleton.
//
// The database it is given is one that cannot be opened: a SQLite file under a
// path whose parent is a regular file, so no connection ever succeeds and the
// health check has a dead database to report. SQLite because it is the engine
// every project links -- a dialect whose connector is not imported has no
// database/sql driver to hand the pool, and the test would stop at sql.Open
// rather than at the request it is about.
//
// The extra modules are registered before boot, which is the only moment a
// module can be added. They are for a test that needs a route the application
// does not have -- one that fails on purpose, to see what the pipeline answers.
// Registering nothing is the ordinary call and is what most of the suite makes.
//
// Exported because both suites use it, which is the whole reason this package
// exists.
func Kernel(t *testing.T, env config.Env, extra ...foundation.Module) *foundation.Application {
	t.Helper()

	// The mailer is the one setting the framework still reads from the
	// environment here, and production refuses the log transport, which is
	// what an unset MAIL_URL means: it writes every message, reset codes
	// included, into the log. A production deployment names a real transport;
	// array:// is the one a test names, and it keeps every message in memory.
	if env.IsProduction() {
		t.Setenv("MAIL_URL", "array://")
	}

	cfg := fwbootstrap.Configuration{
		App: config.App{
			Name: AppName,
			Env:  env,
			// What a real boot answers for this environment: config.Load defaults
			// Debug to "the environment is development", and it is what decides
			// whether the debug page may exist. Left at the zero value, a kernel
			// built here for development would be one no boot produces -- a dev
			// application answering a panic with the production page.
			Debug:    env.Is(config.EnvDev),
			URL:      &url.URL{Scheme: "http", Host: "localhost"},
			HTTPAddr: ":0",
			Timezone: time.UTC,
			Locale:   "en",
			Key:      []byte("0123456789abcdef0123456789abcdef"),
		},
		Database: database.Config{
			Connection: database.DialectSQLite,
			Database:   unopenableSQLite(t),
		},
		Observability: fwbootstrap.Observability{
			LogLevel: slog.LevelError,
			Editor:   "vscode",
		},
		// What the loader answers when SESSION_LIFETIME is not written. Left at
		// zero, the settings refuse it: every session would expire as it was
		// written.
		Session: fwbootstrap.Session{Lifetime: 2 * time.Hour},
	}
	// The App block is the one with rules -- the key length, the environment, the
	// address -- and a test that writes it by hand is a test that can get them
	// wrong. Asking here is what turns that into a line naming the field.
	if err := cfg.App.Validate(); err != nil {
		t.Fatalf("the test configuration is not valid: %v", err)
	}

	sqldb, err := sql.Open(cfg.Database.Connection.Driver(), cfg.Database.DSN())
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = sqldb.Close() })

	settings, err := fwbootstrap.SettingsFrom(cfg)
	if err != nil {
		t.Fatalf("reading the framework's settings: %v", err)
	}
	appCfg, err := appconfig.From(settings)
	if err != nil {
		t.Fatalf("loading the application configuration: %v", err)
	}
	app := compose(t, appCfg, database.Wrap(sqldb, cfg.Database.Connection), extra...)
	return app.Kernel
}

// Application is an application a test composed, with what bootstrap/app.go
// composed it from: the kernel, this application's services and the drivers
// they were built over.
type Application struct {
	// Kernel is the composed application.
	Kernel *foundation.Application
	// AppServiceProvider is this application's services: the account rules,
	// the tokens, the relay, the example resource.
	*providers.AppServiceProvider
	// Drivers is what the framework built from the configuration.
	Drivers *foundation.Drivers

	// DB, Sessions, Queue and Cache are fields of Drivers, named here because
	// almost every test reaches one of them.
	DB       *database.DB
	Sessions *hhttp.SessionStore
	Queue    queue.Queue
	// Cache is the store every replica sees, and nil when no setting resolved
	// one.
	Cache cache.SharedStore
}

// compose builds the application bootstrap/app.go builds, over a database the
// test opened, with the extra modules after the application's own, and boots
// it. It is shut down when the test ends.
func compose(t *testing.T, cfg appconfig.Config, db *database.DB, extra ...foundation.Module) Application {
	t.Helper()
	app, err := Build(cfg, db, extra...)
	if err != nil {
		t.Fatalf("composing the application: %v", err)
	}
	if err := app.Kernel.Boot(context.Background()); err != nil {
		t.Fatalf("Boot: %v", err)
	}
	t.Cleanup(func() { _ = app.Kernel.Shutdown() })
	return app
}

// Build composes the application bootstrap/app.go composes, over cfg and a
// database the caller opened and closes, with the extra modules after the
// application's own. It does not boot.
//
// It answers the first refusal: a setting naming a driver this binary does
// not link, a certificate that is not there, a command name declared twice.
// That is what a test of the composition asserts on.
func Build(cfg appconfig.Config, db *database.DB, extra ...foundation.Module) (Application, error) {
	drivers, err := foundation.NewDriversOver(cfg.Settings, db)
	if err != nil {
		return Application{}, err
	}
	services, err := providers.NewAppServiceProvider(cfg, drivers)
	if err != nil {
		return Application{}, err
	}
	k, err := bootstrap.Configure(cfg, drivers, services).WithProviders(extra...).Create()
	if err != nil {
		return Application{}, err
	}
	return Application{
		Kernel: k, AppServiceProvider: services, Drivers: drivers,
		DB: drivers.DB, Sessions: drivers.Sessions, Queue: drivers.Queue, Cache: drivers.SharedCache,
	}, nil
}

// OpenDatabase connects to the database cfg names, the way the framework's
// drivers do: one DSN, built in one place.
func OpenDatabase(cfg appconfig.Config) (*database.DB, func(), error) {
	return database.Open(cfg.Database.Connection)
}

// Dispatch runs one command line through the console kernel, the way main.go
// does: the application loaded from the environment, the command handed to
// HandleCommand. It answers nil for status zero, and otherwise an error
// carrying what the command wrote to its error stream.
//
// What the command wrote to its output is discarded; Run answers it.
func Dispatch(command string, args []string) error {
	_, err := Run(append([]string{command}, args...)...)
	return err
}

// Run is Dispatch answering what the command wrote to its output as well.
func Run(args ...string) (string, error) {
	app, err := bootstrap.App("test", "test")
	if err != nil {
		return "", err
	}
	var out, errOut bytes.Buffer
	code := app.WithConsole(&out, &errOut, strings.NewReader("")).HandleCommand(context.Background(), args)
	if code != 0 {
		return out.String(), fmt.Errorf("%s exited %d: %s", strings.Join(args, " "), code, strings.TrimSpace(errOut.String()))
	}
	return out.String(), nil
}

// unopenableSQLite answers a SQLite path no connection can open: its parent is a
// regular file, so the driver cannot create the database and nothing can create
// the directory first. A path that merely did not exist yet would be created by
// the first connection, and the database would answer.
func unopenableSQLite(t *testing.T) string {
	t.Helper()
	parent := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(parent, nil, 0o600); err != nil {
		t.Fatalf("writing the file that stands where a directory should: %v", err)
	}
	return filepath.Join(parent, "test.sqlite")
}

// Root is the project root, from inside a suite directory.
//
// The tests that read a file -- the Dockerfile, the workflow, arandu.toml -- run
// two directories down from it now, and a relative path written from the old
// location silently reads nothing: os.ReadFile returns an error the test
// reports as "the file does not say X", which is true and misleading.
func Root(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("tests.Root is not the project root: %v", err)
	}
	return root
}

// File reads one file from the project root.
func File(t *testing.T, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(Root(t), name))
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	return string(body)
}

// Binary builds this application's binary the way the Dockerfile does, with
// the version and the commit stamped into main, and answers its path. It is
// the artisan a deploy runs: a test that runs a command through it proves what
// `aru migrate` and every other forwarded command reach.
func Binary(t *testing.T, version, commit string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "app")
	build := exec.Command("go", "build", "-o", out,
		"-ldflags", "-X main.version="+version+" -X main.commit="+commit, ".")
	build.Dir = Root(t)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building the binary: %v\n%s", err, output)
	}
	return out
}

// Drivers are the browser drivers the journeys in tests/E2E run with, named
// once here: ARANDU_E2E_DRIVER picks one and ARANDU_E2E_BROWSER the engine.
// cdp is the native driver, Chrome over its debugging pipe with the standard
// library alone. A project that installs github.com/hyz-is/arandu-playwright
// adds its Launch here under a name of its own, and no journey changes.
var Drivers = browser.Registry{"cdp": cdp.Launch}

// Server is the application Serve booted, serving on a port of its own over a
// database of the test's own, with what bootstrap/app.go composed it from.
//
// App is a named field rather than an embedded one: both carry a database,
// and the test reaches the application's services through App and the
// server's address, routes and database through the server.
type Server struct {
	*test.Server
	// App is the composed application: its services, its drivers and the
	// kernel serving the socket.
	App Application
}

// Serve boots the application the way its binary boots, on a free port of
// 127.0.0.1, over a database migrated for this test alone, and stops it when
// the test ends.
//
// The database is made on the engine the test environment names:
// ARANDU_TEST_DATABASE_URL, or DATABASE_URL when that is empty. Postgres
// gives every test a copy of a template migrated once per package, named
// arandutest_ and dropped when the test ends. With neither set, outside CI,
// the test runs on a temporary SQLite file, so a clone with nothing installed
// still runs the suite; in CI a missing database fails the test instead of
// skipping it. A test whose assertion no engine decides passes test.SQLite()
// and runs on SQLite everywhere.
//
// The configuration of this application is read from the environment, so the
// database and the address are exported into it with t.Setenv: a test that
// calls Serve cannot call t.Parallel.
func Serve(t *testing.T, opts ...test.Option) *Server {
	t.Helper()
	testEnvironment(t)

	var app Application
	boot := func(ctx context.Context, env test.Env) (test.Application, error) {
		var err error
		app, err = bootOver(ctx, env)
		if err != nil {
			return test.Application{}, err
		}
		return served(app), nil
	}
	options := append([]test.Option{test.Migrate(Migrate), test.ExportEnv()}, engine()...)
	server := test.Serve(t, boot, append(options, opts...)...)
	return &Server{Server: server, App: app}
}

// Boot composes and boots the application over the test's database, the way
// bootstrap/app.go composes it for the binary. It is what Serve boots, for a
// test that calls test.Serve itself; it reads the configuration from the
// environment, so it is handed to test.Serve with test.ExportEnv.
func Boot(ctx context.Context, env test.Env) (test.Application, error) {
	app, err := bootOver(ctx, env)
	if err != nil {
		return test.Application{}, err
	}
	return served(app), nil
}

// Migrate builds the schema through the command a deploy runs, `migrate`,
// over the database the environment names. A test that creates its tables
// another way is a test that keeps passing after a migration breaks.
//
// The command reads DATABASE_URL, which test.ExportEnv points at the database
// being migrated; one that names another database is refused rather than
// migrated.
func Migrate(_ context.Context, env test.Env) error {
	if got := os.Getenv("DATABASE_URL"); got != env.DatabaseURL {
		return errors.New("Migrate reads DATABASE_URL and it does not name the database being migrated: " +
			"hand test.Serve the option test.ExportEnv")
	}
	return Dispatch("migrate", nil)
}

// Open opens a browser page on the server with a driver of Drivers, resolving
// route names through the application's routes.
//
//	page := tests.Open(t, server)
//	page.Visit("notes.index")
func Open(t *testing.T, server *Server) *browser.Page {
	t.Helper()
	routes := func(name string, params ...any) (string, error) {
		values := make([]string, len(params))
		for i, p := range params {
			values[i] = fmt.Sprint(p)
		}
		return server.Route(name, values...), nil
	}
	return browser.Open(t, Drivers, browser.BaseURL(server.URL), browser.Routes(routes))
}

// SignIn signs the account in on the server and returns a browser carrying
// that session.
//
// It is test.As with the account's identity and nothing else: the session is
// started on the application's own store, every request afterwards is loaded
// by its guards, and its SubjectResolver answers who the account is from the
// database -- the roles included -- exactly as for somebody who typed a
// password. A role taken away in the test is gone on the next request.
//
// The browser calls the application in process; Cookies on it is what a page
// of tests.Open is handed to be signed in as the same account.
func SignIn(t *testing.T, server *Server, user *models.User) *test.Client {
	t.Helper()
	return test.As(t, server.Server, auth.Subject{ID: user.ID, Tenant: user.TenantID})
}

// EmptyDatabase answers a database of the test's own with no schema at all, on
// the engine Serve would put the test on, and drops it when the test ends. It
// is for a test of the migrations themselves, which has to start from nothing.
func EmptyDatabase(t *testing.T) *database.DB {
	t.Helper()
	nothing := func(context.Context, test.Env) (test.Application, error) {
		return test.Application{Handler: http.NotFoundHandler()}, nil
	}
	return test.Serve(t, nothing, append([]test.Option{test.Migrate(noSchema)}, engine()...)...).DB()
}

// noSchema is the migration of EmptyDatabase: none.
func noSchema(context.Context, test.Env) error { return nil }

// testEnvironment sets what every application a test serves is configured
// with: its name, the dev environment, a key and the tenant.
func testEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("APP_NAME", AppName)
	t.Setenv("APP_ENV", "dev")
	t.Setenv("APP_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("ARANDU_TENANT_ID", "11111111-1111-4111-8111-111111111111")
}

// engine answers the option that puts a test on SQLite when the environment
// names no test database and this is not CI, and nothing otherwise: the kit
// then reads the engine from the environment, and fails a CI run that named
// none.
func engine() []test.Option {
	if os.Getenv("ARANDU_TEST_DATABASE_URL") != "" || os.Getenv("DATABASE_URL") != "" {
		return nil
	}
	if ci, _ := strconv.ParseBool(os.Getenv("CI")); ci {
		return nil
	}
	return []test.Option{test.SQLite()}
}

// bootOver loads the configuration from the environment and composes and
// boots the application over the test's database, which the kit opened and
// closes.
func bootOver(ctx context.Context, env test.Env) (Application, error) {
	cfg, err := appconfig.Load()
	if err != nil {
		return Application{}, fmt.Errorf("loading the configuration: %w", err)
	}
	app, err := Build(cfg, env.DB)
	if err != nil {
		return Application{}, fmt.Errorf("composing the application: %w", err)
	}
	if err := app.Kernel.Boot(ctx); err != nil {
		return Application{}, fmt.Errorf("Boot: %w", err)
	}
	return app, nil
}

// served is what the kit serves of an application: its handler, the session
// store its guards read, its routes by name, and its shutdown.
func served(app Application) test.Application {
	return test.Application{
		Handler:  app.Kernel.Handler(),
		Sessions: app.Sessions,
		Routes:   app.Kernel.Routes(),
		Shutdown: app.Kernel.Shutdown,
	}
}

// Register creates an account the way the sign-up form does, through the
// account rules app wired, and returns it as the *models.User this
// application's users query builds -- the type whose Subject a session is
// started from.
func Register(t *testing.T, app Application, name, email, password string) *models.User {
	t.Helper()
	registered, err := app.Users.Register(context.Background(), appconfig.Tenant(), account.Registration{
		Name: name, Email: email, Password: password,
	})
	if err != nil {
		t.Fatalf("registering %s: %v", email, err)
	}
	user, ok := registered.(*models.User)
	if !ok {
		t.Fatalf("the account rules registered a %T, want *models.User", registered)
	}
	return user
}

// FromPage loads the page at address and makes every later request of the
// client carry the CSRF token the page holds, as the browser does, and
// returns the client.
//
// A form posts the token as its _token field, but an hx-delete, an hx-put or
// an hx-post sends no form of the page's: the token travels in the
// X-CSRF-Token header the layout's hx-headers names on <body>. A test of a
// write a page makes over htmx loads that page first through this.
func FromPage(t *testing.T, client *test.Client, address string) *test.Client {
	t.Helper()
	page := client.Get(address).AssertOk()
	return client.WithHeader("X-CSRF-Token", CSRFTokenFromPage(t, page.GetContent()))
}

// CSRFTokenFromPage is the CSRF token a page carries: the value of its
// _token field, or else of the X-CSRF-Token entry of its hx-headers. It
// fails the test when the page carries neither.
func CSRFTokenFromPage(t *testing.T, html string) string {
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
