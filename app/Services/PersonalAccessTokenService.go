package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"github.com/arandu-io/hesape/auth"
	"github.com/arandu-io/hesape/database"
	"github.com/arandu-io/hesape/validation"

	models "github.com/arandu-io/arandu/app/Models"
	policies "github.com/arandu-io/arandu/app/Policies"
)

// ErrTokenNotAccepted is what Resolve answers for a token it does not accept:
// one never issued, one revoked, one expired, one whose account is gone. It is
// one error for all of them, so nothing that repeats it can tell a client
// which.
var ErrTokenNotAccepted = errors.New("personal access token: not accepted")

// tokenBytes is how much randomness a token carries. Thirty-two bytes is what
// makes an unsalted, fast digest the right thing to store: nobody can guess a
// token, so the digest has nothing a dictionary can reach.
const tokenBytes = 32

// PersonalAccessTokenService issues, revokes and resolves the bearer tokens an
// account hands to a program so the program can act as it.
//
// A token is stored as its SHA-256 and nothing else. It is returned once, by
// Issue, and kept nowhere: a stored digest that leaks hands out no credential.
// The digest is the one RequireToken computes, middleware.DigestToken, written
// as hexadecimal.
//
// Every token belongs to the tenant this service was built for, which is the
// tenant every sign-in of this deployment belongs to. An application whose
// accounts live in several tenants builds one resolver per way it can tell
// them apart -- a host, a path prefix it routes -- and never reads the tenant
// from the token's request.
type PersonalAccessTokenService struct {
	db     *database.DB
	users  *UserService
	tenant string
	policy policies.PersonalAccessTokenPolicy
	// now is the clock expiry is read against, so a test can move it.
	now func() time.Time
}

// NewPersonalAccessTokenService wires the service over the accounts tokens act
// as, for the tenant they belong to.
func NewPersonalAccessTokenService(db *database.DB, users *UserService, tenant string) *PersonalAccessTokenService {
	return &PersonalAccessTokenService{db: db, users: users, tenant: tenant, now: time.Now}
}

// Issue creates a token that acts as actor, named so the account can tell its
// tokens apart, and returns it with its stored row. The token is in the first
// return value and nowhere else: show it once and drop it.
//
// lifetime is how long the token is accepted, and zero for a token that lasts
// until it is revoked.
func (s *PersonalAccessTokenService) Issue(ctx context.Context, actor auth.Subject, name string, lifetime time.Duration) (string, *models.PersonalAccessToken, error) {
	errs := validation.Errors{}
	validation.Required(errs, "name", name)
	validation.MaxLen(errs, "name", name, 80)
	if lifetime < 0 {
		errs.Add("lifetime", "The lifetime cannot be negative.")
	}
	if errs.Any() {
		return "", nil, errs
	}

	g, err := auth.Authorize(ctx, s.policy, actor, policies.PersonalAccessTokenCreate,
		models.PersonalAccessToken{UserID: actor.ID})
	if err != nil {
		return "", nil, err
	}

	secret := make([]byte, tokenBytes)
	if _, err := rand.Read(secret); err != nil {
		return "", nil, err
	}
	token := base64.RawURLEncoding.EncodeToString(secret)

	record, err := models.PersonalAccessTokens(s.db).New()
	if err != nil {
		return "", nil, err
	}
	record.TenantID = auth.Tenant(g)
	record.UserID = actor.ID
	record.Name = name
	record.TokenDigest = digest(token)
	if lifetime > 0 {
		expires := s.now().Add(lifetime).UTC()
		record.ExpiresAt = &expires
	}
	if _, err := record.Save(ctx, g); err != nil {
		return "", nil, err
	}
	return token, record, nil
}

// Revoke deletes one of actor's tokens. A request carrying it is refused from
// the next one on.
func (s *PersonalAccessTokenService) Revoke(ctx context.Context, actor auth.Subject, id string) error {
	g, err := auth.Authorize(ctx, s.policy, actor, policies.PersonalAccessTokenDelete,
		models.PersonalAccessToken{UserID: actor.ID})
	if err != nil {
		return err
	}
	stored, err := models.PersonalAccessTokens(s.db).FindOrFail(ctx, g, id)
	if err != nil {
		return err
	}
	if _, err := auth.Authorize(ctx, s.policy, actor, policies.PersonalAccessTokenDelete, *stored); err != nil {
		return err
	}
	_, err = stored.Delete(ctx, g)
	return err
}

// Resolve answers the account the token with this digest acts as, read from
// the stored account so its roles are the ones stored for it, or
// ErrTokenNotAccepted.
//
// tokenDigest is the hexadecimal SHA-256 of the token, as RequireToken computes it.
// Nobody is signed in when this runs -- the token is what says who -- so the
// read is under a system grant for this service's tenant, and the row it finds
// is the only thing the answer is built from.
func (s *PersonalAccessTokenService) Resolve(ctx context.Context, tokenDigest string) (auth.Subject, error) {
	//arandu:system-grant a bearer token is resolved before anybody is signed in; the tenant is this deployment's and the digest binds the row
	g := auth.SystemGrant(policies.PersonalAccessTokenResolve, s.tenant)
	found, err := models.PersonalAccessTokens(s.db).Where("token_digest", "=", tokenDigest).First(ctx, g)
	if err != nil {
		return auth.Subject{}, err
	}
	if found == nil || found.ExpiredAt(s.now()) {
		return auth.Subject{}, ErrTokenNotAccepted
	}

	account, err := s.users.FindForAuthentication(ctx, found.TenantID, found.UserID)
	if errors.Is(err, ErrUserNotFound) {
		return auth.Subject{}, ErrTokenNotAccepted
	}
	if err != nil {
		return auth.Subject{}, err
	}
	return account.Subject(), nil
}

// digest is the stored form of a token: its SHA-256, as hexadecimal -- the
// value middleware.DigestToken(token).String() is, written here because a
// service names nothing from the HTTP packages. A token issued here and then
// presented to RequireToken is the proof the two agree.
func digest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
