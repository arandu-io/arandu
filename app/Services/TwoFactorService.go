package services

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/arandu-io/framework/data"
	frameevents "github.com/arandu-io/framework/events"
	"github.com/arandu-io/framework/observability"
	"github.com/arandu-io/framework/security"
	twofactor "github.com/arandu-io/hesape/2fa"
	"github.com/arandu-io/hesape/cache"
	"github.com/arandu-io/hesape/encryption"
	"github.com/arandu-io/hesape/otp"

	appevents "github.com/arandu-io/arandu/app/Events"
	"github.com/arandu-io/arandu/app/Models"
	"github.com/arandu-io/arandu/app/Policies"
	"github.com/arandu-io/arandu/app/Repositories"
)

var (
	// ErrTwoFactorNotEnrolled means no confirmed factor can challenge the account.
	ErrTwoFactorNotEnrolled = repositories.ErrTwoFactorNotEnrolled
	// ErrTwoFactorAlreadyEnabled refuses replacing a working enrolment implicitly.
	ErrTwoFactorAlreadyEnabled = repositories.ErrTwoFactorAlreadyEnabled
	// ErrInvalidRecoveryCode deliberately unwraps to the native invalid-code sentinel.
	ErrInvalidRecoveryCode = fmt.Errorf("%w: recovery code is wrong or already spent", twofactor.ErrInvalidCode)
	// ErrTwoFactorLocked means the account offered too many codes to the
	// sign-in challenge. The pending sign-in is spent: the person signs in
	// again once the window has passed.
	ErrTwoFactorLocked = errors.New("two-factor: too many codes offered to the sign-in challenge")
)

const (
	// ChallengeAttempts is how many codes one account may offer to the sign-in
	// challenge within ChallengeWindow, authenticator and recovery codes
	// together.
	//
	// The count belongs to the account and not to the pending sign-in that
	// carries the attempt. A budget per pending sign-in is a budget per
	// password entry, and whoever has the password can sign in as often as
	// they like; the second factor is the one thing they do not have.
	ChallengeAttempts = 5

	// ChallengeWindow is how long the count lasts, from the first code offered.
	// It outlasts the pending sign-in, so a challenge refused for too many codes
	// cannot be resumed when the count expires: the person has to sign in
	// again.
	ChallengeWindow = 15 * time.Minute
)

// TwoFactorLockedError reports that the sign-in challenge refused a code
// because the account spent its attempts, and the longest it may have to wait
// before trying again.
//
// It matches ErrTwoFactorLocked and twofactor.ErrInvalidCode under errors.Is,
// so a caller that only knows how to refuse a wrong code still refuses this
// one; a caller that knows the difference sends the person back to sign in.
type TwoFactorLockedError struct{ RetryAfter time.Duration }

// Seconds is the retry delay rounded up for an HTTP Retry-After header.
func (e TwoFactorLockedError) Seconds() int {
	seconds := int((e.RetryAfter + time.Second - 1) / time.Second)
	if seconds < 1 {
		return 1
	}
	return seconds
}

// Error implements error.
func (e TwoFactorLockedError) Error() string {
	return ErrTwoFactorLocked.Error() + ", sign in again in " + strconv.Itoa(e.Seconds()) + " seconds"
}

// Unwrap exposes both sentinels the error stands for.
func (e TwoFactorLockedError) Unwrap() []error {
	return []error{ErrTwoFactorLocked, twofactor.ErrInvalidCode}
}

// TwoFactorService owns enrolment and verification of application users.
type TwoFactorService struct {
	db         *data.DB
	repository *repositories.TwoFactorRepository
	policy     policies.TwoFactorPolicy
	userPolicy policies.UserPolicy
	encrypter  *encryption.Encrypter
	outbox     *frameevents.Outbox
	attempts   cache.Store
}

// NewTwoFactorService returns the service with secrets encrypted by appKey.
//
// attempts is the store the codes each account offers to the sign-in challenge
// are counted in. It is required: without it a pending sign-in could be tried
// against every code for as long as it lives. Pass the store the request
// throttle counts in, so every replica spends one budget per account.
func NewTwoFactorService(db *data.DB, appKey []byte, attempts cache.Store) (*TwoFactorService, error) {
	if attempts == nil {
		return nil, errors.New("two-factor: the sign-in challenge needs a cache store to count attempts in")
	}
	encrypter, err := encryption.NewEncrypter(appKey, encryption.AES256GCM)
	if err != nil {
		return nil, err
	}
	return &TwoFactorService{
		db: db, repository: repositories.NewTwoFactorRepository(db),
		encrypter: encrypter, outbox: frameevents.NewOutbox(db), attempts: attempts,
	}, nil
}

