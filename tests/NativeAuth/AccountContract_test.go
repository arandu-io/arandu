package nativeauth_test

import (
	"testing"

	"github.com/arandu-io/hesape/auth/account"
	"github.com/arandu-io/hesape/auth/account/accounttest"
	"github.com/arandu-io/hesape/database/model"

	"github.com/arandu-io/arandu/app/Models"
	"github.com/arandu-io/arandu/app/Policies"
)

// TestAccountContract runs the contract of hesape/auth/account against what
// this application hands it: its users table, migrated by its own migrations,
// the *models.User its users query builds, and its user policy.
//
// The rules are the package's; this is what says they hold here. Signing in,
// registering, verifying, resetting, the second factor and its challenge
// budget, and the rows written before the rules were a package -- each runs
// against a fresh database, so a user model that reads the wrong column or a
// policy that refuses a guest's registration fails here rather than at the
// sign-in screen.
func TestAccountContract(t *testing.T) {
	accounttest.Run(t, func(t *testing.T) accounttest.Fixture {
		db := openNativeAuthDatabase(t)
		return accounttest.Fixture{
			DB:     db.app,
			Query:  func() *model.Builder { return models.Users(db.app).Base() },
			Policy: account.PolicyOf(policies.UserPolicy{}),
		}
	})
}
