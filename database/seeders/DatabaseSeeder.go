package seeders

import "context"

// DatabaseSeeder is the entry point: it decides
// which seeders run and in which order. `aru db:seed` runs this one.
//
// Add a seeder to the list below and to the registry in seeders.go.
type DatabaseSeeder struct{}

// Name is how the seeder is addressed on the command line.
func (DatabaseSeeder) Name() string { return "DatabaseSeeder" }

// Run is intentionally empty. A fresh project must not create an account or
// require administrator credentials merely because its database was seeded.
//
// Create a user only when the application needs one, through the named seeder:
//
//	aru db:seed UserSeeder -e you@example.com -p '<choose-a-password>'
//	aru db:seed UserSeeder -e you@example.com -p '<choose-a-password>' -r admin
//
// Add application-owned seeders here explicitly when they should run as part
// of the project's normal seed set.
func (DatabaseSeeder) Run(ctx context.Context, d Deps) error {
	return nil
}

var _ Seeder = DatabaseSeeder{}
