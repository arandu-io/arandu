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
	"os"
	"strings"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/kernel"
	"github.com/arandu-io/hesape/console"
	"github.com/arandu-io/hesape/database"
	"github.com/arandu-io/hesape/redis/connections"

	appconfig "github.com/arandu-io/arandu/config"
	"github.com/arandu-io/arandu/routes"
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

	case "routes":
		if err := k.Boot(ctx); err != nil {
			return err
		}
		fmt.Print(kernel.FormatRoutes(k.Routes()))
		return nil

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
		// The component's migration commands come before the application's own,
		// for the same reason routes/console.go comes after both: a project must
		// not shadow `aru migrate` and change what a deploy step does.
		//
		// They are built here rather than above the switch because building them
		// wires a migrator, and `aru serve` has no reason to pay for one.
		// The migration commands and the seed commands, both the component's.
		migrationCommands := append(append(migrationCommands(cfg, db, app), seedCommands(cfg, app)...), databaseCommands(cfg, db)...)
		for _, c := range migrationCommands {
			if c.Name != command {
				continue
			}
			return runMigrationCommand(ctx, cfg, c, args, app.Cache)
		}

		// What routes/console.go declares comes last, so an application cannot
		// shadow a built-in command and change what `aru migrate` means.
		if cmd, found := routes.Lookup(command); found {
			if err := k.Boot(ctx); err != nil {
				return err
			}
			defer func() { _ = k.Shutdown() }()
			return cmd.Run(ctx, args)
		}
		return unknownCommand(command, migrationCommands)
	}
}

// runMigrationCommand runs one of the component's commands.
//
// The IO is built here rather than by a console.Application because this
// application dispatches with a switch: what an Application would add over this
// is the listing and the lock, and the lock is the half that matters, so it is
// wired below.
//
// A command that names a lock and finds no issuer refuses rather than running
// unlocked -- see console.Application.guarded. That is why the store is passed
// even when it is nil: nil is the answer that makes an isolated command say the
// cache cannot isolate it, and a lock held inside this process would satisfy
// every type here and isolate nothing.
func runMigrationCommand(ctx context.Context, cfg appconfig.Config, c console.Command, args []string, store *connections.Connection) error {
	if err := refuseCommand(cfg, c, store); err != nil {
		return err
	}
	return console.NewApplication(os.Stdout, os.Stderr, os.Stdin).
		Add(c).
		WithLocks(migrationLocks(store), isolationLockTTL).
		Call(ctx, c.Name, args...)
}

// unknownCommand lists what was available instead. An error that only says the
// command is unknown costs a search; this one ends it.
//
// The migration commands are listed from the same slice the dispatch reads, so
// a command that exists is named and one that does not cannot be: the listing
// and the lookup cannot disagree, which is how migrate:install, migrate:reset
// and migrate:refresh went unmentioned for as long as they went unwired.
func unknownCommand(command string, migrationCommands []console.Command) error {
	names := make([]string, 0, len(migrationCommands))
	for _, c := range migrationCommands {
		names = append(names, c.Name)
	}

	err := fmt.Errorf("unknown command: %s (expected serve, routes, schedule:list, "+
		"schedule:run, work, Version or one of %s)", command, strings.Join(names, ", "))
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
	return database.Open(cfg.Database.Connection)
}