// Required reports whether sign-in must finish a second factor.
func (s *TwoFactorService) Required(ctx context.Context, tenant, userID string) (bool, error) {
	//arandu:system-grant password verification established this pending identity before session creation; tenant and user ID bind the factor read
	return s.repository.Required(ctx, security.SystemGrant(policies.ActionTwoFactorRead, tenant), userID)
}

// Begin stores an unconfirmed encrypted secret and returns native provisioning data.
func (s *TwoFactorService) Begin(ctx context.Context, actor security.Subject, issuer string) (twofactor.Provisioning, error) {
	factor := models.TwoFactor{UserID: actor.ID, TenantID: actor.Tenant}
	manage, err := security.Authorize(ctx, s.policy, actor, policies.ActionTwoFactorManage, factor)
	if err != nil {
		return twofactor.Provisioning{}, err
	}
	user, err := s.self(ctx, actor)
	if err != nil {
		return twofactor.Provisioning{}, err
	}

	provisioning := twofactor.Provisioning{Issuer: issuer, Account: user.Email, Secret: otp.NewSecret()}
	if _, err := provisioning.URI(); err != nil {
		return twofactor.Provisioning{}, err
	}
	secret, err := s.encrypter.EncryptString(otp.EncodeSecret(provisioning.Secret))
	if err != nil {
		return twofactor.Provisioning{}, err
	}
	err = data.Transaction(ctx, s.db, func(ctx context.Context) error {
		_, err := s.repository.Enrol(ctx, manage, models.TwoFactor{UserID: actor.ID, Secret: secret})
		return err
	})
	if err != nil {
		return twofactor.Provisioning{}, err
	}
	return provisioning, nil
}

// Confirm proves the first authenticator code and returns recovery codes once.
func (s *TwoFactorService) Confirm(ctx context.Context, actor security.Subject, code string) ([]string, error) {
	factor := models.TwoFactor{UserID: actor.ID, TenantID: actor.Tenant}
	read, err := security.Authorize(ctx, s.policy, actor, policies.ActionTwoFactorRead, factor)
	if err != nil {
		return nil, err
	}
	user, err := s.self(ctx, actor)
	if err != nil {
		return nil, err
	}
	enrolment, err := s.repository.Find(ctx, read, user.ID)
	if err != nil {
		return nil, err
	}
	if _, err := security.Authorize(ctx, s.policy, actor, policies.ActionTwoFactorRead, enrolment); err != nil {
		return nil, err
	}
	manage, err := security.Authorize(ctx, s.policy, actor, policies.ActionTwoFactorManage, enrolment)
	if err != nil {
		return nil, err
	}
	if enrolment.Enabled() {
		return nil, ErrTwoFactorAlreadyEnabled
	}
	secret, err := s.decryptSecret(enrolment.Secret)
	if err != nil {
		return nil, err
	}
	if err := (twofactor.Authenticator{Guard: replayGuard{s.repository, manage}}).
		Verify(ctx, user.ID, secret, code); err != nil {
		return nil, err
	}
	codes, hashes, err := recoveryCodes()
	if err != nil {
		return nil, err
	}
	err = data.Transaction(ctx, s.db, func(ctx context.Context) error {
		won, err := s.repository.Confirm(ctx, manage, user.ID, time.Now().UTC())
		if err != nil {
			return err
		}
		if !won {
			return ErrTwoFactorAlreadyEnabled
		}
		if err := s.repository.ReplaceRecoveryCodes(ctx, manage, user.ID, hashes); err != nil {
			return err
		}
		return s.record(ctx, manage, appevents.TwoFactorEnabled, user)
	})
	return codes, err
}

// Disable removes the secret, replay state and every recovery code.
func (s *TwoFactorService) Disable(ctx context.Context, actor security.Subject) error {
	grant, err := security.Authorize(ctx, s.policy, actor, policies.ActionTwoFactorManage,
		models.TwoFactor{UserID: actor.ID, TenantID: actor.Tenant})
	if err != nil {
		return err
	}
	user, err := s.self(ctx, actor)
	if err != nil {
		return err
	}
	err = data.Transaction(ctx, s.db, func(ctx context.Context) error {
		if err := s.repository.Disable(ctx, grant, user.ID); err != nil {
			return err
		}
		return s.record(ctx, grant, appevents.TwoFactorDisabled, user)
	})
	if err == nil {
		observability.Log(ctx).Warn("second factor disabled", "user", user)
	}
	return err
}

