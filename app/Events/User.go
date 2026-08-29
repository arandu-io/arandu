// Package events declares this application's domain events.
package events

// User event names are past-tense facts published through the outbox.
const (
	UserRegistered           = "auth.user.registered"
	EmailVerified            = "auth.email.verified"
	PasswordReset            = "auth.password.reset"
	TwoFactorEnabled         = "auth.two_factor.enabled"
	TwoFactorDisabled        = "auth.two_factor.disabled"
	RecoveryCodesRegenerated = "auth.two_factor.recovery_codes.regenerated"
	RecoveryCodeUsed         = "auth.two_factor.recovery_code.used"
)

// User is the safe payload shared by user domain events.
type User struct {
	UserID string `json:"user_id"`
	Tenant string `json:"tenant_id"`
	Email  string `json:"email"`
	Name   string `json:"name,omitempty"`
}
