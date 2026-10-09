package config

import "time"

// DefaultTenant is the tenant a single-tenant application runs under.
//
// It is a constant rather than an empty string on purpose: auth.SystemGrant
// refuses an empty tenant, because a system grant with no tenant reads across
// every customer of the system. An application that never thinks about tenancy
// still writes every row under this value, so growing into multi-tenant later is
// a change of resolver and not a migration of data.
const DefaultTenant = "00000000-0000-4000-8000-000000000001"

// Auth is who may sign in, and for how long.
//
// What it deliberately does not hold is a list of guards and providers. There is
// one way to authenticate -- the application user service, over the users
// table -- and a second configurable path would be a second way to do one
// thing.
//
// It does not hold the session lifetime either. The store is built from
// Config.Framework.Session.Lifetime, which the framework's loader reads from
// SESSION_LIFETIME, and a copy here would answer no question -- and a
// different one the day either reader grew a rule the other had not. One
// variable, one reader.
type Auth struct {
	// Tenant is the tenant every login belongs to. A multi-tenant deployment
	// resolves it per request instead: the sign-in screens `ui auth` publishes
	// take an authui.TenantResolver, in app/Http/Controllers/Auth, and are
	// handed authui.FixedTenant(Tenant) until something else is written.
	Tenant string

	// PasswordResetTTL is how long a reset link stays valid.
	PasswordResetTTL time.Duration
}

// Tenant is the tenant this deployment logs into.
//
// It is exported because the entry point needs it before the configuration is
// loaded -- `aru db:seed` has to know which tenant it is seeding, and a seeder
// that picks its own tenant seeds data nobody can reach.
func Tenant() string { return env("ARANDU_TENANT_ID", DefaultTenant) }

func loadAuth() (Auth, error) {
	passwordResetTTL, err := envSeconds("AUTH_PASSWORD_RESET_TTL", time.Hour)
	if err != nil {
		return Auth{}, err
	}
	return Auth{
		Tenant:           Tenant(),
		PasswordResetTTL: passwordResetTTL,
	}, nil
}
