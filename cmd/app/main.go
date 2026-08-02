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
	"fmt"
	"log"
	"os"
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

	adapter "github.com/arandu-io/database"

	// The engines this binary can speak. Each is its own module, so removing an
	// import removes the driver from the build, from go.sum and from the
	// vulnerability surface -- which is the whole reason they are separate.
	//
	// SQLite is the development default and needs no cgo. Adding MySQL is
	// `go get github.com/arandu-io/database/mysql` plus a line here.
	_ "github.com/arandu-io/database/pgx"
	_ "github.com/arandu-io/database/sqlite"
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

// open connects using whatever DB_CONNECTION says.
//
// The pool policy, the SQLite directory and the message for a driver that is
// configured but not linked all live in the adapter, so every project gets the
// same ones rather than a slightly different copy.
func open(cfg config.Config) (*data.DB, func(), error) {
	return adapter.Open(cfg.Database)
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

	// A module that calls another service takes this client, not one of its own:
	//
	//	billing.New(svc, observability.Client(10*time.Second))
	//
	// Going through it is what puts the call on the request timeline and on the
	// console. A handler that builds its own http.Client is a handler whose
	// 800ms wait shows up as "other", and the timeout is not optional --
	// http.Client has none by default, and a call with no deadline is how one
	// slow dependency turns into every request of the process hanging.

	// The auth service is returned as well as registered: the seeders need it,
	// and reaching into the module to fetch it later would be exactly the kind
	// of hidden coupling the explicit wiring exists to avoid.
	authService := auth.NewService(auth.NewUserRepo(db), sessions, csrf)

	k := kernel.New(cfg)

	k.
		// The pipeline order is the order of execution. Recover comes FIRST, or
		// a panic in any middleware below it escapes without a page.
		Use(
			middleware.Recover(cfg.IsDev(), errorpage.Options{
				Editor:    cfg.Editor,
				AppModule: appModule,
				// What the registered modules know about the state of the
				// system right now -- the outbox falling behind, and whatever
				// the next module reports. It shows up next to the failure
				// somebody is already looking at.
				Diagnose: k.Diagnose,
			}),
			// k.Recorder() is the buffer behind /_arandu/debug. It is nil
			// outside development, and passing nil records nothing -- which is
			// what production does.
			middleware.Observe(cfg.IsDev(), cfg.TracingSecret, k.Recorder()),
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
