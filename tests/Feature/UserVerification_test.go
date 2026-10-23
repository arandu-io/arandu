package feature_test

import (
	"context"
	"testing"

	"github.com/arandu-io/arandu/tests"
)

// TestARegistrationStoresNullUntilTheAddressIsVerified is the half of the
// verification the framework's ModelPersistence kit does not read: the column.
// The kit holds the model, its subject and its JSON to one answer; this holds
// the row this application's users table keeps to it -- SQL NULL for an
// address nobody confirmed, an instant once somebody did -- on the engine the
// suite runs on.
func TestARegistrationStoresNullUntilTheAddressIsVerified(t *testing.T) {
	server := tests.Serve(t)
	user := tests.Register(t, server.App, "Ana Lima", "ana@example.test", "a-long-enough-password")
	ctx := context.Background()

	verifiedAt := func() any {
		t.Helper()
		var stored any
		if err := server.DB().QueryRowContext(ctx, `SELECT verified_at FROM users WHERE id = ?`, user.ID).Scan(&stored); err != nil {
			t.Fatalf("reading the stored verification: %v", err)
		}
		return stored
	}
	if stored := verifiedAt(); stored != nil {
		t.Fatalf("a new registration stored verified_at = %v, want SQL NULL", stored)
	}

	if _, changed, err := server.App.Users.MarkVerified(ctx, user.TenantID, user.ID, user.Email); err != nil || !changed {
		t.Fatalf("verifying the address = %v, %v; want a change", changed, err)
	}
	if stored := verifiedAt(); stored == nil {
		t.Fatal("a verified address left verified_at NULL")
	}
}
