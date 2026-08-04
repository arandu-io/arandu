package main

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

	"github.com/arandu-io/framework/jobs"
	"github.com/arandu-io/framework/kernel"
	"github.com/arandu-io/framework/scheduler"
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

// work drains a job queue until interrupted.
func work(ctx context.Context, k *kernel.Kernel, store jobs.Queue, args []string) error {
	flags := flag.NewFlagSet("work", flag.ContinueOnError)
	queue := flags.String("queue", jobs.DefaultQueue, "which queue to drain")
	workers := flags.Int("workers", 4, "how many jobs to run at once")
	if err := flags.Parse(args); err != nil {
		return err
	}

	// Boot, because a handler reaches the same services a request does and they
	// are wired at boot. A worker that skipped it would be a second, subtly
	// different application.
	if err := k.Boot(ctx); err != nil {
		return err
	}
	defer func() { _ = k.Shutdown() }()

	w := jobs.NewWorker(store, jobs.WorkerOptions{
		Queue:       *queue,
		Concurrency: *workers,
	})
	registerHandlers(w)

	if len(w.Names()) == 0 {
		return fmt.Errorf("no job handlers are registered.\n" +
			"Register them in registerHandlers, in cmd/app/background.go")
	}

	// The worker owns the signal, because it is the process. Draining what is
	// in flight before exiting is what stops a deploy from leaving half-done
	// work behind a lease nobody will release.
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	return w.Run(ctx)
}

// registerHandlers is where a module's job handlers are wired.
//
// Explicit, like the module registration in build(): read it top to bottom and
// you know every kind of work this application does in the background.
func registerHandlers(w *jobs.Worker) {
	// arandu:begin custom
	// w.HandleFunc("invoice.send", invoiceModule.SendInvoice)
	// arandu:end custom
	_ = w
}
