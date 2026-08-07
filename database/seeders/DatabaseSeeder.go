package seeders

import "context"

// DatabaseSeeder is the entry point, the same role it has in Laravel: it decides
// which seeders run and in which order. `aru db:seed` runs this one.
//
// Add a seeder to the list below and to the registry in seeders.go.
type DatabaseSeeder struct{}

// Name is how the seeder is addressed on the command line.
func (DatabaseSeeder) Name() string { return "DatabaseSeeder" }

// Run calls each seeder in order, stopping at the first failure -- seeding that
// half-succeeded leaves a database nobody can reason about.
func (DatabaseSeeder) Run(ctx context.Context, d Deps) error {
	for _, seeder := range []Seeder{
		AdminSeeder{},
	} {
		if err := seeder.Run(ctx, d); err != nil {
			return err
		}
	}
	return nil
}
