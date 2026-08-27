package bootstrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/kernel"
	"github.com/arandu-io/hesape/cache"
	"github.com/arandu-io/hesape/console"
	"github.com/arandu-io/hesape/database"
	dbconsole "github.com/arandu-io/hesape/database/console"
	dbmigrations "github.com/arandu-io/hesape/database/console/migrations"
	"github.com/arandu-io/hesape/database/console/seeds"
	"github.com/arandu-io/hesape/database/migrations"
	"github.com/arandu-io/hesape/database/schema"
	hredis "github.com/arandu-io/hesape/redis"
	"github.com/arandu-io/hesape/redis/connections"

	appconfig "github.com/arandu-io/arandu/config"
	"github.com/arandu-io/arandu/database/seeders"
)

// The migration commands are the component's, not this file's.
//
// They used to be four functions here -- migrate, rollback, status, fresh --
// reimplementing what database/console/migrations already exports as eight, and
// the four were the four somebody had needed so far. migrate:install,
// migrate:reset, migrate:refresh and make:migration did not exist in this
// application because nobody had written them a second time.
//
// None of them runs at boot, and none is reachable from the start-up path.
// `aru migrate` is a step of the deployment pipeline: with N replicas rolling, a
// migrator called from main means N of them racing over one table.

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

// migrationDirectory is where make:migration writes a new file.
//
// It is a directory and not a group, which is the distinction Deps keeps in two
// fields: the application's own migrations register under the default group, so
// the group the commands read is nil and this is where the file goes.
const migrationDirectory = "database/migrations"

// isolationLockTTL is how long an isolated command may hold the lock.
//
// It is the deadlock protection and nothing else: a process that dies partway
// through holds the lock until it expires, and every later run waits it out. So
// it is sized above the longest migration there is rather than against the
// usual one, and an hour of refused runs after a crash is the price of a lock
// that cannot expire under a migrator still using it. The other failure is the
// one that cannot be undone -- two migrators altering one table.
const isolationLockTTL = time.Hour

// newMigrator wires the migrator the migration commands run.
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

	resolver := database.MigrationResolver{Resolver: newConnectionResolver(db)}
	repository := migrations.NewDatabaseMigrationRepository(resolver, migrationTable)

	migrator := migrations.NewMigrator(repository, resolver, nil)
	migrator.SetConnection(migrationConnection)
	return migrator.SetOutput(os.Stdout)
}

// newConnectionResolver holds this application's one connection under the one
// name the migrations know it by.
func newConnectionResolver(db *data.DB) *database.ConnectionResolver {
	connection := database.NewConnection(db.Unwrap(), "", "", map[string]any{
		"driver": string(db.Dialect()),
		"name":   migrationConnection,
	})

	resolver := database.NewConnectionResolver(map[string]database.ConnectionInterface{
		migrationConnection: connection,
	})
	resolver.SetDefaultConnection(migrationConnection)
	return resolver
}

// seedCommands are the two the component exports: db:seed, which runs a seeder,
// and make:seeder, which prints the shape of a new one.
//
// The confirmation in production is the component's, and so is the refusal of
// --class=. What this application answers is which seeders exist and what one is
// allowed to touch -- see database/seeders.
func seedCommands(cfg appconfig.Config, app App) []console.Command {
	return seeds.Commands(seeds.Deps{
		Seed: func(ctx context.Context, name string, args []string) (string, error) {
			if name != "" {
				args = append([]string{name}, args...)
			}
			return seeders.Run(ctx, seeders.Deps{Auth: app.Auth, Tenant: cfg.Auth.Tenant}, args)
		},
		Environment: string(cfg.App.Env),
		SeederPath:  filepath.Join("database", "seeders"),
	})
}

// databaseCommands are the six the component exports for looking at the
// database rather than changing its shape: db, db:show, db:table, db:monitor,
// db:wipe and model:prune.
//
// Tables is a function rather than a schema builder because the catalogue query
// is the schema package's business and the console package does not depend on
// it. Events is nil and Prunables is empty: this application dispatches no
// DatabaseBusy and has nothing to prune, and both degrade to a command that says
// so rather than one that is missing.
func databaseCommands(cfg appconfig.Config, db *data.DB) []console.Command {
	return dbconsole.Commands(dbconsole.Deps{
		Connections: newConnectionResolver(db),
		Wipe:        wipeFor(cfg, db),
		Tables:      tablesFor(db),
		Environment: string(cfg.App.Env),
	})
}

// tablesFor answers the tables of a connection with their row counts.
//
// It reads the catalogue through the schema builder, which is the same door
// every migration writes DDL through -- so what db:show lists and what a
// migration created cannot describe different tables.
func tablesFor(db *data.DB) func(context.Context, string) ([]dbconsole.TableInfo, error) {
	return func(ctx context.Context, connection string) ([]dbconsole.TableInfo, error) {
		name := connection
		if name == "" {
			name = migrationConnection
		}
		conn, err := newConnectionResolver(db).Connection(name)
		if err != nil {
			return nil, err
		}
		concrete, ok := conn.(*database.Connection)
		if !ok {
			return nil, fmt.Errorf("listing the tables needs the concrete connection, and %s resolved to %T", name, conn)
		}

		tables, err := schema.NewBuilder(database.ForSchema(concrete)).GetTables(ctx)
		if err != nil {
			return nil, err
		}

		out := make([]dbconsole.TableInfo, 0, len(tables))
		for _, table := range tables {
			out = append(out, dbconsole.TableInfo{
				Schema: table.Schema,
				Name:   table.Name,
				// The catalogue does not carry a row count, and -1 is what the
				// listing prints as unknown. Counting every table would be a
				// full scan per table on the command that is supposed to be the
				// cheap look.
				Rows: -1,
				Size: table.Size,
			})
		}
		return out, nil
	}
}

