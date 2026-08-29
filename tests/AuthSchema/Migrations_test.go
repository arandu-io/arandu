package authschema_test

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	appmigrations "github.com/arandu-io/arandu/database/migrations"
	"github.com/arandu-io/hesape/database"
	_ "github.com/arandu-io/hesape/database/connectors/sqlite"
	dbmigrations "github.com/arandu-io/hesape/database/migrations"
)

var nativeAuthMigrationNames = []string{
	"20260729_0001_create_users",
	"20260809_0002_add_name_and_verification_to_users",
	"20260828_0003_create_two_factor",
}

const nativeAuthTestPath = "tests/native-auth-schema"

var registerNativeAuthTestPath sync.Once

func TestTheApplicationOwnsTheExactNativeAuthMigrationSubset(t *testing.T) {
	assertNativeAuthProductionCatalog(t)
}

func TestTheNativeAuthCatalogAllowsAdditionalDomainMigrations(t *testing.T) {
	dbmigrations.Register(additionalDomainMigration{})
	assertNativeAuthProductionCatalog(t)
}

func assertNativeAuthProductionCatalog(t *testing.T) {
	t.Helper()
	registered := dbmigrations.Registered(dbmigrations.DefaultPath)
	wanted := make(map[string]struct{}, len(nativeAuthMigrationNames))
	for _, name := range nativeAuthMigrationNames {
		wanted[name] = struct{}{}
	}

	names := make([]string, 0, len(nativeAuthMigrationNames))
	for _, migration := range registered {
		if _, native := wanted[migration.GetName()]; !native {
			continue
		}
		names = append(names, migration.GetName())
		if _, ok := migration.(dbmigrations.ReversibleMigration); !ok {
			t.Errorf("native auth migration %q cannot be rolled back", migration.GetName())
		}
	}

	if !reflect.DeepEqual(names, nativeAuthMigrationNames) {
		t.Fatalf("native auth migration subset = %v, want %v inside the production catalog", names, nativeAuthMigrationNames)
	}
}

func TestFreshNativeAuthMigrationsCreateTheExactPortableSchema(t *testing.T) {
	harness := newMigrationHarness(t)

	applied, err := harness.migrator.Run(context.Background(), []string{nativeAuthTestPath}, dbmigrations.Options{})
	if err != nil {
		t.Fatalf("migrating a fresh database: %v", err)
	}
	if !reflect.DeepEqual(applied, nativeAuthMigrationNames) {
		t.Fatalf("fresh migration order = %v, want %v", applied, nativeAuthMigrationNames)
	}

	assertColumns(t, harness.db, "users", []columnSpec{
		{name: "id", typ: "varchar", required: true, primary: true},
		{name: "tenant_id", typ: "varchar", required: true},
		{name: "email", typ: "varchar", required: true},
		{name: "password", typ: "text", required: true},
		{name: "roles", typ: "text", required: true},
		{name: "created_at", typ: "datetime", required: true},
		{name: "name", typ: "varchar"},
		{name: "verified_at", typ: "datetime"},
	})
	assertColumns(t, harness.db, "user_two_factor", []columnSpec{
		{name: "user_id", typ: "varchar", required: true, primary: true},
		{name: "tenant_id", typ: "varchar", required: true},
		{name: "secret", typ: "text", required: true},
		{name: "confirmed_at", typ: "datetime"},
		{name: "last_used_step", typ: "integer", required: true},
		{name: "created_at", typ: "datetime", required: true},
	})
	assertColumns(t, harness.db, "user_recovery_codes", []columnSpec{
		{name: "id", typ: "varchar", required: true, primary: true},
		{name: "tenant_id", typ: "varchar", required: true},
		{name: "user_id", typ: "varchar", required: true},
		{name: "code_hash", typ: "text", required: true},
		{name: "used_at", typ: "datetime"},
		{name: "created_at", typ: "datetime", required: true},
	})
	assertIndexColumns(t, harness.db, "users", "users_tenant_created_idx", []string{"tenant_id", "created_at", "id"})
	assertIndexColumns(t, harness.db, "user_recovery_codes", "user_recovery_codes_owner_idx", []string{"tenant_id", "user_id"})
}

func TestTheUsersSchemaIsolatesEmailUniquenessByTenant(t *testing.T) {
	harness := newMigrationHarness(t)
	if _, err := harness.migrator.Run(context.Background(), []string{nativeAuthTestPath}, dbmigrations.Options{}); err != nil {
		t.Fatalf("migrating: %v", err)
	}

	insertUser := func(id, tenant string) error {
		_, err := harness.db.ExecContext(context.Background(), `
			INSERT INTO users (id, tenant_id, email, password, roles, created_at)
			VALUES (?, ?, 'same@example.com', 'password-hash', '[]', '2026-08-29 00:00:00')
		`, id, tenant)
		return err
	}
	if err := insertUser("user-a", "tenant-a"); err != nil {
		t.Fatalf("inserting the first tenant: %v", err)
	}
	if err := insertUser("user-b", "tenant-b"); err != nil {
		t.Fatalf("the same email in another tenant was rejected: %v", err)
	}
	if err := insertUser("user-c", "tenant-a"); err == nil {
		t.Fatal("the same email was accepted twice inside one tenant")
	}
}

