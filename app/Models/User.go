// Package models holds this application's domain types.
package models

import (
	"crypto/sha256"
	"database/sql/driver"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/database/model"
)

// User is an account owned by this application.
type User struct {
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
const (
	// RoleAdmin may administer the tenant.
	RoleAdmin = "admin"
	// RoleMember is an ordinary account.
	RoleMember = "member"
)

// Users returns the model for the application-owned users table.
//
// The primary key is text the model generates on insert, so nothing writes an
// id by hand. The table has no updated_at column.
func Users(db *data.DB) *model.Model[User] {
	m := model.NewModel[User]("users", db, db.GetQueryGrammar(), db.GetPostProcessor()).UseUniqueIDs()
	m.UpdatedAtColumn = ""
	return m
}

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

// Verified reports whether the address was confirmed.
func (u User) Verified() bool { return u.VerifiedAt != nil && !u.VerifiedAt.IsZero() }

// Subject returns the session subject derived only from stored account data.
func (u User) Subject() security.Subject {
	return security.Subject{
		ID: u.ID, Tenant: u.TenantID, Roles: append([]string(nil), u.Roles...), Verified: u.Verified(),
	}
}

// PasswordFingerprint identifies the complete current password hash without
// exposing that hash in a signed URL or pending cookie.
func (u User) PasswordFingerprint() string {
	sum := sha256.Sum256([]byte(u.Password))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

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
