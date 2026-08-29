package nativeauth_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	appmigrations "github.com/arandu-io/arandu/database/migrations"
	"github.com/arandu-io/framework/data"
	frameevents "github.com/arandu-io/framework/events"
	"github.com/arandu-io/framework/security"
	twofactor "github.com/arandu-io/hesape/2fa"
	hedatabase "github.com/arandu-io/hesape/database"
	_ "github.com/arandu-io/hesape/database/connectors/sqlite"
	dbmigrations "github.com/arandu-io/hesape/database/migrations"
	"github.com/arandu-io/hesape/encryption"
	"github.com/arandu-io/hesape/hashing"
	"github.com/arandu-io/hesape/otp"

	"github.com/arandu-io/arandu/app/Policies"
	"github.com/arandu-io/arandu/app/Repositories"
	"github.com/arandu-io/arandu/app/Services"
)

func TestPolicyAndGrantRefusalsHappenBeforeTheFirstQuery(t *testing.T) {
	t.Run("user service", func(t *testing.T) {
		service := services.NewUserService(nil)
		if _, err := service.PublicNames(context.Background(), security.Subject{}, []string{"user-a"}); err == nil {
			t.Fatal("an unauthenticated subject reached the public-name query")
		}
	})

	t.Run("two-factor repository", func(t *testing.T) {
		repository := repositories.NewTwoFactorRepository(nil)
		grant := security.SystemGrant(policies.ActionTwoFactorRead, "tenant-a")
		if _, err := repository.SpendStep(context.Background(), grant, "user-a", 42); err == nil {
			t.Fatal("a read-only grant reached the replay update")
		}
	})
}

func TestSecondFactorWritesCannotCrossTheGrantTenant(t *testing.T) {
	db := openNativeAuthDatabase(t)
	seedFactor(t, db.sql, "tenant-a", "user-a", "encrypted", true)

	repository := repositories.NewTwoFactorRepository(db.app)
	grant := security.SystemGrant(policies.ActionTwoFactorManage, "tenant-b")
	won, err := repository.SpendStep(context.Background(), grant, "user-a", 42)
	if err != nil {
		t.Fatalf("spending under another tenant: %v", err)
	}
	if won {
		t.Fatal("the replay update crossed the grant tenant")
	}

	var step int64
	if err := db.sql.QueryRow(`SELECT last_used_step FROM user_two_factor WHERE user_id = 'user-a'`).Scan(&step); err != nil {
		t.Fatalf("reading the replay step: %v", err)
	}
	if step != 0 {
		t.Fatalf("another tenant changed the replay step to %d", step)
	}
}

func TestConcurrentAuthenticatorVerificationHasExactlyOneWinner(t *testing.T) {
	db := openNativeAuthDatabase(t)
	appKey := []byte("0123456789abcdef0123456789abcdef")
	secret := otp.NewSecret()
	encrypter, err := encryption.NewEncrypter(appKey, encryption.AES256GCM)
	if err != nil {
		t.Fatalf("creating the encrypter: %v", err)
	}
	payload, err := encrypter.EncryptString(otp.EncodeSecret(secret))
	if err != nil {
		t.Fatalf("encrypting the authenticator secret: %v", err)
	}
	seedFactor(t, db.sql, "tenant-a", "user-a", payload, true)

	code, err := otp.Default().Generate(secret, time.Now())
	if err != nil {
		t.Fatalf("generating an authenticator code: %v", err)
	}
	service, err := services.NewTwoFactorService(db.app, appKey)
	if err != nil {
		t.Fatalf("creating the second-factor service: %v", err)
	}

	errs := raceCalls(16, func() error {
		return service.VerifyAuthenticator(context.Background(), "tenant-a", "user-a", code)
	})
	assertOneWinner(t, errs, twofactor.ErrReplayed)
}

