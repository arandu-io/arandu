package feature_test

import (
	"context"
	"errors"
	"testing"

	"github.com/arandu-io/framework/security"
	nativeauth "github.com/arandu-io/hesape/auth"

	models "github.com/arandu-io/arandu/app/Models"
	policies "github.com/arandu-io/arandu/app/Policies"
	services "github.com/arandu-io/arandu/app/Services"
	"github.com/arandu-io/arandu/bootstrap"
	factories "github.com/arandu-io/arandu/database/factories"
	"github.com/arandu-io/arandu/tests"
)

// TestTheUserFactoryMakesMembersNobodyCanSignInAs stores a batch through the
// factory and reads it back through the user service: every account is in the
// Grant's tenant, distinct, an ordinary member, and refuses every password.
func TestTheUserFactoryMakesMembersNobodyCanSignInAs(t *testing.T) {
	_, db := tests.App(t)
	ctx := context.Background()
	tenant := bootstrap.Tenant()

	made, err := factories.UserFactory(db).Count(3).Create(ctx, security.SystemGrant(policies.ActionUserCreate, tenant))
	if err != nil {
		t.Fatalf("creating users through the factory: %v", err)
	}
	if len(made) != 3 {
		t.Fatalf("the factory made %d users, want 3", len(made))
	}

	users := services.NewUserService(db)
	seen := map[string]bool{}
	for _, user := range made {
		if seen[user.Email] {
			t.Fatalf("two made users share the address %s", user.Email)
		}
		seen[user.Email] = true

		stored, err := users.Lookup(ctx, tenant, user.Email)
		if err != nil {
			t.Fatalf("reading %s back in the Grant's tenant: %v", user.Email, err)
		}
		if stored.ID != user.ID || stored.TenantID != tenant || !stored.Verified() {
			t.Errorf("stored account = %+v, want id %s, verified, in %s", stored, user.ID, tenant)
		}
		if len(stored.Roles) != 1 || stored.Roles[0] != models.RoleMember {
			t.Errorf("stored roles = %v, want [%s]", stored.Roles, models.RoleMember)
		}
		for _, password := range []string{"", factories.UnusablePassword, "a-long-enough-password"} {
			_, err := users.VerifyCredentials(ctx, tenant, user.Email, password, "127.0.0.1")
			if !errors.Is(err, nativeauth.ErrInvalidCredentials) {
				t.Errorf("signing in as a made user with %q answered %v, want ErrInvalidCredentials", password, err)
			}
		}
	}
}
