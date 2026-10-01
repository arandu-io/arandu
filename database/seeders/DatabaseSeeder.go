package seeders

import "context"

// DatabaseSeeder is the entry point: it decides
// which seeders run and in which order. `aru db:seed` runs this one.
//
// Add a seeder to the list below and to the registry in seeders.go.
type DatabaseSeeder struct{}

// Name is how the seeder is addressed on the command line.
func (DatabaseSeeder) Name() string { return "DatabaseSeeder" }

// Run creates no account anybody can sign in as, and requires no
// administrator credentials: a fresh project must not, merely because its
// database was seeded. In development it seeds the example notes, whose
// authors have no usable password.
//
// Create a user only when the application needs one, through the named seeder:
//
//	aru db:seed UserSeeder -e you@example.com -p '<choose-a-password>'
//	aru db:seed UserSeeder -e you@example.com -p '<choose-a-password>' -r admin
//
// Add application-owned seeders here explicitly when they should run as part
// of the project's normal seed set.
func (DatabaseSeeder) Run(ctx context.Context, d Deps) error {
	if !d.Development {
		return nil
	}
	// The example resource. Remove it with the list under "The example
	// resource" in README.md.
	return NoteSeeder{}.Run(ctx, d)
}

var _ Seeder = DatabaseSeeder{}
