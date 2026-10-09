// Example resource. Remove with the list under "The example resource" in README.md.

package events

import (
	hevents "github.com/arandu-io/hesape/events"
)

// NotePublishedName is the event, in the vocabulary of the domain rather than of the
// database: "note.published", never "note.updated". A consumer that
// has to diff two rows to learn what happened is a consumer coupled to your
// schema.
const NotePublishedName = "note.published"

// NotePublished is what the consumer receives.
//
// Facts that were true when it happened, serialized as JSON. An event that says
// "look it up" is an event that reads a row which has already changed.
type NotePublished struct {
	// Title is the note's title when it was published.
	Title string `json:"title"`
	// Author is the id of the account that wrote the note, and the one the
	// notification of the publication is for.
	Author string `json:"author"`
}

// Event turns the payload into the record the outbox stores.
//
// There is no Dispatch and no Publish, and that is the whole design: the service
// that makes the change stores the event in the SAME transaction as the write,
// and the relay publishes it afterwards.
//
//	err := database.Transaction(ctx, s.db, func(ctx context.Context) error {
//		if _, err := note.Save(ctx, g); err != nil {
//			return err
//		}
//		return s.outbox.Store(ctx, g, []events.Event{appevents.NotePublished{
//			// the payload's fields
//		}.Event(note.ID)})
//	})
//
// Store outside database.Transaction returns ErrNoTransaction on purpose. An
// event stored next to a row that then rolled back is worse than no event, and
// an event stored after the commit is one process crash away from being lost.
func (e NotePublished) Event(aggregateID string) hevents.Event {
	return hevents.Event{
		Name:        NotePublishedName,
		Aggregate:   "note",
		AggregateID: aggregateID,
		Payload:     e,
	}
}

// arandu:begin custom
// Anything about this event that the fields do not say: a computed total, a
// MarshalJSON that renames a key for a consumer you do not control.
// arandu:end custom
