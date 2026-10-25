// Example resource. Remove with the list under "The example resource" in README.md.

package models

import (
	"log/slog"
	"time"

	"github.com/arandu-io/hesape/database/model"
)

// Note is one row of notes.
//
// It embeds the model, so a row returned by a query carries the connection and
// can be saved again. Build a new row with Notes(db).New(): a struct
// literal has no connection and its write methods return model.ErrUnwired.
//
// Notes, NoteQuery and NoteCollection
// are generated beside this file, in NoteQuery.go, by aru model:build.
type Note struct {
	model.Model

	ID       string `db:"id"`
	TenantID string `db:"tenant_id"`
	UserID   string `db:"user_id"`
	Title    string `db:"title"`
	Body     string `db:"body"`
	Pinned   bool   `db:"pinned"`
	// PublishedAt is when the note was published, and nil while it is a draft.
	// Only Publish, below, sets it: no request field reaches it.
	PublishedAt *time.Time `db:"published_at"`
	CreatedAt   time.Time  `db:"created_at"`
	UpdatedAt   time.Time  `db:"updated_at"`
}

// noteTable is the table Note is a row of.
//
// UniqueIDs makes the primary key text the model fills on insert.
// The tenant scope is left at its tenant_id default.
var noteTable = model.NewTable(model.TableSpec{
	Name:      "notes",
	New:       func() model.Entity { return new(Note) },
	UniqueIDs: true,
	// arandu:begin custom
	// Hidden, PerPage, Scopes, Events and the rest of model.TableSpec go here.
	// arandu:end custom
})

// LogValue implements slog.LogValuer, so passing the whole entity to a log call
// records the identifiers and nothing else. Add any sensitive field to the
// custom block below and it stays out of logs, dumps and the debug page.
func (n Note) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("id", n.ID),
		slog.String("tenant", n.TenantID),
	)
}

// arandu:begin custom
// Local scopes are methods on *NoteQuery, relations are registered
// on noteTable in an init function, and MarshalJSON, computed fields
// and anything else about this entity go here too.

// OwnedBy reports whether the note was written by the subject with this id. A
// note has an owner the moment it is stored, so an empty id owns nothing.
func (n Note) OwnedBy(userID string) bool { return userID != "" && n.UserID == userID }

// Published reports whether the note has been published.
func (n Note) Published() bool { return n.PublishedAt != nil }

// Publish is the transition from draft to published, at the time it is given.
//
// It is a rule of the entity and nothing else: it changes this row's fields
// and reads no clock, no database and no Grant. NoteService.Publish decides
// who may ask, passes the time, and saves the row with the event beside it.
func (n *Note) Publish(at time.Time) error {
	if n.Published() {
		return ErrNoteAlreadyPublished
	}
	n.PublishedAt = &at
	return nil
}

// ErrNoteAlreadyPublished refuses a second publication. The router answers it
// 409: the request was understood, and the row is not in the state it needs.
var ErrNoteAlreadyPublished error = noteConflict("this note is already published")

// noteConflict is an error about the state of a note. Its HTTPStatus is what
// the router answers, so no action maps it by hand.
type noteConflict string

// Error implements error.
func (e noteConflict) Error() string { return string(e) }

// HTTPStatus is 409 Conflict.
func (noteConflict) HTTPStatus() int { return 409 }

// arandu:end custom
