// Example resource. Remove with the list under "The example resource" in README.md.

package migrations

import (
	"context"

	"github.com/arandu-io/hesape/database/migrations"
	"github.com/arandu-io/hesape/database/schema"
)

// AddPublishedAtToNotes adds columns to the notes table.
//
// The init below registers it, so nothing lists it and no list can fall out of
// date. What still has to be true is that this package reaches the binary: Go
// leaves out a package nobody imports, and an init that is not linked never
// runs.
//
// Every column is added nullable, with no NOT NULL. During a rollout the
// previous binary is still inserting rows and knows nothing about these columns,
// so a NOT NULL without a default fails on its first insert -- and on every row
// already in the table. Backfill, then tighten it in a later migration.
//
// A migration is immutable once published. Changing this one after it has been
// applied anywhere leaves two schemas in the world under one name -- alter it
// with a new migration instead.
type AddPublishedAtToNotes struct{ migrations.BaseMigration }

func init() { migrations.Register(AddPublishedAtToNotes{}) }

// GetName is the migration's identity, and its order. It is immutable: this
// string is what the applied-migrations table records.
func (AddPublishedAtToNotes) GetName() string { return "2026_10_09_000002_add_published_at_to_notes" }

// Up adds the columns.
//
// The Blueprint sends them as one ALTER per engine's rules rather than one
// statement per column composed here, and it spells the column type per engine:
// what used to be a SQLType written into the template is the grammar's answer,
// which is the only place it can be right for all three.
func (AddPublishedAtToNotes) Up(ctx context.Context, conn migrations.Connection) error {
	return conn.Schema().Table(ctx, "notes", func(table *schema.Blueprint) {
		table.Timestamp("published_at").Nullable()
	})
}

// Down drops the columns.
//
// The indexes go first, and that is not tidiness: SQLite refuses to drop a
// column an index still names, so a Down that dropped the column alone would
// fail on the engine a project runs by default. The Blueprint runs its commands
// in the order they were added.
//
// This used to say the opposite -- that dropping the column drops the index on
// all three engines, so no DROP INDEX was needed -- and it was wrong about
// SQLite and produced a migration that could not be rolled back there. It also
// said DROP INDEX was the one spelling the three engines disagree about, which
// was true and is now the grammar's problem rather than this template's.
func (AddPublishedAtToNotes) Down(ctx context.Context, conn migrations.Connection) error {
	return conn.Schema().Table(ctx, "notes", func(table *schema.Blueprint) {
		table.DropColumn("published_at")
	})
}
