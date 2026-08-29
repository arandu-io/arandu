// Package models holds this application's domain types.
package models

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
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

	// Roles is the decoded application value. RoleData is the portable JSON
	// text stored in the database; services keep both representations aligned.
	Roles    []string `db:"-"`
	RoleData string   `db:"roles"`

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
func Users(db *data.DB) *model.Model[User] {
	m := model.NewModel[User]("users", db, db.GetQueryGrammar(), db.GetPostProcessor())
	m.KeyType = "string"
	m.Incrementing = false
	m.UpdatedAtColumn = ""
	return m
}

// DecodeRoles reads the portable JSON column into Roles.
func (u *User) DecodeRoles() error {
	u.Roles = nil
	if u.RoleData == "" {
		u.Roles = []string{}
		return nil
	}
	return json.Unmarshal([]byte(u.RoleData), &u.Roles)
}

// EncodeRoles returns Roles as portable JSON text.
func (u User) EncodeRoles() (string, error) {
	roles := u.Roles
	if roles == nil {
		roles = []string{}
	}
	b, err := json.Marshal(roles)
	return string(b), err
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
