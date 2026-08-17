package config

import "time"

// DefaultTenant is the tenant a single-tenant application runs under.
//
// It is a constant rather than an empty string on purpose: security.SystemGrant
// refuses an empty tenant, because a system grant with no tenant reads across
// every customer of the system. An application that never thinks about tenancy
// still writes every row under this value, so growing into multi-tenant later is
// a change of resolver and not a migration of data.
const DefaultTenant = "00000000-0000-4000-8000-000000000001"

// Auth is who may sign in, and for how long.
//
// What it deliberately does not hold is a list of guards and providers. There is
// one way to authenticate -- the auth module, over the users table -- and a
// second configurable path would be a second way to do one thing.
type Auth struct {
	// Tenant is the tenant every login belongs to. A multi-tenant deployment
	// resolves it from the host name instead; see auth.TenantResolver.
	Tenant string

	// PasswordMinLength is the shortest password accepted at registration.
	PasswordMinLength int

	// PasswordResetTTL is how long a reset link stays valid.
	PasswordResetTTL time.Duration

	// SessionTTL is how long a signed-in session lasts. It is SESSION_TTL, the
	// same value the session store is built with.
	SessionTTL time.Duration
}

// Tenant is the tenant this deployment logs into.
//
// It is exported because the entry point needs it before the configuration is
// loaded -- `aru db:seed` has to know which tenant it is seeding, and a seeder
// that picks its own tenant seeds data nobody can reach.
func Tenant() string { return env("ARANDU_TENANT_ID", DefaultTenant) }

func loadAuth() Auth {
	return Auth{
		Tenant:            Tenant(),
		PasswordMinLength: envInt("AUTH_PASSWORD_MIN_LENGTH", 12),
		PasswordResetTTL:  envSeconds("AUTH_PASSWORD_RESET_TTL", time.Hour),
		SessionTTL:        envSeconds("SESSION_TTL", 12*time.Hour),
	}
}
