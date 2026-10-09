package policies

import (
	"context"
	"fmt"

	"github.com/arandu-io/hesape/auth"

	models "github.com/arandu-io/arandu/app/Models"
)

// The actions of PersonalAccessToken. Constants rather than strings at the call site: a
// typo in an action name would silently authorize nothing, or worse, everything.
//
// They carry the entity in the name because every policy in the application
// lives in this package now, and five constants called ActionView would not
// compile past the first module.
const (
	// PersonalAccessTokenView is reading one record.
	PersonalAccessTokenView auth.Action = "personal_access_token.view"
	// PersonalAccessTokenList is paging through the records.
	PersonalAccessTokenList auth.Action = "personal_access_token.list"
	// PersonalAccessTokenCreate is adding one.
	PersonalAccessTokenCreate auth.Action = "personal_access_token.create"
	// PersonalAccessTokenUpdate is changing one.
	PersonalAccessTokenUpdate auth.Action = "personal_access_token.update"
	// PersonalAccessTokenDelete is removing one: revoking the token.
	PersonalAccessTokenDelete auth.Action = "personal_access_token.delete"
	// PersonalAccessTokenResolve is finding the account a presented token acts
	// as. Nobody is signed in yet when it happens -- the token is what will say
	// who -- so it is never asked of this policy: it names the system grant
	// PersonalAccessTokenService.ResolveToken reads under.
	PersonalAccessTokenResolve auth.Action = "personal_access_token.resolve"
)

// PersonalAccessTokenPolicy is the only authority over who does what with PersonalAccessToken.
//
// IT DENIES EVERYTHING. That is deliberate: a generated policy that allowed
// anything would be a hole shipped by default, in every project that ran the
// generator. Open what this module actually needs, and nothing else.
type PersonalAccessTokenPolicy struct{}

// Compile-time proof that the policy answers about this entity and no other.
var _ auth.Policy[models.PersonalAccessToken] = PersonalAccessTokenPolicy{}

// Can decides whether the subject may perform the action.
func (PersonalAccessTokenPolicy) Can(ctx context.Context, s auth.Subject, a auth.Action, p models.PersonalAccessToken) error {
	// Tenant isolation comes first and applies to every action. Without it every
	// check below would be pointless in a multi-tenant system.
	if p.ID != "" && p.TenantID != s.Tenant {
		return fmt.Errorf("personal_access_token belongs to another tenant")
	}

	// arandu:begin custom
	// Every action needs somebody signed in to the tenant: the zero subject is
	// refused before any rule below.
	if s.ID == "" || s.Tenant == "" {
		return fmt.Errorf("a personal access token is issued and revoked by somebody signed in")
	}

	switch a {
	// An account issues tokens for itself and revokes its own. The service
	// asks about the row it read, or about the one it is about to write -- so
	// a token that acts as another account is issued and revoked by nobody.
	// Listing and showing tokens are closed until a screen needs them.
	case PersonalAccessTokenCreate, PersonalAccessTokenDelete:
		if p.OwnedBy(s.ID) {
			return nil
		}
		return fmt.Errorf("a personal access token is managed only by the account it acts as")
	}
	// arandu:end custom

	return fmt.Errorf("no rule allows %s on personal_access_token", a)
}
