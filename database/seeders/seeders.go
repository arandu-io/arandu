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

	"github.com/arandu-io/framework/modules/auth"
	"github.com/arandu-io/hesape/database"
)

// Deps carries what seeders are allowed to touch. It is explicit for the same
// reason the rest of the wiring is: a seeder that can reach anything is a seeder
// nobody can review.
type Deps struct {
	Auth *auth.Service
	// Tenant is the tenant seeded rows belong to. It comes from the application,
	// never from the seeder: a seeder that picks its own tenant seeds data nobody
	// can reach.
	Tenant string

	// Args is what came after the seeder's name on the command line.
	//
	// It is here rather than on the Seeder interface because most seeders take
	// nothing, and widening Run would put an ignored parameter on every one of
	// them forever. A seeder that reads flags says so by reading this field.
	Args []string
}

// Seeder is one unit of seeding.
//
// It is database.Seeder[Deps] under a local name, and not a second declaration
// of it: the two were structurally identical -- Name and Run(ctx, Deps) -- and a
// second interface with the same methods is a second thing to keep in step for
// no gain. The alias is what lets a seeder in this package read as a seeder of
// this application while the engine below takes it as it is.
type Seeder = database.Seeder[Deps]

// registry lists every seeder that can be addressed by name. DatabaseSeeder
// decides which ones run by default, and in which order.
var registry = []Seeder{
	DatabaseSeeder{},
	UserSeeder{},
	AdminSeeder{},
}

// Run executes DatabaseSeeder, or the one named first on the command line, and
// answers the name of the one that ran.
//
//	aru db:seed                                    every seeder, in order
//	aru db:seed PostSeeder                         one of them
//	aru db:seed UserSeeder -e a@b.com -p secret    one of them, with arguments
//
// The work is database.Seed's, and what stays here is what only this application
// can answer: which seeders exist, what a seeder is allowed to touch, and which
// one runs when no name is given.
//
// This used to be ninety lines that reimplemented that engine -- the positional
// name, the refusal of --class=, the lookup with the list of what is available,
// and the two flag readers -- once per application, four times, with three
// different contents. The refusal in particular is a sentence a person reads and
// retypes, and it now comes from one place.
func Run(ctx context.Context, deps Deps, args []string) (string, error) {
	return database.Seed(ctx, registry, "DatabaseSeeder", args, func(rest []string) Deps {
		deps.Args = rest
		return deps
	})
}

// Flag reads `-name value` out of the arguments, and Switch reports whether a
// valueless one is present. They are the component's, under the names the
// seeders here already call them by.
var (
	Flag   = database.Flag
	Switch = database.Switch
)
