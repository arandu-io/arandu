---
name: arandu-async
description: Work that does not happen inside the request in an Arandu (Go) application -- background jobs and the worker, scheduled tasks, domain events through the outbox, listeners, notifications, mail and console commands. Use when the request is to "send an email", "notify the user", "run this every night", "do this in the background", "react when X happens", "retry this", "add a command", or when a queue, the scheduler, the outbox, a listener or a notification is involved. Covers aru make:job, make:event, make:listener, make:notification, make:mail and make:command, and aru schedule:list, schedule:run and queue:work.
license: MIT
---

# Background work, events and notifications

## When to use

Anything that runs after the response, on a schedule, or because something
happened: a job, a scheduled task, an event and its listeners, a notification,
a mail, a console command. The service that a job or a listener calls is
`arandu-module`; an external system it reaches is `arandu-integrations`.

## Before you start

- Read the example's chain: `NoteService.Publish` stores `note.published`;
  `app/Listeners/NotifyNoteAuthor.go` reacts and sends
  `app/Notifications/NotePublished.go`; `Schedule` in `routes/console.go`
  enqueues `app/Jobs/SendNotesDigest.go` every night, `notes:digest` in
  `app/Console/Commands` enqueues it now, and the worker runs it.
- Know where each runs. The relay and the scheduler run inside `aru serve`;
  jobs run in `aru queue:work`, the same binary with another argument. Both are
  the framework's: `main.go` hands the command line to its console kernel.

## Contracts and imports

| piece | contract |
| --- | --- |
| job | `app/Jobs/<Name>.go`: a payload struct of facts and ids; `Dispatch<Name>(ctx, q queue.Queue, g auth.Grant, in)`, which `jobs.New` builds with the Grant's tenant and action; a handler implementing `queue.Handler` -- `Handle(ctx, g auth.Grant, j *jobs.Job) error` -- with its services as fields |
| worker | `RegisterJobs(w *queue.Worker) error` on `app/Providers/AppServiceProvider.go`, handed to `WithJobs` in `bootstrap/app.go`: `w.Handle(appjobs.<Name>Name, appjobs.New<Name>Handler(p.Notes))`. It is proved once at the boot, so a name given two handlers stops every command |
| scheduled task | a `foundation.Task` (`github.com/arandu-io/hesape/foundation`) returned by `Schedule` in `routes/console.go`, over the `ConsoleDeps` the provider answers: `ID`, five-field `Spec`, `Scope` (`Global` or `PerTenant`), `Singleton`, `Timeout`, `Action`, and `Run(ctx, g auth.Grant) error`; the Grant is built from `Action` and the tenant |
| event | `app/Events/<Name>.go`: a past-tense name in the domain's words (`note.published`), a payload of facts, and `Event(aggregateID) events.Event` |
| storing it | `outbox.Store(ctx, g, []events.Event{...})` inside `database.Transaction`, beside the write; the outbox is `events.NewOutbox(db)` from `github.com/arandu-io/framework/events` |
| listener | `app/Listeners/<Name>.go`: an `events.Publisher` -- `Publish(ctx, e events.Stored) error` -- that returns early for other names and decodes the payload into a struct of its own |
| notification | `app/Notifications/<Name>.go`: `Key()`, `Via(to) []ChannelName` and one method per channel (`ToDatabase`), sent with `notifier.Send(ctx, g, to, n)` where `g` is a Grant for `notifications.ActionSend` |
| command | a function in `app/Console/Commands` answering a `console.Command` (`github.com/arandu-io/hesape/console`) -- `Name`, `Description`, `Run(ctx, o *console.IO) error`, flags from `o.Flags()` over `o.Args()` -- listed in `Commands` in `routes/console.go`. A name the framework answers (`migrate`, `queue:work`, ...) stops the boot naming both |

## Procedure

1. **A fact that happened is an event.** `aru make:event NotePublished
   --aggregate=note --event-name=note.published --fields "title:string"`. The
   service stores it in the transaction of the write it describes, so an event
   is never stored for a write that rolled back, and a write is never committed
   without its event.
