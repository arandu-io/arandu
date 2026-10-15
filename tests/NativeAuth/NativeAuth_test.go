package nativeauth_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	frameevents "github.com/arandu-io/framework/events"
	"github.com/arandu-io/hesape/auth"
	"github.com/arandu-io/hesape/auth/account"
	hedatabase "github.com/arandu-io/hesape/database"
	_ "github.com/arandu-io/hesape/database/connectors/sqlite"
	dbmigrations "github.com/arandu-io/hesape/database/migrations"
	"github.com/arandu-io/hesape/database/model"

	"github.com/arandu-io/arandu/app/Models"
	"github.com/arandu-io/arandu/app/Policies"
	appmigrations "github.com/arandu-io/arandu/database/migrations"
)

// The account rules are hesape/auth/account's, and what they do is proved by
// its contract suite against this application's users table, in
// AccountContract_test.go. The tests in this package prove the half that is
// this application's: the user model the rules hand back, the user policy they
// ask, and the roles column they know nothing about.

func TestTheUserPolicyRefusesAPublicNameReadWithoutAReader(t *testing.T) {
	db := openNativeAuthDatabase(t)
	users := newUsers(t, db)
	if _, err := users.PublicNames(context.Background(), auth.Subject{}, []string{"user-a"}); err == nil {
		t.Fatal("a subject with no tenant was answered the public names")
	}
}

func TestRegistrationPersistsSQLNullUntilEmailIsVerified(t *testing.T) {
	db := openNativeAuthDatabase(t)
	users := newUsers(t, db)
	ctx := context.Background()

	registered, err := users.Register(ctx, "tenant-a", account.Registration{
		Name: "Ana", Email: "ana@example.test", Password: "a-long-enough-password",
	})
	if err != nil {
		t.Fatalf("registering an unverified user: %v", err)
	}
	assertUserVerificationContract(t, registered, false)

	var verifiedAt any
	if err := db.sql.QueryRow(`SELECT verified_at FROM users WHERE id = ?`, registered.AccountID()).Scan(&verifiedAt); err != nil {
		t.Fatalf("reading the stored verification state: %v", err)
	}
	if verifiedAt != nil {
		t.Fatalf("a new registration stored verified_at = %v, want SQL NULL", verifiedAt)
	}

	verified, changed, err := users.MarkVerified(ctx, "tenant-a", registered.AccountID(), registered.AccountEmail())
	if err != nil || !changed {
		t.Fatalf("verifying the address = %v, %v; want a change", changed, err)
	}
	assertUserVerificationContract(t, verified, true)
}

func TestSeededVerifiedUserPreservesVerificationContracts(t *testing.T) {
	db := openNativeAuthDatabase(t)
	users := newUsers(t, db)
	ctx := context.Background()

	seeded, err := users.Provision(ctx, "tenant-a", account.Draft{
		Name: "Admin", Email: "admin@example.test", Password: "a-long-enough-password", Verified: true,
		Fill: func(a account.Account) error {
			a.(*models.User).Roles = models.Roles{models.RoleAdmin}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("seeding a verified user: %v", err)
	}
	assertUserVerificationContract(t, seeded, true)

	found, err := users.Lookup(ctx, "tenant-a", seeded.AccountEmail())
	if err != nil {
		t.Fatalf("reading the seeded user: %v", err)
	}
	assertUserVerificationContract(t, found, true)
}

// nativeAuthDatabase is one migrated SQLite database, as the driver handle the
// assertions read and as the application handle the rules write through.
type nativeAuthDatabase struct {
	sql *sql.DB
	app *hedatabase.DB
}

// openNativeAuthDatabase migrates a fresh database with what the account
// rules read and write: the outbox, this application's users table and the
// second factor's tables, created by the migration hesape/auth/account owns.
func openNativeAuthDatabase(t *testing.T) nativeAuthDatabase {
	t.Helper()

	path := filepath.Join(t.TempDir(), "native-auth.sqlite")
	handle, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("opening SQLite: %v", err)
	}
	handle.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = handle.Close() })

	connection := hedatabase.NewConnection(handle, path, "", map[string]any{
		"driver": string(hedatabase.DialectSQLite),
		"name":   "native-auth",
	})
	migrationConnection := hedatabase.ForMigrations(connection)
	for _, migration := range frameevents.NewModule().Migrations() {
		if err := migration.Up(context.Background(), migrationConnection); err != nil {
			t.Fatalf("applying %s: %v", migration.GetName(), err)
		}
	}
	application := []dbmigrations.Migration{
		appmigrations.CreateUsers{},
		appmigrations.AddNameAndVerificationToUsers{},
	}
	for _, migration := range append(application, account.Migrations()...) {
		if err := migration.Up(context.Background(), migrationConnection); err != nil {
			t.Fatalf("applying %s: %v", migration.GetName(), err)
		}
	}

	return nativeAuthDatabase{sql: handle, app: hedatabase.Wrap(handle, hedatabase.DialectSQLite)}
}

// newUsers builds the account rules the way bootstrap/app.go does: over this
// application's users query and user policy.
func newUsers(t *testing.T, db nativeAuthDatabase) *account.Users {
	t.Helper()
	users, err := account.NewUsers(account.Config{
		DB:     db.app,
		Query:  func() *model.Builder { return models.Users(db.app).Base() },
		Policy: account.PolicyOf(policies.UserPolicy{}),
	})
	if err != nil {
		t.Fatalf("building the account rules: %v", err)
	}
	return users
}

// assertUserVerificationContract holds the account the rules hand back to what
// this application's model promises: it is the *models.User the users query
// built, its subject carries the verification, and its JSON shows verified_at
// exactly when there is one and never the password.
func assertUserVerificationContract(t *testing.T, a account.Account, want bool) {
	t.Helper()

	user, ok := a.(*models.User)
	if !ok {
		t.Fatalf("the account rules handed back a %T, want *models.User", a)
	}
	if user.IsVerified() != want {
		t.Errorf("User.IsVerified() = %v, want %v", user.IsVerified(), want)
	}
	if user.Subject().Verified != want {
		t.Errorf("User.Subject().Verified = %v, want %v", user.Subject().Verified, want)
	}

	encoded, err := json.Marshal(user)
	if err != nil {
		t.Fatalf("marshalling the user: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("reading the user JSON: %v", err)
	}
	_, hasVerifiedAt := fields["verified_at"]
	if hasVerifiedAt != want {
		t.Errorf("user JSON has verified_at = %v, want %v: %s", hasVerifiedAt, want, encoded)
	}
	if want {
		var verifiedAt time.Time
		if err := json.Unmarshal(fields["verified_at"], &verifiedAt); err != nil {
			t.Errorf("user JSON has an invalid verified_at: %v: %s", err, encoded)
		} else if verifiedAt.IsZero() {
			t.Errorf("user JSON has a zero verified_at: %s", encoded)
		}
	}
	if _, exposed := fields["password"]; exposed {
		t.Errorf("user JSON exposed password: %s", encoded)
	}
}
