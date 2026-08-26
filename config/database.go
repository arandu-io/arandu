package config

import (
	"time"

	"github.com/arandu-io/framework/foundation/bootstrap"
	"github.com/arandu-io/hesape/database"
)

// Database is the connection this application opens, and the pool it keeps.
//
// The connection itself is not re-read here. The framework already parsed
// DB_CONNECTION and the rest, validated them and built the DSN, and a second
// parse in the application would be a second answer to the same question. This
// struct carries that value and adds what only the application decides: how big
// the pool is and how long a connection lives.
type Database struct {
	// Connection is the engine and its credentials, as the framework parsed
	// them. Hand it to the adapter; never build a DSN by hand.
	Connection database.Config

	// MaxOpenConns caps the connections in flight. Zero means unlimited, which
	// is how a burst of traffic turns into "too many connections" on the server
	// and takes every other application with it.
	MaxOpenConns int

	// MaxIdleConns is how many stay open between queries.
	MaxIdleConns int

	// ConnMaxLifetime retires a connection after this long. A managed database
	// that rotates behind a proxy hands out connections that stop working, and
	// a lifetime is what makes the pool notice without a failed request.
	ConnMaxLifetime time.Duration
}

func loadDatabase(base bootstrap.Configuration) (Database, error) {
	maxOpenConns, err := envInt("DB_MAX_OPEN_CONNS", 25)
	if err != nil {
		return Database{}, err
	}
	maxIdleConns, err := envInt("DB_MAX_IDLE_CONNS", 5)
	if err != nil {
		return Database{}, err
	}
	connMaxLifetime, err := envSeconds("DB_CONN_MAX_LIFETIME", time.Hour)
	if err != nil {
		return Database{}, err
	}
	return Database{
		// Converted, never re-parsed: the two structs are the same fields under
		// two names, and DATABASE_URL is read once, where everything else is. The
		// conversion goes when the adapter this value is handed to takes the
		// other name.
		Connection:      database.Config(base.Database),
		MaxOpenConns:    maxOpenConns,
		MaxIdleConns:    maxIdleConns,
		ConnMaxLifetime: connMaxLifetime,
	}, nil
}
