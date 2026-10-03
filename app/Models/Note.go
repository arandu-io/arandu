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

	ID        string    `db:"id"`
	TenantID  string    `db:"tenant_id"`
	UserID    string    `db:"user_id"`
	Title     string    `db:"title"`
	Body      string    `db:"body"`
	Pinned    bool      `db:"pinned"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
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

// arandu:end custom
