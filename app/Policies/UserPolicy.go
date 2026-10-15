// Package policies holds this application's authorization decisions.
package policies

import (
	"context"
	"fmt"

	"github.com/arandu-io/hesape/auth"
	"github.com/arandu-io/hesape/auth/account"

	"github.com/arandu-io/arandu/app/Models"
)

// UserPolicy is the only authority over application users.
//
// The account rules of hesape/auth/account ask it through account.PolicyOf,
// with the *User the users query built, and run every operation a signed-in
// subject makes on an account under the Grant it issues. The actions are that
// package's, so the strings this policy compares against and the ones it is
// asked about are one declaration.
type UserPolicy struct{}

// Can decides whether subject may perform action on user and denies by default.
func (UserPolicy) Can(_ context.Context, subject auth.Subject, action auth.Action, user *models.User) error {
	if user == nil {
		return fmt.Errorf("no account to decide about")
	}
	if action == account.ActionUserNamesPublic {
		if subject.Tenant == "" || user.TenantID == "" || user.TenantID != subject.Tenant {
			return fmt.Errorf("public user names require the reader's tenant")
		}
		return nil
	}

	if user.TenantID != "" && user.TenantID != subject.Tenant {
		return fmt.Errorf("resource belongs to another tenant")
	}

	switch action {
	case account.ActionUserView, account.ActionUserUpdate:
		if subject.ID == user.ID || subject.HasRole(models.RoleAdmin) {
			return nil
		}
	case account.ActionUserCreate:
		if subject.HasRole(models.RoleAdmin) || subject.IsGuest() && len(user.Roles) == 0 {
			return nil
		}
	case account.ActionUserDelete:
		if subject.HasRole(models.RoleAdmin) {
			return nil
		}
	}
	return fmt.Errorf("insufficient role for %s", action)
}

var _ auth.Policy[*models.User] = UserPolicy{}
