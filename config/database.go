package config

import (
	"github.com/arandu-io/framework/foundation/bootstrap"
	"github.com/arandu-io/hesape/database"
)

// Database is the connection this application opens, and the pool it keeps.
//
// Nothing here is read from the environment. The framework parses the whole of
// it -- DATABASE_URL for where the database is, and the three pool settings for
// how many connections to hold and how long one lives -- validates it and builds
// the DSN. This struct is a view of that value under this package's names, the
// way App is a view of what the framework parsed for the application itself.
//
// It carried its own readers for the three pool settings until the framework
// grew them, and for a while it was the only reader there was. Two readers of
// one variable is two answers the day one of them grows a rule the other has
// not, so when the second appeared the first went rather than both staying.
type Database struct {
	// Connection is the engine, its credentials and its pool, as the framework
	// parsed them. Hand it to the adapter; never build a DSN by hand.
	//
	// The pool fields on it read zero as the adapter's own default, which is
	// what an unset variable leaves behind. There is no way to ask for an
	// unbounded pool, and a variable that is present and cannot be used --
	// unparseable, zero or negative -- refuses the boot naming itself.
	Connection database.Config
}

// loadDatabase presents what the framework already parsed.
//
// It returns no error because it reads nothing that could be wrong: every
// refusal for this domain happens where the parsing does, at
// bootstrap.LoadConfiguration, before this package is reached.
func loadDatabase(base bootstrap.Configuration) Database {
	return Database{Connection: base.Database}
}
