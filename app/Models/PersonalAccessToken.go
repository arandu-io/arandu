package models

import (
	"log/slog"
	"time"

	"github.com/arandu-io/hesape/database/model"
)

// PersonalAccessToken is one row of personal_access_tokens: a bearer token an
// account issued for a program to act as it, kept as the digest of the token
// and never as the token. The token itself is shown once, when it is issued.
//
// It embeds the model, so a row returned by a query carries the connection and
// can be saved again. Build a new row with PersonalAccessTokens(db).New(): a struct
// literal has no connection and its write methods return model.ErrUnwired.
//
// PersonalAccessTokens, PersonalAccessTokenQuery and PersonalAccessTokenCollection
// are generated beside this file, in PersonalAccessTokenQuery.go, by aru model:build.
type PersonalAccessToken struct {
	model.Model

	ID       string `db:"id"`
	TenantID string `db:"tenant_id"`
	// UserID is the account the token acts as. A request that carries the
	// token is that account, in this tenant, and the policies decide the rest.
	UserID string `db:"user_id"`
	// Name is what the account called the token, to recognise it in a list.
	Name string `db:"name"`
	// TokenDigest is middleware.DigestToken of the token, as hexadecimal: what
	// RequireToken hands the resolver and the only form the token is stored in.
	TokenDigest string `db:"token_digest"`
	// ExpiresAt is when the token stops being accepted, and nil for a token
	// that lasts until it is revoked.
	ExpiresAt *time.Time `db:"expires_at"`
	CreatedAt time.Time  `db:"created_at"`
	UpdatedAt time.Time  `db:"updated_at"`
}

// personalAccessTokenTable is the table PersonalAccessToken is a row of.
//
// UniqueIDs makes the primary key text the model fills on insert.
// The tenant scope is left at its tenant_id default.
var personalAccessTokenTable = model.NewTable(model.TableSpec{
	Name:      "personal_access_tokens",
	New:       func() model.Entity { return new(PersonalAccessToken) },
	UniqueIDs: true,
	// arandu:begin custom
	// Hidden, PerPage, Scopes, Events and the rest of model.TableSpec go here.
	// arandu:end custom
})

// LogValue implements slog.LogValuer, so passing the whole entity to a log call
// records the identifiers and nothing else. Add any sensitive field to the
// custom block below and it stays out of logs, dumps and the debug page.
func (p PersonalAccessToken) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("id", p.ID),
		slog.String("tenant", p.TenantID),
	)
}

// arandu:begin custom
// Local scopes are methods on *PersonalAccessTokenQuery, relations are registered
// on personalAccessTokenTable in an init function, and MarshalJSON, computed fields
// and anything else about this entity go here too.

// OwnedBy reports whether the token acts as the account with this id. A token
// is issued to an account, so an empty id owns nothing.
func (p PersonalAccessToken) OwnedBy(userID string) bool {
	return userID != "" && p.UserID == userID
}

// ExpiredAt reports whether the token is no longer accepted at the time given.
// The time is an argument: the entity reads no clock of its own.
func (p PersonalAccessToken) ExpiredAt(now time.Time) bool {
	return p.ExpiresAt != nil && !now.Before(*p.ExpiresAt)
}

// arandu:end custom