func TestConcurrentRecoveryRedemptionHasExactlyOneWinner(t *testing.T) {
	db := openNativeAuthDatabase(t)
	appKey := []byte("0123456789abcdef0123456789abcdef")
	seedFactor(t, db.sql, "tenant-a", "user-a", "encrypted", true)

	codes, err := twofactor.GenerateRecoveryCodes(1)
	if err != nil {
		t.Fatalf("generating a recovery code: %v", err)
	}
	code := codes[0]
	hash, err := hashing.Make("arandu:two-factor-recovery:" + twofactor.NormalizeCode(code))
	if err != nil {
		t.Fatalf("hashing the recovery code: %v", err)
	}
	if _, err := db.sql.Exec(`
		INSERT INTO user_recovery_codes (id, tenant_id, user_id, code_hash, used_at, created_at)
		VALUES ('recovery-a', 'tenant-a', 'user-a', ?, NULL, ?)
	`, hash, time.Now().UTC()); err != nil {
		t.Fatalf("seeding the recovery code: %v", err)
	}

	service, err := services.NewTwoFactorService(db.app, appKey)
	if err != nil {
		t.Fatalf("creating the second-factor service: %v", err)
	}
	errs := raceCalls(16, func() error {
		return service.ConsumeRecovery(context.Background(), "tenant-a", "user-a", code)
	})
	assertOneWinner(t, errs, services.ErrInvalidRecoveryCode)
}

type nativeAuthDatabase struct {
	sql *sql.DB
	app *data.DB
}

func openNativeAuthDatabase(t *testing.T) nativeAuthDatabase {
	t.Helper()

	path := filepath.Join(t.TempDir(), "native-auth.sqlite")
	handle, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("opening SQLite: %v", err)
	}
	handle.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = handle.Close() })

	connection := hedatabase.NewConnection(handle, path, "", map[string]any{
		"driver": string(hedatabase.DialectSQLite),
		"name":   "native-auth",
	})
	migrationConnection := hedatabase.ForMigrations(connection)
	for _, migration := range frameevents.NewModule().Migrations() {
		if err := migration.Up(context.Background(), migrationConnection); err != nil {
			t.Fatalf("applying %s: %v", migration.GetName(), err)
		}
	}
	for _, migration := range []dbmigrations.Migration{
		appmigrations.CreateUsers{},
		appmigrations.AddNameAndVerificationToUsers{},
		appmigrations.CreateTwoFactor{},
	} {
		if err := migration.Up(context.Background(), migrationConnection); err != nil {
			t.Fatalf("applying %s: %v", migration.GetName(), err)
		}
	}

	return nativeAuthDatabase{sql: handle, app: data.Wrap(handle, data.DialectSQLite)}
}

func seedFactor(t *testing.T, db *sql.DB, tenant, user, secret string, confirmed bool) {
	t.Helper()

	if _, err := db.Exec(`
		INSERT INTO users (id, tenant_id, name, email, password, roles, verified_at, created_at)
		VALUES (?, ?, 'Test User', ?, 'stored-password-hash', '[]', ?, ?)
	`, user, tenant, user+"@example.test", time.Now().UTC(), time.Now().UTC()); err != nil {
		t.Fatalf("seeding the user: %v", err)
	}

	var confirmedAt any
	if confirmed {
		confirmedAt = time.Now().UTC()
	}
	if _, err := db.Exec(`
		INSERT INTO user_two_factor
			(user_id, tenant_id, secret, confirmed_at, last_used_step, created_at)
		VALUES (?, ?, ?, ?, 0, ?)
	`, user, tenant, secret, confirmedAt, time.Now().UTC()); err != nil {
		t.Fatalf("seeding the second factor: %v", err)
	}
}

func raceCalls(count int, call func() error) []error {
	start := make(chan struct{})
	errs := make([]error, count)
	var ready sync.WaitGroup
	ready.Add(count)
	var done sync.WaitGroup
	done.Add(count)
	for index := range count {
		go func() {
			defer done.Done()
			ready.Done()
			<-start
			errs[index] = call()
		}()
	}
	ready.Wait()
	close(start)
	done.Wait()
	return errs
}

func assertOneWinner(t *testing.T, errs []error, loser error) {
	t.Helper()

	winners := 0
	for _, err := range errs {
		switch {
		case err == nil:
			winners++
		case errors.Is(err, loser):
		default:
			t.Errorf("concurrent attempt returned %v, want %v", err, loser)
		}
	}
	if winners != 1 {
		t.Fatalf("concurrent attempts produced %d winners, want exactly one", winners)
	}
}
