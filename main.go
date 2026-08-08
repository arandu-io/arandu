// Command arandu is the entry point of this application.
//
// It is both halves of what is usually two artifacts, a front controller and a
// console script: the binary is
// the server, and the same binary is the command line. There is no front
// controller and no document root, because in Go the process listens itself.
//
// The wiring is not here. bootstrap/app.go composes the application and hands
// it back ready; this file decides which of the composed pieces the requested
// command needs, and starts it.
//
// `aru serve` runs this binary with that argument, because only this binary
// knows which modules exist.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/kernel"

	"github.com/arandu-io/arandu/bootstrap"
	appconfig "github.com/arandu-io/arandu/config"
	"github.com/arandu-io/arandu/database/seeders"
	"github.com/arandu-io/arandu/routes"

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

// version and commit are stamped by the build. See the Dockerfile.
var (
	version = "dev"
	commit  = "unknown"
)

// tenantID is the tenant this deployment logs into.
//
// It reads the configuration rather than the environment directly, so there is
// one answer to the question and it is in config/auth.go.
func tenantID() string { return appconfig.Tenant() }

func main() {
	// The arguments are read together, because reading them apart is how `go run
	// .` -- no command, no arguments, the first thing anybody types after a
	// clone -- panicked on os.Args[2:] before it printed a line.
	command, args := "serve", []string{}
	if len(os.Args) > 1 {
		command, args = os.Args[1], os.Args[2:]
	}

	if err := dispatch(command, args); err != nil {
		log.Fatal(err)
	}
}

// dispatch runs one command against a fully wired application.
//
// Every command builds the same application, and that is the point: `aru work`
// reaches the same services a request does, so a worker is never a second,
// subtly different program.
func dispatch(command string, args []string) error {
	cfg, err := appconfig.Load()
	if err != nil {
		return err
	}

	db, closeDB, err := open(cfg)
	if err != nil {
		return err
	}
	defer closeDB()

	app := bootstrap.Build(cfg, db)
	k := app.Kernel
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
		return fresh(ctx, cfg, db, k.Migrations())

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

	case "version":
		fmt.Printf("%s %s (%s)\n", cfg.App.Name, version, commit)
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
		"migrate:status, migrate:fresh, routes, db:seed, schedule:list, schedule:run, work or version)", command)
	if help := routes.Help(); help != "" {
		return fmt.Errorf("%w\n\n%s", err, help)
	}
	return err
}

// open connects using whatever DB_CONNECTION says.
//
// The pool policy, the SQLite directory and the message for a driver that is
// configured but not linked all live in the adapter, so every project gets the
// same ones rather than a slightly different copy.
func open(cfg appconfig.Config) (*data.DB, func(), error) {
	return adapter.Open(cfg.Database.Connection)
}
