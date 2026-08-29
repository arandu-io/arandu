// Package services holds this application's business rules.
package services

import (
	"context"
	"crypto/hmac"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/arandu-io/framework/data"
	frameevents "github.com/arandu-io/framework/events"
	"github.com/arandu-io/framework/observability"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/auth"
	authusers "github.com/arandu-io/hesape/auth/users"
	"github.com/arandu-io/hesape/database/model"
	"github.com/arandu-io/hesape/database/query"
	"github.com/arandu-io/hesape/hashing"

	appevents "github.com/arandu-io/arandu/app/Events"
	"github.com/arandu-io/arandu/app/Models"
	"github.com/arandu-io/arandu/app/Policies"
)

var (
	// ErrUserNotFound means no user matched inside the grant's tenant.
	ErrUserNotFound = errors.New("user: not found")
	// ErrEmailTaken means an address already belongs to an account in this tenant.
	ErrEmailTaken = errors.New("user: email already registered in this tenant")
	// ErrVerificationAddressChanged refuses a code issued for an older address.
	ErrVerificationAddressChanged = errors.New("user: verification was issued for a different address")
	// ErrResetLinkSpent refuses reset state invalidated by an account change.
	ErrResetLinkSpent = errors.New("user: password reset is no longer valid")
)

// TooManyAttemptsError reports the remaining sign-in lockout without naming an account.
type TooManyAttemptsError struct{ RetryAfter time.Duration }

// Seconds is the retry delay rounded up for an HTTP Retry-After header.
func (e TooManyAttemptsError) Seconds() int {
	seconds := int((e.RetryAfter + time.Second - 1) / time.Second)
	if seconds < 1 {
		return 1
	}
	return seconds
}

// Error implements error.
func (e TooManyAttemptsError) Error() string {
	return "user: too many attempts, try again in " + strconv.Itoa(e.Seconds()) + " seconds"
}

// UserService owns application user rules and persistence through Model[User].
type UserService struct {
	db       *data.DB
	policy   policies.UserPolicy
	outbox   *frameevents.Outbox
	throttle security.SignInThrottle
}

// NewUserService returns the application user service.
func NewUserService(db *data.DB) *UserService {
	return &UserService{
		db: db, outbox: frameevents.NewOutbox(db), throttle: security.NewMemoryThrottle(),
	}
}

// credentialUser is the narrow private adapter the native provider hydrates.
// Remember-token methods intentionally have no storage behind them: this
// application creates sessions only through SessionStore after all factors.
type credentialUser struct{ *model.Model[models.User] }

func newCredentialUser(db *data.DB) *credentialUser {
	return &credentialUser{Model: models.Users(db)}
}

func (u *credentialUser) GetAuthIdentifierName() string { return "id" }
func (u *credentialUser) GetAuthIdentifier() any        { return u.Entity.ID }
func (u *credentialUser) GetAuthPasswordName() string   { return "password" }
func (u *credentialUser) GetAuthPassword() string       { return u.Entity.Password }
func (*credentialUser) GetRememberToken() string        { return "" }
func (*credentialUser) SetRememberToken(string)         {}
func (*credentialUser) GetRememberTokenName() string    { return "" }

func (u *credentialUser) domain() (models.User, error) {
	user := *u.Entity
	if err := user.DecodeRoles(); err != nil {
		return models.User{}, fmt.Errorf("user: unreadable roles for %s: %w", user.ID, err)
	}
	return user, nil
}

// credentialProvider prevents the broad provider contract from introducing a
// remember-token persistence path that this application does not own.
type credentialProvider struct{ *authusers.ModelUserProvider }

func (*credentialProvider) RetrieveByToken(context.Context, any, string) (auth.Authenticatable, error) {
	return nil, nil
}

func (*credentialProvider) UpdateRememberToken(_ context.Context, user auth.Authenticatable, token string) error {
	user.SetRememberToken(token)
	return nil
}

