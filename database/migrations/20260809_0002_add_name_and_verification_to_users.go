package migrations

import (
	"context"

	"github.com/arandu-io/hesape/database/migrations"
	"github.com/arandu-io/hesape/database/schema"
)

// AddNameAndVerificationToUsers adds rollout-safe user profile columns.
type AddNameAndVerificationToUsers struct{ migrations.BaseMigration }

// GetName returns the historical migration identity.
func (AddNameAndVerificationToUsers) GetName() string {
	return "20260809_0002_add_name_and_verification_to_users"
}

// Up adds nullable profile and verification columns.
func (AddNameAndVerificationToUsers) Up(ctx context.Context, conn migrations.Connection) error {
	return conn.Schema().Table(ctx, "users", func(table *schema.Blueprint) {
		table.String("name").Nullable()
		table.Timestamp("verified_at").Nullable()
	})
}

// Down removes the profile and verification columns.
func (AddNameAndVerificationToUsers) Down(ctx context.Context, conn migrations.Connection) error {
	return conn.Schema().Table(ctx, "users", func(table *schema.Blueprint) {
		table.DropColumn("name", "verified_at")
	})
}

func init() { migrations.Register(AddNameAndVerificationToUsers{}) }

var _ migrations.ReversibleMigration = AddNameAndVerificationToUsers{}
