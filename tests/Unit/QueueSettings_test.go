package unit_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/arandu-io/arandu/bootstrap"
	appconfig "github.com/arandu-io/arandu/config"
)

// The four queue variables, from .env to the worker that runs the jobs.
//
// They were read and validated once and then dropped: `aru work` built its
// worker from a queue name and a job count written into the flag declarations,
// so no setting of QUEUE_WORKERS changed anything, QUEUE_RETRY_AFTER left every
// lease at the component's own five minutes while the file beside it said
// ninety seconds, and QUEUE_MAX_ATTEMPTS parked a job after a number nobody
// chose. Nothing failed, which is why it lasted -- a lease that is the wrong
// length is a lease that works until a job runs longer than it.
//
// The assertions are made on the settings the worker is built with, because
// that is the far end reachable without a job queue and a clock: the worker
// itself does not return until it is interrupted, and every number below is one
// it was handed.

// queueEnv is a configuration that loads, with the four variables cleared so a
// value exported in a shell cannot answer for one a case did not set.
func queueEnv(t *testing.T) {
	t.Helper()

	t.Setenv("APP_ENV", "dev")
	t.Setenv("APP_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("DATABASE_URL", "sqlite://"+filepath.Join(t.TempDir(), "test.sqlite"))

	for _, key := range []string{
		"QUEUE_DEFAULT", "QUEUE_WORKERS", "QUEUE_RETRY_AFTER", "QUEUE_MAX_ATTEMPTS",
	} {
		t.Setenv(key, "")
	}
}

// TestTheQueueSettingsReachTheWorker is the whole path: four variables, the
// configuration this application parsed them into, and the options `aru work`
// builds its worker from.
func TestTheQueueSettingsReachTheWorker(t *testing.T) {
	queueEnv(t)

	// Four values nothing would choose on its own, so a setting that failed to
	// travel arrives as something else and is told apart from one that did. The
	// defaults they have to differ from are the queue component's and are not
	// written here: a test that restated them would be one more copy to update.
	t.Setenv("QUEUE_DEFAULT", "invoices")
	t.Setenv("QUEUE_WORKERS", "7")
	t.Setenv("QUEUE_RETRY_AFTER", "23")
	t.Setenv("QUEUE_MAX_ATTEMPTS", "3")

	cfg, err := appconfig.Load()
	if err != nil {
		t.Fatalf("loading the configuration: %v", err)
	}

	opts, err := bootstrap.WorkerOptions(cfg.Queue, nil)
	if err != nil {
		t.Fatalf("building the worker options: %v", err)
	}

	if opts.Queue != "invoices" {
		t.Errorf("the worker drains %q, want invoices: QUEUE_DEFAULT was read and dropped", opts.Queue)
	}
	if opts.Concurrency != 7 {
		t.Errorf("the worker runs %d jobs at once, want 7: QUEUE_WORKERS was read and dropped", opts.Concurrency)
	}
	if opts.Lease != 23*time.Second {
		t.Errorf("the worker leases a job for %s, want 23s: QUEUE_RETRY_AFTER was read and dropped", opts.Lease)
	}
	if opts.MaxTries != 3 {
		t.Errorf("the worker retries a job %d times, want 3: QUEUE_MAX_ATTEMPTS was read and dropped", opts.MaxTries)
	}
}

// TestTheWorkerFlagsOverrideTheConfiguredSettings is the other half.
//
// The two settings that have a flag are the two that belong to one invocation:
// draining a backlog on another queue, and widening a single process. The flag
// wins, and it wins over a value that arrived -- which is what tells an override
// apart from a flag that was carrying its own default all along.
func TestTheWorkerFlagsOverrideTheConfiguredSettings(t *testing.T) {
	queueEnv(t)
	t.Setenv("QUEUE_DEFAULT", "invoices")
	t.Setenv("QUEUE_WORKERS", "7")
	t.Setenv("QUEUE_RETRY_AFTER", "23")

	cfg, err := appconfig.Load()
	if err != nil {
		t.Fatalf("loading the configuration: %v", err)
	}

	opts, err := bootstrap.WorkerOptions(cfg.Queue, []string{"--queue=reports", "--workers=2"})
	if err != nil {
		t.Fatalf("building the worker options: %v", err)
	}

	if opts.Queue != "reports" {
		t.Errorf("the worker drains %q, want reports: --queue did not override QUEUE_DEFAULT", opts.Queue)
	}
	if opts.Concurrency != 2 {
		t.Errorf("the worker runs %d jobs at once, want 2: --workers did not override QUEUE_WORKERS", opts.Concurrency)
	}
	// The lease has no flag, so it is still the configured one. Asserted here
	// because a flag set that quietly grew one would be the second answer this
	// arrangement exists to prevent.
	if opts.Lease != 23*time.Second {
		t.Errorf("the worker leases a job for %s, want 23s: the flags changed a setting they do not own", opts.Lease)
	}
}

// TestAnUnsetQueueSettingLeavesTheComponentsOwnAnswer.
//
// Leaving a variable out is how the queue component is asked for its own
// default, and it answers a zero as exactly that. This application writes no
// number of its own for those, so what an empty environment produces is a set of
// options carrying nothing -- and a default appearing here later would be a
// second copy of one this package does not own.
func TestAnUnsetQueueSettingLeavesTheComponentsOwnAnswer(t *testing.T) {
	queueEnv(t)

	cfg, err := appconfig.Load()
	if err != nil {
		t.Fatalf("loading the configuration: %v", err)
	}

	opts, err := bootstrap.WorkerOptions(cfg.Queue, nil)
	if err != nil {
		t.Fatalf("building the worker options: %v", err)
	}

	if opts.Queue != cfg.Queue.Default {
		t.Errorf("the worker drains %q and the configuration says %q", opts.Queue, cfg.Queue.Default)
	}
	if opts.Concurrency != cfg.Queue.Workers {
		t.Errorf("the worker runs %d jobs at once and the configuration says %d", opts.Concurrency, cfg.Queue.Workers)
	}
	if opts.Lease != cfg.Queue.RetryAfter {
		t.Errorf("the worker leases a job for %s and the configuration says %s", opts.Lease, cfg.Queue.RetryAfter)
	}
	if opts.MaxTries != cfg.Queue.MaxAttempts {
		t.Errorf("the worker retries a job %d times and the configuration says %d", opts.MaxTries, cfg.Queue.MaxAttempts)
	}
}
