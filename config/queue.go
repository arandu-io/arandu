package config

import (
	"fmt"
	"time"
)

// QueueConnection is where queued work is stored.
type QueueConnection string

// The supported connections. Both implement the same queue.Queue contract, so
// moving between them changes this value and nothing else.
const (
	// QueueDatabase stores jobs in a table of the application's own database,
	// which is what makes a job commitable by the same transaction as the row it
	// is about.
	QueueDatabase QueueConnection = "database"
	// QueueRedis stores them over RESP, for volume beyond a table.
	QueueRedis QueueConnection = "redis"
)

// Queue is where queued work is stored, and how a worker runs it.
//
// The four settings below are handed to the worker whole, and none of them is
// given a default here. An unset variable leaves a zero, and the queue component
// reads a zero on each of them as the value it keeps by default -- so leaving
// one out is how that component is asked for its own answer, and a number
// written here as well would be a second place to change one. The two would
// disagree the day only one was edited, and the lease is where that costs
// something: a number shorter than the longest handler hands running work to a
// second worker.
//
// A value that is present and cannot be used stops the boot naming itself.
// Zero and a negative are refused with the unparseable ones: there is no worker
// count of none to ask for, and leaving the variable out is how the default is
// asked for.
type Queue struct {
	Connection QueueConnection

	// Default is the queue a job goes to when it names none, and the queue
	// `aru work` drains when it is not told otherwise. One name for both, so a
	// deployment cannot dispatch into a queue no worker is reading.
	Default string

	// Workers is how many jobs one `aru work` process runs at once. The
	// --workers flag overrides it for one invocation.
	Workers int

	// RetryAfter is how long a lease lasts. A job whose worker died is picked up
	// again after it, so the value has to exceed the longest job or the same
	// work runs twice.
	RetryAfter time.Duration

	// MaxAttempts is how many times a failing job is retried before it is
	// parked. Parked, not dropped: work that vanished is work nobody can
	// reconstruct.
	MaxAttempts int
}

func loadQueue() (Queue, error) {
	connection := QueueConnection(env("QUEUE_CONNECTION", string(QueueDatabase)))
	switch connection {
	case QueueDatabase:
	case QueueRedis:
		raw := env("REDIS_URL", "")
		if raw == "" {
			return Queue{}, fmt.Errorf("QUEUE_CONNECTION %q requires REDIS_URL", connection)
		}
		if err := validateRedisURL(raw); err != nil {
			return Queue{}, err
		}
	default:
		return Queue{}, fmt.Errorf("QUEUE_CONNECTION has unsupported value %q; expected database or redis", connection)
	}
	workers, err := envCount("QUEUE_WORKERS")
	if err != nil {
		return Queue{}, err
	}
	// Zero here is the component's own lease, which is why the fallback is not a
	// duration: envSeconds refuses a value that is present and not positive, so
	// the only way to reach this zero is to leave the variable out.
	retryAfter, err := envSeconds("QUEUE_RETRY_AFTER", 0)
	if err != nil {
		return Queue{}, err
	}
	maxAttempts, err := envCount("QUEUE_MAX_ATTEMPTS")
	if err != nil {
		return Queue{}, err
	}
	return Queue{
		Connection:  connection,
		Default:     env("QUEUE_DEFAULT", ""),
		Workers:     workers,
		RetryAfter:  retryAfter,
		MaxAttempts: maxAttempts,
	}, nil
}