func TestAnExistingAuthSchemaUpgradesOnceAndRollsBackOnlyTheNewBatch(t *testing.T) {
	harness := newMigrationHarness(t)
	registered := dbmigrations.Registered(nativeAuthTestPath)

	if err := harness.migrator.RunPending(context.Background(), registered[:2], dbmigrations.Options{}); err != nil {
		t.Fatalf("installing the historical user schema: %v", err)
	}
	if tableExists(t, harness.db, "user_two_factor") {
		t.Fatal("the historical schema unexpectedly contains second-factor storage")
	}

	applied, err := harness.migrator.Run(context.Background(), []string{nativeAuthTestPath}, dbmigrations.Options{})
	if err != nil {
		t.Fatalf("upgrading the historical schema: %v", err)
	}
	if !reflect.DeepEqual(applied, nativeAuthMigrationNames[2:]) {
		t.Fatalf("upgrade applied %v, want only %v", applied, nativeAuthMigrationNames[2:])
	}

	replayed, err := harness.migrator.Run(context.Background(), []string{nativeAuthTestPath}, dbmigrations.Options{})
	if err != nil {
		t.Fatalf("replaying the applied catalog: %v", err)
	}
	if len(replayed) != 0 {
		t.Fatalf("replay applied migrations again: %v", replayed)
	}

	reverted, err := harness.migrator.Rollback(context.Background(), []string{nativeAuthTestPath}, dbmigrations.Options{})
	if err != nil {
		t.Fatalf("rolling back the upgrade: %v", err)
	}
	if !reflect.DeepEqual(reverted, nativeAuthMigrationNames[2:]) {
		t.Fatalf("rollback reverted %v, want only %v", reverted, nativeAuthMigrationNames[2:])
	}
	if !tableExists(t, harness.db, "users") {
		t.Fatal("rolling back the second-factor upgrade removed the historical users table")
	}
	if tableExists(t, harness.db, "user_two_factor") || tableExists(t, harness.db, "user_recovery_codes") {
		t.Fatal("rolling back the second-factor upgrade left its tables behind")
	}
}

type migrationHarness struct {
	db         *sql.DB
	migrator   *dbmigrations.Migrator
	repository *dbmigrations.DatabaseMigrationRepository
}

func newMigrationHarness(t *testing.T) migrationHarness {
	t.Helper()
	registerNativeAuthTestPath.Do(func() {
		dbmigrations.Register(appmigrations.CreateUsers{}, nativeAuthTestPath)
		dbmigrations.Register(appmigrations.AddNameAndVerificationToUsers{}, nativeAuthTestPath)
		dbmigrations.Register(appmigrations.CreateTwoFactor{}, nativeAuthTestPath)
	})

	databasePath := filepath.Join(t.TempDir(), "auth-schema.sqlite")
	handle, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatalf("opening SQLite: %v", err)
	}
	handle.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = handle.Close() })

	const connectionName = "auth-schema"
	connection := database.NewConnection(handle, databasePath, "", map[string]any{
		"driver": string(database.DialectSQLite),
		"name":   connectionName,
	})
	connections := database.NewConnectionResolver(map[string]database.ConnectionInterface{
		connectionName: connection,
	})
	connections.SetDefaultConnection(connectionName)

	resolver := database.MigrationResolver{Resolver: connections}
	repository := dbmigrations.NewDatabaseMigrationRepository(resolver, "auth_schema_migrations")
	migrator := dbmigrations.NewMigrator(repository, resolver, nil)
	migrator.SetConnection(connectionName)
	if err := repository.CreateRepository(context.Background()); err != nil {
		t.Fatalf("creating the migration ledger: %v", err)
	}

	return migrationHarness{db: handle, migrator: migrator, repository: repository}
}

// additionalDomainMigration simulates what `aru make:module` adds beside the
// native auth history. It is registered only in this test binary and must not
// change which three migrations the auth proof audits or executes.
type additionalDomainMigration struct{ dbmigrations.BaseMigration }

func (additionalDomainMigration) GetName() string { return "20260829_9999_create_invoices" }

func (additionalDomainMigration) Up(context.Context, dbmigrations.Connection) error { return nil }

func (additionalDomainMigration) Down(context.Context, dbmigrations.Connection) error { return nil }

type columnSpec struct {
	name     string
	typ      string
	required bool
	primary  bool
}

func assertColumns(t *testing.T, db *sql.DB, table string, want []columnSpec) {
	t.Helper()

	rows, err := db.QueryContext(context.Background(), fmt.Sprintf(`PRAGMA table_info(%q)`, table))
	if err != nil {
		t.Fatalf("reading %s columns: %v", table, err)
	}
	defer rows.Close()

	var got []columnSpec
	for rows.Next() {
		var (
			position     int
			name         string
			typ          string
			required     int
			defaultValue sql.NullString
			primary      int
		)
		if err := rows.Scan(&position, &name, &typ, &required, &defaultValue, &primary); err != nil {
			t.Fatalf("scanning %s columns: %v", table, err)
		}
		got = append(got, columnSpec{
			name:     name,
			typ:      strings.ToLower(typ),
			required: required == 1,
			primary:  primary == 1,
		})
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading %s columns: %v", table, err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s columns = %#v, want %#v", table, got, want)
	}
}

func tableExists(t *testing.T, db *sql.DB, table string) bool {
	t.Helper()

	var count int
	if err := db.QueryRowContext(context.Background(),
		`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table,
	).Scan(&count); err != nil {
		t.Fatalf("checking table %s: %v", table, err)
	}
	return count == 1
}

func assertIndexColumns(t *testing.T, db *sql.DB, table, index string, want []string) {
	t.Helper()

	rows, err := db.QueryContext(context.Background(), fmt.Sprintf(`PRAGMA index_info(%q)`, index))
	if err != nil {
		t.Fatalf("reading %s.%s: %v", table, index, err)
	}
	defer rows.Close()

	var got []string
	for rows.Next() {
		var sequence, position int
		var name string
		if err := rows.Scan(&sequence, &position, &name); err != nil {
			t.Fatalf("scanning %s.%s: %v", table, index, err)
		}
		got = append(got, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading %s.%s: %v", table, index, err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s.%s columns = %v, want %v", table, index, got, want)
	}
}
