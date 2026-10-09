// Example resource. Remove with the list under "The example resource" in README.md.

package feature_test

import (
	"context"
	"testing"

	"github.com/arandu-io/hesape/auth"

	models "github.com/arandu-io/arandu/app/Models"
	policies "github.com/arandu-io/arandu/app/Policies"
	factories "github.com/arandu-io/arandu/database/factories"
)

// TestCommentsAreScopedByTenant stores comments for one tenant and reads
// them with another tenant's Grant. The rows exist and the queries run, and
// nothing comes back: the model takes the tenant from the Grant and from
// nowhere else, so to a second tenant the table is empty.
//
// It is the evidence for the line TestEveryTableWithATenantColumnIsOneThatFiltersByTenant
// asks for: that one reads the catalogue and requires a claim for comments,
// this one proves the claim.
func TestCommentsAreScopedByTenant(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	stored, err := factories.Comments(db).Count(2).Create(ctx, auth.SystemGrant(policies.CommentCreate, "tenant-a"))
	if err != nil {
		t.Fatalf("storing comments for tenant-a: %v", err)
	}

	theirs := auth.SystemGrant(policies.CommentList, "tenant-b")
	found, err := models.Comments(db).Get(ctx, theirs)
	if err != nil {
		t.Fatalf("listing as tenant-b: %v", err)
	}
	if len(found) != 0 {
		t.Errorf("tenant-b read %d of tenant-a's comments", len(found))
	}
	for _, row := range stored {
		if _, err := models.Comments(db).FindOrFail(ctx, theirs, row.ID); err == nil {
			t.Errorf("tenant-b found tenant-a's comment %s by its id", row.ID)
		}
	}

	ours, err := models.Comments(db).Get(ctx, auth.SystemGrant(policies.CommentList, "tenant-a"))
	if err != nil || len(ours) < len(stored) {
		t.Errorf("tenant-a read %d of its %d comments (%v): the scope hides rows from their own tenant", len(ours), len(stored), err)
	}
}

// arandu:begin custom
// arandu:end custom
