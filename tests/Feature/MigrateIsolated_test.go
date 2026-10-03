package feature_test

import (
	"context"
	"strings"
	"testing"

	"github.com/arandu-io/arandu/bootstrap"
)

// Every migration command takes a lock before it does anything, and that is not
// a flag anybody passes: console.Command.Isolated names it, and the application
// hands the commands the store it takes it from.
//
// It used to be `migrate --isolated`, opt-in, and only on the forward run --
// the four hand-written commands this application had could not lock a rollback
// or a reset, so the flag was refused there. The component locks all of them,
// and a lock somebody has to remember to ask for is a lock that is not taken on
// the deploy where it mattered.
//
// Three things have to hold, and each fails silently on its own. The lock has
// to be taken before anything is applied, or it is decoration. A replica that
// did not take it has to report success, or every rollout of more than one
// replica fails. And a lock nobody else can see has to be refused in
// production, or the command says isolated and is not.

// TestTheReplicaThatDidNotGetTheLockAppliesNothingAndSucceeds.
//
// This is the whole design. What that replica sees is a run that did not
// happen and no error -- the schema is being changed by whoever got there
// first, and this process carries on and lets the application start. Reporting
// it as a failure would fail the deployment the lock exists to serve.
//
// "Another process holds it" is a single answer no real store can be asked to
// give on demand, so the store is the fake connector's and it is told to give
// it. What the lock store does with SET NX across processes is the connector's
// claim, proved against a real server in github.com/arandu-io/hesape/redis.
func TestTheReplicaThatDidNotGetTheLockAppliesNothingAndSucceeds(t *testing.T) {
	server := borrowRESP(t)
	server.holdLock()
	sqliteEnv(t)
	useStore(t, server.address)

	if err := bootstrap.Dispatch("migrate", nil); err != nil {
		t.Fatalf("a replica that did not take the lock reported a failure: %v", err)
	}
	if tableExists(t, "users") {
		t.Error("it migrated anyway: the lock was held by another process and this one applied the schema")
	}
}

