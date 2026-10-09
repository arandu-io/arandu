// Example resource. Remove with the list under "The example resource" in README.md.

package notifications

import (
	hnotifications "github.com/arandu-io/hesape/notifications"
	"github.com/arandu-io/hesape/notifications/channels"
	"github.com/arandu-io/hesape/notifications/messages"
)

// NotePublished is one thing this application tells somebody.
//
// It is sent by a service, after the policy issued the Grant, through the
// Notifier bootstrap/app.go builds with the channels the application has:
//
//	err := s.notifier.Send(ctx, g, recipient, notifications.NotePublished{})
//
// What it carries is its fields, filled by the service that sends it. It never
// reads a model or the database itself: by the time it is built, everything it
// says is known.
type NotePublished struct {
	hnotifications.NotificationBase

	// arandu:begin custom
	// What the message needs to say, as fields: Number string, Amount int64.

	// NoteID is the note that was published, for the link the bell menu draws.
	NoteID string
	// Title is the note's title when it was published.
	Title string
	// arandu:end custom
}

// NotePublishedKey is the stable name of this kind of notification. It is written
// to the stored row and is what Suppress silences, so it stays the same when
// the type is renamed.
const NotePublishedKey hnotifications.Key = "note-published"

// Key names the kind.
func (n NotePublished) Key() hnotifications.Key { return NotePublishedKey }

// Via is which channels this notification takes for this recipient. The
// recipient is an argument because the answer can depend on them: somebody who
// turned e-mail off gets the stored row and nothing else, and that decision
// belongs here rather than in a filter downstream.
func (n NotePublished) Via(to hnotifications.Notifiable) []hnotifications.ChannelName {
	return []hnotifications.ChannelName{hnotifications.ChannelDatabase}
}

// ToDatabase is the row the bell menu renders from: the payload stored in the
// notifications table, as JSON, for the recipient to read back.
func (n NotePublished) ToDatabase(to hnotifications.Notifiable) messages.Database {
	return messages.NewDatabase(map[string]any{
		"title": "Note published",
		// arandu:begin custom
		"note_id":    n.NoteID,
		"note_title": n.Title,
		// arandu:end custom
	})
}

// Compile-time proof that the notification answers every channel Via names:
// a channel without its method is a recipient who never receives it.
var (
	_ hnotifications.Notification   = NotePublished{}
	_ channels.DatabaseNotification = NotePublished{}
)
