package config

import (
	"time"

	"github.com/arandu-io/framework/foundation/bootstrap"
	"github.com/arandu-io/hesape/database"
)

// Database is the connection this application opens, and the pool it keeps.
//
// The connection itself is not re-read here. The framework already parsed
// DATABASE_URL -- one variable, which is the whole of where the database is --
// validated it and built the DSN, and a second parse in the application would be
// a second answer to the same question. This struct carries that value and
// completes it with what only the application decides: how big the pool is and
// how long a connection lives.
//
// Those three are fields of the connection rather than of this struct. They were
// both for a while, and only one of the two copies was ever handed to the
// adapter -- so DB_MAX_OPEN_CONNS, DB_MAX_IDLE_CONNS and DB_CONN_MAX_LIFETIME
// were read, validated and dropped, and every pool ran on the defaults. One home
// for a value is what keeps that from being possible again.
type Database struct {
	// Connection is the engine, its credentials and its pool: what the framework
	// parsed, completed by loadDatabase. Hand it to the adapter; never build a
	// DSN by hand.
	Connection database.Config
}

// loadDatabase completes the parsed connection with the pool this application
// asks for.
//
// What each of the three does is documented on the fields they are written to,
// and two of them are worth repeating here because they are what somebody
// setting the variables expects to be otherwise:
//
//   - there is no way to ask for an unbounded pool. Zero on any of the three
//     means the adapter's own default, never database/sql's meaning for zero,
//     and the defaults below are that same number written where .env.example can
//     point at it.
//   - SQLite gets a single writer whatever DB_MAX_OPEN_CONNS says. It serializes
//     writes anyway, so a larger pool would turn the wait into "database is
//     locked" rather than into throughput.
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

	// Completed, never re-parsed: DATABASE_URL is read once, where everything
	// else is, and the three fields written here are the ones ParseURL never
	// touches -- how many connections to hold is not part of where the database
	// is.
	connection := base.Database
	connection.MaxOpenConns = maxOpenConns
	connection.MaxIdleConns = maxIdleConns
	connection.ConnMaxLifetime = connMaxLifetime

	return Database{Connection: connection}, nil
}
