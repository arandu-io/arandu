package seeders

import (
	"context"
	"errors"
	"fmt"
	"os"
)

// AdminSeeder creates the first administrator of a tenant.
//
// It exists because there is no other way in: every repository call needs a
// Grant, and a Grant needs a subject, so the first user cannot be created
// through the application itself. This is the one place that breaks the circle
// with a system grant, and it does it in the open rather than at boot -- seeding
// that happens by itself is how a known password reaches production.
//
// Credentials come from the environment rather than from flags: a password typed
// as an argument lands in the shell history and in the process list.
type AdminSeeder struct{}

// Name is how the seeder is addressed on the command line.
func (AdminSeeder) Name() string { return "AdminSeeder" }

// Run creates the administrator, or leaves the existing one alone.
func (AdminSeeder) Run(ctx context.Context, d Deps) error {
	email := os.Getenv("ARANDU_ADMIN_EMAIL")
	password := os.Getenv("ARANDU_ADMIN_PASSWORD")
	if email == "" || password == "" {
		return errors.New("set ARANDU_ADMIN_EMAIL and ARANDU_ADMIN_PASSWORD before seeding the administrator")
	}
	if d.Auth == nil {
		return errors.New("the auth service is not wired")
	}

	tenant := d.Tenant
	if tenant == "" {
		return errors.New("the tenant is not wired: seeding into an empty tenant would create a user nobody can log in as")
	}

	user, err := d.Auth.EnsureAdmin(ctx, tenant, email, password)
	if err != nil {
		return err
	}

	fmt.Printf("administrator %s ready in tenant %s\n", user.Email, user.TenantID)
	return nil
}

// compile-time proof that every seeder honors the contract. A seeder that drifts
// from the interface fails the build rather than failing when someone runs it.
var (
	_ Seeder = DatabaseSeeder{}
	_ Seeder = AdminSeeder{}
)
