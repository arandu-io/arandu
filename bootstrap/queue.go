package bootstrap

import (
	"os"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/kernel"
	"github.com/arandu-io/hesape/bus"
	"github.com/arandu-io/hesape/console"
	"github.com/arandu-io/hesape/queue"
	qconsole "github.com/arandu-io/hesape/queue/console"
	"github.com/arandu-io/hesape/queue/failed"
	hredis "github.com/arandu-io/hesape/redis"
)

// The queue commands are the component's, not this file's.
//
// They were listed by the CLI and answered by nothing: `aru` forwards fourteen
// queue commands to this binary, and thirteen of them reached the default
// branch and came back as unknown. What the component already exports is the
// whole set, so wiring them is naming the collaborators rather than writing the
// commands a second time -- the same move the migration commands made.
//
// None of them runs at boot, and none is reachable from the start-up path.

// queueConnection is the name this application's one queue is registered under.
//
// It is the name the component's database driver already answers to, so
// `aru queue:pause database:default` names the connection the worker drains and
// not a second spelling of it.
const queueConnection = "database"

// queueDeps are the queue's collaborators, built once.
//
// Once, because two of them own a table: the provider knows which table the
// failed jobs are in and the repository knows which one the batches are in, and
// the migrations below are asked of these same values. A second provider built
// for the migrations could be given another table name, and then `aru migrate`
// would create one table and `aru queue:failed` would read another.
type queueDeps struct {
	manager  *queue.QueueManager
	failures *failed.DatabaseFailedJobProvider
	batches  *bus.DatabaseBatchRepository
}

// newQueueDeps wires them against this application's database and cache.
func newQueueDeps(app App, db *data.DB) queueDeps {
	manager := queue.NewQueueManager().Extend(queueConnection, app.Queue)

	// The pause flag and the restart signal are written to the cache, and the
	// worker is another process reading it -- so the store has to be one both
	// can see. With the in-process cache there is none, and the manager is left
	// without one on purpose: it answers ErrNoCache, which is the honest
	// failure. An array store here would satisfy the type, record the pause
	// inside this process, and let `aru queue:pause` report success against a
	// worker that never sees it.
	if app.Cache != nil {
		manager = manager.SetCache(hredis.NewRedisStore(app.Cache))
	}

	return queueDeps{
		manager: manager,
		// The empty table name is the component's default, and it is passed
		// rather than spelled here for the reason the type exists: the name
		// belongs to the component, and a copy of it in this file is a second
		// place for it to be changed.
		failures: failed.NewDatabaseFailedJobProvider(db, ""),
		batches:  bus.NewDatabaseBatchRepository(db),
	}
}

// commands are the thirteen this application dispatches.
//
// # queue:work is not among them, and that is the single worker
//
// `aru queue:work` hands this binary the argument `work`, which the switch in
// console.go answers with the worker in background.go: handlers registered from
// registerHandlers, the signal owned so a deploy drains what is in flight, and
// --workers parsed. Adding the component's queue:work beside it would put a
// second worker in this binary -- reachable by typing the other name, with its
// own flags and its own idea of which handlers exist -- and two ways to start a
// worker is two workers the day somebody starts both.
//
// So the one that already runs stays the one that runs, and the twelve that
// could not run now can.
//
// The event dispatcher is nil in the two commands that take one, the same way
// databaseCommands passes no Events: this application dispatches no QueueBusy
// and no JobRetryRequested, and both commands degrade to doing the work without
// announcing it rather than to not existing.
func (d queueDeps) commands() []console.Command {
	return []console.Command{
		// The listener restarts a child worker after each job, and the child is
		// this binary with the argument the switch answers -- os.Args[0] and
		// not `aru`, because `aru queue:listen` is what started this process and
		// a child going back through the CLI would be a second hop that adds a
		// process and answers the same.
		qconsole.NewListenCommand(queue.NewListener(".", os.Args[0], "work"), queue.ListenerOptions{}).Command(),
		qconsole.NewRestartCommand(d.manager).Command(),
		qconsole.NewPauseCommand(d.manager).Command(),
		qconsole.NewResumeCommand(d.manager).Command(),
		qconsole.NewClearCommand(d.manager).Command(),
		qconsole.NewMonitorCommand(d.manager, nil).Command(),

		// The dead letter list and the five that are the reason it is not a
		// table nobody touches.
		qconsole.NewListFailedCommand(d.failures).Command(),
		qconsole.NewRetryCommand(d.failures, d.manager, nil).Command(),
		qconsole.NewForgetFailedCommand(d.failures).Command(),
		qconsole.NewFlushFailedCommand(d.failures).Command(),
		qconsole.NewPruneFailedJobsCommand(d.failures).Command(),

		qconsole.NewRetryBatchCommand(d.batches, d.failures, d.manager).Command(),
		qconsole.NewPruneBatchesCommand(d.batches).Command(),
	}
}

// migrations are the two tables those commands read and write.
//
// The jobs table arrives with the queue module, which asks the driver for it.
// These two arrive with nothing: the failed jobs table belongs to the provider
// and the batches table to the repository, and neither is a module this
// application registers. Without them `aru queue:failed` is wired and answers
// that there is no such table, which is a command that still cannot run.
//
// They are asked of the values in this struct rather than of a fresh pair, so
// the table a migration creates is the table a command reads. See queueDeps.
func (d queueDeps) migrations() []kernel.Migration {
	return append(d.failures.Migrations(), bus.Migrations()...)
}
