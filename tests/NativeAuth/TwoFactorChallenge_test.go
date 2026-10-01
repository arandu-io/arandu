package nativeauth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	twofactor "github.com/arandu-io/hesape/2fa"
	"github.com/arandu-io/hesape/cache"
	"github.com/arandu-io/hesape/encryption"
	"github.com/arandu-io/hesape/otp"

	"github.com/arandu-io/arandu/app/Services"
)

// challengeFixture is one enrolled account, the secret its authenticator
// holds, and the store its challenge attempts are counted in.
type challengeFixture struct {
	db     nativeAuthDatabase
	appKey []byte
	secret []byte
	store  cache.Store
}

func newChallengeFixture(t *testing.T, users ...string) challengeFixture {
	t.Helper()

	db := openNativeAuthDatabase(t)
	appKey := []byte("0123456789abcdef0123456789abcdef")
	encrypter, err := encryption.NewEncrypter(appKey, encryption.AES256GCM)
	if err != nil {
		t.Fatalf("creating the encrypter: %v", err)
	}
	secret := otp.NewSecret()
	payload, err := encrypter.EncryptString(otp.EncodeSecret(secret))
	if err != nil {
		t.Fatalf("encrypting the authenticator secret: %v", err)
	}
	for _, user := range users {
		seedFactor(t, db.sql, "tenant-a", user, payload, true)
	}
	return challengeFixture{db: db, appKey: appKey, secret: secret, store: cache.NewArrayStore()}
}

// service is one instance of the application -- one replica, or the code
// behind one browser's session. Every instance built from a fixture counts in
// the same store, which is what a deployment shares.
func (f challengeFixture) service(t *testing.T) *services.TwoFactorService {
	t.Helper()
	service, err := services.NewTwoFactorService(f.db.app, f.appKey, f.store)
	if err != nil {
		t.Fatalf("creating the second-factor service: %v", err)
	}
	return service
}

// codes returns the code the authenticator shows now, and one that is not it.
func (f challengeFixture) codes(t *testing.T) (right, wrong string) {
	t.Helper()
	right, err := otp.Default().Generate(f.secret, time.Now())
	if err != nil {
		t.Fatalf("generating an authenticator code: %v", err)
	}
	digits := []byte(right)
	digits[0] = '0' + (digits[0]-'0'+1)%10
	return right, string(digits)
}

// TestTheChallengeRefusesTheRightCodeOnceTheBudgetIsSpent.
//
// Five wrong codes and the sixth attempt is refused even though it is the right
// one: a budget that a correct guess could still slip past is a budget that only
// slows the guessing down. The attempt that spends the budget already answers
// locked, so the sign-in screen can end the pending sign-in on that request.
//
// Signing in again does not hand the budget back. It belongs to the account, so
// whoever has the password -- and can therefore sign in as often as they like --
// gets no fresh guesses for it.
func TestTheChallengeRefusesTheRightCodeOnceTheBudgetIsSpent(t *testing.T) {
	fixture := newChallengeFixture(t, "user-a")
	service := fixture.service(t)
	ctx := context.Background()
	right, wrong := fixture.codes(t)

	for attempt := 1; attempt < services.ChallengeAttempts; attempt++ {
		err := service.VerifyAuthenticator(ctx, "tenant-a", "user-a", wrong)
		if !errors.Is(err, twofactor.ErrInvalidCode) || errors.Is(err, services.ErrTwoFactorLocked) {
			t.Fatalf("wrong code %d answered %v, want an invalid code that is not yet locked", attempt, err)
		}
	}

	err := service.VerifyAuthenticator(ctx, "tenant-a", "user-a", wrong)
	if !errors.Is(err, services.ErrTwoFactorLocked) {
		t.Fatalf("wrong code %d answered %v, want the challenge locked", services.ChallengeAttempts, err)
	}
	// A caller written before the lock existed still refuses it as a wrong code.
	if !errors.Is(err, twofactor.ErrInvalidCode) {
		t.Errorf("the lock does not match twofactor.ErrInvalidCode: %v", err)
	}
	var locked services.TwoFactorLockedError
	if !errors.As(err, &locked) || locked.Seconds() <= 0 {
		t.Errorf("the lock carries no retry delay: %v", err)
	}

	if err := service.VerifyAuthenticator(ctx, "tenant-a", "user-a", right); !errors.Is(err, services.ErrTwoFactorLocked) {
		t.Fatalf("the right code after the budget was spent answered %v, want the challenge locked", err)
	}
	if err := service.ConsumeRecovery(ctx, "tenant-a", "user-a", "ABCDE-FGHIJ"); !errors.Is(err, services.ErrTwoFactorLocked) {
		t.Fatalf("a recovery code after the budget was spent answered %v, want the challenge locked", err)
	}

	// A new password sign-in asks whether a factor is required and then writes
	// a new pending sign-in. Neither gives the budget back.
	if required, err := service.Required(ctx, "tenant-a", "user-a"); err != nil || !required {
		t.Fatalf("Required = %v, %v", required, err)
	}
	if err := service.VerifyAuthenticator(ctx, "tenant-a", "user-a", right); !errors.Is(err, services.ErrTwoFactorLocked) {
		t.Fatalf("the right code after signing in again answered %v, want the challenge still locked", err)
	}

	// The refused right code never reached the replay guard, so the step it
	// belongs to is unspent.
	var step int64
	if err := fixture.db.sql.QueryRow(`SELECT last_used_step FROM user_two_factor WHERE user_id = 'user-a'`).Scan(&step); err != nil {
		t.Fatalf("reading the replay step: %v", err)
	}
	if step != 0 {
		t.Fatalf("a locked challenge spent time step %d", step)
	}
}