// RegenerateRecoveryCodes replaces every previous recovery code.
func (s *TwoFactorService) RegenerateRecoveryCodes(ctx context.Context, actor security.Subject) ([]string, error) {
	factor := models.TwoFactor{UserID: actor.ID, TenantID: actor.Tenant}
	read, err := security.Authorize(ctx, s.policy, actor, policies.ActionTwoFactorRead, factor)
	if err != nil {
		return nil, err
	}
	user, err := s.self(ctx, actor)
	if err != nil {
		return nil, err
	}
	enrolment, err := s.repository.Find(ctx, read, user.ID)
	if err != nil {
		return nil, err
	}
	if _, err := security.Authorize(ctx, s.policy, actor, policies.ActionTwoFactorRead, enrolment); err != nil {
		return nil, err
	}
	manage, err := security.Authorize(ctx, s.policy, actor, policies.ActionTwoFactorManage, enrolment)
	if err != nil {
		return nil, err
	}
	if !enrolment.Enabled() {
		return nil, ErrTwoFactorNotEnrolled
	}
	codes, hashes, err := recoveryCodes()
	if err != nil {
		return nil, err
	}
	err = data.Transaction(ctx, s.db, func(ctx context.Context) error {
		if err := s.repository.ReplaceRecoveryCodes(ctx, manage, user.ID, hashes); err != nil {
			return err
		}
		return s.record(ctx, manage, appevents.RecoveryCodesRegenerated, user)
	})
	return codes, err
}

// VerifyAuthenticator checks and atomically spends a confirmed TOTP time step.
//
// It is the sign-in challenge, so it spends one of the account's
// ChallengeAttempts before the code is looked at, and returns a
// TwoFactorLockedError once they are gone -- even for the right code. A
// success gives the budget back.
func (s *TwoFactorService) VerifyAuthenticator(ctx context.Context, tenant, userID, code string) error {
	return s.challenge(ctx, tenant, userID, func() error {
		return s.verifyAuthenticator(ctx, tenant, userID, code)
	})
}

func (s *TwoFactorService) verifyAuthenticator(ctx context.Context, tenant, userID, code string) error {
	//arandu:system-grant a signed pending sign-in has no session subject; its tenant and user ID bind this authenticator read
	read := security.SystemGrant(policies.ActionTwoFactorRead, tenant)
	enrolment, err := s.repository.Find(ctx, read, userID)
	if err != nil {
		return err
	}
	if !enrolment.Enabled() {
		return ErrTwoFactorNotEnrolled
	}
	secret, err := s.decryptSecret(enrolment.Secret)
	if err != nil {
		return err
	}
	//arandu:system-grant a signed pending sign-in has no session subject; its tenant and user ID bind replay spending after code verification
	manage := security.SystemGrant(policies.ActionTwoFactorManage, tenant)
	return (twofactor.Authenticator{Guard: replayGuard{s.repository, manage}}).
		Verify(ctx, userID, secret, code)
}

// ConsumeRecovery atomically spends one recovery code of a confirmed factor.
//
// It draws on the same ChallengeAttempts as VerifyAuthenticator: a budget per
// kind of code would be two budgets, and alternating between them would double
// it.
func (s *TwoFactorService) ConsumeRecovery(ctx context.Context, tenant, userID, code string) error {
	return s.challenge(ctx, tenant, userID, func() error {
		return s.consumeRecovery(ctx, tenant, userID, code)
	})
}

func (s *TwoFactorService) consumeRecovery(ctx context.Context, tenant, userID, code string) error {
	//arandu:system-grant a signed pending sign-in has no session subject; its tenant and user ID bind this recovery-factor read
	read := security.SystemGrant(policies.ActionTwoFactorRead, tenant)
	required, err := s.repository.Required(ctx, read, userID)
	if err != nil {
		return err
	}
	if !required {
		return ErrTwoFactorNotEnrolled
	}
	//arandu:system-grant a signed pending sign-in has no session subject; its tenant and user ID bind the recovery audit identity
	user, err := s.findUser(ctx, security.SystemGrant(policies.ActionUserView, tenant), userID)
	if err != nil {
		return err
	}
	//arandu:system-grant a signed pending sign-in has no session subject; its tenant and user ID bind one recovery-code spend
	manage := security.SystemGrant(policies.ActionTwoFactorManage, tenant)
	err = data.Transaction(ctx, s.db, func(ctx context.Context) error {
		spent, err := (recoveryStore{s.repository, manage}).Consume(ctx, user.ID, code)
		if err != nil {
			return err
		}
		if !spent {
			return ErrInvalidRecoveryCode
		}
		return s.record(ctx, manage, appevents.RecoveryCodeUsed, user)
	})
	if err != nil {
		return err
	}
	observability.Log(ctx).Warn("recovery code used", "user", user)
	return nil
}

