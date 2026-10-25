// Example resource. Remove with the list under "The example resource" in README.md.

package models

import (
	"log/slog"
	"time"

	"github.com/arandu-io/hesape/database/model"
)

// Comment is one row of comments.
//
// It embeds the model, so a row returned by a query carries the connection and
// can be saved again. Build a new row with Comments(db).New(): a struct
// literal has no connection and its write methods return model.ErrUnwired.
//
// Comments, CommentQuery and CommentCollection
// are generated beside this file, in CommentQuery.go, by aru model:build.
type Comment struct {
	model.Model

	ID        string    `db:"id"`
	TenantID  string    `db:"tenant_id"`
	NoteID    string    `db:"note_id"`
	UserID    string    `db:"user_id"`
	Body      string    `db:"body"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

// commentTable is the table Comment is a row of.
//
// UniqueIDs makes the primary key text the model fills on insert.
// The tenant scope is left at its tenant_id default.
var commentTable = model.NewTable(model.TableSpec{
	Name:      "comments",
	New:       func() model.Entity { return new(Comment) },
	UniqueIDs: true,
	// arandu:begin custom
	// Hidden, PerPage, Scopes, Events and the rest of model.TableSpec go here.
	// arandu:end custom
})

// LogValue implements slog.LogValuer, so passing the whole entity to a log call
// records the identifiers and nothing else. Add any sensitive field to the
// custom block below and it stays out of logs, dumps and the debug page.
func (co Comment) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("id", co.ID),
		slog.String("tenant", co.TenantID),
	)
}

// arandu:begin custom
// Local scopes are methods on *CommentQuery, relations are registered
// on commentTable in an init function, and MarshalJSON, computed fields
// and anything else about this entity go here too.
//
// The rules of the entity itself go here as well: an invariant, a derived
// value, a transition that changes only this row's fields. They are pure -- no
// database, no network, no Grant, and no clock read here.

// OwnedBy reports whether the comment was written by the subject with this id.
// A comment has an author the moment it is stored, so an empty id owns nothing.
func (co Comment) OwnedBy(userID string) bool { return userID != "" && co.UserID == userID }

// arandu:end custom
