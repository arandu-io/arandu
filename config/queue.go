package config

import "time"

// QueueConnection is where queued work is stored.
type QueueConnection string

// The supported connections. Both implement the same jobs.Queue contract, so
// moving between them changes this value and nothing else (RULE 11).
const (
	// QueueDatabase stores jobs in a table of the application's own database,
	// which is what makes a job commitable by the same transaction as the row it
	// is about.
	QueueDatabase QueueConnection = "database"
	// QueueKV stores them over RESP, for volume beyond a table.
	QueueKV QueueConnection = "kv"
)

// Queue is what config/queue.php holds.
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

func loadQueue() Queue {
	return Queue{
		Connection:  QueueConnection(env("QUEUE_CONNECTION", string(QueueDatabase))),
		Default:     env("QUEUE_DEFAULT", "default"),
		Workers:     envInt("QUEUE_WORKERS", 4),
		RetryAfter:  envSeconds("QUEUE_RETRY_AFTER", 90*time.Second),
		MaxAttempts: envInt("QUEUE_MAX_ATTEMPTS", 5),
	}
}
