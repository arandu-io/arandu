package feature_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/jobs"
	"github.com/arandu-io/framework/kernel"
	"github.com/arandu-io/framework/scheduler"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/queue"

	"github.com/arandu-io/arandu/bootstrap"
)

// The shape doc 16 describes, end to end and against a real database: the
// scheduler fires, the task enqueues a job, the queue persists it, the worker
// runs it. "Scheduler dispatches, queue persists" is one sentence and four
// moving parts, and this is the test that they fit together.

func TestTheSchedulerEnqueuesAndTheWorkerRuns(t *testing.T) {
	db := migratedDB(t)
	store := queue.New(db)
	ctx := context.Background()

	// A task that does what a scheduled task should: decide what work exists,
	// and enqueue it. The work itself belongs to a handler, where it gets the
	// retry budget the scheduler deliberately does not offer.
	task := kernel.Task{
		ID:      "billing.close",
		Spec:    "0 3 * * *",
		Scope:   kernel.PerTenant,
		Action:  "invoice.send",
		Timeout: 10 * time.Second,
		Run: func(ctx context.Context, g security.Grant) error {
			j, err := jobs.New(g, "", "invoice.send", map[string]string{"cycle": "2026-08"})
			if err != nil {
				return err
			}
			return store.Push(ctx, g, j)
		},
	}

	sched, err := scheduler.New([]kernel.Task{task}, scheduler.Options{
		Tenants: func(context.Context) ([]string, error) { return []string{bootstrap.Tenant()}, nil },
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Run it now rather than waiting for 3am, on the same path the loop uses.
	if err := sched.RunNow(ctx, "billing.close", bootstrap.Tenant()); err != nil {
		t.Fatalf("RunNow: %v", err)
	}

	pending, err := store.Pending(ctx, "")
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	if pending != 1 {
		t.Fatalf("%d jobs queued, want 1", pending)
	}

	// And the worker drains it, under a Grant rebuilt from the job.
	var mu sync.Mutex
	var ranFor string
	var cycle string
	done := make(chan struct{})

	w := jobs.NewWorker(store, jobs.WorkerOptions{Poll: 5 * time.Millisecond})
	w.HandleFunc("invoice.send", func(_ context.Context, g security.Grant, j jobs.Job) error {
		var payload struct {
			Cycle string `json:"cycle"`
		}
		_ = j.Decode(&payload)

		mu.Lock()
		ranFor, cycle = g.Subject().Tenant, payload.Cycle
		mu.Unlock()

		close(done)
		return nil
	})

	workerCtx, stop := context.WithCancel(ctx)
	go func() { _ = w.Run(workerCtx) }()
	defer stop()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("the worker never ran the job")
	}

	mu.Lock()
	defer mu.Unlock()
	if ranFor != bootstrap.Tenant() {
		t.Errorf("the handler ran for %q, want the tenant that scheduled it", ranFor)
	}
	if cycle != "2026-08" {
		t.Errorf("the payload did not survive the round trip: %q", cycle)
	}
}

// TestAJobIsCommittedWithTheWriteThatProducedIt is why the table-backed queue is
// the default. The work and the row it is about commit together or not at all.
func TestAJobIsCommittedWithTheWriteThatProducedIt(t *testing.T) {
	db := migratedDB(t)
	store := queue.New(db)
	ctx := context.Background()
	refused := errors.New("the rule said no")

	err := data.Transaction(ctx, db, func(ctx context.Context) error {
		g := security.SystemGrant("invoice.send", bootstrap.Tenant())
		j, err := jobs.New(g, "", "invoice.send", nil)
		if err != nil {
			return err
		}
		if err := store.Push(ctx, g, j); err != nil {
			return err
		}
		return refused
	})
	if !errors.Is(err, refused) {
		t.Fatalf("err = %v", err)
	}

	if pending, err := store.Pending(ctx, ""); err != nil || pending != 0 {
		t.Fatalf("%d jobs survived a rolled back transaction (%v)", pending, err)
	}
}

// TestTheSchedulerIsWiredIntoTheApplication: the module has to be registered and
// booted, or `aru schedule:list` reports nothing forever and nobody notices.
func TestTheSchedulerIsWiredIntoTheApplication(t *testing.T) {
	sqliteEnv(t)
	if err := bootstrap.Dispatch("migrate", nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	cfg, db, _ := openForTest(t)
	app := bootstrap.Build(cfg, db)

	if app.Scheduler == nil {
		t.Fatal("the scheduler module was not registered")
	}
	if app.Queue == nil {
		t.Fatal("the queue store was not wired")
	}

	if err := app.Kernel.Boot(context.Background()); err != nil {
		t.Fatalf("Boot: %v", err)
	}
	t.Cleanup(func() { _ = app.Kernel.Shutdown() })

	// The jobs table is part of the schema the application migrates, or
	// `aru work` fails on a table that does not exist.
	if _, err := app.Queue.Pending(context.Background(), ""); err != nil {
		t.Fatalf("the jobs table is missing from the migrations: %v", err)
	}
}
