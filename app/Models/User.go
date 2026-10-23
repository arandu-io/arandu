// Package models holds this application's domain types.
package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/arandu-io/hesape/auth"
	"github.com/arandu-io/hesape/auth/account"
	"github.com/arandu-io/hesape/database/model"
)

// User is an account owned by this application.
//
// It embeds the model, so a row returned by a query carries the connection and
// can be saved again. Build a new row with Users(db).New(): a struct literal
// has no connection and its write methods return model.ErrUnwired.
//
// Users, UserQuery and UserCollection are generated beside this file, in
// UserQuery.go, by aru model:build.
type User struct {
	model.Model

	ID       string `db:"id"`
	TenantID string `db:"tenant_id"`
	Name     string `db:"name"`
	Email    string `db:"email"`
	Password string `db:"password"`

	// Roles is stored as a JSON array of text in one portable column. The type
	// reads and writes that column itself, so there is one representation.
	Roles Roles `db:"roles"`

	VerifiedAt *time.Time `db:"verified_at"`
	CreatedAt  time.Time  `db:"created_at"`
}

// The roles this application recognises. Strings, because that is what the
// column holds and what a Policy compares against.
//
// They are the fixed roles of the deployment's one tenant. Roles people grant
// to others -- permission groups, a member's role in an organization -- belong
// to github.com/hyz-is/arandu-permission, which fills the subject's roles
// through User.Subject; once it is installed nothing writes this column.
const (
	// RoleAdmin may administer the tenant.
	RoleAdmin = "admin"
	// RoleMember is an ordinary account.
	RoleMember = "member"
)

// userTable is the table User is a row of: the application-owned users table.
//
// UniqueIDs makes the primary key text the model generates on insert, so
// nothing writes an id by hand. The table has no updated_at column. The tenant
// scope is left at its tenant_id default.
var userTable = model.NewTable(model.TableSpec{
	Name:            "users",
	New:             func() model.Entity { return new(User) },
	UniqueIDs:       true,
	UpdatedAtColumn: model.NoColumn,
})

// Roles is the set of roles an account carries, stored as a JSON array of text.
//
// It implements sql.Scanner and driver.Valuer, which is how the model reads and
// writes a column whose Go shape is not a scalar: the field is the value the
// application uses, and the column is its JSON text.
type Roles []string

// Value writes the roles as a JSON array. No roles is "[]", never NULL.
func (r Roles) Value() (driver.Value, error) {
	if r == nil {
		r = Roles{}
	}
	b, err := json.Marshal([]string(r))
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// Scan reads the JSON array the column holds. An empty column is no roles.
func (r *Roles) Scan(src any) error {
	var text []byte
	switch v := src.(type) {
	case nil:
		*r = Roles{}
		return nil
	case string:
		text = []byte(v)
	case []byte:
		text = v
	default:
		return fmt.Errorf("user: roles column holds %T, want JSON text", src)
	}
	if len(text) == 0 {
		*r = Roles{}
		return nil
	}
	var roles []string
	if err := json.Unmarshal(text, &roles); err != nil {
		return fmt.Errorf("user: unreadable roles: %w", err)
	}
	if roles == nil {
		roles = []string{}
	}
	*r = roles
	return nil
}

// Subject returns the session subject derived only from stored account data.
func (u User) Subject() auth.Subject {
	return auth.Subject{
		ID: u.ID, Tenant: u.TenantID, Roles: append([]string(nil), u.Roles...), Verified: u.IsVerified(),
	}
}

// The six readers below are what the account rules of hesape/auth/account
// read a user through: signing in, registering, verifying, resetting and the
// second factor are that package's, written once over these. The row, the
// fill and the insert they also need come from the embedded model.

// AccountID is the primary key.
func (u User) AccountID() string { return u.ID }

// AccountTenant is the tenant the account belongs to.
func (u User) AccountTenant() string { return u.TenantID }

// AccountEmail is the stored address.
func (u User) AccountEmail() string { return u.Email }

// AccountName is the name the account is shown by.
func (u User) AccountName() string { return u.Name }

// PasswordHash is the stored password hash. MarshalJSON and LogValue keep it
// out of every response and log line.
func (u User) PasswordHash() string { return u.Password }

// IsVerified reports whether the address was confirmed.
func (u User) IsVerified() bool { return u.VerifiedAt != nil && !u.VerifiedAt.IsZero() }

// The account rules hand back the *User the users query builds.
var _ account.Account = (*User)(nil)

// MarshalJSON keeps password material and its persistence representation out
// of responses and dumps.
func (u User) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID         string     `json:"id"`
		TenantID   string     `json:"tenant_id"`
		Name       string     `json:"name"`
		Email      string     `json:"email"`
		Roles      []string   `json:"roles"`
		VerifiedAt *time.Time `json:"verified_at,omitempty"`
		CreatedAt  time.Time  `json:"created_at"`
	}{
		ID: u.ID, TenantID: u.TenantID, Name: u.Name, Email: u.Email,
		Roles: append([]string(nil), u.Roles...), VerifiedAt: u.VerifiedAt, CreatedAt: u.CreatedAt,
	})
}

// LogValue records only identifiers when a whole User reaches structured logs.
func (u User) LogValue() slog.Value {
	return slog.GroupValue(slog.String("id", u.ID), slog.String("tenant", u.TenantID))
}

// arandu:begin custom
// Local scopes are methods on *UserQuery, relations are registered on
// userTable in an init function, and anything else about this entity goes
// here too.

// arandu:end custom
