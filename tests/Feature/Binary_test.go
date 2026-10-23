package feature_test

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	appjobs "github.com/arandu-io/arandu/app/Jobs"
	"github.com/arandu-io/arandu/tests"
)

// The binary is the artisan: main.go loads the application and hands the
// command line to the framework's console kernel, and `aru migrate`, `aru
// queue:work` and every other forwarded command run this binary with the name.
// The tests below build it the way the Dockerfile does and run every command
// the skeleton answered before the kernel moved into the framework, so what is
// proved is the process a deploy starts, not a function a test called.
//
// queue:listen is the one command not run: it starts a child worker and
// restarts it whenever it exits, so it is a process that does not end. It is
// in the listing beside the rest.

// binaryTenant is the tenant every command below runs for.
const binaryTenant = "11111111-1111-4111-8111-111111111111"

// artisan runs the binary with one environment and one working directory, the
// way a deploy step runs it from the project root.
type artisan struct {
	path string
	dir  string
	env  []string
}

func newArtisan(t *testing.T) artisan {
	t.Helper()
	dir := t.TempDir()
	env := append(os.Environ(),
		"APP_NAME="+tests.AppName,
		"APP_ENV=dev",
		"APP_KEY=0123456789abcdef0123456789abcdef",
		"DATABASE_URL=sqlite://"+filepath.Join(dir, "app.sqlite"),
		"ARANDU_TENANT_ID="+binaryTenant,
		"CACHE_STORE=memory",
		"SESSION_DRIVER=memory",
		"QUEUE_CONNECTION=database",
		"REDIS_URL=",
		"LOG_LEVEL=error",
	)
	return artisan{path: tests.Binary(t, "v9.9.9-binary", "c0ffee"), dir: dir, env: env}
}

// command is the process for one command line, not yet started.
func (a artisan) command(ctx context.Context, args ...string) (*exec.Cmd, *bytes.Buffer, *bytes.Buffer) {
	cmd := exec.CommandContext(ctx, a.path, args...)
	cmd.Dir, cmd.Env = a.dir, a.env
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	return cmd, &out, &errOut
}

// run runs one command line to the end and answers what it wrote and the
// status it exited with.
func (a artisan) run(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd, out, errOut := a.command(ctx, args...)
	err := cmd.Run()
	if exit, ok := err.(*exec.ExitError); ok {
		return out.String(), errOut.String(), exit.ExitCode()
	}
	if err != nil {
		t.Fatalf("running %s: %v", strings.Join(args, " "), err)
	}
	return out.String(), errOut.String(), 0
}

// ok runs a command line that has to succeed, and answers what it printed.
func (a artisan) ok(t *testing.T, args ...string) string {
	t.Helper()
	out, errOut, code := a.run(t, args...)
	if code != 0 {
		t.Fatalf("%s exited %d:\n%s%s", strings.Join(args, " "), code, out, errOut)
	}
	return out
}

// interrupted starts a command that runs until it is told to stop, waits for
// ready, sends SIGTERM and answers the status it exited with.
func (a artisan) interrupted(t *testing.T, ready func() bool, args ...string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd, out, errOut := a.command(ctx, args...)
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting %s: %v", strings.Join(args, " "), err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()

	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	poll := time.NewTicker(50 * time.Millisecond)
	defer poll.Stop()
	for !ready() {
		select {
		case err := <-exited:
			t.Fatalf("%s exited before it was ready (%v):\n%s%s", strings.Join(args, " "), err, out, errOut)
		case <-deadline.C:
			_ = cmd.Process.Kill()
			t.Fatalf("%s was not ready in 30s:\n%s%s", strings.Join(args, " "), out, errOut)
		case <-poll.C:
		}
	}

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("interrupting %s: %v", strings.Join(args, " "), err)
	}
	select {
	case err := <-exited:
		if exit, ok := err.(*exec.ExitError); ok {
			t.Logf("%s:\n%s%s", strings.Join(args, " "), out, errOut)
			return exit.ExitCode()
		}
		if err != nil {
			t.Fatalf("waiting for %s: %v", strings.Join(args, " "), err)
		}
		return 0
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("%s did not stop in 30s after SIGTERM", strings.Join(args, " "))
		return -1
	}
}