// TestTheReplicaThatGotTheLockMigrates: the other side of the same run, so that
// "applies nothing" is not passing because nothing ever migrates.
func TestTheReplicaThatGotTheLockMigrates(t *testing.T) {
	server := borrowRESP(t)
	sqliteEnv(t)
	useStore(t, server.address)

	if err := bootstrap.Dispatch("migrate", nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !tableExists(t, "users") {
		t.Error("the replica took the lock and applied nothing")
	}
}

// TestNothingIsAppliedWhenTheLockCannotBeReached.
//
// A store that does not answer is not permission to migrate. Without the lock
// this is N replicas altering one schema at once, which is the failure the lock
// exists to prevent and the one that cannot be undone.
func TestNothingIsAppliedWhenTheLockCannotBeReached(t *testing.T) {
	address, _ := unansweredRESP(t)
	sqliteEnv(t)
	useStore(t, address)

	err := bootstrap.Dispatch("migrate", nil)
	if err == nil {
		t.Fatal("the migration ran without ever taking the lock")
	}
	// The console refuses an isolated command when nothing gave it a lock to
	// take. That refusal passing for this one would mean the issuer was never
	// wired and the test proves nothing about the lock.
	if strings.Contains(err.Error(), "lock issuer") {
		t.Fatalf("the command was never given a lock to take: %v", err)
	}
	if tableExists(t, "users") {
		t.Error("the schema was applied without the lock")
	}
}

// TestIsolationIsRefusedWithTheInProcessStoreInProduction.
//
// A lock held inside this process is invisible to the replica beside it, so a
// run isolated by one is not isolated at all. In development that is the honest
// width of the lock and `aru migrate` on SQLite has to work; in production it
// is a command that says it is isolated and is not, which is worse than one
// that does not start.
func TestIsolationIsRefusedWithTheInProcessStoreInProduction(t *testing.T) {
	sqliteEnv(t)
	t.Setenv("APP_ENV", "prod")
	t.Setenv("CACHE_STORE", "memory")

	err := bootstrap.Dispatch("migrate", nil)
	if err == nil {
		t.Fatal("it migrated, and called that isolated")
	}
	if !strings.Contains(err.Error(), "CACHE_STORE") {
		t.Errorf("the refusal does not say what to configure: %v", err)
	}
	// A refused command leaves nothing behind that says it tried.
	if tableExists(t, "arandu_migrations") {
		t.Error("the tracking table was created by a command that refused to run")
	}
}

// TestTheInProcessStoreIsEnoughInDevelopment: the other side of the refusal.
//
// A project starts on SQLite with no Redis and `aru migrate` has to work, so
// the lock is wired from the in-process store and is as wide as this process.
// Refusing here would make the framework unusable before the first deploy.
func TestTheInProcessStoreIsEnoughInDevelopment(t *testing.T) {
	sqliteEnv(t)
	t.Setenv("CACHE_STORE", "memory")

	if err := bootstrap.Dispatch("migrate", nil); err != nil {
		t.Fatalf("migrate with the in-process store in development: %v", err)
	}
	if !tableExists(t, "users") {
		t.Error("it took the in-process lock and applied nothing")
	}
}

// TestEveryMigrationCommandIsIsolated: the reason the flag is gone.
//
// Rollback, reset, refresh and fresh alter the schema too, and N replicas
// undoing it over each other is the same failure migrate was locked against.
// The old --isolated was refused on all four, which named the limitation and
// called it a design.
//
// Three of the five are refused for what they do before they are refused for
// the lock, and that is the narrower answer -- a person who typed migrate:reset
// against production wants to be told that, not to be told about the cache. So
// this test no longer reaches the lock on those three, and it does not need to:
// the lock refusal exists to stop an unisolated run in production, and a command
// that does not run in production at all cannot have one. In development the
// in-process lock is accepted on purpose, which is what makes `aru migrate` work
// on a fresh clone with no Redis.
func TestEveryMigrationCommandIsIsolated(t *testing.T) {
	for _, command := range []string{"migrate", "migrate:rollback", "migrate:reset", "migrate:refresh", "migrate:fresh"} {
		t.Run(command, func(t *testing.T) {
			sqliteEnv(t)
			t.Setenv("APP_ENV", "prod")
			t.Setenv("CACHE_STORE", "memory")

			err := bootstrap.Dispatch(command, nil)
			if err == nil {
				t.Fatalf("%s ran in production with a lock nobody else can see", command)
			}
			if emptiesTheSchema[command] {
				if !strings.Contains(err.Error(), "APP_ENV=dev") {
					t.Errorf("%s was not refused for what it does: %v", command, err)
				}
				return
			}
			if !strings.Contains(err.Error(), "CACHE_STORE") {
				t.Errorf("%s is not isolated: %v", command, err)
			}
		})
	}
}

// emptiesTheSchema is the set bootstrap keeps out of production, restated here
// because the map over there is unexported and a test that imported it would be
// asserting that a map equals itself.
var emptiesTheSchema = map[string]bool{
	"migrate:fresh":   true,
	"migrate:reset":   true,
	"migrate:refresh": true,
}

// TestTheCommandsThatEmptyTheSchemaDoNotRunInProduction.
//
// Rolling a binary back is routine and reversible; rolling a schema back is
// often neither, and the three below do not roll one back -- they take all of
// it. Only migrate:fresh was refused, for the reason "it drops every table",
// which was already true of the other two: migrate:reset runs every Down, and
// every Down here is a DROP TABLE, and migrate:refresh runs that and then
// rebuilds an empty schema over it.
//
// Each one is asserted twice: refused, and having left nothing behind. A command
// that refused after taking the first step would satisfy the first half.
func TestTheCommandsThatEmptyTheSchemaDoNotRunInProduction(t *testing.T) {
	for command := range emptiesTheSchema {
		t.Run(command, func(t *testing.T) {
			sqliteEnv(t)
			if err := bootstrap.Dispatch("migrate", nil); err != nil {
				t.Fatalf("migrate: %v", err)
			}
			t.Setenv("APP_ENV", "prod")

			err := bootstrap.Dispatch(command, nil)
			if err == nil {
				t.Fatalf("%s emptied a production schema", command)
			}
			if !strings.Contains(err.Error(), "APP_ENV=dev") {
				t.Errorf("the refusal does not say where the command does run: %v", err)
			}
			// The way out has to be in the refusal. Somebody undoing a bad
			// release needs the command that undoes one batch, and an error that
			// only forbids sends them to the source of the CLI to find it.
			if !strings.Contains(err.Error(), "migrate:rollback") {
				t.Errorf("the refusal does not name the command that undoes a release: %v", err)
			}
			if !tableExists(t, "users") {
				t.Error("the refusal came after the schema was already gone")
			}
		})
	}
}

// TestRollbackStillRunsInProduction is the other side of the refusal above, and
// the reason it is not simply "no schema-down in production".
//
// Undoing the batch a bad release applied is a real deployment operation, and a
// guard that took it away would refuse the safe rollback in order to prevent the
// unsafe one. It stays, scoped to one batch, and isolated.
func TestRollbackStillRunsInProduction(t *testing.T) {
	sqliteEnv(t)
	if err := bootstrap.Dispatch("migrate", nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	t.Setenv("APP_ENV", "prod")
	useStore(t, borrowRESP(t).address)

	if err := bootstrap.Dispatch("migrate:rollback", nil); err != nil {
		t.Fatalf("migrate:rollback was refused in production: %v", err)
	}
}

// useStore points the cache configuration at an address.
func useStore(t *testing.T, address string) {
	t.Helper()
	t.Setenv("CACHE_STORE", "redis")
	t.Setenv("REDIS_URL", "redis://"+address)
}

// tableExists reports whether the schema has a table of that name.
//
// It reads the database itself rather than the migrator's own report, because
// what is being checked is whether a run happened at all -- and a report is
// exactly what a run that did not happen still produces.
func tableExists(t *testing.T, name string) bool {
	t.Helper()

	_, db, _ := openForTest(t)
	var count int
	err := db.Unwrap().QueryRowContext(context.Background(),
		"SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", name).Scan(&count)
	if err != nil {
		t.Fatalf("reading the schema: %v", err)
	}
	return count > 0
}
