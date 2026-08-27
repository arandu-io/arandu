package feature_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"testing"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/hesape/database"

	"github.com/arandu-io/arandu/bootstrap"
	appconfig "github.com/arandu-io/arandu/config"
)

// The three pool variables, from .env to the pool database/sql actually keeps.
//
// They were read and validated once and then dropped: the connection handed to
// the adapter carried the URL and nothing else, so no setting of
// DB_MAX_OPEN_CONNS changed anything and every pool ran on whatever the adapter
// decides on its own. Nothing failed, which is why it lasted -- a pool that is
// the wrong size is a pool that works until the traffic that needed the number
// arrives.
//
// This is where that is checked from, and it stays here whichever package does
// the reading. Where the variables are parsed has already moved once; what has
// to keep being true is that a number written in .env is the number the driver
// ends up with, and no assertion below names the package that parsed it.
//
// The assertions are made on the pool rather than on the struct wherever
// database/sql will answer, because the struct is exactly what was right the
// whole time.

// The engine that is not there.
//
// MaxOpenConnections is the one of the three that database/sql reports back, and
// SQLite never reports the configured number: the adapter pins that engine to a
// single writer whatever a project asks for, so the question cannot be put to
// it. Asking a real server instead would mean a test that needs one installed,
// which is a test nobody runs before pushing.
//
// A driver that connects to nothing answers it. Opening resolves a driver name
// and builds a pool -- the sizes are applied before anything is sent, and a
// PingContext on a connection implementing no driver.Pinger is satisfied by
// getting one. So every number read below is the number database/sql was given,
// through the same Open every command in this application calls.
const poolDriverName = "arandu-pool-test"

func init() { sql.Register(poolDriverName, poolDriver{}) }

type poolDriver struct{}

func (poolDriver) Open(string) (driver.Conn, error) { return poolConn{}, nil }

type poolConn struct{}

func (poolConn) Prepare(string) (driver.Stmt, error) { return nil, io.EOF }
func (poolConn) Close() error                        { return nil }
func (poolConn) Begin() (driver.Tx, error)           { return nil, io.EOF }

// poolConnector links that driver to the one dialect this project does not link
// a connector for. A connector says which driver name it registered and never
// opens anything itself, which is the whole of what the adapter reads.
type poolConnector struct{}

func (poolConnector) Dialect() database.Dialect { return data.DialectMySQL }
func (poolConnector) DriverName() string        { return poolDriverName }

// borrowMySQL links the connector above, or skips when the project has since
// linked a real one.
//
// Registering a second driver for a dialect that already has one panics, by
// design: two drivers for one engine is an import nobody meant to add. A project
// that adds github.com/arandu-io/hesape/database/connectors/mysql is therefore
// told to point this test at another engine rather than being met with a panic
// from a test file it did not write.
func borrowMySQL(t *testing.T) {
	t.Helper()

	if name, err := database.DriverName(data.DialectMySQL); err == nil && name != poolDriverName {
		t.Skipf("mysql is linked to the %q driver here, and this test borrows that dialect: "+
			"point it at an engine this binary does not speak", name)
	}
	database.Register(poolConnector{})
}

