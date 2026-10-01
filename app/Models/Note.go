// Example resource. Remove with the list under "The example resource" in README.md.

package models

import (
	"log/slog"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/hesape/database/model"
)

// Note is one row of notes.
//
// It embeds the model, so a row returned by a query carries the connection and
// can be saved again. Build new rows through Notes: a struct literal has
// no connection and its write methods return model.ErrUnwired.
type Note struct {
	model.Model[Note]

	ID        string    `db:"id"`
	TenantID  string    `db:"tenant_id"`
	UserID    string    `db:"user_id"`
	Title     string    `db:"title"`
	Body      string    `db:"body"`
	Pinned    bool      `db:"pinned"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

// Notes returns the configured model for notes.
//
// UseUniqueIDs makes the primary key text the model fills on insert.
// The tenant scope is left at its tenant_id default.
func Notes(db *data.DB) *model.Model[Note] {
	return model.NewModel[Note]("notes", db, db.GetQueryGrammar(), db.GetPostProcessor()).UseUniqueIDs()
}

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
// MarshalJSON, computed fields and anything else about this entity go here.

// OwnedBy reports whether the note was written by the subject with this id. A
// note has an owner the moment it is stored, so an empty id owns nothing.
func (n Note) OwnedBy(userID string) bool { return userID != "" && n.UserID == userID }

// arandu:end custom
