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

	// Args is what came after the seeder's name on the command line.
	//
	// It is here rather than on the Seeder interface because most seeders take
	// nothing, and widening Run would put an ignored parameter on every one of
	// them forever. A seeder that reads flags says so by reading this field.
	Args []string
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
	UserSeeder{},
	AdminSeeder{},
}

// Run executes DatabaseSeeder, or the one named first on the command line.
//
//	aru db:seed                                    every seeder, in order
//	aru db:seed PostSeeder                         one of them
//	aru db:seed UserSeeder -e a@b.com -p secret    one of them, with arguments
//
// The name is positional. Laravel spells it `--class=`, and this used to as
// well; a name that is sometimes a flag and sometimes a word is two spellings of
// one thing (RULE 9), so the flag form is refused with the word to use instead
// rather than accepted quietly.
//
// Everything after the name reaches the seeder as Deps.Args, unparsed. What a
// flag means is the seeder's business: this function does not know that
// UserSeeder has a -p, and adding a seeder does not edit this file.
func Run(ctx context.Context, deps Deps, args []string) error {
	name := "DatabaseSeeder"

	if len(args) > 0 {
		if value, ok := strings.CutPrefix(args[0], "--class="); ok {
			return fmt.Errorf("--class= is not how a seeder is named any more.\n\n    aru db:seed %s", value)
		}
		if !strings.HasPrefix(args[0], "-") {
			name, args = args[0], args[1:]
		}
	}
	deps.Args = args

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

// Flag reads `-name value` out of the arguments.
//
// A hand-written reader rather than the flag package, because the flag package
// wants to own os.Args, prints its own usage to stderr and calls os.Exit on a
// mistake -- in the middle of a command that has already opened a database.
//
// It accepts `-name value` and `-name=value`. A long form (`--name`) is the same
// flag: nobody should have to remember how many dashes a seeder wanted.
func Flag(args []string, name string) (string, bool) {
	short, long := "-"+name, "--"+name
	for i, a := range args {
		if a == short || a == long {
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				return args[i+1], true
			}
			// Present with nothing after it. Reporting it as absent would turn a
			// typed-but-empty password into the demo fallback.
			return "", true
		}
		for _, prefix := range []string{short + "=", long + "="} {
			if value, ok := strings.CutPrefix(a, prefix); ok {
				return value, true
			}
		}
	}
	return "", false
}

// Switch reports whether a valueless flag is present.
func Switch(args []string, name string) bool {
	for _, a := range args {
		if a == "-"+name || a == "--"+name {
			return true
		}
	}
	return false
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
