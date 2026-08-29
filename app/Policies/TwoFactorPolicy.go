package policies

import (
	"context"
	"fmt"

	"github.com/arandu-io/framework/security"

	"github.com/arandu-io/arandu/app/Models"
)

const (
	// ActionTwoFactorRead permits reading an enrolment, including its encrypted secret.
	ActionTwoFactorRead security.Action = "user.two-factor.read"
	// ActionTwoFactorManage permits changing an enrolment and spending its codes.
	ActionTwoFactorManage security.Action = "user.two-factor.manage"
)

// TwoFactorPolicy restricts enrolment management to the account owner.
type TwoFactorPolicy struct{}

// Can decides whether subject may read or manage the enrolment.
func (TwoFactorPolicy) Can(_ context.Context, subject security.Subject, action security.Action, factor models.TwoFactor) error {
	if factor.TenantID != "" && factor.TenantID != subject.Tenant {
		return fmt.Errorf("resource belongs to another tenant")
	}
	if (action == ActionTwoFactorRead || action == ActionTwoFactorManage) && subject.ID != "" && subject.ID == factor.UserID {
		return nil
	}
	return fmt.Errorf("insufficient role for %s", action)
}

var _ security.Policy[models.TwoFactor] = TwoFactorPolicy{}