// waiting answers how many jobs queue:monitor reports on the default queue.
func (a artisan) waiting(t *testing.T) string {
	t.Helper()
	for _, line := range strings.Split(a.ok(t, "queue:monitor", "database:default"), "\n") {
		if fields := strings.Fields(line); len(fields) >= 2 && fields[0] == "database:default" {
			return fields[1]
		}
	}
	t.Fatal("queue:monitor printed no row for database:default")
	return ""
}

// TestEveryCommandAnswersThroughTheBinary walks the commands in the order a
// person meets them, against one database.
func TestEveryCommandAnswersThroughTheBinary(t *testing.T) {
	a := newArtisan(t)

	t.Run("version prints what the build stamped", func(t *testing.T) {
		want := tests.AppName + " v9.9.9-binary (c0ffee)\n"
		if got := a.ok(t, "version"); got != want {
			t.Fatalf("version printed %q, want %q", got, want)
		}
	})

	t.Run("the listing names every command, the application's among them", func(t *testing.T) {
		listed := a.ok(t, "list")
		for _, name := range []string{
			"serve", "routes", "version", "vendor:publish",
			"migrate", "migrate:install", "migrate:rollback", "migrate:reset", "migrate:refresh",
			"migrate:fresh", "migrate:status", "make:migration",
			"db", "db:show", "db:table", "db:monitor", "db:wipe", "db:seed", "make:seeder", "model:prune",
			"queue:work", "queue:listen", "queue:restart", "queue:pause", "queue:resume", "queue:clear",
			"queue:monitor", "queue:failed", "queue:retry", "queue:forget", "queue:flush",
			"queue:prune-failed", "queue:retry-batch", "queue:prune-batches",
			"schedule:list", "schedule:run", "cache:clear", "cache:forget", "cache:prune-stale-tags",
			"notes:digest",
		} {
			if !strings.Contains(listed, "  "+name+" ") {
				t.Errorf("the listing does not name %s:\n%s", name, listed)
			}
		}
	})

	t.Run("an unknown command is refused with the listing", func(t *testing.T) {
		_, errOut, code := a.run(t, "no:such-command")
		if code != 1 || !strings.Contains(errOut, "unknown command: no:such-command") || !strings.Contains(errOut, "migrate") {
			t.Fatalf("exit %d, stderr:\n%s", code, errOut)
		}
	})

	t.Run("the migration commands", func(t *testing.T) {
		a.ok(t, "migrate:install")
		a.ok(t, "migrate")
		if status := a.ok(t, "migrate:status"); !strings.Contains(status, "create_users") || !strings.Contains(status, "Ran") {
			t.Errorf("migrate:status does not report the users migration as ran:\n%s", status)
		}
		a.ok(t, "migrate:rollback")
		a.ok(t, "migrate")
		a.ok(t, "migrate:refresh")
		a.ok(t, "migrate:reset")
		a.ok(t, "migrate:fresh")

		a.ok(t, "make:migration", "add_flag_to_notes")
		created, err := filepath.Glob(filepath.Join(a.dir, "database", "migrations", "*_add_flag_to_notes.go"))
		if err != nil || len(created) != 1 {
			t.Errorf("make:migration wrote %v (%v), want one file under database/migrations", created, err)
		}
	})

	t.Run("the database commands", func(t *testing.T) {
		a.ok(t, "db")
		a.ok(t, "db:show")
		if table := a.ok(t, "db:table", "users"); !strings.Contains(table, "users") {
			t.Errorf("db:table users does not describe the users table:\n%s", table)
		}
		a.ok(t, "db:monitor")
		a.ok(t, "db:seed")
		a.ok(t, "db:seed", "UserSeeder", "-e", "admin@example.test", "-p", "a-long-enough-password", "-r", "admin")
		a.ok(t, "make:seeder", "PostSeeder")
		a.ok(t, "model:prune")
	})

	t.Run("routes lists the application's routes", func(t *testing.T) {
		for _, name := range []string{"routes", "route:list"} {
			if listed := a.ok(t, name); !strings.Contains(listed, "/notes") {
				t.Errorf("%s does not list the notes routes:\n%s", name, listed)
			}
		}
	})

	t.Run("the schedule, and the command that queues the same work", func(t *testing.T) {
		listed := a.ok(t, "schedule:list")
		if !strings.Contains(listed, appjobs.SendNotesDigestName) || !strings.Contains(listed, "0 2 * * *") {
			t.Fatalf("schedule:list does not list the nightly digest:\n%s", listed)
		}
		a.ok(t, "schedule:run", appjobs.SendNotesDigestName, "--tenant="+binaryTenant)
		if queued := a.ok(t, "notes:digest"); !strings.Contains(queued, "queued") {
			t.Errorf("notes:digest printed %q", queued)
		}
		if got := a.waiting(t); got != "2" {
			t.Errorf("%s jobs are waiting after schedule:run and notes:digest, want 2", got)
		}
	})

	t.Run("queue:work drains the queue and stops on SIGTERM", func(t *testing.T) {
		drained := func() bool { return a.waiting(t) == "0" }
		if code := a.interrupted(t, drained, "queue:work", "--workers=1"); code != 0 {
			t.Fatalf("queue:work exited %d after SIGTERM", code)
		}
		// Drained by running, not by giving up: the handlers WithJobs
		// registered took both digests.
		if failed := a.ok(t, "queue:failed", "--tenant="+binaryTenant); !strings.Contains(failed, "no failed jobs") {
			t.Errorf("the worker failed the digests instead of running them:\n%s", failed)
		}
		// work is the name the CLI hands the binary for queue:work. Nothing
		// is queued, so it is ready once it has had the time to start.
		started := time.Now()
		running := func() bool { return time.Since(started) > 2*time.Second }
		if code := a.interrupted(t, running, "work"); code != 0 {
			t.Fatalf("work exited %d after SIGTERM", code)
		}
	})

	t.Run("the queue's other commands", func(t *testing.T) {
		for _, args := range [][]string{
			{"queue:clear", "--force"},
			{"queue:monitor", "database:default"},
			{"queue:failed", "--tenant=" + binaryTenant},
			{"queue:forget", "--tenant=" + binaryTenant, "no-such-job"},
			{"queue:flush", "--tenant=" + binaryTenant},
			{"queue:prune-failed", "--tenant=" + binaryTenant},
			{"queue:prune-batches", "--tenant=" + binaryTenant},
		} {
			a.ok(t, args...)
		}
		// These answer, and refuse for what is not there: no shared store to
		// record a restart or a pause in, no failed job and no batch to retry.
		// Refused is fine; unknown is the regression.
		for _, args := range [][]string{
			{"queue:restart"},
			{"queue:pause", "database:default"},
			{"queue:resume", "database:default"},
			{"queue:retry", "--tenant=" + binaryTenant, "no-such-job"},
			{"queue:retry-batch", "--tenant=" + binaryTenant, "no-such-batch"},
		} {
			if _, errOut, _ := a.run(t, args...); strings.Contains(errOut, "unknown command") {
				t.Errorf("%s is not answered: %s", args[0], errOut)
			}
		}
	})

	t.Run("vendor:publish and the cache commands", func(t *testing.T) {
		a.ok(t, "vendor:publish")
		a.ok(t, "cache:clear", "--tenant="+binaryTenant)
		a.ok(t, "cache:forget", "--tenant="+binaryTenant, "a-key")
		a.ok(t, "cache:prune-stale-tags")
	})

	t.Run("no arguments serves until SIGTERM", func(t *testing.T) {
		address := freeAddress(t)
		a := a
		a.env = append(append([]string{}, a.env...), "HTTP_ADDR="+address)
		healthy := func() bool {
			res, err := http.Get(fmt.Sprintf("http://%s/_arandu/health", address))
			if err != nil {
				return false
			}
			_ = res.Body.Close()
			return res.StatusCode == http.StatusOK
		}
		if code := a.interrupted(t, healthy); code != 0 {
			t.Fatalf("serve exited %d after SIGTERM", code)
		}
		if code := a.interrupted(t, healthy, "serve"); code != 0 {
			t.Fatalf("serve exited %d after SIGTERM", code)
		}
	})

	t.Run("db:wipe empties the development database", func(t *testing.T) {
		a.ok(t, "db:wipe")
		if status, _, _ := a.run(t, "migrate:status"); strings.Contains(status, "Ran") {
			t.Errorf("migrate:status still reports ran migrations after db:wipe:\n%s", status)
		}
	})
}

// freeAddress answers a loopback address nothing listens on.
func freeAddress(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("finding a free port: %v", err)
	}
	address := l.Addr().String()
	_ = l.Close()
	return address
}
