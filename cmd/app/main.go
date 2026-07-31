// Command app is the entry point of an Arandu application.
//
// This file is the equivalent of Laravel's bootstrap/app.php: the single place
// where the application is composed. Notice that the wiring is explicit and
// visible -- no dependency appears by magic. If you want to know where UserRepo
// comes from, it is written here, and `aru make:module` regenerates this file
// when you add a module.
//
// It also dispatches the subcommands that need the registered modules: serve,
// migrate, routes and db:seed. `aru serve` runs this binary with that argument,
// because only this binary knows which modules exist.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/arandu-io/framework/config"
	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/events"
	"github.com/arandu-io/framework/httpx/middleware"
	"github.com/arandu-io/framework/kernel"
	"github.com/arandu-io/framework/modules/auth"
	"github.com/arandu-io/framework/observability/errorpage"
	"github.com/arandu-io/framework/security"

	"github.com/arandu-io/arandu/database/seeders"

	// Drivers register themselves. They live in the project, not in the
	// framework: that is what keeps the core at two dependencies.
	//
	// SQLite is the development default and needs no cgo. Remove the driver you
	// do not use and the binary stops carrying it.
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

// appModule is this project's module path. The error page uses it to tell your
// frames from the framework's, and shows yours expanded.
const appModule = "github.com/arandu-io/arandu"

// defaultTenant is the tenant a single-tenant application runs under.
//
// It is a constant rather than an empty string on purpose: security.SystemGrant
// refuses an empty tenant, because a system grant with no tenant reads across
// every customer of the system. An application that never thinks about tenancy
// still writes every row under this value, so growing into multi-tenant later is
// a change of resolver and not a migration of data.
//
// Set ARANDU_TENANT_ID to override it.
const defaultTenant = "00000000-0000-4000-8000-000000000001"

// tenantID is the tenant this deployment logs into.
func tenantID() string {
	if id := os.Getenv("ARANDU_TENANT_ID"); id != "" {
		return id
	}
	return defaultTenant
}

func main() {
	command := "serve"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}

	if err := dispatch(command, os.Args[2:]); err != nil {
		log.Fatal(err)
	}
}

func dispatch(command string, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	db, closeDB, err := open(cfg)
	if err != nil {
		return err
	}
	defer closeDB()

	k, authService := build(cfg, db)
	ctx := context.Background()

	switch command {
	case "serve":
		if err := k.Boot(ctx); err != nil {
			return err
		}
		return k.Run(ctx)

	case "migrate":
		return migrate(ctx, db, k.Migrations())

	case "migrate:rollback":
		return rollback(ctx, db, k.Migrations())

	case "migrate:status":
		return migrateStatus(ctx, db, k.Migrations())

	case "migrate:fresh":
		return fresh(ctx, db, k.Migrations())

	case "routes":
		if err := k.Boot(ctx); err != nil {
			return err
		}
		fmt.Print(kernel.FormatRoutes(k.Routes()))
		return nil

	case "db:seed":
		return seeders.Run(ctx, seeders.Deps{Auth: authService, Tenant: tenantID()}, args)

	default:
		return fmt.Errorf("unknown command: %s (expected serve, migrate, migrate:rollback, migrate:status, migrate:fresh, routes or db:seed)", command)
	}
}

// open connects using whatever DB_CONNECTION says. The DSN and the driver name
// both come from the configuration, so switching from SQLite to Postgres is a
// change in .env and nothing else.
func open(cfg config.Config) (*data.DB, func(), error) {
	// SQLite creates the database file but never the directory above it.
	if path := cfg.Database.SQLitePath(); path != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, nil, fmt.Errorf("creating the database directory: %w", err)
		}
	}

	sqldb, err := sql.Open(cfg.Database.Connection.Driver(), cfg.Database.DSN())
	if err != nil {
		return nil, nil, fmt.Errorf("opening %s: %w", cfg.Database.Redacted(), err)
	}

	if cfg.Database.Connection == data.DialectSQLite {
		// One writer. SQLite serializes writes anyway, and letting the pool open
		// more connections only converts the wait into "database is locked".
		sqldb.SetMaxOpenConns(1)
	} else {
		// Bounded pool: the default is unlimited, which turns one traffic spike
		// into "too many connections" on the database rather than a queue here.
		sqldb.SetMaxOpenConns(25)
		sqldb.SetMaxIdleConns(5)
		sqldb.SetConnMaxLifetime(time.Hour)
	}

	return data.Wrap(sqldb, cfg.Database.Connection), func() { _ = sqldb.Close() }, nil
}

// build wires the application. Everything below is ordinary Go: read it top to
// bottom and you know the whole application.
func build(cfg config.Config, db *data.DB) (*kernel.Kernel, *auth.Service) {
	csrf := security.NewCSRF(cfg.AppKey, cfg.CSRFTTL)

	// The core ships the in-memory session backend, which is right for one
	// instance and wrong for two: behind a load balancer, half the requests land
	// on the replica that never saw the login. Behind more than one pod, swap
	// this for kv.NewSessionBackend(client) -- github.com/arandu-io/kv, same
	// interface, one line. The same applies to the limiter below.
	sessions := security.NewSessionStore(cfg.AppKey, cfg.SessionTTL, !cfg.IsDev(), security.NewMemoryBackend())

	limiter := middleware.NewMemoryLimiter()

	// The auth service is returned as well as registered: the seeders need it,
	// and reaching into the module to fetch it later would be exactly the kind
	// of hidden coupling the explicit wiring exists to avoid.
	authService := auth.NewService(auth.NewUserRepo(db), sessions, csrf)

	k := kernel.New(cfg).
		// The pipeline order is the order of execution. Recover comes FIRST, or
		// a panic in any middleware below it escapes without a page.
		Use(
			middleware.Recover(cfg.IsDev(), errorpage.Options{
				Editor:    cfg.Editor,
				AppModule: appModule,
			}),
			middleware.Observe(cfg.IsDev(), cfg.TracingSecret),
			middleware.SecurityHeaders(cfg.IsDev()),
			middleware.RateLimit(limiter, 300, time.Minute, middleware.KeyBySession(sessions.IDFromRequest)),
			middleware.CSRFProtect(csrf, sessions.IDFromRequest),
		).
		Register(
			// Single tenant: every login belongs to one constant. A multi-tenant
			// application swaps this for a resolver that reads the host name --
			// same code path, same queries, one line different.
			auth.New(authService, auth.FixedTenant(tenantID())),
			// The outbox table. A module that records domain events stores them
			// in the same transaction as the write, and this is what brings the
			// table those rows land in -- see doc 27.
			events.NewModule(),
			// `aru make:module` adds the next modules here.
		)

	return k, authService
}
