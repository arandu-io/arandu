package routes

import (
	"context"
	"time"

	"github.com/arandu-io/hesape/auth"
	"github.com/arandu-io/hesape/console"
	"github.com/arandu-io/hesape/foundation"
	"github.com/arandu-io/hesape/queue"

	commands "github.com/arandu-io/arandu/app/Console/Commands"
	appjobs "github.com/arandu-io/arandu/app/Jobs"
	policies "github.com/arandu-io/arandu/app/Policies"
	services "github.com/arandu-io/arandu/app/Services"
)

// ConsoleDeps is what this application's commands and scheduled tasks reach,
// built by app/Providers.
type ConsoleDeps struct {
	Notes  *services.NoteService
	Queue  queue.Queue
	Tenant string
}

// Commands are this application's own console commands. The code of each lives
// in app/Console/Commands.
//
// The framework's commands -- serve, routes, migrate, db:seed, queue:work,
// schedule:list and the rest -- are not here: the kernel composes them, and a
// name declared here that it already answers stops the boot naming both.
func Commands(d ConsoleDeps) []console.Command {
	return []console.Command{
		// arandu:begin custom
		// The example resource's command. Remove it with the list under "The
		// example resource" in README.md.
		commands.NotesDigest(d.Queue, d.Tenant),
		// arandu:end custom
	}
}

// Schedule is this application's scheduled work, beside what the registered
// modules declare. The scheduler runs it in the process that serves.
func Schedule(d ConsoleDeps) []foundation.Task {
	return []foundation.Task{
		// arandu:begin custom
		// The example resource's nightly digest. Remove it with the list under
		// "The example resource" in README.md.
		{
			ID:        appjobs.SendNotesDigestName,
			Spec:      "0 2 * * *",
			Scope:     foundation.PerTenant,
			Singleton: true,
			Timeout:   time.Minute,
			Action:    policies.NoteList,
			Run: func(ctx context.Context, g auth.Grant) error {
				return appjobs.DispatchSendNotesDigest(ctx, d.Queue, g, appjobs.SendNotesDigest{
					Since: time.Now().Add(-24 * time.Hour),
				})
			},
		},
		// arandu:end custom
	}
}