2. **A reaction to it is a listener.** `aru make:listener NotifyNoteAuthor
   --event=note.published`, then add it to `listeners.Each` in
   `app/Providers/AppServiceProvider.go`. The relay calls it after the commit, at least once: what
   it does has to be safe to do twice -- an upsert keyed by `e.ID`, a call with
   an idempotency key -- and an error means "try again later", never "skip".
3. **Telling a person is a notification.** `aru make:notification NotePublished
   --channels=database` writes the notification and its test; it is sent with
   the `Notifier` the provider builds (`p.Notifier`), from the listener or the
   service that decided to tell somebody.
4. **Work worth retrying is a job.** `aru make:job SendNotesDigest
   --event-name=notes.digest --fields "since:timestamp"`. The caller that holds
   the queue dispatches it: a listener, after the write that stored its event
   has committed, or a task in `Schedule` in `routes/console.go`, as the digest
   is.
   A service never does -- the job's package imports `app/Services` once its
   handler takes a service, and a service importing it back is an import cycle.
   The handler calls a service; `j.UUID` is stable across retries and is the
   key to deduplicate on.
5. **Recurring work is a task that enqueues a job.** Declare it in
   `Schedule` in `routes/console.go`; the task decides what work exists and the job
   does it, with the retries a task does not get. A task that reads a tenant's
   rows is `PerTenant`, and runs for each tenant `WithTenants` answers --
   `AppServiceProvider.Tenants` here.
6. **An operator's action is a command.** A function in
   `app/Console/Commands` that takes what it reaches and answers a
   `console.Command`, as `NotesDigest` does, and a line in `Commands` in
   `routes/console.go`. `aru make:command` v0.74.0 still writes a type whose
   `Run` takes `[]string`; wrap it, or write the function by hand.

## Commands

- `aru make:job <Name> --event-name=<name> --fields "..." [--services=<Entity>,...]`
- `aru make:event <Name> --aggregate=<entity> --event-name=<entity.verb> --fields "..."`
- `aru make:listener <Name> --event=<entity.verb>`
- `aru make:notification <Name> --channels=database`
- `aru make:mail <Name> --subject "..." --fields "..."`, `aru make:command <Name> --signature=notes:prune --description="..."`
- `aru schedule:list`, `aru schedule:run <id> --tenant=<id>`
- `aru queue:work`, `aru queue:failed --tenant=<id>`, `aru queue:retry --tenant=<id>`

## Example

A listener that reacts to the example's event, and a task that enqueues the
example's job every Monday:

```go compile
package example

import (
	"context"
	"time"

	"github.com/arandu-io/hesape/auth"
	"github.com/arandu-io/hesape/events"
	hfoundation "github.com/arandu-io/hesape/foundation"
	"github.com/arandu-io/hesape/log"
	"github.com/arandu-io/hesape/queue"

	appevents "<module>/app/Events"
	appjobs "<module>/app/Jobs"
	policies "<module>/app/Policies"
)

// LogPublication writes a line for every published note, after the write
// committed. A repeat writes the line again, which is safe.
type LogPublication struct{}

var _ events.Publisher = LogPublication{}

// Publish answers note.published and nothing else.
func (LogPublication) Publish(ctx context.Context, e events.Stored) error {
	if e.Name != appevents.NotePublishedName {
		return nil
	}
	var published struct {
		Title string `json:"title"`
	}
	if err := e.Decode(&published); err != nil {
		return err
	}
	log.For(ctx).Info("note published", "note", e.AggregateID, "title", published.Title)
	return nil
}

// WeeklyDigest decides that a digest is due, per tenant, and enqueues it.
func WeeklyDigest(q queue.Queue) hfoundation.Task {
	return hfoundation.Task{
		ID:        "notes.weekly-digest",
		Spec:      "0 8 * * 1",
		Scope:     hfoundation.PerTenant,
		Singleton: true,
		Timeout:   time.Minute,
		Action:    policies.NoteList,
		Run: func(ctx context.Context, g auth.Grant) error {
			return appjobs.DispatchSendNotesDigest(ctx, q, g, appjobs.SendNotesDigest{
				Since: time.Now().AddDate(0, 0, -7),
			})
		},
	}
}
```