func (s *UserService) credentials(tenant string) *auth.CredentialVerifier {
	provider := authusers.NewModelUserProvider(
		hashing.ForAuth(nil),
		func() auth.Authenticatable { return newCredentialUser(s.db) },
		func(context.Context) *query.Builder { return models.Users(s.db).NewBaseQueryBuilder() },
		tenant,
	)
	return auth.NewCredentialVerifier(&credentialProvider{ModelUserProvider: provider}, nil, true, 0)
}

// VerifyCredentials validates a password without creating request identity or a session.
func (s *UserService) VerifyCredentials(ctx context.Context, tenant, email, password, client string) (models.User, error) {
	email = NormalizeEmail(email)
	retry, allowed := s.throttle.Attempt(tenant, email, client)
	if !allowed {
		return models.User{}, TooManyAttemptsError{RetryAfter: retry}
	}

	verified, err := s.credentials(tenant).Verify(ctx, map[string]any{"email": email, "password": password})
	if err != nil {
		if !errors.Is(err, auth.ErrInvalidCredentials) {
			s.throttle.Refund(tenant, email, client)
		}
		return models.User{}, err
	}
	adapted, ok := verified.(*credentialUser)
	if !ok {
		s.throttle.Refund(tenant, email, client)
		return models.User{}, fmt.Errorf("user: credential provider returned %T", verified)
	}
	user, err := adapted.domain()
	if err != nil {
		s.throttle.Refund(tenant, email, client)
		return models.User{}, err
	}
	s.throttle.Clear(tenant, email, client)
	observability.Log(ctx).Info("login credentials verified", "user", user)
	return user, nil
}

// Register creates one unverified, unprivileged account after guest policy authorization.
func (s *UserService) Register(ctx context.Context, tenant, name, email, password string) (models.User, error) {
	if strings.TrimSpace(name) == "" || NormalizeEmail(email) == "" || len(password) < security.MinPasswordLen {
		return models.User{}, fmt.Errorf("user: invalid registration input")
	}
	candidate := models.User{TenantID: tenant, Name: strings.TrimSpace(name), Email: NormalizeEmail(email), Roles: []string{}}
	grant, err := security.Authorize(ctx, s.policy, security.Guest(tenant), policies.ActionUserCreate, candidate)
	if err != nil {
		return models.User{}, err
	}
	candidate.Password, err = hashing.Make(password)
	if err != nil {
		return models.User{}, fmt.Errorf("user: hashing password: %w", err)
	}

	var created models.User
	err = data.Transaction(ctx, s.db, func(ctx context.Context) error {
		created, err = s.create(ctx, grant, candidate)
		if err != nil {
			return err
		}
		return s.record(ctx, grant, appevents.UserRegistered, created)
	})
	return created, err
}

// FindForAuthentication reads an account during a pre-authentication flow.
func (s *UserService) FindForAuthentication(ctx context.Context, tenant, userID string) (models.User, error) {
	return s.find(ctx, security.SystemGrant(policies.ActionUserView, tenant), policies.ActionUserView, userID)
}

// Lookup reads an account by normalized address for application-owned flows.
func (s *UserService) Lookup(ctx context.Context, tenant, email string) (models.User, error) {
	grant := security.SystemGrant(policies.ActionUserView, tenant)
	if err := grant.Check(policies.ActionUserView); err != nil {
		return models.User{}, err
	}
	user, err := models.Users(s.db).Where("email", "=", NormalizeEmail(email)).First(ctx, grant)
	return decodeUser(user, err)
}

// PublicNames resolves a tenant-scoped projection after policy authorization.
func (s *UserService) PublicNames(ctx context.Context, reader security.Subject, ids []string) (map[string]string, error) {
	grant, err := security.Authorize(ctx, s.policy, reader, policies.ActionUserNamesPublic, models.User{TenantID: reader.Tenant})
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return map[string]string{}, nil
	}
	if len(ids) > 200 {
		ids = ids[:200]
	}
	values := make([]any, len(ids))
	for i := range ids {
		values[i] = ids[i]
	}
	rows, err := models.Users(s.db).NewQuery().WhereIn("id", values).Get(ctx, grant, "id", "name", "email")
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, user := range rows {
		name := user.Name
		if name == "" {
			name, _, _ = strings.Cut(user.Email, "@")
		}
		out[user.ID] = name
	}
	return out, nil
}

