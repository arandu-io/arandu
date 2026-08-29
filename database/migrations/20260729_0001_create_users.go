package migrations

import (
	"context"

	"github.com/arandu-io/hesape/database/migrations"
	"github.com/arandu-io/hesape/database/schema"
)

// CreateUsers creates the application-owned users table.
type CreateUsers struct{ migrations.BaseMigration }

// GetName returns the historical migration identity.
func (CreateUsers) GetName() string { return "20260729_0001_create_users" }

// Up creates the users table and its tenant-scoped indexes.
func (CreateUsers) Up(ctx context.Context, conn migrations.Connection) error {
	return conn.Schema().Create(ctx, "users", func(table *schema.Blueprint) {
		table.String("id").Primary()
		table.String("tenant_id")
		table.String("email")
		table.Text("password")
		table.Text("roles")
		table.Timestamp("created_at")

		table.Unique([]string{"tenant_id", "email"})
		table.Index([]string{"tenant_id", "created_at", "id"}, "users_tenant_created_idx")
	})
}

// Down drops the users table.
func (CreateUsers) Down(ctx context.Context, conn migrations.Connection) error {
	return conn.Schema().DropIfExists(ctx, "users")
}

func init() { migrations.Register(CreateUsers{}) }

var _ migrations.ReversibleMigration = CreateUsers{}
