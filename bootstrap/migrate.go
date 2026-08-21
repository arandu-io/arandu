package bootstrap

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/kernel"
	"github.com/arandu-io/hesape/cache"
	"github.com/arandu-io/hesape/database"
	"github.com/arandu-io/hesape/database/migrations"
	hredis "github.com/arandu-io/hesape/redis"
	"github.com/arandu-io/hesape/redis/connections"

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

// migrateFlags is what the migration commands read off the command line.
//
// It carries the migrator's own options and the one flag that is not one:
// --isolated does not change what a run does, it changes who is allowed to
// start it, so it belongs to this file and never travels to the migrator.
type migrateFlags struct {
	migrations.Options

	// Isolated says one process migrates and the rest carry on.
	Isolated bool
}

// migrateOptions reads the flags the migration commands take.
//
// --pretend prints the statements a run would send and sends none of them.
// --step gives every migration its own batch on the way up, so each can be
// undone on its own; --step=N on the way down is how many to undo.
//
// --isolated takes a lock every replica can see, so that a release command run
// by each container of a rollout migrates once. It belongs to migrate: there is
// no isolated rollback, and a flag parsed and then ignored is worse than one
// that is refused.
func migrateOptions(args []string) (migrateFlags, error) {
	var flags migrateFlags

	for _, arg := range args {
		switch {
		case arg == "--pretend":
			flags.Pretend = true

		case arg == "--step":
			flags.Step = true

		case arg == "--isolated":
			flags.Isolated = true

		case strings.HasPrefix(arg, "--step="):
			count := strings.TrimPrefix(arg, "--step=")
			steps, err := strconv.Atoi(count)
			if err != nil || steps < 1 {
				return flags, fmt.Errorf("--step= takes the number of migrations to roll back, and %q is not one", count)
			}
			flags.Steps = steps

		default:
			return flags, fmt.Errorf("unknown flag: %s (expected --pretend, --step, --step=N or --isolated)", arg)
		}
	}

	return flags, nil
}

// refuseIsolation is what the commands that cannot isolate answer.
//
// They cannot because there is nothing to isolate them with: the migrator locks
// a run forward and offers no locked rollback or reset. Accepting the flag and
// doing nothing with it would be a command that reports itself as isolated
// while N replicas roll back over each other.
func refuseIsolation(command string, flags migrateFlags) error {
	if !flags.Isolated {
		return nil
	}
	return fmt.Errorf("--isolated is a flag of migrate, and %s does not take it: "+
		"only the forward run can be locked, and undoing a schema on one replica while another undoes it too "+
		"is not something a lock here would prevent", command)
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

// isolationLockTTL is how long an isolated run may hold the lock.
//
// It is the deadlock protection and nothing else: a process that dies partway
// through holds the lock until it expires, and every later run waits it out. So
// it is sized above the longest migration there is rather than against the
// usual one, and an hour of refused runs after a crash is the price of a lock
// that cannot expire under a migrator still using it. The other failure is the
// one that cannot be undone -- two migrators altering one table.
const isolationLockTTL = time.Hour

// migrate applies everything that has not been applied yet.
//
// Without --isolated it is the plain run, which is correct because `aru migrate`
// is a step of the pipeline and a pipeline step happens once. --isolated is for
// the deployment that calls it from the release command of every container
// instead: one of them takes the lock and migrates, and the rest apply nothing
// and carry on.
//
// The store is the one the cache configuration named, and a nil one is refused
// rather than worked around. A lock inside this process would satisfy every
// type here and isolate nothing.
func migrate(ctx context.Context, db *data.DB, moduleMigrations []kernel.Migration, flags migrateFlags, store *connections.Connection) error {
	// Refused before the tracking table is created, and before anything else
	// touches the database: a command that cannot do what it was asked should
	// leave nothing behind that says it tried.
	if flags.Isolated && store == nil {
		return fmt.Errorf("--isolated needs a store every replica can see, and CACHE_STORE names the in-process one: " +
			"a lock held inside this process is invisible to the replica beside it, so the run would report itself " +
			"isolated while N of them migrated at once. Set CACHE_STORE=redis and REDIS_URL")
	}

	migrator := newMigrator(db, moduleMigrations)

	if err := prepareRepository(ctx, migrator, flags.Pretend); err != nil {
		return err
	}

	if !flags.Isolated {
		_, err := migrator.Run(ctx, nil, flags.Options)
		return err
	}

	locks := cache.NewLocks(hredis.NewRedisStore(store))
	migrator.IsolateWith(func(name string) migrations.IsolationLock {
		return locks.Lock(name, isolationLockTTL)
	})

	// The answer to branch on is whether the run took the lock, and it is never
	// the error. A replica that did not take it applied nothing, and that is
	// success: the schema is being changed by whichever replica got there
	// first, and this one's job is to carry on and let the application start.
	// Reporting it as a failure would fail every deployment that rolls more
	// than one replica, which is the deployment the flag exists for. The
	// migrator has already said so on stdout.
	_, _, err := migrator.RunIsolated(ctx, nil, flags.Options)
	return err
}

// rollback undoes the last batch, or as many migrations as --step=N names.
//
// A migration that declares no Down is not reversed, and that is not an error:
// the migrator asks for the reverse half by type assertion, so a Down with the
// wrong signature fails the build rather than rolling nothing back at run time.
func rollback(ctx context.Context, db *data.DB, moduleMigrations []kernel.Migration, flags migrateFlags) error {
	if err := refuseIsolation("migrate:rollback", flags); err != nil {
		return err
	}

	migrator := newMigrator(db, moduleMigrations)

	if err := prepareRepository(ctx, migrator, flags.Pretend); err != nil {
		return err
	}

	_, err := migrator.Rollback(ctx, nil, flags.Options)
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
func fresh(ctx context.Context, cfg appconfig.Config, db *data.DB, moduleMigrations []kernel.Migration, flags migrateFlags) error {
	if !cfg.App.IsDev() {
		return fmt.Errorf("migrate:fresh drops every table and only runs with APP_ENV=dev (this is %s)", cfg.App.Env)
	}
	if err := refuseIsolation("migrate:fresh", flags); err != nil {
		return err
	}

	migrator := newMigrator(db, moduleMigrations)

	if err := prepareRepository(ctx, migrator, flags.Pretend); err != nil {
		return err
	}

	if _, err := migrator.Reset(ctx, nil, flags.Pretend); err != nil {
		return err
	}

	_, err := migrator.Run(ctx, nil, flags.Options)
	return err
}