// MarkVerified conditionally stamps the address captured when its code was issued.
func (s *UserService) MarkVerified(ctx context.Context, tenant, userID, capturedEmail string) (models.User, bool, error) {
	user, err := s.FindForAuthentication(ctx, tenant, userID)
	if err != nil {
		return models.User{}, false, err
	}
	if NormalizeEmail(user.Email) != NormalizeEmail(capturedEmail) {
		return models.User{}, false, ErrVerificationAddressChanged
	}
	if user.Verified() {
		return user, false, nil
	}

	grant := security.SystemGrant(policies.ActionUserUpdate, tenant)
	if err := grant.Check(policies.ActionUserUpdate); err != nil {
		return models.User{}, false, err
	}
	at := time.Now().UTC()
	var changed int64
	err = data.Transaction(ctx, s.db, func(ctx context.Context) error {
		changed, err = models.Users(s.db).Where("id", "=", userID).
			Where("email", "=", NormalizeEmail(capturedEmail)).WhereNull("verified_at").
			Update(ctx, grant, map[string]any{"verified_at": at})
		if err != nil || changed == 0 {
			return err
		}
		user.VerifiedAt = at
		return s.record(ctx, grant, appevents.EmailVerified, user)
	})
	if err != nil {
		return models.User{}, false, err
	}
	if changed == 0 {
		fresh, findErr := s.FindForAuthentication(ctx, tenant, userID)
		if findErr != nil {
			return models.User{}, false, findErr
		}
		if NormalizeEmail(fresh.Email) != NormalizeEmail(capturedEmail) {
			return models.User{}, false, ErrVerificationAddressChanged
		}
		return fresh, false, nil
	}
	return user, true, nil
}

// ResetPassword conditionally replaces the password bound to captured account state.
func (s *UserService) ResetPassword(ctx context.Context, tenant, userID, capturedEmail, capturedPasswordHash, newPassword string) (models.User, error) {
	user, err := s.FindForAuthentication(ctx, tenant, userID)
	if err != nil {
		return models.User{}, err
	}
	if !hmac.Equal([]byte(NormalizeEmail(user.Email)), []byte(NormalizeEmail(capturedEmail))) ||
		!hmac.Equal([]byte(user.PasswordFingerprint()), []byte(capturedPasswordHash)) {
		return models.User{}, ErrResetLinkSpent
	}
	return s.replacePassword(ctx, user, newPassword)
}

// ConfirmPassword revalidates the password for an existing session subject.
func (s *UserService) ConfirmPassword(ctx context.Context, subject security.Subject, password, client string) error {
	if subject.ID == "" || subject.Tenant == "" {
		return fmt.Errorf("user: password confirmation needs a subject")
	}
	identity := "confirm:" + subject.ID
	retry, allowed := s.throttle.Attempt(subject.Tenant, identity, client)
	if !allowed {
		return TooManyAttemptsError{RetryAfter: retry}
	}
	user, err := s.FindForAuthentication(ctx, subject.Tenant, subject.ID)
	if err != nil {
		if !errors.Is(err, ErrUserNotFound) {
			s.throttle.Refund(subject.Tenant, identity, client)
			return err
		}
		return auth.ErrInvalidCredentials
	}
	if err := hashing.Check(password, user.Password); err != nil {
		return auth.ErrInvalidCredentials
	}
	s.throttle.Clear(subject.Tenant, identity, client)
	return nil
}

// EnsureAdmin creates the first administrator or returns the existing account.
func (s *UserService) EnsureAdmin(ctx context.Context, tenant, email, password string) (models.User, error) {
	return s.EnsureUser(ctx, tenant, "", email, password, []string{models.RoleAdmin}, true)
}

// EnsureUser creates a seeded account idempotently.
func (s *UserService) EnsureUser(ctx context.Context, tenant, name, email, password string, roles []string, verified bool) (models.User, error) {
	if existing, err := s.Lookup(ctx, tenant, email); err == nil {
		return existing, nil
	} else if !errors.Is(err, ErrUserNotFound) {
		return models.User{}, err
	}
	hash, err := hashing.Make(password)
	if err != nil {
		return models.User{}, err
	}
	user := models.User{TenantID: tenant, Name: name, Email: NormalizeEmail(email), Password: hash, Roles: roles}
	if verified {
		user.VerifiedAt = time.Now().UTC()
	}
	return s.create(ctx, security.SystemGrant(policies.ActionUserCreate, tenant), user)
}

