// Package tests is the base every test in this project builds on.
//
// It is a package the suites import rather than a base class they extend. What
// belongs here is what more than one suite needs -- booting the application,
// opening a database, reading a file from the project root -- and nothing else.
// A helper used by one test belongs beside it.
//
// The two suites:
//
//	tests/Feature/  boots the application and makes a request
//	tests/Unit/     checks one thing without booting anything
package tests

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arandu-io/framework/arandutest"

	"github.com/arandu-io/framework/foundation"
	fwbootstrap "github.com/arandu-io/framework/foundation/bootstrap"
	"github.com/arandu-io/framework/security"
	hesapetest "github.com/arandu-io/hesape/arandutest"
	"github.com/arandu-io/hesape/auth"
	"github.com/arandu-io/hesape/auth/account"
	"github.com/arandu-io/hesape/cache"
	"github.com/arandu-io/hesape/config"
	"github.com/arandu-io/hesape/database"
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
	Sessions *security.SessionStore
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

// Open connects to the database cfg names, the way the framework's drivers
// do: one DSN, built in one place.
func Open(cfg appconfig.Config) (*database.DB, func(), error) {
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

// App boots the whole application on a throwaway SQLite database, migrated, and
// returns a browser for it.
//
// A fresh database and a request client in one call, which is the difference
// between a feature test worth writing and one nobody writes: every alternative
// starts with twelve lines of environment, connection and migration that have
// nothing to do with what is being proved.
//
// SQLite in a temporary directory, so the tests need nothing installed and two
// of them cannot see each other's rows. The file goes with t.TempDir.
func App(t *testing.T) (*arandutest.Client, *database.DB) {
	t.Helper()
	app := Booted(t)
	return arandutest.NewClient(t, app.Kernel.Handler()), app.DB
}

// Booted is what App boots, returned whole: the services, the session store and
// the database, for a test that has to reach one of them as well as browse.
func Booted(t *testing.T) Application {
	t.Helper()

	t.Setenv("APP_NAME", AppName)
	t.Setenv("APP_ENV", "dev")
	t.Setenv("APP_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("DATABASE_URL", "sqlite://"+filepath.Join(t.TempDir(), "test.sqlite"))
	t.Setenv("ARANDU_TENANT_ID", "11111111-1111-4111-8111-111111111111")

	// The schema, through the same command a deploy runs. A test that creates
	// tables another way is a test that keeps passing after a migration breaks.
	if err := Dispatch("migrate", nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	cfg, err := appconfig.Load()
	if err != nil {
		t.Fatalf("loading the configuration: %v", err)
	}
	db, closeDB, err := Open(cfg)
	if err != nil {
		t.Fatalf("opening the database: %v", err)
	}
	t.Cleanup(closeDB)
	return compose(t, cfg, db)
}

// SignedIn returns a browser for app whose requests carry a real session for
// the subject, started by the same store the route guards read.
//
// The session is started the way the sign-in screen starts one, through
// SessionStore.Start, so RequireAuth loads it and puts the subject on the
// request exactly as it does for somebody who typed a password. The address it
// is started on exists only in front of this client's handler: the
// application has no such route.
//
// It is the browser hesape answers with, which speaks every verb a resource
// registers -- PUT and DELETE as well as GET and POST -- and takes a header for
// what a browser would send that a test has to say.
func SignedIn(t *testing.T, app Application, subject auth.Subject) *hesapetest.Client {
	t.Helper()
	const signInHere = "/_tests/sign-in"
	handler := app.Kernel.Handler()
	client := hesapetest.NewClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != signInHere {
			handler.ServeHTTP(w, r)
			return
		}
		if _, err := app.Sessions.Start(r.Context(), w, subject); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	client.Get(signInHere).AssertStatus(http.StatusNoContent)
	return client
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
