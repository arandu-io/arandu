// Command app is the entry point of an Arandu application.
//
// This file is the equivalent of Laravel's bootstrap/app.php: the single place
// where the application is composed. Notice that the wiring is explicit and
// visible -- no dependency appears by magic. If you want to know where UserRepo
// comes from, it is written here, and `aru make:module` regenerates this file
// when you add a module.
//
// It also dispatches the subcommands that need the registered modules: serve,
// migrate and routes. `aru serve` runs this binary with that argument, because
// only this binary knows which modules exist.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/arandu-io/framework/config"
	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/httpx/middleware"
	"github.com/arandu-io/framework/kernel"
	"github.com/arandu-io/framework/modules/auth"
	"github.com/arandu-io/framework/observability/errorpage"
	"github.com/arandu-io/framework/security"

	// The driver registers itself. It lives in the project, not in the
	// framework: that is what keeps the core at two dependencies.
	_ "github.com/jackc/pgx/v5/stdlib"
)

// appModule is this project's module path. The error page uses it to tell your
// frames from the framework's, and shows yours expanded.
const appModule = "github.com/arandu-io/arandu"

func main() {
	command := "serve"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}

	if err := dispatch(command); err != nil {
		log.Fatal(err)
	}
}

func dispatch(command string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	sqldb, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("opening the database: %w", err)
	}
	defer func() { _ = sqldb.Close() }()

	// Bounded pool: the default is unlimited, which turns one traffic spike into
	// "too many connections" on the database rather than a queue in the app.
	sqldb.SetMaxOpenConns(25)
	sqldb.SetMaxIdleConns(5)
	sqldb.SetConnMaxLifetime(time.Hour)

	db := data.Wrap(sqldb)
	k, authService := build(cfg, db)

	ctx := context.Background()

	switch command {
	case "serve":
		if err := k.Boot(ctx); err != nil {
			return err
		}
		return k.Run(ctx)

	case "migrate":
		applied, err := data.Migrate(ctx, db, k.Migrations())
		if err != nil {
			return err
		}
		if len(applied) == 0 {
			fmt.Println("no pending migrations")
			return nil
		}
		for _, id := range applied {
			fmt.Println("applied", id)
		}
		return nil

	case "routes":
		if err := k.Boot(ctx); err != nil {
			return err
		}
		fmt.Print(kernel.FormatRoutes(k.Routes()))
		return nil

	case "seed:admin":
		return seedAdmin(ctx, authService)

	default:
		return fmt.Errorf("unknown command: %s (expected serve, migrate, routes or seed:admin)", command)
	}
}

// build wires the application. Everything below is ordinary Go: read it top to
// bottom and you know the whole application.
func build(cfg config.Config, db *data.DB) (*kernel.Kernel, *auth.Service) {
	csrf := security.NewCSRF(cfg.AppKey, cfg.CSRFTTL)

	// The core ships the in-memory session backend, which is right for one
	// instance. Behind more than one pod, swap it for the redis adapter.
	sessions := security.NewSessionStore(cfg.AppKey, cfg.SessionTTL, !cfg.IsDev(), security.NewMemoryBackend())

	limiter := middleware.NewMemoryLimiter()

	// The auth service is returned as well as registered: seed:admin needs it,
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
			// Single tenant: every login belongs to ARANDU_TENANT_ID, which
			// seed:admin prints when it generates one. A multi-tenant application
			// swaps this for a resolver that reads the host name.
			auth.New(authService, auth.FixedTenant(os.Getenv("ARANDU_TENANT_ID"))),
			// `aru make:module` adds the next modules here.
		)

	return k, authService
}
