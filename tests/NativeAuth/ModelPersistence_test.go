package nativeauth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/arandu-io/framework/security"
	twofactor "github.com/arandu-io/hesape/2fa"
	"github.com/arandu-io/hesape/cache"
	"github.com/arandu-io/hesape/database"
	"github.com/arandu-io/hesape/hashing"
	"github.com/arandu-io/hesape/otp"

	"github.com/arandu-io/arandu/app/Models"
	"github.com/arandu-io/arandu/app/Policies"
	"github.com/arandu-io/arandu/app/Repositories"
	"github.com/arandu-io/arandu/app/Services"
)

// TestTheSecondFactorLifecycle walks one account through every write the
// second factor makes: an unfinished enrolment that may be replaced, the
// confirmation that may happen once, recovery codes that are spent once and
// replaced together, and the removal of all of it.
func TestTheSecondFactorLifecycle(t *testing.T) {
	db := openNativeAuthDatabase(t)
	ctx := context.Background()
	users := services.NewUserService(db.app)
	user, err := users.Register(ctx, "tenant-a", "Ana", "ana@example.test", "a-long-enough-password")
	if err != nil {
		t.Fatalf("registering the account: %v", err)
	}
	if user.TenantID != "tenant-a" || user.ID == "" {
		t.Fatalf("the registered account is %+v, want an id in tenant-a", user)
	}
	actor := security.Subject{ID: user.ID, Tenant: user.TenantID}
	factors, err := services.NewTwoFactorService(db.app, []byte("0123456789abcdef0123456789abcdef"), cache.NewArrayStore())
	if err != nil {
		t.Fatalf("creating the second-factor service: %v", err)
	}

	if _, err := factors.Begin(ctx, actor, "Arandu"); err != nil {
		t.Fatalf("beginning an enrolment: %v", err)
	}
	// An unfinished enrolment is replaced by the next one, and it is stored
	// unconfirmed: NULL, which is what the confirmation asks for.
	provisioning, err := factors.Begin(ctx, actor, "Arandu")
	if err != nil {
		t.Fatalf("replacing an unfinished enrolment: %v", err)
	}
	var unconfirmed int
	if err := db.sql.QueryRow(`SELECT count(*) FROM user_two_factor WHERE user_id = ? AND confirmed_at IS NULL`, user.ID).
		Scan(&unconfirmed); err != nil || unconfirmed != 1 {
		t.Fatalf("unconfirmed enrolments = %d (%v), want 1", unconfirmed, err)
	}
	if required, err := factors.Required(ctx, "tenant-a", user.ID); err != nil || required {
		t.Fatalf("Required before confirmation = %v, %v; want false", required, err)
	}

	code, err := otp.Default().Generate(provisioning.Secret, time.Now())
	if err != nil {
		t.Fatalf("generating the first code: %v", err)
	}
	codes, err := factors.Confirm(ctx, actor, code)
	if err != nil {
		t.Fatalf("confirming the enrolment: %v", err)
	}
	if len(codes) == 0 {
		t.Fatal("the confirmation returned no recovery codes")
	}
	if required, err := factors.Required(ctx, "tenant-a", user.ID); err != nil || !required {
		t.Fatalf("Required after confirmation = %v, %v; want true", required, err)
	}
	if _, err := factors.Begin(ctx, actor, "Arandu"); !errors.Is(err, services.ErrTwoFactorAlreadyEnabled) {
		t.Fatalf("beginning over a working factor answered %v, want ErrTwoFactorAlreadyEnabled", err)
	}
	if _, err := factors.Confirm(ctx, actor, code); !errors.Is(err, services.ErrTwoFactorAlreadyEnabled) {
		t.Fatalf("confirming twice answered %v, want ErrTwoFactorAlreadyEnabled", err)
	}

	if err := factors.ConsumeRecovery(ctx, "tenant-a", user.ID, codes[0]); err != nil {
		t.Fatalf("spending a recovery code: %v", err)
	}
	if err := factors.ConsumeRecovery(ctx, "tenant-a", user.ID, codes[0]); !errors.Is(err, twofactor.ErrInvalidCode) {
		t.Fatalf("spending a recovery code twice answered %v, want an invalid code", err)
	}

	replaced, err := factors.RegenerateRecoveryCodes(ctx, actor)
	if err != nil {
		t.Fatalf("regenerating the recovery codes: %v", err)
	}
	if err := factors.ConsumeRecovery(ctx, "tenant-a", user.ID, codes[1]); !errors.Is(err, twofactor.ErrInvalidCode) {
		t.Fatalf("a code from the replaced set answered %v, want an invalid code", err)
	}
	if err := factors.ConsumeRecovery(ctx, "tenant-a", user.ID, replaced[0]); err != nil {
		t.Fatalf("spending a code from the new set: %v", err)
	}

	if err := factors.Disable(ctx, actor); err != nil {
		t.Fatalf("disabling the factor: %v", err)
	}
	if required, err := factors.Required(ctx, "tenant-a", user.ID); err != nil || required {
		t.Fatalf("Required after disabling = %v, %v; want false", required, err)
	}
	var left int
	if err := db.sql.QueryRow(`SELECT count(*) FROM user_recovery_codes WHERE user_id = ?`, user.ID).Scan(&left); err != nil || left != 0 {
		t.Fatalf("recovery codes left after disabling = %d (%v), want 0", left, err)
	}
	if err := factors.Disable(ctx, actor); !errors.Is(err, services.ErrTwoFactorNotEnrolled) {
		t.Fatalf("disabling twice answered %v, want ErrTwoFactorNotEnrolled", err)
	}
}

