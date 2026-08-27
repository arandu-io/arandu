package feature_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/events"
	"github.com/arandu-io/framework/modules/auth"
	"github.com/arandu-io/framework/observability"
	"github.com/arandu-io/hesape/database"
	"github.com/arandu-io/hesape/database/schema"

	"github.com/arandu-io/arandu/bootstrap"
)

// What a rollout is, and what these two tests execute.
//
// `aru migrate` is a step of the pipeline and runs before the new binary is
// rolled out, so there is a window -- the whole window, on a slow rollout -- in
// which the PREVIOUS binary is serving against the NEW schema. Everything the
// project says about migrations is a promise about that window: a new column is
// nullable or has a default, and dropping one takes two releases.
//
// Nothing executed that promise until now. The suite proved that `migrate` runs,
// that it takes a lock and that a rollback is dispatched, and every one of those
// passes over a migration that breaks every replica still running the release
// before it.
//
// # What is under test, and what a process can stand in for
//
// The process running these tests is the previous binary. It was built against
// the schema `migrate` produces, it holds the column list it was compiled with,
// and it is booted and serving before anything below changes the database
// underneath it. The next release's schema change is then applied to that live
// database, and the SAME booted application is asked to keep working.
//
// That is the property, and it is the whole property this level can reach: the
// running code did not compile against the new columns, which is exactly what
// makes it the previous binary with respect to this change. What it does NOT
// reach is a literally older compiled artifact -- an older tag, built, launched,
// pointed at the migrated database. That needs two builds and two processes, and
// it belongs to a pipeline rather than to `go test`. The failure mode it would
// catch and this one would not is a change of behaviour between the two releases
// that has nothing to do with the schema.

