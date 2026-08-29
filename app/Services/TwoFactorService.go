package services

import (
	"context"
	"fmt"
	"time"

	"github.com/arandu-io/framework/data"
	frameevents "github.com/arandu-io/framework/events"
	"github.com/arandu-io/framework/observability"
	"github.com/arandu-io/framework/security"
	twofactor "github.com/arandu-io/hesape/2fa"
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
)

// TwoFactorService owns enrolment and verification of application users.
type TwoFactorService struct {
	db         *data.DB
	repository *repositories.TwoFactorRepository
	policy     policies.TwoFactorPolicy
	userPolicy policies.UserPolicy
	encrypter  *encryption.Encrypter
	outbox     *frameevents.Outbox
}

// NewTwoFactorService returns the service with secrets encrypted by appKey.
func NewTwoFactorService(db *data.DB, appKey []byte) (*TwoFactorService, error) {
	encrypter, err := encryption.NewEncrypter(appKey, encryption.AES256GCM)
	if err != nil {
		return nil, err
	}
	return &TwoFactorService{
		db: db, repository: repositories.NewTwoFactorRepository(db),
		encrypter: encrypter, outbox: frameevents.NewOutbox(db),
	}, nil
}

// Required reports whether sign-in must finish a second factor.
func (s *TwoFactorService) Required(ctx context.Context, tenant, userID string) (bool, error) {
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
	manage, err := security.Authorize(ctx, s.policy, actor, policies.ActionTwoFactorManage, factor)
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
	manage, err := security.Authorize(ctx, s.policy, actor, policies.ActionTwoFactorManage, factor)
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
func (s *TwoFactorService) VerifyAuthenticator(ctx context.Context, tenant, userID, code string) error {
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
	manage := security.SystemGrant(policies.ActionTwoFactorManage, tenant)
	return (twofactor.Authenticator{Guard: replayGuard{s.repository, manage}}).
		Verify(ctx, userID, secret, code)
}

// ConsumeRecovery atomically spends one recovery code of a confirmed factor.
func (s *TwoFactorService) ConsumeRecovery(ctx context.Context, tenant, userID, code string) error {
	read := security.SystemGrant(policies.ActionTwoFactorRead, tenant)
	required, err := s.repository.Required(ctx, read, userID)
	if err != nil {
		return err
	}
	if !required {
		return ErrTwoFactorNotEnrolled
	}
	user, err := s.findUser(ctx, security.SystemGrant(policies.ActionUserView, tenant), userID)
	if err != nil {
		return err
	}
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