// TestEverySecondFactorWriteStaysInsideTheGrantTenant runs each persistence
// operation under another tenant's Grant against rows of tenant-a, and proves
// none of them read, changed or removed one.
func TestEverySecondFactorWriteStaysInsideTheGrantTenant(t *testing.T) {
	db := openNativeAuthDatabase(t)
	ctx := context.Background()
	seedFactor(t, db.sql, "tenant-a", "user-a", "encrypted", true)
	seedFactor(t, db.sql, "tenant-a", "user-b", "encrypted", false)
	repository := repositories.NewTwoFactorRepository(db.app)

	const recovery = "ABCDE-FGHIJ"
	hash, err := hashing.Make("arandu:two-factor-recovery:" + twofactor.NormalizeCode(recovery))
	if err != nil {
		t.Fatalf("hashing the recovery code: %v", err)
	}
	if _, err := db.sql.Exec(`
		INSERT INTO user_recovery_codes (id, tenant_id, user_id, code_hash, used_at, created_at)
		VALUES ('recovery-a', 'tenant-a', 'user-a', ?, NULL, ?)
	`, hash, time.Now().UTC()); err != nil {
		t.Fatalf("seeding the recovery code: %v", err)
	}

	read := security.SystemGrant(policies.ActionTwoFactorRead, "tenant-b")
	manage := security.SystemGrant(policies.ActionTwoFactorManage, "tenant-b")

	if _, err := repository.Find(ctx, read, "user-a"); !errors.Is(err, repositories.ErrTwoFactorNotEnrolled) {
		t.Errorf("Find under another tenant answered %v, want ErrTwoFactorNotEnrolled", err)
	}
	if _, err := repository.Enrol(ctx, manage, models.TwoFactor{UserID: "user-b", Secret: "other"}); err == nil {
		t.Error("Enrol under another tenant replaced tenant-a's unfinished enrolment")
	}
	if won, err := repository.Confirm(ctx, manage, "user-b", time.Now()); err != nil || won {
		t.Errorf("Confirm under another tenant = %v, %v; want false", won, err)
	}
	if err := repository.Disable(ctx, manage, "user-a"); !errors.Is(err, repositories.ErrTwoFactorNotEnrolled) {
		t.Errorf("Disable under another tenant answered %v, want ErrTwoFactorNotEnrolled", err)
	}
	if err := repository.ReplaceRecoveryCodes(ctx, manage, "user-a", []string{"replacement-hash"}); err != nil {
		t.Errorf("ReplaceRecoveryCodes under another tenant: %v", err)
	}
	if spent, err := repository.ConsumeRecoveryCode(ctx, manage, "user-a", recovery); err != nil || spent {
		t.Errorf("ConsumeRecoveryCode under another tenant = %v, %v; want false", spent, err)
	}

	var secret string
	var confirmed any
	if err := db.sql.QueryRow(`SELECT secret, confirmed_at FROM user_two_factor WHERE user_id = 'user-b'`).
		Scan(&secret, &confirmed); err != nil {
		t.Fatalf("reading tenant-a's unfinished enrolment: %v", err)
	}
	if secret != "encrypted" || confirmed != nil {
		t.Errorf("tenant-a's unfinished enrolment became secret=%q confirmed_at=%v", secret, confirmed)
	}
	var factors, unspent int
	if err := db.sql.QueryRow(`SELECT count(*) FROM user_two_factor WHERE tenant_id = 'tenant-a'`).Scan(&factors); err != nil || factors != 2 {
		t.Errorf("tenant-a enrolments = %d (%v), want 2", factors, err)
	}
	if err := db.sql.QueryRow(`SELECT count(*) FROM user_recovery_codes WHERE tenant_id = 'tenant-a' AND used_at IS NULL`).
		Scan(&unspent); err != nil || unspent != 1 {
		t.Errorf("tenant-a unspent recovery codes = %d (%v), want 1", unspent, err)
	}
}

