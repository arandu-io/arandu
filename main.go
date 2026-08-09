// Command arandu is the entry point of this application.
//
// It is both halves of what is usually two artifacts, a front controller and a
// console script: the binary is the server, and the same binary is the command
// line. There is no front controller and no document root, because in Go the
// process listens itself.
//
// Almost nothing is here. The wiring is bootstrap/app.go and the commands are
// bootstrap/console.go, for the reason Laravel's index.php is twenty lines:
// `main` cannot be imported, so anything living here is anything a test cannot
// reach -- and the tests worth having boot the whole application.
//
// `aru serve` runs this binary with that argument, because only this binary
// knows which modules exist.
package main

import (
	"log"
	"os"

	"github.com/arandu-io/arandu/bootstrap"
)

func main() {
	// The arguments are read together, because reading them apart is how
	// `go run .` -- no command, no arguments, the first thing anybody types
	// after a clone -- panicked on os.Args[2:] before it printed a line.
	command, args := "serve", []string{}
	if len(os.Args) > 1 {
		command, args = os.Args[1], os.Args[2:]
	}

	if err := bootstrap.Dispatch(command, args); err != nil {
		log.Fatal(err)
	}
}