// migrationCommands are the eight the component exports, wired to this
// application's migrator, seeders and schema.
//
// The lock is the cache the configuration named, and every command that must
// not overlap another declares the same one -- see console.Command.Isolated.
// A store this process holds by itself would satisfy the types and isolate
// nothing, so a binary configured with the in-process cache gets no lock issuer
// and the isolated commands say so rather than reporting themselves isolated.
func migrationCommands(cfg appconfig.Config, db *data.DB, app App) []console.Command {
	// The queue's two tables are added to what the modules declared, because
	// nothing declares them: the jobs table arrives with the queue module,
	// which asks its driver, and the failed jobs and batches tables belong to a
	// provider and a repository this application wires by hand. Without them
	// `aru queue:failed` is dispatched and answers that there is no such table.
	moduleMigrations := append(app.Kernel.Migrations(), newQueueDeps(app, db).migrations()...)

	return dbmigrations.Commands(dbmigrations.Deps{
		Migrator:      newMigrator(db, moduleMigrations),
		Creator:       migrations.NewMigrationCreator(""),
		MigrationPath: migrationDirectory,
		// Nil, not the directory: this application registers its own
		// migrations in the default group, so every group is the right answer
		// and naming one would read a key that does not exist.
		MigrationGroups: nil,
		Seed:            seedFor(cfg, app),
		Wipe:            wipeFor(cfg, db),
	})
}

// seedFor is what --seed reaches, and it is the same entry point db:seed uses.
//
// The seeder name arrives as the component's --seeder value, and an empty one
// means the root seeder, which is what seeders.Run answers to no arguments.
func seedFor(cfg appconfig.Config, app App) func(context.Context, string) error {
	return func(ctx context.Context, name string) error {
		var args []string
		if name != "" {
			args = []string{name}
		}
		_, err := seeders.Run(ctx, seeders.Deps{Auth: app.Auth, Tenant: cfg.Auth.Tenant}, args)
		return err
	}
}

// wipeFor is what migrate:fresh drops the schema with.
//
// It is handed over as a function because "drop every table" is not a
// capability a framework should assume it has -- see dbmigrations.Deps.Wipe.
// The refusal outside development is in refuseCommand, which runs before the
// command does: a command that cannot do what it was asked should leave nothing
// behind that says it tried.
func wipeFor(_ appconfig.Config, db *data.DB) func(context.Context, string) error {
	return func(ctx context.Context, connection string) error {
		resolver := newConnectionResolver(db)
		name := connection
		if name == "" {
			name = migrationConnection
		}
		conn, err := resolver.Connection(name)
		if err != nil {
			return err
		}
		concrete, ok := conn.(*database.Connection)
		if !ok {
			return fmt.Errorf("migrate:fresh needs the concrete connection to reach the schema builder, and %s resolved to %T", name, conn)
		}
		return schema.NewBuilder(database.ForSchema(concrete)).DropAllTables(ctx)
	}
}

// migrationLocks is the lock issuer the isolated commands take their lock from.
//
// The migration commands name the "migrate" lock and the queue's two prunes
// name their own, so one issuer is always wired and the width of the lock is
// the width of the cache. With a shared store it is every replica; with the
// in-process one it is this process, which is honest for `aru migrate` on
// SQLite and useless for a rollout.
//
// That is why it is refused outside development in migrationLocksFor: a lock
// held inside this process is invisible to the replica beside it, so the run
// would report itself isolated while N of them migrated at once. The old
// spelling of this was an --isolated flag that refused the same case, and it
// refused only when somebody remembered to pass it.
func migrationLocks(store *connections.Connection) *cache.Locks {
	if store == nil {
		return cache.NewLocks(cache.NewArrayStore())
	}
	return cache.NewLocks(hredis.NewRedisStore(store))
}

// devOnly are the commands this application refuses to run outside development.
//
// migrate:fresh drops every table. The usual guard for a command like this is a
// confirmation prompt in production, and a framework whose thesis is that the
// compiler enforces the rules should not rely on somebody reading a prompt at
// 3am. This one simply does not run there.
//
// It is checked before the lock, because it is the narrower answer: a person
// who typed migrate:fresh against production wants to be told that, not to be
// told about the cache.
var devOnly = map[string]bool{"migrate:fresh": true}

// refuseCommand answers why this command may not run, or nil.
//
// The two refusals are ordered narrowest first, and both are this application's
// policy rather than the component's -- the component runs the command it is
// given, with the lock it was handed.
func refuseCommand(cfg appconfig.Config, c console.Command, store *connections.Connection) error {
	if devOnly[c.Name] && !cfg.App.IsDev() {
		return fmt.Errorf("%s drops every table and only runs with APP_ENV=dev (this is %s)", c.Name, cfg.App.Env)
	}

	// Only the isolated commands need a lock, so only they are refused for the
	// want of one. migrate:status reads, and make:migration writes a file.
	if c.Isolated != "" && store == nil && !cfg.App.IsDev() {
		return fmt.Errorf("%s takes a lock every replica can see, and CACHE_STORE names the in-process one: "+
			"a lock held inside this process is invisible to the replica beside it, so a rollout would run it "+
			"N times at once (this is APP_ENV=%s). Set CACHE_STORE=redis and REDIS_URL", c.Name, cfg.App.Env)
	}
	return nil
}
