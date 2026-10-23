package unit_test

import (
	"testing"

	"github.com/arandu-io/hesape/auth"
	"github.com/arandu-io/hesape/auth/account"

	policies "github.com/arandu-io/arandu/app/Policies"
)

// TestThePoliciesThatStayDeclareTheirWrites: the account and token policies
// are not part of the example, and each says which of its actions write, so
// a Grant issued to view an account or a token cannot change one. An action a
// policy does not name reads.
func TestThePoliciesThatStayDeclareTheirWrites(t *testing.T) {
	for _, c := range []struct {
		policy auth.Writes
		action auth.Action
		writes bool
	}{
		{policies.UserPolicy{}, account.ActionUserCreate, true},
		{policies.UserPolicy{}, account.ActionUserUpdate, true},
		{policies.UserPolicy{}, account.ActionUserDelete, true},
		{policies.UserPolicy{}, account.ActionUserView, false},
		{policies.UserPolicy{}, account.ActionUserNamesPublic, false},
		{policies.UserPolicy{}, "user.unknown", false},
		{policies.PersonalAccessTokenPolicy{}, policies.PersonalAccessTokenCreate, true},
		{policies.PersonalAccessTokenPolicy{}, policies.PersonalAccessTokenUpdate, true},
		{policies.PersonalAccessTokenPolicy{}, policies.PersonalAccessTokenDelete, true},
		{policies.PersonalAccessTokenPolicy{}, policies.PersonalAccessTokenView, false},
		{policies.PersonalAccessTokenPolicy{}, policies.PersonalAccessTokenList, false},
		{policies.PersonalAccessTokenPolicy{}, policies.PersonalAccessTokenResolve, false},
	} {
		if got := c.policy.Writes(c.action); got != c.writes {
			t.Errorf("%T.Writes(%s) = %v, want %v", c.policy, c.action, got, c.writes)
		}
	}
}
