package bootstrap

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/arandu-io/framework/kernel"
	"github.com/arandu-io/framework/scheduler"
	"github.com/arandu-io/hesape/queue"

	appconfig "github.com/arandu-io/arandu/config"
)

// The background commands: what runs outside a request.
//
// All three are this same binary with another argument, which is what keeps the
// deploy at one artifact (doc 17). There is no second image to build, no second
// thing to monitor, and no way for the worker to be running a different version
// of the code than the server.

// scheduleList prints the registered tasks with their next run.
func scheduleList(sched *scheduler.Module) error {
	s := sched.Scheduler()
	if s == nil {
		fmt.Println("no scheduled tasks.")
		fmt.Println("A module declares them with Schedule() []kernel.Task -- see doc 16.")
		return nil
	}

	tasks := s.List()
	if len(tasks) == 0 {
		fmt.Println("no scheduled tasks.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "id\tschedule\tscope\tsingleton\tnext run\tlast run")
	for _, t := range tasks {
		next := "never"
		if !t.Next.IsZero() {
			next = t.Next.Format(time.RFC3339)
		}
		last := "-"
		if !t.LastRun.IsZero() {
			last = t.LastRun.Format(time.RFC3339)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%t\t%s\t%s\n", t.ID, t.Spec, t.Scope, t.Singleton, next, last)
	}
	if err := w.Flush(); err != nil {
		return err
	}

	// A task that failed is the reason somebody ran this command.
	for _, t := range tasks {
		if t.LastError != "" {
			fmt.Printf("\n%s failed on its last run: %s\n", t.ID, t.LastError)
		}
	}
	return nil
}

// scheduleRun runs one task now, on the same path the scheduler uses.
//
// Same lock, same Grant, same instrumentation. A manual run that took a
// different route would be a back door, and the two would drift.
func scheduleRun(ctx context.Context, sched *scheduler.Module, args []string) error {
	flags := flag.NewFlagSet("schedule:run", flag.ContinueOnError)
	tenant := flags.String("tenant", "", "which tenant, for a task whose scope is per tenant")

	var id string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		id, args = args[0], args[1:]
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	if id == "" {
		return fmt.Errorf("usage: aru schedule:run <id> [--tenant=<id>]\n`aru schedule:list` shows the registered tasks")
	}

	s := sched.Scheduler()
	if s == nil {
		return fmt.Errorf("this application has no scheduled tasks")
	}
	return s.RunNow(ctx, id, *tenant)
}

// WorkerOptions builds the options for the internal `work` subcommand that
// `aru queue:work` delegates to: the four queue settings, and the flags that
// override them for one invocation.
//
// It is exported for the reason Open is. What has to be checkable is whether a
// number written in the environment is the number the worker is given, and the
// worker itself does not return until it is interrupted -- so the only way to
// ask is to read the settings it was built from.
//
// The flags default to what the configuration states rather than to numbers of
// their own. A flag carrying its own default would be a second answer to a
// question that already has one, reachable by leaving the flag out -- and the
// four settings were read, validated and then dropped for exactly as long as
// this function did not exist.
//
// Nothing is defaulted here. An unset variable arrives as a zero, and the queue
// component reads a zero on any of these as the value it keeps by default, so a
// number written here would be a second copy of one this package does not own.
func WorkerOptions(cfg appconfig.Queue, args []string) (queue.WorkerOptions, error) {
	flags := flag.NewFlagSet("work", flag.ContinueOnError)
	name := flags.String("queue", cfg.Default, "which queue to drain")
	workers := flags.Int("workers", cfg.Workers, "how many jobs to run at once")
	if err := flags.Parse(args); err != nil {
		return queue.WorkerOptions{}, err
	}

	return queue.WorkerOptions{
		Queue:       *name,
		Concurrency: *workers,
		// The lease and the attempt count have no flag, because neither is a
		// property of one invocation: a lease shorter than the longest handler
		// hands running work to a second worker, and that is true of every
		// worker draining the queue rather than of the one somebody just
		// started.
		Lease:    cfg.RetryAfter,
		MaxTries: cfg.MaxAttempts,
	}, nil
}

// work drains a job queue until interrupted.
func work(ctx context.Context, k *kernel.Kernel, store queue.Queue, cfg appconfig.Queue, args []string) error {
	opts, err := WorkerOptions(cfg, args)
	if err != nil {
		return err
	}

	// Boot, because a handler reaches the same services a request does and they
	// are wired at boot. A worker that skipped it would be a second, subtly
	// different application.
	if err := k.Boot(ctx); err != nil {
		return err
	}
	defer func() { _ = k.Shutdown() }()

	// A finished job lands on /_arandu/debug with its queries and its timeline,
	// exactly like a request -- and only when something is recording. In
	// production without a tracing secret this is nil, so the worker builds no
	// Collector at all.
	//
	// It is set here rather than in WorkerOptions because it is not a setting:
	// it comes from the assembled application, and what that function answers
	// has to be answerable from the configuration alone.
	opts.Recorder = k.Recorder()

	w := queue.NewWorker(store, opts)
	registerHandlers(w)

	if len(w.Names()) == 0 {
		return fmt.Errorf("no job handlers are registered.\n" +
			"Register them in registerHandlers, in background.go")
	}

	// The worker owns the signal, because it is the process. Draining what is
	// in flight before exiting is what stops a deploy from leaving half-done
	// work behind a lease nobody will release.
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	// The loop answers with an exit status as well as an error, and the status
	// is dropped here because a command dispatched from main.go has nowhere to
	// put one. Nothing is lost by it: the status is anything other than zero
	// only when a memory limit stops the worker, and this one sets none, so the
	// interrupt above is the only way the loop ends.
	_, err = w.Daemon(ctx)
	return err
}

// registerHandlers is where a module's job handlers are wired.
//
// Explicit, like the module registration in bootstrap.Build: read it top to
// bottom and you know every kind of work this application does in the
// background.
func registerHandlers(w *queue.Worker) {
	// arandu:begin custom
	// w.HandleFunc("invoice.send", invoiceModule.SendInvoice)
	// arandu:end custom
	_ = w
}
