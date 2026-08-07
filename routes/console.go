package routes

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// Command is one console command this application adds.
//
// Laravel's routes/console.php registers closures against Artisan. This is the
// same idea with the reflection removed: a command is a value with a name, a
// sentence of help and a function, and the compiler checks the function.
type Command struct {
	// Name is what the command is called on the command line. Lowercase, with a
	// colon for the group: "invoice:close".
	Name string
	// Description is the one line `aru` prints next to it.
	Description string
	// Run does the work. It receives whatever followed the command name.
	Run func(ctx context.Context, args []string) error
}

// Console returns this application's own commands.
//
// The built-in ones -- serve, migrate, routes, db:seed, work, schedule -- are
// not here. They live in main.go because they need the registered modules, and
// listing them twice would be two answers to "what can this binary do".
func Console() []Command {
	return []Command{
		// arandu:begin custom
		// {
		// 	Name:        "invoice:close",
		// 	Description: "Close the open invoices of the current period",
		// 	Run:         func(ctx context.Context, args []string) error { return nil },
		// },
		// arandu:end custom
	}
}

// Lookup finds a command by name.
func Lookup(name string) (Command, bool) {
	for _, c := range Console() {
		if c.Name == name {
			return c, true
		}
	}
	return Command{}, false
}

// Names lists the registered commands, sorted, for an error message that has to
// say what was available instead.
func Names() []string {
	commands := Console()
	out := make([]string, 0, len(commands))
	for _, c := range commands {
		out = append(out, c.Name)
	}
	sort.Strings(out)
	return out
}

// Help renders the commands for the terminal. It returns an empty string when
// the application declares none, so the caller prints nothing rather than a
// heading over a blank list.
func Help() string {
	commands := Console()
	if len(commands) == 0 {
		return ""
	}

	sort.Slice(commands, func(i, j int) bool { return commands[i].Name < commands[j].Name })

	var b strings.Builder
	b.WriteString("commands of this application:\n")
	for _, c := range commands {
		fmt.Fprintf(&b, "  %-24s %s\n", c.Name, c.Description)
	}
	return b.String()
}