// challenge spends one of the account's sign-in challenge attempts, then runs
// verify.
//
// The attempt is counted before the code is checked, never after: a count
// taken after the answer lets every request that arrives in between through,
// and parallel requests are exactly how a budget is outrun. The attempt that
// spends the last of the budget on a wrong code already refuses as locked, so
// the caller can end the pending sign-in on that same request.
//
// The count is a counter in the store and not a rate limiter's window. A
// limiter's windows are fixed to the clock, so a budget spent a second before
// one closes would be whole again a second later, with the same pending
// sign-in still valid. The counter's expiry is set by the first attempt and
// never moved, which is what makes ChallengeWindow a promise.
//
// A store that cannot count refuses the attempt. The request throttle fails
// open because it guards capacity; this guards an account.
func (s *TwoFactorService) challenge(ctx context.Context, tenant, userID string, verify func() error) error {
	key := challengeKey(tenant, userID)
	attempt, err := s.attempts.Increment(ctx, key, 1, ChallengeWindow)
	if err != nil {
		return fmt.Errorf("two-factor: counting the challenge attempt: %w", err)
	}
	if attempt > ChallengeAttempts {
		return TwoFactorLockedError{RetryAfter: ChallengeWindow}
	}

	if err := verify(); err != nil {
		if attempt == ChallengeAttempts {
			observability.Log(ctx).Warn("second-factor challenge locked after too many codes",
				"tenant", tenant, "user_id", userID)
			return TwoFactorLockedError{RetryAfter: ChallengeWindow}
		}
		return err
	}

	// The code was right and is spent. Failing to give the budget back costs
	// the person nothing they did not already have, so it is reported and the
	// sign-in goes ahead.
	if err := s.attempts.Forget(ctx, key); err != nil {
		observability.Log(ctx).Warn("clearing the second-factor challenge attempts", "error", err)
	}
	return nil
}

// challengeKey names the counter of one account. The tenant is part of it
// because a user id is only unique inside its tenant, and both are quoted so no
// pair of ids can spell the other's key.
func challengeKey(tenant, userID string) string {
	return "two-factor-challenge:" + strconv.Quote(tenant) + ":" + strconv.Quote(userID)
}

func (s *TwoFactorService) self(ctx context.Context, actor security.Subject) (models.User, error) {
	view, err := security.Authorize(ctx, s.userPolicy, actor, policies.ActionUserView,
		models.User{ID: actor.ID, TenantID: actor.Tenant})
	if err != nil {
		return models.User{}, err
	}
	return s.findUser(ctx, view, actor.ID)
}

func (s *TwoFactorService) findUser(ctx context.Context, grant security.Grant, userID string) (models.User, error) {
	if err := grant.Check(policies.ActionUserView); err != nil {
		return models.User{}, err
	}
	user, err := models.Users(s.db).Where("id", "=", userID).First(ctx, grant)
	return decodeUser(user, err)
}

func (s *TwoFactorService) record(ctx context.Context, grant security.Grant, name string, user models.User) error {
	return s.outbox.Store(ctx, grant, []frameevents.Event{{
		Name: name, Aggregate: "user", AggregateID: user.ID,
		Payload: appevents.User{UserID: user.ID, Tenant: user.TenantID, Email: user.Email, Name: user.Name},
	}})
}

func (s *TwoFactorService) decryptSecret(payload string) ([]byte, error) {
	encoded, err := s.encrypter.DecryptString(payload)
	if err != nil {
		return nil, err
	}
	return otp.DecodeSecret(encoded)
}

type replayGuard struct {
	repository *repositories.TwoFactorRepository
	grant      security.Grant
}

func (g replayGuard) Spend(ctx context.Context, subject string, step uint64) (bool, error) {
	return g.repository.SpendStep(ctx, g.grant, subject, step)
}

type recoveryStore struct {
	repository *repositories.TwoFactorRepository
	grant      security.Grant
}

func (s recoveryStore) Consume(ctx context.Context, subject, code string) (bool, error) {
	return s.repository.ConsumeRecoveryCode(ctx, s.grant, subject, code)
}

var (
	_ twofactor.ReplayGuard   = replayGuard{}
	_ twofactor.RecoveryStore = recoveryStore{}
)

func recoveryCodes() ([]string, []string, error) {
	codes, err := twofactor.GenerateRecoveryCodes(twofactor.DefaultRecoveryCodes)
	if err != nil {
		return nil, nil, err
	}
	hashes := make([]string, 0, len(codes))
	for _, code := range codes {
		hash, err := repositories.HashRecoveryCode(code)
		if err != nil {
			return nil, nil, err
		}
		hashes = append(hashes, hash)
	}
	return codes, hashes, nil
}