// TestThePoolSettingsReachTheDriver is the whole path: three variables, the
// configuration the framework parsed and this application completed, the adapter
// Open hands it to, and the pool at the far end.
func TestThePoolSettingsReachTheDriver(t *testing.T) {
	borrowMySQL(t)

	t.Setenv("APP_ENV", "dev")
	t.Setenv("APP_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("ARANDU_TENANT_ID", "11111111-1111-4111-8111-111111111111")
	t.Setenv("DATABASE_URL", "mysql://arandu:arandu@127.0.0.1:3306/arandu")

	// Three numbers small enough that nothing would choose them for a pool, so a
	// value that failed to travel arrives as something else and is told apart
	// from one that did. The defaults they have to differ from are the adapter's
	// and are not written here: a test that restated them would be one more copy
	// to update, and the one nobody runs daily is the copy that goes stale.
	t.Setenv("DB_MAX_OPEN_CONNS", "7")
	t.Setenv("DB_MAX_IDLE_CONNS", "3")
	t.Setenv("DB_CONN_MAX_LIFETIME", "1")

	cfg, err := appconfig.Load()
	if err != nil {
		t.Fatalf("loading the configuration: %v", err)
	}

	// The connection is what Open passes, whole. Two of the three are invisible
	// from the pool afterwards -- database/sql reports neither the idle limit nor
	// the lifetime as a number -- so they are read here, on the one value the
	// adapter is given.
	conn := cfg.Database.Connection
	if conn.MaxOpenConns != 7 {
		t.Errorf("the connection carries MaxOpenConns %d, want 7: DB_MAX_OPEN_CONNS was read and dropped", conn.MaxOpenConns)
	}
	if conn.MaxIdleConns != 3 {
		t.Errorf("the connection carries MaxIdleConns %d, want 3: DB_MAX_IDLE_CONNS was read and dropped", conn.MaxIdleConns)
	}
	if conn.ConnMaxLifetime != time.Second {
		t.Errorf("the connection carries ConnMaxLifetime %s, want 1s: DB_CONN_MAX_LIFETIME was read and dropped", conn.ConnMaxLifetime)
	}

	db, closeDB, err := bootstrap.Open(cfg)
	if err != nil {
		t.Fatalf("opening the database: %v", err)
	}
	t.Cleanup(closeDB)

	pool := db.Unwrap()
	if got := pool.Stats().MaxOpenConnections; got != 7 {
		t.Errorf("the pool holds at most %d connections, want 7: the number reached the configuration and not the driver", got)
	}

	// And the lifetime, which database/sql reports as a count rather than as a
	// duration: a connection retired for age is one MaxLifetimeClosed. The
	// default is long enough that this never moves inside a test, which is what
	// makes it an assertion about the second that was configured rather than
	// about the pool existing.
	//
	// Polled rather than slept through once: the connection opened by Open's own
	// ping is retired either by the cleaner or by the next request for it, and
	// which of the two gets there first is not something to assert.
	deadline := time.Now().Add(10 * time.Second)
	for pool.Stats().MaxLifetimeClosed == 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		if err := pool.PingContext(context.Background()); err != nil {
			t.Fatalf("ping: %v", err)
		}
	}
	if pool.Stats().MaxLifetimeClosed == 0 {
		t.Error("no connection was retired for age in ten seconds, with a lifetime of one second: " +
			"DB_CONN_MAX_LIFETIME did not reach the pool")
	}
}

// TestSQLiteTakesOneWriterWhateverThePoolAsksFor is the exception, and it is the
// adapter's rather than this application's.
//
// It is asserted here because the skeleton opens SQLite by default: a project
// that raises DB_MAX_OPEN_CONNS on a laptop and sees no change is looking at
// this, not at the wiring the test above covers. SQLite serializes writes, so a
// larger pool converts the wait into "database is locked" -- which reads like
// corruption and is really a pool setting.
func TestSQLiteTakesOneWriterWhateverThePoolAsksFor(t *testing.T) {
	sqliteEnv(t)
	t.Setenv("DB_MAX_OPEN_CONNS", "7")

	cfg, err := appconfig.Load()
	if err != nil {
		t.Fatalf("loading the configuration: %v", err)
	}
	if cfg.Database.Connection.MaxOpenConns != 7 {
		t.Fatalf("the connection carries MaxOpenConns %d, want 7", cfg.Database.Connection.MaxOpenConns)
	}

	db, closeDB, err := bootstrap.Open(cfg)
	if err != nil {
		t.Fatalf("opening the database: %v", err)
	}
	t.Cleanup(closeDB)

	if got := db.Unwrap().Stats().MaxOpenConnections; got != 1 {
		t.Errorf("the SQLite pool holds at most %d connections, want 1", got)
	}
}
