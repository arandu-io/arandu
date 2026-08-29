// Package repositories holds persistence needed beyond ordinary model queries.
package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"
	twofactor "github.com/arandu-io/hesape/2fa"
	"github.com/arandu-io/hesape/hashing"

	"github.com/arandu-io/arandu/app/Models"
	"github.com/arandu-io/arandu/app/Policies"
)

var (
	// ErrTwoFactorNotEnrolled means an account has no stored enrolment.
	ErrTwoFactorNotEnrolled = errors.New("two-factor: this account has no enrolment")
	// ErrTwoFactorAlreadyEnabled refuses replacement of a working factor.
	ErrTwoFactorAlreadyEnabled = errors.New("two-factor: this account already has an enabled enrolment")
)

const recoveryCodePrefix = "arandu:two-factor-recovery:"

// HashRecoveryCode applies the single persistence format used for recovery
// codes. The repository owns both hashing and checking so the two operations
// cannot silently drift to different prefixes or normalization rules.
func HashRecoveryCode(code string) (string, error) {
	hash, err := hashing.Make(recoveryCodeMaterial(code))
	if err != nil {
		return "", fmt.Errorf("two-factor: hashing recovery code: %w", err)
	}
	return hash, nil
}

func recoveryCodeMaterial(code string) string {
	return recoveryCodePrefix + twofactor.NormalizeCode(code)
}

// TwoFactorRepository owns only the atomic persistence operations that a Model
// cannot express: replay protection and one-time recovery consumption.
type TwoFactorRepository struct{ db *data.DB }

// NewTwoFactorRepository returns the specialized second-factor store.
func NewTwoFactorRepository(db *data.DB) *TwoFactorRepository {
	return &TwoFactorRepository{db: db}
}

// Find reads one enrolment under the supplied read grant.
func (r *TwoFactorRepository) Find(ctx context.Context, grant security.Grant, userID string) (models.TwoFactor, error) {
	if err := grant.Check(policies.ActionTwoFactorRead); err != nil {
		return models.TwoFactor{}, err
	}
	row := r.db.QueryRowContext(ctx,
		`SELECT user_id, tenant_id, secret, confirmed_at, last_used_step, created_at
		 FROM user_two_factor WHERE user_id = ? AND tenant_id = ?`, userID, data.Tenant(grant))
	return scanTwoFactor(row)
}