// TestThePreviousBinaryKeepsServingWhileTheNewSchemaIsInPlace is the promise.
//
// The additive shapes -- a nullable column, a column with a default, a table
// nothing in this binary has heard of -- go in underneath a booted application,
// and it keeps answering: the page, the probe, a write and a read.
//
// The read is the half that is easy to leave out and the half that fails first.
// A repository that selected * and scanned into a fixed list of destinations
// would break on the very next login the moment a column appears, and no INSERT
// anywhere would have noticed.
func TestThePreviousBinaryKeepsServingWhileTheNewSchemaIsInPlace(t *testing.T) {
	app, db := bootedApp(t)
	handler := app.Kernel.Handler()
	ctx := context.Background()

	const password = "a-long-enough-password"

	// Serving at the release the schema was migrated for, so that everything
	// below is a change and not the starting state.
	assertServing(t, handler)
	if _, err := app.Auth.Register(ctx, bootstrap.Tenant(), auth.RegisterRequest{
		Name:                 "Ana",
		Email:                "ana@example.test",
		Password:             password,
		PasswordConfirmation: password,
	}); err != nil {
		t.Fatalf("registering before the schema changed: %v", err)
	}

	// The next release's migration, applied while the application above is
	// booted and holding its connection. Through the schema builder rather than
	// through hand-written SQL: that is the door conn.Schema() opens for a
	// migration, so what lands on the database is DDL a migration can emit.
	builder := schemaBuilder(t, db)
	err := builder.Table(ctx, "users", func(table *schema.Blueprint) {
		// Nullable, which is one of the two shapes the rule allows. The previous
		// binary's INSERT does not name this column, and a NOT NULL column with
		// no default would fail every one of those inserts.
		table.String("locale").Nullable()
	})
	if err != nil {
		t.Fatalf("adding a nullable column: %v", err)
	}
	err = builder.Table(ctx, "users", func(table *schema.Blueprint) {
		// The other allowed shape. Not null, and safe anyway, because the value
		// the previous binary does not supply is the one the database fills in.
		table.String("plan").Default("free")
	})
	if err != nil {
		t.Fatalf("adding a column with a default: %v", err)
	}
	err = builder.Create(ctx, "rollout_invoices", func(table *schema.Blueprint) {
		table.String("id").Primary()
		table.String("tenant_id")
		table.Timestamp("created_at")
	})
	if err != nil {
		t.Fatalf("creating the new release's table: %v", err)
	}

	// The schema really did change. Without this the rest of the test would pass
	// just as well over three statements that quietly did nothing.
	assertUsersHasColumns(t, builder, "locale", "plan")
	if has, err := builder.HasTable(ctx, "rollout_invoices"); err != nil || !has {
		t.Fatalf("the new release's table is not there (%v), so nothing below is a rollout", err)
	}

	// From here down, every assertion is the previous binary working against a
	// schema it was not compiled for.

	assertServing(t, handler)

	// A write. The INSERT names the columns this binary knows and none of the
	// new ones, which is what the defaulted column has to tolerate.
	if _, err := app.Auth.Register(ctx, bootstrap.Tenant(), auth.RegisterRequest{
		Name:                 "Bruno",
		Email:                "bruno@example.test",
		Password:             password,
		PasswordConfirmation: password,
	}); err != nil {
		t.Fatalf("the previous binary cannot write against the new schema: %v", err)
	}

	// A read, of a row written before the change and a row written after it. It
	// goes through the login path, which is the SELECT with the old column list
	// scanned into the struct this binary was compiled with.
	for _, email := range []string{"ana@example.test", "bruno@example.test"} {
		if _, err := app.Auth.Authenticate(ctx, bootstrap.Tenant(), email, password, "127.0.0.1"); err != nil {
			t.Fatalf("the previous binary cannot read %s back against the new schema: %v", email, err)
		}
	}

	// The outbox, which is the write path that has to commit inside a
	// transaction. Two registrations stored two events, and the relay this
	// application wired publishes them and marks them.
	outbox := events.NewOutbox(db)
	pending, err := outbox.PendingAll(ctx, 10)
	if err != nil {
		t.Fatalf("reading the outbox against the new schema: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("the outbox holds %d events after two registrations, want 2", len(pending))
	}
	if err := app.Relay.Drain(observability.WithLogger(ctx, slog.New(&publishedEvents{}))); err != nil {
		t.Fatalf("the relay cannot drain against the new schema: %v", err)
	}
	if left, err := outbox.PendingAll(ctx, 10); err != nil || len(left) != 0 {
		t.Fatalf("%d events are still pending after a pass (%v)", len(left), err)
	}
}

// TestThePreviousBinaryStopsServingWhenTheNewSchemaTakesAColumnAway is the
// control, and without it the test above proves nothing.
//
// "Everything still works" is the answer a weak assertion gives too. So the
// forbidden shape goes in the same way, on the same live application, and the
// previous binary has to break: dropping a column in the release that stops
// writing it is what "dropping one takes two releases" forbids, and this is the
// window it breaks in.
func TestThePreviousBinaryStopsServingWhenTheNewSchemaTakesAColumnAway(t *testing.T) {
	app, db := bootedApp(t)
	ctx := context.Background()

	const password = "a-long-enough-password"

	if _, err := app.Auth.Register(ctx, bootstrap.Tenant(), auth.RegisterRequest{
		Name:                 "Ana",
		Email:                "ana@example.test",
		Password:             password,
		PasswordConfirmation: password,
	}); err != nil {
		t.Fatalf("registering before the schema changed: %v", err)
	}

	// A column this binary still writes and still reads, taken away by the
	// release rolling out over it.
	builder := schemaBuilder(t, db)
	err := builder.Table(ctx, "users", func(table *schema.Blueprint) {
		table.DropColumn("name")
	})
	if err != nil {
		t.Fatalf("dropping the column: %v", err)
	}

	if _, err := app.Auth.Register(ctx, bootstrap.Tenant(), auth.RegisterRequest{
		Name:                 "Bruno",
		Email:                "bruno@example.test",
		Password:             password,
		PasswordConfirmation: password,
	}); err == nil {
		t.Error("the previous binary wrote a column the new schema had taken away, so this test cannot tell a safe migration from an unsafe one")
	}
	if _, err := app.Auth.Authenticate(ctx, bootstrap.Tenant(), "ana@example.test", password, "127.0.0.1"); err == nil {
		t.Error("the previous binary read a column the new schema had taken away, so this test cannot tell a safe migration from an unsafe one")
	}
}

// schemaBuilder opens the schema builder over the connection the application is
// already serving on.
//
// The three lines are the ones bootstrap/migrate.go builds its resolver from,
// and they are here rather than borrowed because that resolver is unexported and
// exporting it would put a door in the application for the benefit of a test.
// What matters is that the DDL below goes through the same grammar a migration's
// conn.Schema() goes through, and the connection is what carries that.
func schemaBuilder(t *testing.T, db *data.DB) *schema.Builder {
	t.Helper()
	conn := database.NewConnection(db.Unwrap(), "", "", map[string]any{
		"driver": string(db.Dialect()),
		"name":   "default",
	})
	return schema.NewBuilder(database.ForSchema(conn))
}

// assertServing asks the application the two questions a rollout asks it: does
// it answer a page, and does it answer the probe.
func assertServing(t *testing.T, handler http.Handler) {
	t.Helper()

	page := httptest.NewRecorder()
	handler.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
	if page.Code != http.StatusOK {
		t.Fatalf("the landing page answered %d: %s", page.Code, page.Body.String())
	}

	probe := httptest.NewRecorder()
	handler.ServeHTTP(probe, httptest.NewRequest(http.MethodGet, "/_arandu/health", nil))
	if probe.Code != http.StatusOK {
		t.Fatalf("the probe answered %d: %s", probe.Code, probe.Body.String())
	}
}

// assertUsersHasColumns fails unless the table carries every column named.
func assertUsersHasColumns(t *testing.T, builder *schema.Builder, want ...string) {
	t.Helper()

	columns, err := builder.GetColumns(context.Background(), "users")
	if err != nil {
		t.Fatalf("reading the columns of users: %v", err)
	}
	present := make(map[string]bool, len(columns))
	for _, c := range columns {
		present[c.Name] = true
	}
	for _, name := range want {
		if !present[name] {
			t.Fatalf("users has no %s column, so the schema did not change and nothing below is a rollout", name)
		}
	}
}
