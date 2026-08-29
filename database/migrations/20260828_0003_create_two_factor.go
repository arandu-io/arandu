package migrations

import (
	"context"

	"github.com/arandu-io/hesape/database/migrations"
	"github.com/arandu-io/hesape/database/schema"
)

const (
	twoFactorTable    = "user_two_factor"
	recoveryCodeTable = "user_recovery_codes"
)

// CreateTwoFactor creates the application-owned second-factor tables.
type CreateTwoFactor struct{ migrations.BaseMigration }

// GetName returns the historical migration identity.
func (CreateTwoFactor) GetName() string { return "20260828_0003_create_two_factor" }

// Up creates enrollment and one-time recovery-code storage.
func (CreateTwoFactor) Up(ctx context.Context, conn migrations.Connection) error {
	if err := conn.Schema().Create(ctx, twoFactorTable, func(table *schema.Blueprint) {
		table.String("user_id").Primary()
		table.String("tenant_id")
		table.Text("secret")
		table.Timestamp("confirmed_at").Nullable()
		table.BigInteger("last_used_step")
		table.Timestamp("created_at")
	}); err != nil {
		return err
	}

	return conn.Schema().Create(ctx, recoveryCodeTable, func(table *schema.Blueprint) {
		table.String("id").Primary()
		table.String("tenant_id")
		table.String("user_id")
		table.Text("code_hash")
		table.Timestamp("used_at").Nullable()
		table.Timestamp("created_at")

		table.Index([]string{"tenant_id", "user_id"}, "user_recovery_codes_owner_idx")
	})
}

// Down drops recovery codes before their owning enrollment table.
func (CreateTwoFactor) Down(ctx context.Context, conn migrations.Connection) error {
	if err := conn.Schema().DropIfExists(ctx, recoveryCodeTable); err != nil {
		return err
	}
	return conn.Schema().DropIfExists(ctx, twoFactorTable)
}

func init() { migrations.Register(CreateTwoFactor{}) }

var _ migrations.ReversibleMigration = CreateTwoFactor{}