// Enrol atomically replaces only an unfinished enrolment.
func (r *TwoFactorRepository) Enrol(ctx context.Context, grant security.Grant, factor models.TwoFactor) (models.TwoFactor, error) {
	if err := grant.Check(policies.ActionTwoFactorManage); err != nil {
		return models.TwoFactor{}, err
	}
	if factor.Secret == "" {
		return models.TwoFactor{}, fmt.Errorf("two-factor: refusing to store an empty secret")
	}
	tenant := data.Tenant(grant)
	if _, err := r.db.ExecContext(ctx,
		`DELETE FROM user_two_factor WHERE user_id = ? AND tenant_id = ? AND confirmed_at IS NULL`,
		factor.UserID, tenant); err != nil {
		return models.TwoFactor{}, err
	}
	factor.TenantID = tenant
	factor.CreatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO user_two_factor (user_id, tenant_id, secret, confirmed_at, last_used_step, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`, factor.UserID, tenant, factor.Secret, nil, int64(0), factor.CreatedAt)
	if err != nil {
		if uniqueViolation(err) {
			return models.TwoFactor{}, ErrTwoFactorAlreadyEnabled
		}
		return models.TwoFactor{}, err
	}
	return factor, nil
}

// Confirm stamps an unfinished enrolment and reports whether this call won.
func (r *TwoFactorRepository) Confirm(ctx context.Context, grant security.Grant, userID string, at time.Time) (bool, error) {
	if err := grant.Check(policies.ActionTwoFactorManage); err != nil {
		return false, err
	}
	result, err := r.db.ExecContext(ctx,
		`UPDATE user_two_factor SET confirmed_at = ?
		 WHERE user_id = ? AND tenant_id = ? AND confirmed_at IS NULL`, at.UTC(), userID, data.Tenant(grant))
	return changed(result, err, "confirm an enrolment")
}

// Required reports whether the account has a confirmed enrolment.
func (r *TwoFactorRepository) Required(ctx context.Context, grant security.Grant, userID string) (bool, error) {
	factor, err := r.Find(ctx, grant, userID)
	if errors.Is(err, ErrTwoFactorNotEnrolled) {
		return false, nil
	}
	return factor.Enabled(), err
}

// Disable removes the enrolment and all of its recovery codes.
func (r *TwoFactorRepository) Disable(ctx context.Context, grant security.Grant, userID string) error {
	if err := grant.Check(policies.ActionTwoFactorManage); err != nil {
		return err
	}
	tenant := data.Tenant(grant)
	if _, err := r.db.ExecContext(ctx,
		`DELETE FROM user_recovery_codes WHERE user_id = ? AND tenant_id = ?`, userID, tenant); err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx,
		`DELETE FROM user_two_factor WHERE user_id = ? AND tenant_id = ?`, userID, tenant)
	ok, err := changed(result, err, "disable an enrolment")
	if err != nil {
		return err
	}
	if !ok {
		return ErrTwoFactorNotEnrolled
	}
	return nil
}

// SpendStep atomically records a higher authenticator time step.
func (r *TwoFactorRepository) SpendStep(ctx context.Context, grant security.Grant, userID string, step uint64) (bool, error) {
	if err := grant.Check(policies.ActionTwoFactorManage); err != nil {
		return false, err
	}
	result, err := r.db.ExecContext(ctx,
		`UPDATE user_two_factor SET last_used_step = ?
		 WHERE user_id = ? AND tenant_id = ? AND last_used_step < ?`,
		int64(step), userID, data.Tenant(grant), int64(step))
	return changed(result, err, "spend an authenticator time step")
}

// ReplaceRecoveryCodes replaces the entire recovery set with password hashes.
func (r *TwoFactorRepository) ReplaceRecoveryCodes(ctx context.Context, grant security.Grant, userID string, hashes []string) error {
	if err := grant.Check(policies.ActionTwoFactorManage); err != nil {
		return err
	}
	tenant := data.Tenant(grant)
	if _, err := r.db.ExecContext(ctx,
		`DELETE FROM user_recovery_codes WHERE user_id = ? AND tenant_id = ?`, userID, tenant); err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, hash := range hashes {
		if hash == "" {
			return fmt.Errorf("two-factor: refusing to store an empty recovery hash")
		}
		id, err := data.NewID()
		if err != nil {
			return err
		}
		if _, err := r.db.ExecContext(ctx,
			`INSERT INTO user_recovery_codes (id, tenant_id, user_id, code_hash, used_at, created_at)
			 VALUES (?, ?, ?, ?, ?, ?)`, id, tenant, userID, hash, nil, now); err != nil {
			return err
		}
	}
	return nil
}

// ConsumeRecoveryCode atomically spends a matching unspent recovery hash.
func (r *TwoFactorRepository) ConsumeRecoveryCode(ctx context.Context, grant security.Grant, userID, code string) (bool, error) {
	if err := grant.Check(policies.ActionTwoFactorManage); err != nil {
		return false, err
	}
	if twofactor.NormalizeCode(code) == "" {
		return false, nil
	}
	tenant := data.Tenant(grant)
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, code_hash FROM user_recovery_codes
		 WHERE user_id = ? AND tenant_id = ? AND used_at IS NULL`, userID, tenant)
	if err != nil {
		return false, err
	}
	type candidate struct{ id, hash string }
	var candidates []candidate
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.id, &item.hash); err != nil {
			_ = rows.Close()
			return false, err
		}
		candidates = append(candidates, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return false, err
	}
	if err := rows.Close(); err != nil {
		return false, err
	}
	for _, item := range candidates {
		if err := hashing.Check(recoveryCodeMaterial(code), item.hash); err != nil {
			continue
		}
		result, err := r.db.ExecContext(ctx,
			`UPDATE user_recovery_codes SET used_at = ?
			 WHERE id = ? AND tenant_id = ? AND used_at IS NULL AND user_id = ?`,
			time.Now().UTC(), item.id, tenant, userID)
		return changed(result, err, "spend a recovery code")
	}
	return false, nil
}

type rowScanner interface{ Scan(...any) error }

func scanTwoFactor(row rowScanner) (models.TwoFactor, error) {
	var factor models.TwoFactor
	var confirmed sql.NullTime
	var step int64
	err := row.Scan(&factor.UserID, &factor.TenantID, &factor.Secret, &confirmed, &step, &factor.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return models.TwoFactor{}, ErrTwoFactorNotEnrolled
	}
	if err != nil {
		return models.TwoFactor{}, err
	}
	factor.ConfirmedAt = confirmed.Time.UTC()
	if step > 0 {
		factor.LastUsedStep = uint64(step)
	}
	return factor, nil
}

func changed(result sql.Result, err error, operation string) (bool, error) {
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("two-factor: the driver cannot report whether it could %s: %w", operation, err)
	}
	return rows == 1, nil
}

func uniqueViolation(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique") || strings.Contains(message, "duplicate") || strings.Contains(message, "23505")
}
