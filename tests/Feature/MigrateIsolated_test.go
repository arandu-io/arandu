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

// `migrate --isolated` is the belt for a deployment that calls migrate from the
// release command of every container instead of once from the pipeline: one
// replica takes a lock every replica can see, migrates, and the rest apply
// nothing and carry on.
//
// Three things have to hold, and each fails silently on its own. The lock has
// to be taken before anything is applied, or the flag is decoration. A replica
// that did not take it has to report success, or every rollout of more than one
// replica fails. And a lock nobody else can see has to be refused, or the
// command says isolated and is not.

// TestTheReplicaThatDidNotGetTheLockAppliesNothingAndSucceeds.
//
// This is the whole design of the flag. What that replica sees is an empty
// list, a run that did not happen, and no error -- the schema is being changed
// by whoever got there first, and this process carries on and lets the
// application start. Reporting it as a failure would fail the deployment the
// flag exists to serve.
func TestTheReplicaThatDidNotGetTheLockAppliesNothingAndSucceeds(t *testing.T) {
	sqliteEnv(t)
	useStore(t, startStore(t, lockHeldByAnother))

	if err := bootstrap.Dispatch("migrate", []string{"--isolated"}); err != nil {
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

	if err := bootstrap.Dispatch("migrate", []string{"--isolated"}); err != nil {
		t.Fatalf("migrate --isolated: %v", err)
	}
	if !tableExists(t, "users") {
		t.Error("the replica took the lock and applied nothing")
	}
}

// TestNothingIsAppliedWhenTheLockCannotBeReached.
//
// A store that does not answer is not permission to migrate. Without the lock
// this is N replicas altering one schema at once, which is the failure the flag
// exists to prevent and the one that cannot be undone.
func TestNothingIsAppliedWhenTheLockCannotBeReached(t *testing.T) {
	sqliteEnv(t)
	// Port 1 is reserved and nothing listens on it, so the connection is
	// refused at once rather than left to time out.
	useStore(t, "127.0.0.1:1")

	err := bootstrap.Dispatch("migrate", []string{"--isolated"})
	if err == nil {
		t.Fatal("the migration ran without ever taking the lock")
	}
	// The migrator refuses an isolated run when nothing gave it a lock to take.
	// That refusal passing for this one would mean the issuer was never wired
	// and the test proves nothing about the lock.
	if strings.Contains(err.Error(), "lock issuer") {
		t.Fatalf("the migrator was never given a lock to take: %v", err)
	}
	if tableExists(t, "users") {
		t.Error("the schema was applied without the lock")
	}
}

// TestIsolationIsRefusedWithTheInProcessStore.
//
// A lock held inside this process is invisible to the replica beside it, so a
// run isolated by one is not isolated at all. Refusing is the difference the
// flag is worth having: a command that says it is isolated and is not is worse
// than one that does not start.
func TestIsolationIsRefusedWithTheInProcessStore(t *testing.T) {
	sqliteEnv(t)
	t.Setenv("CACHE_STORE", "memory")

	err := bootstrap.Dispatch("migrate", []string{"--isolated"})
	if err == nil {
		t.Fatal("it migrated, and called that isolated")
	}
	// Told apart from the flag not parsing at all, which was the state before
	// and would satisfy any test that only asks for an error.
	if strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("--isolated is not being read as a flag: %v", err)
	}
	if !strings.Contains(err.Error(), "CACHE_STORE") {
		t.Errorf("the refusal does not say what to configure: %v", err)
	}
	// A refused command leaves nothing behind that says it tried.
	if tableExists(t, "arandu_migrations") {
		t.Error("the tracking table was created by a command that refused to run")
	}
}

// TestIsolationBelongsToMigrate: the commands that cannot be isolated say so.
// Accepting the flag and ignoring it would be a rollback reporting itself
// isolated while N replicas undo the schema over each other.
func TestIsolationBelongsToMigrate(t *testing.T) {
	for _, command := range []string{"migrate:rollback", "migrate:fresh"} {
		t.Run(command, func(t *testing.T) {
			sqliteEnv(t)
			useStore(t, startStore(t, lockFree))

			if err := bootstrap.Dispatch("migrate", nil); err != nil {
				t.Fatalf("migrate: %v", err)
			}

			err := bootstrap.Dispatch(command, []string{"--isolated"})
			if err == nil {
				t.Fatalf("%s took --isolated and did nothing with it", command)
			}
			if !strings.Contains(err.Error(), "--isolated") {
				t.Errorf("the refusal does not name the flag: %v", err)
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