## Do not

- Start a goroutine in a constructor, a service or `Boot` to do work later: it
  dies with the process and nobody retries it. A job does.
- Call a listener, publish an event or send a notification from inside the
  transaction that produced it: the reader sees a write that may still roll
  back. Store the event; the relay publishes after the commit.
- Dispatch a job from a service. The handler calls services, so the job's
  package imports `app/Services`, and the import back is a cycle the compiler
  refuses. Store an event; its listener dispatches the job.
- Share the producer's payload type with a listener. The listener decodes into
  a struct of its own, so the two deploy apart.
- Return nil from a listener or a handler that failed: the outbox and the queue
  keep the work only when told it was not done.
- Use `auth.SystemGrant` in a listener without its reason on the line before:
  `//arandu:system-grant <reason>`. The example's listener rebuilds the Grant
  from the tenant sealed into the outbox row, for sending a notification and
  nothing else.

## Extending it

The handler's collaborators, the listener's work and a notification's fields
and payload go in their custom blocks. A second listener for the same event is
a second type in `listeners.Each`, never a switch in the first. A task is one
more entry in `Schedule`, a command one more in `Commands`.

## Wiring

All in two files, by hand:

- `app/Providers/AppServiceProvider.go`: the listener in `listeners.Each`,
  built with what it needs (the example's takes `p.Notifier`); the notifier
  itself, with its channels; `Tenants`, the list a per-tenant task expands to;
  the handler in `RegisterJobs`, built from the services the provider holds;
  and what a task or a command reaches, in `ConsoleDeps` through `Console`.
  `aru make:job --services=<Entity>` writes the constructor with those
  services; v0.74.0 prints the line for `registerHandlers` in
  `bootstrap/background.go`, which is `RegisterJobs` here, with `p.` where it
  says `app.`.
- `routes/console.go`: the task in `Schedule` and the command in `Commands`.

`bootstrap/app.go` already hands all of it to the framework --
`WithCommands`, `WithSchedule`, `WithTenants`, `WithJobs` -- and does not
change for a new job, task or command.

## Acceptance test

- An event: the write and the outbox row in one transaction, and a refused
  write storing none (`TestTheAuthorPublishesANoteOnceWithItsEvent`).
- A listener: drive the application's own relay with `app.Relay.Drain(ctx)`
  and read its effect (`TestTheAuthorOfAPublishedNoteIsToldThroughTheOutbox`).
- A notification: its generated test, and its payload.
- A task and its job: the schedule `schedule:list` prints, `schedule:run` on
  the path the scheduler takes, the job on the queue, and a worker running it
  with fakes (`TestTheNightlyDigestIsScheduledQueuedAndSentWithoutTheNetwork`).
- A command: run through the built binary, as
  `TestEveryCommandAnswersThroughTheBinary` runs `notes:digest`.

## Limits

There is no mail channel for notifications: the notifier needs a
`channels.Mailer`, and none is built over the application's `mail.Mailer`, so a
notification that names `mail` is refused with `notifications.ErrNoChannel`.
Mail itself works through the drivers' `Mail` (`drivers.Mail` in a provider)
and `aru make:mail`. Delivery is
at-least-once everywhere and nothing checks that an effect is safe to repeat;
`aru doctor` reads the code, it does not run it. A `PerTenant` task runs only
for the tenants the scheduler's `Tenants` returns -- this project returns the
one tenant every login belongs to.

## Gates

Run them all, in this order, as `AGENTS.md` lists them:

```sh
export GOWORK=off
aru model:build --check
aru view:build
gofmt -l $(find . -name '*.go' -not -path '*/testdata/*' -not -name '*.kyse.go')
go vet ./...
bash tests/test-layout-guard.sh
go test -race ./...
go build ./...
aru doctor
```

<!-- arandu:begin custom -->
<!-- arandu:end custom -->
