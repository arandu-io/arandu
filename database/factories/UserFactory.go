// Package factories builds domain objects with sensible values.
//
// A seeder or a test says what it cares about and the factory fills the rest.
// What is different is that nothing here touches the database. A factory
// returns a value; storing it is a repository call, and a repository call
// needs a Grant -- so a factory cannot become a back door around the policy
// that guards the table.
package factories

import (
	"fmt"
	"time"

	"github.com/arandu-io/arandu/app/Models"
)

// UserFactory builds users.
//
// The zero value is usable: the defaults below apply, and every field is
// overridable through the With methods.
type UserFactory struct {
	tenant string
	email  string
	roles  []string
}

// NewUserFactory returns a factory for one tenant.
//
// The tenant is required rather than defaulted. A factory that picks its own
// would build rows nobody can reach, and the failure would only show up when
// somebody tried to log in (RULE 14).
func NewUserFactory(tenant string) UserFactory {
	return UserFactory{tenant: tenant, roles: []string{models.RoleMember}}
}

// WithEmail sets the address instead of generating one.
func (f UserFactory) WithEmail(email string) UserFactory {
	f.email = email
	return f
}

// WithRoles sets the roles.
func (f UserFactory) WithRoles(roles ...string) UserFactory {
	f.roles = roles
	return f
}

// Make returns one user, numbered so a batch has distinct addresses.
//
// The Password field is left empty on purpose. A factory that produced a hash
// would need the hashing parameters, and a factory that produced a plaintext
// password would put one in a fixture. The service hashes; this builds the rest.
func (f UserFactory) Make(n int) models.User {
	email := f.email
	if email == "" {
		email = fmt.Sprintf("user%d@example.test", n)
	}
	return models.User{
		TenantID:  f.tenant,
		Email:     email,
		Roles:     append([]string(nil), f.roles...),
		CreatedAt: time.Now().UTC(),
	}
}

// MakeMany returns n users.
func (f UserFactory) MakeMany(n int) []models.User {
	out := make([]models.User, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, f.Make(i))
	}
	return out
}
