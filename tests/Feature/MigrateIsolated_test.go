package feature_test

import (
	"bufio"
	"context"
	"io"
	"net"
	"strconv"
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
func TestTheReplicaThatDidNotGetTheLockAppliesNothingAndSucceeds(t *testing.T) {
	sqliteEnv(t)
	useStore(t, startStore(t, lockHeldByAnother))

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
	sqliteEnv(t)
	useStore(t, startStore(t, lockFree))

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
	sqliteEnv(t)
	// Port 1 is reserved and nothing listens on it, so the connection is
	// refused at once rather than left to time out.
	useStore(t, "127.0.0.1:1")

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
			// migrate:fresh is refused for dropping every table before it is
			// refused for the lock, and that is the narrower answer.
			if command == "migrate:fresh" {
				if !strings.Contains(err.Error(), "APP_ENV=dev") {
					t.Errorf("migrate:fresh was not refused for what it does: %v", err)
				}
				return
			}
			if !strings.Contains(err.Error(), "CACHE_STORE") {
				t.Errorf("%s is not isolated: %v", command, err)
			}
		})
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

// Whether the store answers that the lock was free or that somebody else has
// it. They are the two outcomes an isolated run has, and the second one is the
// one the design is about.
const (
	lockFree           = true
	lockHeldByAnother  = false
	respNil            = "$-1\r\n"
	respOK             = "+OK\r\n"
	respUnknownCommand = "-ERR unknown command\r\n"
)

// startStore runs a key-value store that answers what a lock asks and nothing
// else, and returns its address.
//
// A server of this test's own rather than one the machine has to be running:
// the suite installs nothing, and the answer to "the lock was already taken" is
// a single reply that no real store can be asked to give on demand.
func startStore(t *testing.T, free bool) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go serveStore(conn, free)
		}
	}()

	return listener.Addr().String()
}

func serveStore(conn net.Conn, free bool) {
	defer func() { _ = conn.Close() }()

	reader := bufio.NewReader(conn)
	for {
		command, err := readCommand(reader)
		if err != nil {
			return
		}
		if _, err := conn.Write([]byte(replyTo(command, free))); err != nil {
			return
		}
	}
}

// readCommand reads one command and answers its first word, lowercased.
//
// Only the name is kept: what the reply is does not depend on the arguments in
// any case this serves, and a parser that decoded them would be a second
// implementation of RESP to keep correct.
func readCommand(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	line = strings.TrimRight(line, "\r\n")

	if !strings.HasPrefix(line, "*") {
		// An inline command, which is what a client sends before it knows what
		// it is talking to.
		fields := strings.Fields(line)
		if len(fields) == 0 {
			return "", nil
		}
		return strings.ToLower(fields[0]), nil
	}

	count, err := strconv.Atoi(line[1:])
	if err != nil {
		return "", err
	}

	name := ""
	for i := 0; i < count; i++ {
		header, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		size, err := strconv.Atoi(strings.TrimRight(header, "\r\n")[1:])
		if err != nil {
			return "", err
		}
		// The two trailing bytes are the terminator, and they are read so that
		// the next command starts where it should.
		body := make([]byte, size+2)
		if _, err := io.ReadFull(reader, body); err != nil {
			return "", err
		}
		if i == 0 {
			name = strings.ToLower(string(body[:size]))
		}
	}
	return name, nil
}

func replyTo(command string, free bool) string {
	switch command {
	case "hello":
		// Answered the way a server that predates it answers, which is what
		// makes the client fall back rather than give up.
		return respUnknownCommand
	case "ping":
		return "+PONG\r\n"
	case "set":
		// The only reply that matters. NX writes the token and answers OK when
		// the key was free, and answers nothing at all when it was not.
		if free {
			return respOK
		}
		return respNil
	case "get":
		return respNil
	case "exec":
		return "*0\r\n"
	case "del":
		return ":1\r\n"
	default:
		return respOK
	}
}