// TestADuplicateAddressIsRecognisedByTheDriverCode registers one address twice
// in a tenant. The second write is refused by the unique index, and the error
// says so twice over: as the account rule the screens check for, and as the
// unique violation the router answers with 409. It is never read off the
// driver's message.
func TestADuplicateAddressIsRecognisedByTheDriverCode(t *testing.T) {
	db := openNativeAuthDatabase(t)
	ctx := context.Background()
	users := services.NewUserService(db.app)
	if _, err := users.Register(ctx, "tenant-a", "Ana", "ana@example.test", "a-long-enough-password"); err != nil {
		t.Fatalf("registering the address: %v", err)
	}

	_, err := users.Register(ctx, "tenant-a", "Ana again", " ANA@example.test ", "a-long-enough-password")
	if !errors.Is(err, services.ErrEmailTaken) {
		t.Fatalf("the second registration answered %v, want ErrEmailTaken", err)
	}
	if !errors.Is(err, database.ErrUniqueViolation) {
		t.Fatalf("the second registration answered %v, want it to match database.ErrUniqueViolation", err)
	}

	// The same address in another tenant is another account.
	if _, err := users.Register(ctx, "tenant-b", "Ana", "ana@example.test", "a-long-enough-password"); err != nil {
		t.Fatalf("registering the address in another tenant: %v", err)
	}
}

// TestRolesAreOneValueInGoAndJSONInTheColumn stores an account with a role and
// reads it back: the field the policies compare against is what was written,
// and the column holds the portable JSON text every engine can store.
func TestRolesAreOneValueInGoAndJSONInTheColumn(t *testing.T) {
	db := openNativeAuthDatabase(t)
	ctx := context.Background()
	users := services.NewUserService(db.app)

	admin, err := users.EnsureAdmin(ctx, "tenant-a", "root@example.test", "a-long-enough-password")
	if err != nil {
		t.Fatalf("creating the administrator: %v", err)
	}
	found, err := users.Lookup(ctx, "tenant-a", "root@example.test")
	if err != nil {
		t.Fatalf("reading the administrator back: %v", err)
	}
	if len(found.Roles) != 1 || found.Roles[0] != models.RoleAdmin || !found.Subject().HasRole(models.RoleAdmin) {
		t.Fatalf("roles read back = %v, want [%s]", found.Roles, models.RoleAdmin)
	}
	var stored string
	if err := db.sql.QueryRow(`SELECT roles FROM users WHERE id = ?`, admin.ID).Scan(&stored); err != nil {
		t.Fatalf("reading the column: %v", err)
	}
	if stored != `["admin"]` {
		t.Fatalf("the column holds %q, want the JSON array", stored)
	}

	member, err := users.Register(ctx, "tenant-a", "Ana", "ana@example.test", "a-long-enough-password")
	if err != nil {
		t.Fatalf("registering a member: %v", err)
	}
	if err := db.sql.QueryRow(`SELECT roles FROM users WHERE id = ?`, member.ID).Scan(&stored); err != nil || stored != "[]" {
		t.Fatalf("a member's column holds %q (%v), want []", stored, err)
	}
	if _, err := users.Lookup(ctx, "tenant-a", "nobody@example.test"); !errors.Is(err, services.ErrUserNotFound) {
		t.Fatalf("looking up an unknown address answered %v, want ErrUserNotFound", err)
	}
}
