package models

import (
	"encoding/json"
	"log/slog"
	"time"
)

const redactedSecret = "[redacted]"

// TwoFactor is one account's application-owned authenticator enrolment.
type TwoFactor struct {
	UserID       string
	TenantID     string
	Secret       string
	ConfirmedAt  time.Time
	LastUsedStep uint64
	CreatedAt    time.Time
}

// Enabled reports whether the enrolment was proved with its first code.
func (t TwoFactor) Enabled() bool { return !t.ConfirmedAt.IsZero() }

// MarshalJSON keeps the encrypted secret out of responses and debug dumps.
func (t TwoFactor) MarshalJSON() ([]byte, error) {
	marker := ""
	if t.Secret != "" {
		marker = redactedSecret
	}
	return json.Marshal(struct {
		UserID   string `json:"user_id"`
		TenantID string `json:"tenant_id"`
		Enabled  bool   `json:"enabled"`
		Secret   string `json:"secret"`
	}{UserID: t.UserID, TenantID: t.TenantID, Enabled: t.Enabled(), Secret: marker})
}

// LogValue records only the account and whether its factor is active.
func (t TwoFactor) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("user", t.UserID),
		slog.String("tenant", t.TenantID),
		slog.Bool("enabled", t.Enabled()),
	)
}

// String prevents formatting a factor from exposing its encrypted secret.
func (t TwoFactor) String() string {
	if t.UserID == "" {
		return "two factor: none"
	}
	if t.Enabled() {
		return "two factor: enabled for " + t.UserID
	}
	return "two factor: awaiting confirmation for " + t.UserID
}