// SetPassword replaces one account password for operator-owned flows.
func (s *UserService) SetPassword(ctx context.Context, tenant, email, password string) (models.User, error) {
	user, err := s.Lookup(ctx, tenant, email)
	if err != nil {
		return models.User{}, err
	}
	return s.replacePassword(ctx, user, password)
}

func (s *UserService) replacePassword(ctx context.Context, user models.User, plain string) (models.User, error) {
	hash, err := hashing.Make(plain)
	if err != nil {
		return models.User{}, fmt.Errorf("user: hashing password: %w", err)
	}
	//arandu:system-grant guest and operator password replacement has no session subject; tenant lookup plus ID/current-hash compare-and-swap bounds the write
	grant := security.SystemGrant(policies.ActionUserUpdate, user.TenantID)
	if err := grant.Check(policies.ActionUserUpdate); err != nil {
		return models.User{}, err
	}
	var changed int64
	err = data.Transaction(ctx, s.db, func(ctx context.Context) error {
		changed, err = models.Users(s.db).Where("id", "=", user.ID).Where("password", "=", user.Password).
			Update(ctx, grant, map[string]any{"password": hash})
		if err != nil {
			return err
		}
		if changed != 1 {
			return ErrResetLinkSpent
		}
		user.Password = hash
		return s.record(ctx, grant, appevents.PasswordReset, user)
	})
	return user, err
}

func (s *UserService) create(ctx context.Context, grant security.Grant, user models.User) (models.User, error) {
	if err := grant.Check(policies.ActionUserCreate); err != nil {
		return models.User{}, err
	}
	if user.Password == "" {
		return models.User{}, fmt.Errorf("user: refusing to store an empty password hash")
	}
	roles, err := user.EncodeRoles()
	if err != nil {
		return models.User{}, err
	}
	if user.ID == "" {
		user.ID, err = data.NewID()
		if err != nil {
			return models.User{}, err
		}
	}
	attributes := map[string]any{
		"id": user.ID, "tenant_id": data.Tenant(grant), "name": nullableString(user.Name),
		"email": NormalizeEmail(user.Email), "password": user.Password, "roles": roles,
		"verified_at": nullableTime(user.VerifiedAt),
	}
	created, err := models.Users(s.db).Create(ctx, grant, attributes)
	if err != nil {
		if isUniqueViolation(err) {
			return models.User{}, ErrEmailTaken
		}
		return models.User{}, err
	}
	return decodeUser(created, nil)
}

func (s *UserService) find(ctx context.Context, grant security.Grant, action security.Action, id string) (models.User, error) {
	if err := grant.Check(action); err != nil {
		return models.User{}, err
	}
	user, err := models.Users(s.db).Where("id", "=", id).First(ctx, grant)
	return decodeUser(user, err)
}

func decodeUser(user *models.User, err error) (models.User, error) {
	if err != nil {
		return models.User{}, err
	}
	if user == nil {
		return models.User{}, ErrUserNotFound
	}
	if err := user.DecodeRoles(); err != nil {
		return models.User{}, fmt.Errorf("user: unreadable roles for %s: %w", user.ID, err)
	}
	return *user, nil
}

func (s *UserService) record(ctx context.Context, grant security.Grant, name string, user models.User) error {
	return s.outbox.Store(ctx, grant, []frameevents.Event{{
		Name: name, Aggregate: "user", AggregateID: user.ID,
		Payload: appevents.User{UserID: user.ID, Tenant: user.TenantID, Email: user.Email, Name: user.Name},
	}})
}

// NormalizeEmail is the single normalization used on user reads and writes.
func NormalizeEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC()
}

func isUniqueViolation(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique") || strings.Contains(message, "duplicate") || strings.Contains(message, "23505")
}
