package nativeauth_test

import (
	"context"
	"errors"
	"testing"

	"github.com/arandu-io/hesape/auth/account"
	"github.com/arandu-io/hesape/database/model"

	"github.com/arandu-io/arandu/app/Models"
	"github.com/arandu-io/arandu/app/Policies"
)

// TestRolesAreOneValueInGoAndJSONInTheColumn stores an account with a role and
// reads it back: the field the policies compare against is what was written,
// and the column holds the portable JSON text every engine can store. Roles
// are this application's column, written by Draft.Fill, and the account rules
// read and write the row around it without knowing it is there.
func TestRolesAreOneValueInGoAndJSONInTheColumn(t *testing.T) {
	db := openNativeAuthDatabase(t)
	ctx := context.Background()
	users := newUsers(t, db)

	admin, err := users.Provision(ctx, "tenant-a", account.Draft{
		Email: "root@example.test", Password: "a-long-enough-password", Verified: true,
		Fill: func(a account.Account) error {
			a.(*models.User).Roles = models.Roles{models.RoleAdmin}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("creating the administrator: %v", err)
	}
	read, err := users.Lookup(ctx, "tenant-a", "root@example.test")
	if err != nil {
		t.Fatalf("reading the administrator back: %v", err)
	}
	found, ok := read.(*models.User)
	if !ok {
		t.Fatalf("the account rules read a %T, want *models.User", read)
	}
	if len(found.Roles) != 1 || found.Roles[0] != models.RoleAdmin || !found.Subject().HasRole(models.RoleAdmin) {
		t.Fatalf("roles read back = %v, want [%s]", found.Roles, models.RoleAdmin)
	}
	var stored string
	if err := db.sql.QueryRow(`SELECT roles FROM users WHERE id = ?`, admin.AccountID()).Scan(&stored); err != nil {
		t.Fatalf("reading the column: %v", err)
	}
	if stored != `["admin"]` {
		t.Fatalf("the column holds %q, want the JSON array", stored)
	}

	member, err := users.Register(ctx, "tenant-a", account.Registration{
		Name: "Ana", Email: "ana@example.test", Password: "a-long-enough-password",
	})
	if err != nil {
		t.Fatalf("registering a member: %v", err)
	}
	if err := db.sql.QueryRow(`SELECT roles FROM users WHERE id = ?`, member.AccountID()).Scan(&stored); err != nil || stored != "[]" {
		t.Fatalf("a member's column holds %q (%v), want []", stored, err)
	}
	if _, err := users.Lookup(ctx, "tenant-a", "nobody@example.test"); !errors.Is(err, account.ErrUserNotFound) {
		t.Fatalf("looking up an unknown address answered %v, want ErrUserNotFound", err)
	}
}

// TestARegistrationCannotGrantItselfARole is the user policy's half of
// registration: a guest may create an account only with no roles, so a form
// that smuggled one in is refused before anything is stored.
func TestARegistrationCannotGrantItselfARole(t *testing.T) {
	db := openNativeAuthDatabase(t)
	ctx := context.Background()
	users, err := account.NewUsers(account.Config{
		DB:     db.app,
		Query:  func() *model.Builder { return models.Users(db.app).Base() },
		Policy: account.PolicyOf(policies.UserPolicy{}),
		Hooks: account.Hooks{Prepare: func(_ context.Context, _ *account.Registration, a account.Account) error {
			a.(*models.User).Roles = models.Roles{models.RoleAdmin}
			return nil
		}},
	})
	if err != nil {
		t.Fatalf("building the account rules: %v", err)
	}

	if _, err := users.Register(ctx, "tenant-a", account.Registration{
		Name: "Mallory", Email: "mallory@example.test", Password: "a-long-enough-password",
	}); err == nil {
		t.Fatal("a guest registered an account holding the admin role")
	}
	var count int
	if err := db.sql.QueryRow(`SELECT count(*) FROM users`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("users stored after the refusal = %d (%v), want 0", count, err)
	}
}
