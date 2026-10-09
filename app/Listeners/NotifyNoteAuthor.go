// Example resource. Remove with the list under "The example resource" in README.md.

package listeners

import (
	"context"

	"github.com/arandu-io/hesape/auth"
	"github.com/arandu-io/hesape/events"
	"github.com/arandu-io/hesape/log"
	hnotifications "github.com/arandu-io/hesape/notifications"

	notifications "github.com/arandu-io/arandu/app/Notifications"
)

// NotifyNoteAuthor answers `note.published`: it tells the note's author, in
// the bell menu, that the note is out.
//
// It implements events.Publisher: the relay calls Publish once the event is
// committed and readable, never inside the transaction that wrote it. That is
// why the notification is sent here rather than by NoteService.Publish: a
// publication that rolled back has no event, so nobody is told about it.
type NotifyNoteAuthor struct {
	// The collaborators this listener needs are fields, set where it is wired in
	// bootstrap/app.go. A listener that reaches for a global is a listener no
	// test can pin.
	notifier *hnotifications.Notifier
}

// NewNotifyNoteAuthor returns the listener, sending through the Notifier
// bootstrap/app.go builds with the application's channels.
func NewNotifyNoteAuthor(notifier *hnotifications.Notifier) *NotifyNoteAuthor {
	return &NotifyNoteAuthor{notifier: notifier}
}

// Compile-time proof that this satisfies what the relay calls.
var _ events.Publisher = (*NotifyNoteAuthor)(nil)

// Publish handles one committed event.
//
// # Delivery is at-least-once
//
// The relay can hand the same event twice: a publish that succeeded and an
// acknowledgement that did not is indistinguishable from a publish that failed.
// Whatever this does has to be safe to do again -- an upsert keyed by e.ID, a
// row that records what was already sent, an API call with an idempotency key.
// Here a repeat stores a second row in the author's bell menu, which is the
// cost this example accepts; a listener whose repeat costs more keys on e.ID.
//
// Nothing checks this for you. `aru doctor` reads the AST and never runs
// code, so it cannot see whether an effect repeats.
//
// # Returning an error
//
// An error means "not delivered": the relay keeps the event and tries again,
// backing off, and parks it in the dead letter after the attempts run out.
// Returning nil on a failure loses the event silently, which is the one outcome
// the outbox exists to prevent.
func (n *NotifyNoteAuthor) Publish(ctx context.Context, e events.Stored) error {
	// One listener per event name, so this returns early rather than growing a
	// switch. A switch here is how a listener becomes the place every event
	// passes through.
	if e.Name != "note.published" {
		return nil
	}

	// The payload is the JSON the producer stored. Unmarshal it into a struct
	// declared HERE, not shared with the producer: a consumer that compiles
	// against the producer's type is a consumer that has to be deployed with it.
	log.For(ctx).Info("event received", "event", e.Name, "id", e.ID)

	// arandu:begin custom
	var published struct {
		Title  string `json:"title"`
		Author string `json:"author"`
	}
	if err := e.Decode(&published); err != nil {
		return err
	}

	//arandu:system-grant the relay delivers a committed event with no request and no subject; the Grant is rebuilt from the tenant sealed into the outbox row, for sending a notification and nothing else
	g := auth.SystemGrant(hnotifications.ActionSend, e.TenantID)
	return n.notifier.Send(ctx, g, author(published.Author), notifications.NotePublished{
		NoteID: e.AggregateID,
		Title:  published.Title,
	})
	// arandu:end custom
}

// author is who a note's publication is told to: the account, by the id the
// event carries. The database channel stores a row for an id and needs nothing
// else, so there is no row to read for it.
type author string

// NotifiableID is the account's id.
func (a author) NotifiableID() string { return string(a) }

// NotifiableType names what the id is the id of.
func (author) NotifiableType() string { return "user" }

// RouteFor answers no address: this notification is stored, never sent
// anywhere an address is needed.
func (author) RouteFor(hnotifications.ChannelName) string { return "" }