// TestTheChallengeBudgetBelongsToTheAccountAndNotToTheSession.
//
// Two instances stand for two browsers -- or two replicas -- each carrying a
// pending sign-in for the same account. Alternating between them does not buy a
// second budget, and the budget of one account is not spent by another's.
func TestTheChallengeBudgetBelongsToTheAccountAndNotToTheSession(t *testing.T) {
	fixture := newChallengeFixture(t, "user-a", "user-b")
	first, second := fixture.service(t), fixture.service(t)
	ctx := context.Background()
	right, wrong := fixture.codes(t)

	for attempt := 1; attempt <= services.ChallengeAttempts; attempt++ {
		instance := first
		if attempt%2 == 0 {
			instance = second
		}
		_ = instance.VerifyAuthenticator(ctx, "tenant-a", "user-a", wrong)
	}
	for name, instance := range map[string]*services.TwoFactorService{"first": first, "second": second} {
		if err := instance.VerifyAuthenticator(ctx, "tenant-a", "user-a", right); !errors.Is(err, services.ErrTwoFactorLocked) {
			t.Fatalf("the %s session answered %v after the account spent its budget across both", name, err)
		}
	}

	if err := first.VerifyAuthenticator(ctx, "tenant-a", "user-b", right); err != nil {
		t.Fatalf("another account of the same tenant was refused: %v", err)
	}
	if err := first.VerifyAuthenticator(ctx, "tenant-b", "user-a", right); errors.Is(err, services.ErrTwoFactorLocked) {
		t.Fatalf("the same user id in another tenant was locked: %v", err)
	}
}

// TestASuccessfulChallengeGivesTheBudgetBack: somebody who mistyped twice and
// then got in has not used up anything the next sign-in needs.
func TestASuccessfulChallengeGivesTheBudgetBack(t *testing.T) {
	fixture := newChallengeFixture(t, "user-a")
	service := fixture.service(t)
	ctx := context.Background()
	right, wrong := fixture.codes(t)

	for range services.ChallengeAttempts - 1 {
		_ = service.VerifyAuthenticator(ctx, "tenant-a", "user-a", wrong)
	}
	if err := service.VerifyAuthenticator(ctx, "tenant-a", "user-a", right); err != nil {
		t.Fatalf("the right code within the budget answered %v", err)
	}
	for attempt := 1; attempt < services.ChallengeAttempts; attempt++ {
		if err := service.VerifyAuthenticator(ctx, "tenant-a", "user-a", wrong); errors.Is(err, services.ErrTwoFactorLocked) {
			t.Fatalf("wrong code %d after a success answered locked: the success did not give the budget back", attempt)
		}
	}
}

// failingStore is a store that cannot count.
type failingStore struct{ cache.Store }

func (failingStore) Increment(context.Context, string, int64, time.Duration) (int64, error) {
	return 0, errors.New("store unreachable")
}

// TestAChallengeThatCannotBeCountedIsRefused. The request throttle fails open
// because it guards capacity; this guards an account, and an attempt nobody
// counted is an attempt outside the budget.
func TestAChallengeThatCannotBeCountedIsRefused(t *testing.T) {
	fixture := newChallengeFixture(t, "user-a")
	service, err := services.NewTwoFactorService(fixture.db.app, fixture.appKey, failingStore{cache.NewArrayStore()})
	if err != nil {
		t.Fatalf("creating the second-factor service: %v", err)
	}
	right, _ := fixture.codes(t)
	if err := service.VerifyAuthenticator(context.Background(), "tenant-a", "user-a", right); err == nil {
		t.Fatal("the right code was accepted with no store to count the attempt in")
	}

	if _, err := services.NewTwoFactorService(fixture.db.app, fixture.appKey, nil); err == nil {
		t.Fatal("the service was built with nothing to count challenge attempts in")
	}
}
