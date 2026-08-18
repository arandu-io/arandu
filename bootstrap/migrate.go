package bootstrap

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/kernel"
	"github.com/arandu-io/hesape/database"
	"github.com/arandu-io/hesape/database/migrations"

	appconfig "github.com/arandu-io/arandu/config"
)

// The migration commands are the conventional ones, because the steps of a
// deploy should be the ones a developer already knows: migrate,
// migrate:rollback, migrate:status, migrate:fresh.
//
// None of them runs at boot, and none of them is reachable from the start-up
// path. `aru migrate` is a step of the deployment pipeline: with N replicas
// rolling, a migrator called from main means N of them racing over one table.

// migrationConnection is the name this application's single connection is
// registered under.
//
// A migration that names no connection runs on the default one, which is this,
// so the name is only ever written down by a migration that sets
// BaseMigration.Connection to reach somewhere else.
const migrationConnection = "default"

// migrationTable is where applied migration names are recorded.
//
// It is written out here rather than taken from the component's default, and
// the two are not the same string: the default is "migrations" and this is the
// name every database this application has ever migrated already carries.
// Pointing a new binary at the default would find an empty table on a database
// that is fully migrated, and answer by applying every migration a second time.
//
// So it does not become the default by tidying: the value is the compatibility,
// and changing it is a data migration of the tracking table rather than a
// rename.
const migrationTable = "arandu_migrations"

// modulePath is the group the registered modules' migrations are put in.
//
// It is spelled like a path because that is what `--path=` takes, and nothing
// opens it: it is a key, kept apart from the application's own group so the two
// halves stay tellable apart in a listing.
const modulePath = "modules"

// newMigrator wires the migrator the four migration commands run.
//
// The migrations component reaches a connection through a resolver rather than
// being handed one, because a migration may name the connection it runs on.
// This application opens exactly one, so the resolver holds exactly one.
//
// database.ForMigrations, which MigrationResolver applies on the way out, is
// what supplies the transaction per migration and the statement capture
// --pretend prints: the adapted connection answers both of the optional
// interfaces the migrator asks for.
//
// The modules' migrations are put in the registry here because that is the
// only place the migrator reads. A module is a value the kernel already holds,
// so it is asked for them rather than announcing them from init() the way the
// application's own do -- and rollback and status look a recorded name up in
// the registry, so a module's migration that never reached it would be reported
// as missing and skipped at the moment somebody needs it undone.
//
// Output goes to stdout because the migrator prints each migration and how it
// turned out. Nothing below reprints what it already said.
func newMigrator(db *data.DB, moduleMigrations []kernel.Migration) *migrations.Migrator {
	for _, migration := range moduleMigrations {
		migrations.Register(migration, modulePath)
	}

	connection := database.NewConnection(db.Unwrap(), "", "", map[string]any{
		"driver": string(db.Dialect()),
		"name":   migrationConnection,
	})

	inner := database.NewConnectionResolver(map[string]database.ConnectionInterface{
		migrationConnection: connection,
	})
	inner.SetDefaultConnection(migrationConnection)

	resolver := database.MigrationResolver{Resolver: inner}
	repository := migrations.NewDatabaseMigrationRepository(resolver, migrationTable)

	migrator := migrations.NewMigrator(repository, resolver, nil)
	migrator.SetConnection(migrationConnection)
	return migrator.SetOutput(os.Stdout)
}

// migrateOptions reads the flags the migration commands take.
//
// --pretend prints the statements a run would send and sends none of them.
// --step gives every migration its own batch on the way up, so each can be
// undone on its own; --step=N on the way down is how many to undo.
func migrateOptions(args []string) (migrations.Options, error) {
	var options migrations.Options

	for _, arg := range args {
		switch {
		case arg == "--pretend":
			options.Pretend = true

		case arg == "--step":
			options.Step = true

		case strings.HasPrefix(arg, "--step="):
			count := strings.TrimPrefix(arg, "--step=")
			steps, err := strconv.Atoi(count)
			if err != nil || steps < 1 {
				return options, fmt.Errorf("--step= takes the number of migrations to roll back, and %q is not one", count)
			}
			options.Steps = steps

		default:
			return options, fmt.Errorf("unknown flag: %s (expected --pretend, --step or --step=N)", arg)
		}
	}

	return options, nil
}

