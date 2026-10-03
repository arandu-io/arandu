// Package repositories holds the persistence a service shares with something
// other than itself.
package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"
	twofactor "github.com/arandu-io/hesape/2fa"
	"github.com/arandu-io/hesape/database"
	"github.com/arandu-io/hesape/database/model"
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

// TwoFactorRepository holds the second-factor writes that are decided by the
// row they change: a time step spent only when it is higher than the last one,
// an enrolment confirmed only while it is unconfirmed, a recovery code spent
// only while it is unspent. Each is one conditional update through the model,
// and the number of rows it changed is the answer -- which is what makes two
// concurrent attempts produce exactly one winner.
//
// It is a type of its own rather than methods on the service because the
// replay guard and the recovery store hand these writes to the native
// two-factor package, each with the Grant it was given.
//
// Every method checks its Grant before the first query, and every query is
// scoped by the model to the Grant's tenant.
type TwoFactorRepository struct{ db *data.DB }

// NewTwoFactorRepository returns the second-factor store.
func NewTwoFactorRepository(db *data.DB) *TwoFactorRepository {
	return &TwoFactorRepository{db: db}
}

// Find reads one enrolment under the supplied read grant.
func (r *TwoFactorRepository) Find(ctx context.Context, grant security.Grant, userID string) (models.TwoFactor, error) {
	if err := grant.Check(policies.ActionTwoFactorRead); err != nil {
		return models.TwoFactor{}, err
	}
	factor, err := models.TwoFactors(r.db).FindOrFail(ctx, grant, userID)
	if errors.Is(err, model.ErrModelNotFound) {
		return models.TwoFactor{}, ErrTwoFactorNotEnrolled
	}
	if err != nil {
		return models.TwoFactor{}, err
	}
	return *factor, nil
}

// Enrol atomically replaces only an unfinished enrolment.
func (r *TwoFactorRepository) Enrol(ctx context.Context, grant security.Grant, factor models.TwoFactor) (models.TwoFactor, error) {
	if err := grant.Check(policies.ActionTwoFactorManage); err != nil {
		return models.TwoFactor{}, err
	}
	if factor.Secret == "" {
		return models.TwoFactor{}, fmt.Errorf("two-factor: refusing to store an empty secret")
	}
	if _, err := models.TwoFactors(r.db).WhereKey(factor.UserID).WhereNull("confirmed_at").
		Delete(ctx, grant); err != nil {
		return models.TwoFactor{}, err
	}
	record, err := models.TwoFactors(r.db).New()
	if err != nil {
		return models.TwoFactor{}, err
	}
	record.UserID = factor.UserID
	record.TenantID = data.Tenant(grant)
	record.Secret = factor.Secret
	if _, err := record.Save(ctx, grant); err != nil {
		// A confirmed enrolment survived the delete above, and the key is the
		// account: the insert is refused rather than replacing a working factor.
		if errors.Is(err, database.ErrUniqueViolation) {
			return models.TwoFactor{}, ErrTwoFactorAlreadyEnabled
		}
		return models.TwoFactor{}, err
	}
	return *record, nil
}

// Confirm stamps an unfinished enrolment and reports whether this call won.
func (r *TwoFactorRepository) Confirm(ctx context.Context, grant security.Grant, userID string, at time.Time) (bool, error) {
	if err := grant.Check(policies.ActionTwoFactorManage); err != nil {
		return false, err
	}
	changed, err := models.TwoFactors(r.db).WhereKey(userID).WhereNull("confirmed_at").
		Update(ctx, grant, map[string]any{"confirmed_at": at.UTC()})
	return changed == 1, err
}

// Required reports whether the account has a confirmed enrolment.
func (r *TwoFactorRepository) Required(ctx context.Context, grant security.Grant, userID string) (bool, error) {
	if err := grant.Check(policies.ActionTwoFactorRead); err != nil {
		return false, err
	}
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
	if _, err := models.RecoveryCodes(r.db).Where("user_id", "=", userID).Delete(ctx, grant); err != nil {
		return err
	}
	removed, err := models.TwoFactors(r.db).WhereKey(userID).Delete(ctx, grant)
	if err != nil {
		return err
	}
	if removed != 1 {
		return ErrTwoFactorNotEnrolled
	}
	return nil
}

// SpendStep atomically records a higher authenticator time step.
func (r *TwoFactorRepository) SpendStep(ctx context.Context, grant security.Grant, userID string, step uint64) (bool, error) {
	if err := grant.Check(policies.ActionTwoFactorManage); err != nil {
		return false, err
	}
	changed, err := models.TwoFactors(r.db).WhereKey(userID).Where("last_used_step", "<", int64(step)).
		Update(ctx, grant, map[string]any{"last_used_step": int64(step)})
	return changed == 1, err
}

// ReplaceRecoveryCodes replaces the entire recovery set with password hashes.
func (r *TwoFactorRepository) ReplaceRecoveryCodes(ctx context.Context, grant security.Grant, userID string, hashes []string) error {
	if err := grant.Check(policies.ActionTwoFactorManage); err != nil {
		return err
	}
	if _, err := models.RecoveryCodes(r.db).Where("user_id", "=", userID).Delete(ctx, grant); err != nil {
		return err
	}
	tenant := data.Tenant(grant)
	for _, hash := range hashes {
		if hash == "" {
			return fmt.Errorf("two-factor: refusing to store an empty recovery hash")
		}
		record, err := models.RecoveryCodes(r.db).New()
		if err != nil {
			return err
		}
		record.TenantID = tenant
		record.UserID = userID
		record.CodeHash = hash
		if _, err := record.Save(ctx, grant); err != nil {
			return err
		}
	}
	return nil
}

// ConsumeRecoveryCode atomically spends a matching unspent recovery hash.
//
// The hashes are compared here, one by one, because a password hash cannot be
// looked up by value. The write that spends the match is conditional on the
// code still being unspent, so of two concurrent redemptions of one code only
// one changes the row.
func (r *TwoFactorRepository) ConsumeRecoveryCode(ctx context.Context, grant security.Grant, userID, code string) (bool, error) {
	if err := grant.Check(policies.ActionTwoFactorManage); err != nil {
		return false, err
	}
	if twofactor.NormalizeCode(code) == "" {
		return false, nil
	}
	unspent, err := models.RecoveryCodes(r.db).Where("user_id", "=", userID).WhereNull("used_at").
		Get(ctx, grant, "id", "code_hash")
	if err != nil {
		return false, err
	}
	for _, candidate := range unspent {
		if err := hashing.Check(recoveryCodeMaterial(code), candidate.CodeHash); err != nil {
			continue
		}
		changed, err := models.RecoveryCodes(r.db).WhereKey(candidate.ID).Where("user_id", "=", userID).
			WhereNull("used_at").Update(ctx, grant, map[string]any{"used_at": time.Now().UTC()})
		return changed == 1, err
	}
	return false, nil
}
