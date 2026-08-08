// Package models holds this application's domain types.
//
// There is no Active Record here. A model is a struct with fields and behaviour;
// it has no save(), no find() and no static query builder, and it does not know
// a database exists. Persistence is a repository, and every repository method
// takes a security.Grant -- which is what makes a query without a policy
// impossible to write rather than merely discouraged.
package models

import (
	"context"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/modules/auth"
	"github.com/arandu-io/framework/security"
)

// User is the authenticated account.
//
// It is an alias of the auth module's type, not a copy. Two structs describing
// one table is two ways to describe one thing (RULE 9), and the copy is the one
// that drifts: the module owns the columns, the migration and the hashing, and
// this name is here so app/Models/User.go is where the user model is found.
//
// The password hash never leaves the type: User implements MarshalJSON and
// LogValue so a dump, a log line or a JSON response cannot carry it.
type User = auth.User

// The roles this application recognises. Strings, because that is what the
// column holds and what a Policy compares against.
const (
	// RoleAdmin may administer the tenant.
	RoleAdmin = "admin"
	// RoleMember is an ordinary account.
	RoleMember = "member"
)

// UserRepository is the only door to the users table.
//
// Every method takes a Grant, including the reads. A read path without a policy
// is a cross-tenant data leak with a technical name (RULE 17), so List and Find
// are on this interface for exactly the same reason Create is.
//
// It is declared here, next to the model, and implemented in app/Repositories --
// or, as below, by the module that owns the table. The interface is what lets a
// service be tested against a fake without a database.
type UserRepository interface {
	// Find returns one user by id, scoped to the grant's tenant.
	Find(ctx context.Context, g security.Grant, id string) (User, error)
	// FindByEmail is how a sign-in looks an account up.
	FindByEmail(ctx context.Context, g security.Grant, email string) (User, error)
	// Create stores a new user under the grant's tenant.
	Create(ctx context.Context, g security.Grant, u User) (User, error)
	// Update replaces a user's mutable fields.
	Update(ctx context.Context, g security.Grant, u User) (User, error)
	// Delete removes one user.
	Delete(ctx context.Context, g security.Grant, id string) error
	// List returns a page of users, filtered by the grant's tenant.
	List(ctx context.Context, g security.Grant, q data.Query) ([]User, error)
}

// The auth module's repository is the implementation this application uses. The
// assertion is compile-time: if the contract above and the module below ever
// disagree, the build stops here instead of at the first call site.
var _ UserRepository = (*auth.UserRepo)(nil)
