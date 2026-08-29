// Package policies holds this application's authorization decisions.
package policies

import (
	"context"
	"fmt"

	"github.com/arandu-io/framework/security"

	"github.com/arandu-io/arandu/app/Models"
)

// User actions are constants so a misspelling cannot silently widen a policy.
const (
	ActionUserView        security.Action = "user.view"
	ActionUserCreate      security.Action = "user.create"
	ActionUserUpdate      security.Action = "user.update"
	ActionUserDelete      security.Action = "user.delete"
	ActionUserNamesPublic security.Action = "user.names.public"
)

// UserPolicy is the only authority over application users.
type UserPolicy struct{}

// Can decides whether subject may perform action on user and denies by default.
func (UserPolicy) Can(_ context.Context, subject security.Subject, action security.Action, user models.User) error {
	if action == ActionUserNamesPublic {
		if subject.Tenant == "" || user.TenantID == "" || user.TenantID != subject.Tenant {
			return fmt.Errorf("public user names require the reader's tenant")
		}
		return nil
	}

	if user.TenantID != "" && user.TenantID != subject.Tenant {
		return fmt.Errorf("resource belongs to another tenant")
	}

	switch action {
	case ActionUserView, ActionUserUpdate:
		if subject.ID == user.ID || subject.HasRole(models.RoleAdmin) {
			return nil
		}
	case ActionUserCreate:
		if subject.HasRole(models.RoleAdmin) || subject.IsGuest() && len(user.Roles) == 0 {
			return nil
		}
	case ActionUserDelete:
		if subject.HasRole(models.RoleAdmin) {
			return nil
		}
	}
	return fmt.Errorf("insufficient role for %s", action)
}

var _ security.Policy[models.User] = UserPolicy{}
