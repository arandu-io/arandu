// Package seeders holds the database seeders of this project.
//
// The shape is the conventional one: DatabaseSeeder is the entry point, it calls the
// other seeders in order, and `aru db:seed --class=<name>` runs a single one.
// The difference is that a seeder here is a type satisfying an interface rather
// than a class discovered by reflection -- so a seeder that does not compile is
// caught at build time, not when someone runs it against production.
package seeders

import (
	"context"
	"fmt"
	"strings"

	"github.com/arandu-io/framework/modules/auth"
)

// Deps carries what seeders are allowed to touch. It is explicit for the same
// reason the rest of the wiring is: a seeder that can reach anything is a seeder
// nobody can review.
type Deps struct {
	Auth *auth.Service
	// Tenant is the tenant seeded rows belong to. It comes from the application,
	// never from the seeder: a seeder that picks its own tenant seeds data nobody
	// can reach (RULE 14).
	Tenant string
}

// Seeder is one unit of seeding.
type Seeder interface {
	// Name is how the seeder is addressed on the command line.
	Name() string
	// Run performs the seeding. It must be safe to run twice: a seeder that
	// fails on the second run cannot be part of a deploy.
	Run(ctx context.Context, d Deps) error
}

// registry lists every seeder that can be addressed by name. DatabaseSeeder
// decides which ones run by default, and in which order.
var registry = []Seeder{
	DatabaseSeeder{},
	AdminSeeder{},
}

// Run executes DatabaseSeeder, or the one named by --class.
func Run(ctx context.Context, deps Deps, args []string) error {
	name := "DatabaseSeeder"
	for _, arg := range args {
		if value, ok := strings.CutPrefix(arg, "--class="); ok {
			name = value
			continue
		}
		return fmt.Errorf("unknown argument %q (expected --class=<name>)", arg)
	}

	seeder, err := lookup(name)
	if err != nil {
		return err
	}
	if err := seeder.Run(ctx, deps); err != nil {
		return fmt.Errorf("%s: %w", seeder.Name(), err)
	}
	fmt.Printf("seeded %s\n", seeder.Name())
	return nil
}

func lookup(name string) (Seeder, error) {
	for _, s := range registry {
		if strings.EqualFold(s.Name(), name) {
			return s, nil
		}
	}

	available := make([]string, 0, len(registry))
	for _, s := range registry {
		available = append(available, s.Name())
	}
	return nil, fmt.Errorf("unknown seeder %q (available: %s)", name, strings.Join(available, ", "))
}
