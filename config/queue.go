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
type Queue struct {
	Connection QueueConnection

	// Default is the queue a job goes to when it names none.
	Default string

	// Workers is how many jobs one `aru work` process runs at once.
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
	workers, err := envInt("QUEUE_WORKERS", 4)
	if err != nil {
		return Queue{}, err
	}
	retryAfter, err := envSeconds("QUEUE_RETRY_AFTER", 90*time.Second)
	if err != nil {
		return Queue{}, err
	}
	maxAttempts, err := envInt("QUEUE_MAX_ATTEMPTS", 5)
	if err != nil {
		return Queue{}, err
	}
	return Queue{
		Connection:  connection,
		Default:     env("QUEUE_DEFAULT", "default"),
		Workers:     workers,
		RetryAfter:  retryAfter,
		MaxAttempts: maxAttempts,
	}, nil
}
