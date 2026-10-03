package connectors_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/arandu-io/hesape/cache"
	"github.com/arandu-io/hesape/database"

	"github.com/arandu-io/arandu/bootstrap"
)

// What a setting that names an engine this binary does not link answers.
//
// The project links SQLite and nothing else, and every other engine is two
// lines: a `go get` and a blank import in bootstrap/app.go. So a setting naming
// one of them without the import is not a typo the process could correct at
// run time -- it is an import missing from the build, and the boot has to stop
// and print both lines. The text is pinned whole, because it is the one place a
// person learns what to type.
//
// It is a package of its own and not a file in tests/Feature, and that is the
// point of it: the feature suite lends the binary a connector for "redis" so it
// can check the wiring behind the import, and a registration cannot be taken
// back. A binary that has registered one cannot be asked what happens without
// it. This one registers nothing, so it links exactly what the application
// links.
//
// Each test skips once the project links the connector it is about, because
// from then on there is no missing import to report.

// boot sets the environment of a project that runs with nothing installed, and
// answers what the binary says when started with the change applied on top.
func boot(t *testing.T, change map[string]string) error {
	t.Helper()

	t.Setenv("APP_ENV", "dev")
	t.Setenv("APP_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("ARANDU_TENANT_ID", "11111111-1111-4111-8111-111111111111")
	t.Setenv("DATABASE_URL", "sqlite://"+filepath.Join(t.TempDir(), "test.sqlite"))
	t.Setenv("CACHE_STORE", "memory")
	t.Setenv("SESSION_DRIVER", "memory")
	t.Setenv("QUEUE_CONNECTION", "database")
	t.Setenv("REDIS_URL", "")
	for name, value := range change {
		t.Setenv(name, value)
	}

	// `routes` builds the whole application and serves nothing, which is the
	// cheapest command that reaches every place a connector is asked for.
	return bootstrap.Dispatch("routes", nil)
}

// notLinked is the message, written out rather than built: a test that called
// the function producing it would agree with any wording.
func notLinked(setting, driver, linked, module string) string {
	return setting + " asks for " + driver + " and no connector for it is linked into this binary (linked: " + linked + ").\n" +
		"Add it:\n" +
		"\n" +
		"    go get " + module + "\n" +
		"\n" +
		"and blank-import it in bootstrap/app.go, next to the other connectors:\n" +
		"\n" +
		"    _ \"" + module + "\""
}

// assertRefused fails unless err is exactly want.
func assertRefused(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatal("the application started, and the setting names a connector this binary does not link")
	}
	if err.Error() != want {
		t.Errorf("the boot stopped with\n%s\n\nwant\n%s", err, want)
	}
}

// skipUnlessNoCacheConnector skips once any cache connector is linked: the
// message lists what is, and "none" is the answer only while nothing is.
func skipUnlessNoCacheConnector(t *testing.T) {
	t.Helper()
	if linked := cache.Registered(); len(linked) > 0 {
		t.Skipf("this binary links the cache connectors %v, so there is no missing import to report", linked)
	}
}

// TestTheDefaultConfigurationNeedsNothingTheBinaryDoesNotLink.
//
// The other side of every refusal below: the values .env.example ships with --
// SQLite, the in-process cache and sessions, the queue in a table -- start a
// binary that links SQLite and nothing else. A new project runs before anybody
// installs anything.
func TestTheDefaultConfigurationNeedsNothingTheBinaryDoesNotLink(t *testing.T) {
	if err := boot(t, nil); err != nil {
		t.Fatalf("the default configuration did not start: %v", err)
	}
}

// TestCacheStoreRedisWithoutItsConnectorStopsTheBoot.
func TestCacheStoreRedisWithoutItsConnectorStopsTheBoot(t *testing.T) {
	skipUnlessNoCacheConnector(t)

	err := boot(t, map[string]string{"CACHE_STORE": "redis", "REDIS_URL": "redis://127.0.0.1:6379"})
	assertRefused(t, err, notLinked("CACHE_STORE", "redis", "none", "github.com/arandu-io/hesape/redis"))
}

// TestSessionDriverRedisWithoutItsConnectorStopsTheBoot.
//
// The session names the store on its own, with the cache left in the process,
// and the message names the setting that was written -- SESSION_DRIVER, not
// CACHE_STORE -- so the person reading it looks at the right line of .env.
func TestSessionDriverRedisWithoutItsConnectorStopsTheBoot(t *testing.T) {
	skipUnlessNoCacheConnector(t)

	err := boot(t, map[string]string{"SESSION_DRIVER": "redis", "REDIS_URL": "redis://127.0.0.1:6379"})
	assertRefused(t, err, notLinked("SESSION_DRIVER", "redis", "none", "github.com/arandu-io/hesape/redis"))
}

// TestDatabaseURLPostgresWithoutItsConnectorStopsTheBoot.
//
// Postgres was linked into every project whether it used it or not. It is not
// any more, so a DATABASE_URL that names it is the case a project meets first,
// and the message has to point at the file the import goes in.
func TestDatabaseURLPostgresWithoutItsConnectorStopsTheBoot(t *testing.T) {
	if linked := database.Registered(); !slices.Equal(linked, []database.Dialect{database.DialectSQLite}) {
		t.Skipf("this binary links the database connectors %v, and the message below is the one for SQLite alone", linked)
	}

	err := boot(t, map[string]string{"DATABASE_URL": "postgres://arandu:arandu@127.0.0.1:5432/arandu"})
	assertRefused(t, err, notLinked("DATABASE_URL", "pgsql", "sqlite", "github.com/arandu-io/hesape/database/connectors/pgx"))
}
