// Console is the command side of this application.
//
// It lives in bootstrap rather than in main so that the entry point stays thin
// and what it runs is a package. `main` cannot be imported, so anything that
// lives there is anything a test cannot reach -- and the tests that matter most
// here are the ones that boot the whole application and make a request.
//
// tests/Feature/ imports this.
package bootstrap

import (
	"context"
	"fmt"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/kernel"

	appconfig "github.com/arandu-io/arandu/config"
	"github.com/arandu-io/arandu/database/seeders"
	"github.com/arandu-io/arandu/routes"

	adapter "github.com/arandu-io/database"
)

// Version and Commit are stamped by the build. See the Dockerfile.
var (
	Version = "dev"
	Commit  = "unknown"
)

// tenantID is the tenant this deployment logs into.
//
// It reads the configuration rather than the environment directly, so there is
// one answer to the question and it is in config/auth.go.
func Tenant() string { return appconfig.Tenant() }

// dispatch runs one command against a fully wired application.
//
// Every command builds the same application, and that is the point: `aru work`
// reaches the same services a request does, so a worker is never a second,
// subtly different program.
func Dispatch(command string, args []string) error {
	cfg, err := appconfig.Load()
	if err != nil {
		return err
	}

	db, closeDB, err := Open(cfg)
	if err != nil {
		return err
	}
	defer closeDB()

	app, err := Build(cfg, db)
	if err != nil {
		return err
	}
	k := app.Kernel
	ctx := context.Background()

	switch command {
	case "serve":
		if err := k.Boot(ctx); err != nil {
			return err
		}
		return k.Run(ctx)

	case "migrate":
		options, err := migrateOptions(args)
		if err != nil {
			return err
		}
		return migrate(ctx, db, k.Migrations(), options)

	case "migrate:rollback":
		options, err := migrateOptions(args)
		if err != nil {
			return err
		}
		return rollback(ctx, db, k.Migrations(), options)

	case "migrate:status":
		return migrateStatus(ctx, db, k.Migrations())

	case "migrate:fresh":
		options, err := migrateOptions(args)
		if err != nil {
			return err
		}
		return fresh(ctx, cfg, db, k.Migrations(), options)

	case "routes":
		if err := k.Boot(ctx); err != nil {
			return err
		}
		fmt.Print(kernel.FormatRoutes(k.Routes()))
		return nil

	case "db:seed":
		return seeders.Run(ctx, seeders.Deps{Auth: app.Auth, Tenant: cfg.Auth.Tenant}, args)

	case "schedule:list":
		if err := k.Boot(ctx); err != nil {
			return err
		}
		defer func() { _ = k.Shutdown() }()
		return scheduleList(app.Scheduler)

	case "schedule:run":
		if err := k.Boot(ctx); err != nil {
			return err
		}
		defer func() { _ = k.Shutdown() }()
		return scheduleRun(ctx, app.Scheduler, args)

	case "work":
		return work(ctx, k, app.Queue, args)

	case "Version":
		fmt.Printf("%s %s (%s)\n", cfg.App.Name, Version, Commit)
		return nil

	default:
		// What routes/console.go declares comes last, so an application cannot
		// shadow a built-in command and change what `aru migrate` means.
		if cmd, found := routes.Lookup(command); found {
			if err := k.Boot(ctx); err != nil {
				return err
			}
			defer func() { _ = k.Shutdown() }()
			return cmd.Run(ctx, args)
		}
		return unknownCommand(command)
	}
}

// unknownCommand lists what was available instead. An error that only says the
// command is unknown costs a search; this one ends it.
func unknownCommand(command string) error {
	err := fmt.Errorf("unknown command: %s (expected serve, migrate, migrate:rollback, "+
		"migrate:status, migrate:fresh, routes, db:seed, schedule:list, schedule:run, work or Version)", command)
	if help := routes.Help(); help != "" {
		return fmt.Errorf("%w\n\n%s", err, help)
	}
	return err
}

// open connects using whatever the scheme of DATABASE_URL says.
//
// The pool policy, the SQLite directory and the message for a driver that is
// configured but not linked all live in the adapter, so every project gets the
// same ones rather than a slightly different copy.
// Open connects, using the configuration this application was given.
//
// Exported because the feature tests open the same database the commands do:
// two ways to connect is two places for a DSN to be built differently, and the
// one nobody runs daily is the one that drifts.
func Open(cfg appconfig.Config) (*data.DB, func(), error) {
	return adapter.Open(cfg.Database.Connection)
}
