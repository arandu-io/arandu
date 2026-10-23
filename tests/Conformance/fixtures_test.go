package conformance_test

import (
	"testing"
	"time"

	"github.com/arandu-io/framework/test/conformance"
	"github.com/arandu-io/hesape/auth/account"
	"github.com/arandu-io/hesape/auth/account/accounttest"
	"github.com/arandu-io/hesape/database"
	"github.com/arandu-io/hesape/database/migrations"
	"github.com/arandu-io/hesape/database/model"

	clients "github.com/arandu-io/arandu/app/Clients"
	models "github.com/arandu-io/arandu/app/Models"
	policies "github.com/arandu-io/arandu/app/Policies"
	appmigrations "github.com/arandu-io/arandu/database/migrations"
	"github.com/arandu-io/arandu/tests"
)

// schemaFixture is this application's users migrations, oldest first, and an
// empty database of the test engine for each run.
func schemaFixture() conformance.SchemaFixture {
	return conformance.SchemaFixture{
		Users: []migrations.Migration{
			appmigrations.CreateUsers{},
			appmigrations.AddNameAndVerificationToUsers{},
		},
		Open: func(t *testing.T) *database.Connection {
			db := tests.EmptyDatabase(t)
			return database.NewConnection(db.Unwrap(), "", "", map[string]any{
				"driver": string(db.Dialect()),
				"name":   "conformance",
			})
		},
	}
}

// accountFixture is a database migrated the way a deploy migrates it --
// served by tests.Serve, so it is the engine the suite runs on -- with this
// application's users query and user policy.
func accountFixture(t *testing.T) accounttest.Fixture {
	t.Helper()
	db := tests.Serve(t).DB()
	return accounttest.Fixture{
		DB:     db,
		Query:  func() *model.Builder { return models.Users(db).Base() },
		Policy: account.PolicyOf(policies.UserPolicy{}),
	}
}

// modelFixture is accountFixture with the roles column this application keeps
// on its users table, written through the user model.
func modelFixture(t *testing.T) conformance.ModelFixture {
	t.Helper()
	return conformance.ModelFixture{
		Fixture: accountFixture(t),
		Role:    models.RoleAdmin,
		GrantRole: func(a account.Account, role string) error {
			a.(*models.User).Roles = models.Roles{role}
			return nil
		},
	}
}

// redacted are the values of this application that carry a secret into a log
// call or a JSON encoding: the account with its password hash, the newsletter
// client's configuration with its token, and the personal access token's row
// with its digest, which is only ever logged.
func redacted() []conformance.Redacted {
	const (
		hash   = "$argon2id$v=19$m=65536,t=3,p=2$c2FsdHNhbHQ$aGFzaC1zZWNyZXQtNGQ5Ng"
		token  = "newsletter-token-7c21e0"
		digest = "9f2c6d1e8b7a4f3c2d1e0f9a8b7c6d5e4f3a2b1c0d9e8f7a6b5c4d3e2f1a0b9c"
	)
	return []conformance.Redacted{
		{
			Name: "the account",
			Value: models.User{
				ID: "user-5b1d", TenantID: "tenant-a", Name: "Ana Lima",
				Email: "ana@example.test", Password: hash, Roles: models.Roles{models.RoleMember},
			},
			Secrets: []string{hash},
			Keeps:   []string{"user-5b1d", "tenant-a"},
		},
		{
			Name:    "the newsletter client's configuration",
			Value:   clients.NewsletterConfig{BaseURL: "https://newsletter.example.test", Token: token, Timeout: 5 * time.Second},
			Secrets: []string{token},
			Keeps:   []string{"https://newsletter.example.test"},
		},
		{
			Name:    "a personal access token",
			Value:   models.PersonalAccessToken{ID: "pat-0e3a", TenantID: "tenant-a", UserID: "user-5b1d", Name: "deploy script", TokenDigest: digest},
			Secrets: []string{digest},
			Keeps:   []string{"pat-0e3a", "tenant-a"},
			LogOnly: true,
		},
	}
}
