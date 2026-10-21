// Command arandu is the entry point of this application: the server and the
// command line in one binary. It loads the application and hands the command
// to the framework's console kernel; no arguments is serve.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/arandu-io/arandu/bootstrap"
)

// version and commit are stamped by the build. See the Dockerfile.
var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	app, err := bootstrap.App(version, commit)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(app.HandleCommand(context.Background(), os.Args[1:]))
}