// prepareRepository creates the tracking table if it is not there yet.
//
// It is skipped while pretending, and the run is refused instead when the
// table is missing: a command that promises to send no statement and creates a
// table anyway is a command nobody can trust the second time.
func prepareRepository(ctx context.Context, migrator *migrations.Migrator, pretend bool) error {
	if !pretend {
		return migrator.GetRepository().CreateRepository(ctx)
	}
	if !migrator.RepositoryExists(ctx) {
		return fmt.Errorf("--pretend reads the %s table to know what is pending, and it does not exist yet: run migrate once without it", migrationTable)
	}
	return nil
}

// migrate applies everything that has not been applied yet.
func migrate(ctx context.Context, db *data.DB, moduleMigrations []kernel.Migration, options migrations.Options) error {
	migrator := newMigrator(db, moduleMigrations)

	if err := prepareRepository(ctx, migrator, options.Pretend); err != nil {
		return err
	}

	_, err := migrator.Run(ctx, nil, options)
	return err
}

// rollback undoes the last batch, or as many migrations as --step=N names.
//
// A migration that declares no Down is not reversed, and that is not an error:
// the migrator asks for the reverse half by type assertion, so a Down with the
// wrong signature fails the build rather than rolling nothing back at run time.
func rollback(ctx context.Context, db *data.DB, moduleMigrations []kernel.Migration, options migrations.Options) error {
	migrator := newMigrator(db, moduleMigrations)

	if err := prepareRepository(ctx, migrator, options.Pretend); err != nil {
		return err
	}

	_, err := migrator.Rollback(ctx, nil, options)
	return err
}

// migrateStatus prints every migration with the batch it ran in.
//
// The recorded names and the registered ones are printed together rather than
// one of them: a migration whose file is gone still holds a row, and a run that
// hid it would report a schema nobody can rebuild.
func migrateStatus(ctx context.Context, db *data.DB, moduleMigrations []kernel.Migration) error {
	migrator := newMigrator(db, moduleMigrations)

	// A database nothing has run against yet has no tracking table, and that is
	// not an error here: every migration is pending, which is exactly what
	// somebody asking for the status before the first deploy wants to read.
	batches := map[string]int{}
	if migrator.RepositoryExists(ctx) {
		recorded, err := migrator.GetRepository().GetMigrationBatches(ctx)
		if err != nil {
			return err
		}
		batches = recorded
	}

	registered := migrator.GetMigrationFiles(nil)

	names := make([]string, 0, len(registered)+len(batches))
	seen := make(map[string]bool, len(registered)+len(batches))
	for name := range batches {
		seen[name] = true
		names = append(names, name)
	}
	for name := range registered {
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	// The name carries the order, here as everywhere else.
	sort.Strings(names)

	if len(names) == 0 {
		fmt.Println("no migrations are registered")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "MIGRATION\tBATCH\tSTATUS")
	for _, name := range names {
		batch, ran := batches[name]
		switch {
		case !ran:
			fmt.Fprintf(w, "%s\t-\tpending\n", name)
		case registered[name] == nil:
			fmt.Fprintf(w, "%s\t%d\tran, no longer registered\n", name, batch)
		default:
			fmt.Fprintf(w, "%s\t%d\tran\n", name, batch)
		}
	}
	return w.Flush()
}

// fresh rolls everything back and applies it again.
//
// It refuses to run outside development. The usual guard for a command like this is a
// confirmation prompt in production; a framework whose thesis is that the
// compiler enforces the rules should not rely on someone reading a prompt at
// 3am, so this one simply does not run there.
func fresh(ctx context.Context, cfg appconfig.Config, db *data.DB, moduleMigrations []kernel.Migration, options migrations.Options) error {
	if !cfg.App.IsDev() {
		return fmt.Errorf("migrate:fresh drops every table and only runs with APP_ENV=dev (this is %s)", cfg.App.Env)
	}

	migrator := newMigrator(db, moduleMigrations)

	if err := prepareRepository(ctx, migrator, options.Pretend); err != nil {
		return err
	}

	if _, err := migrator.Reset(ctx, nil, options.Pretend); err != nil {
		return err
	}

	_, err := migrator.Run(ctx, nil, options)
	return err
}
