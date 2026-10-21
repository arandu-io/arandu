// Package commands holds this application's console commands. Each is a
// function that builds a console.Command over what it reaches; the list of
// them is routes/console.go.
package commands

import (
	"context"
	"time"

	"github.com/arandu-io/hesape/auth"
	"github.com/arandu-io/hesape/console"
	"github.com/arandu-io/hesape/queue"

	appjobs "github.com/arandu-io/arandu/app/Jobs"
	policies "github.com/arandu-io/arandu/app/Policies"
)

// NotesDigest queues the nightly digest of the last day's notes now, for one
// tenant, on the path the scheduled task takes. Remove it with the list under
// "The example resource" in README.md.
func NotesDigest(q queue.Queue, tenant string) console.Command {
	return console.Command{
		Name:        "notes:digest",
		Description: "queue the digest of the last day's notes now",
		Run: func(ctx context.Context, o *console.IO) error {
			g := auth.SystemGrant(policies.NoteList, tenant)
			err := appjobs.DispatchSendNotesDigest(ctx, q, g, appjobs.SendNotesDigest{Since: time.Now().Add(-24 * time.Hour)})
			if err != nil {
				return err
			}
			o.Line("queued the notes digest for %s", tenant)
			return nil
		},
	}
}
