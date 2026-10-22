package feature_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/arandu-io/hesape/auth"
	accounts "github.com/arandu-io/hesape/auth/account"
	"github.com/arandu-io/hesape/http/middleware"

	appmiddleware "github.com/arandu-io/arandu/app/Http/Middleware"
	models "github.com/arandu-io/arandu/app/Models"
	policies "github.com/arandu-io/arandu/app/Policies"
	appconfig "github.com/arandu-io/arandu/config"
	factories "github.com/arandu-io/arandu/database/factories"
	"github.com/arandu-io/arandu/tests"
)

// tokenAccounts boots the application with two accounts of its tenant and
// answers them as the subjects their sessions would carry.
func tokenAccounts(t *testing.T) (tests.Application, auth.Subject, auth.Subject) {
	t.Helper()
	app := tests.Booted(t)
	members, err := factories.Users(app.DB).Count(2).
		Create(context.Background(), auth.SystemGrant(accounts.ActionUserCreate, appconfig.Tenant()))
	if err != nil {
		t.Fatalf("creating the accounts: %v", err)
	}
	return app, members[0].Subject(), members[1].Subject()
}

// TestAnIssuedTokenIsResolvedToItsAccountAndNothingElse walks a token's life
// through the resolver RequireToken is given, with the digest RequireToken
// computes -- which is also the proof that the digest the service stores is
// that one.
func TestAnIssuedTokenIsResolvedToItsAccountAndNothingElse(t *testing.T) {
	app, ana, bea := tokenAccounts(t)
	ctx := context.Background()
	resolver := appmiddleware.NewPersonalAccessTokens(app.Tokens)

	token, stored, err := app.Tokens.Issue(ctx, ana, "deploy script", 0)
	if err != nil {
		t.Fatalf("issuing a token: %v", err)
	}
	if stored.TokenDigest == token || stored.TokenDigest != middleware.DigestToken(token).String() {
		t.Fatalf("the stored digest is %q, want the hexadecimal SHA-256 of the token and never the token", stored.TokenDigest)
	}

	who, err := resolver.ResolveToken(ctx, middleware.DigestToken(token))
	if err != nil {
		t.Fatalf("resolving the token just issued: %v", err)
	}
	if who.ID != ana.ID || who.Tenant != ana.Tenant {
		t.Fatalf("the token resolved to %+v, want the account that issued it, %+v", who, ana)
	}

	if _, err := resolver.ResolveToken(ctx, middleware.DigestToken("a token nobody issued")); !errors.Is(err, middleware.ErrUnknownToken) {
		t.Errorf("an unknown token resolved with %v, want ErrUnknownToken", err)
	}

	// Another account revokes nothing it does not own, and the token stays.
	if err := app.Tokens.Revoke(ctx, bea, stored.ID); err == nil {
		t.Fatal("another account revoked the token")
	}
	if _, err := resolver.ResolveToken(ctx, middleware.DigestToken(token)); err != nil {
		t.Fatalf("a refused revocation took the token away: %v", err)
	}

	if err := app.Tokens.Revoke(ctx, ana, stored.ID); err != nil {
		t.Fatalf("revoking the token: %v", err)
	}
	if _, err := resolver.ResolveToken(ctx, middleware.DigestToken(token)); !errors.Is(err, middleware.ErrUnknownToken) {
		t.Errorf("a revoked token resolved with %v, want ErrUnknownToken", err)
	}
}

// TestAnExpiredTokenIsNotAccepted: a token issued with a lifetime stops
// resolving when it ends, with the same answer as one nobody issued.
func TestAnExpiredTokenIsNotAccepted(t *testing.T) {
	app, ana, _ := tokenAccounts(t)
	ctx := context.Background()
	resolver := appmiddleware.NewPersonalAccessTokens(app.Tokens)

	token, _, err := app.Tokens.Issue(ctx, ana, "a minute", time.Minute)
	if err != nil {
		t.Fatalf("issuing a token: %v", err)
	}
	if _, err := resolver.ResolveToken(ctx, middleware.DigestToken(token)); err != nil {
		t.Fatalf("a token inside its lifetime was refused: %v", err)
	}

	short, _, err := app.Tokens.Issue(ctx, ana, "already over", time.Nanosecond)
	if err != nil {
		t.Fatalf("issuing a token: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	if _, err := resolver.ResolveToken(ctx, middleware.DigestToken(short)); !errors.Is(err, middleware.ErrUnknownToken) {
		t.Errorf("an expired token resolved with %v, want ErrUnknownToken", err)
	}
}

// TestATokenIsIssuedByAnAccountForItself: the zero subject issues nothing, and
// a token needs a name to be told apart by.
func TestATokenIsIssuedByAnAccountForItself(t *testing.T) {
	app, ana, _ := tokenAccounts(t)
	ctx := context.Background()

	if _, _, err := app.Tokens.Issue(ctx, auth.Subject{}, "nobody's", 0); err == nil {
		t.Error("a token was issued with nobody signed in")
	}
	if _, _, err := app.Tokens.Issue(ctx, ana, "", 0); err == nil {
		t.Error("a token was issued with no name")
	}
}

// TestAClosedAccountActsThroughNoToken: closing an account released its
// address and locked its password, and every token it had issued went on
// acting as it. The close now revokes them in its own transaction, and a
// token that reaches an account closed or refused anyway is not accepted.
func TestAClosedAccountActsThroughNoToken(t *testing.T) {
	app := tests.Booted(t)
	ctx := context.Background()
	resolver := appmiddleware.NewPersonalAccessTokens(app.Tokens)

	const password = "a password long enough to close with"
	registered, err := app.Users.Register(ctx, appconfig.Tenant(), accounts.Registration{
		Name: "Ana", Email: "ana.closes@example.test", Password: password,
	})
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	ana := registered.(*models.User).Subject()

	token, _, err := app.Tokens.Issue(ctx, ana, "deploy script", 0)
	if err != nil {
		t.Fatalf("issuing a token: %v", err)
	}
	if _, err := resolver.ResolveToken(ctx, middleware.DigestToken(token)); err != nil {
		t.Fatalf("a token of an open account was refused: %v", err)
	}

	if err := app.Users.Close(ctx, ana, password, "127.0.0.1"); err != nil {
		t.Fatalf("closing the account: %v", err)
	}
	if _, err := resolver.ResolveToken(ctx, middleware.DigestToken(token)); !errors.Is(err, middleware.ErrUnknownToken) {
		t.Fatalf("a token of a closed account resolved with %v, want ErrUnknownToken", err)
	}
	// The close deleted the row, and the refusal is not only the resolver's.
	left, err := models.PersonalAccessTokens(app.DB).Where("user_id", "=", ana.ID).
		First(ctx, auth.SystemGrant(policies.PersonalAccessTokenList, appconfig.Tenant()))
	if err != nil {
		t.Fatalf("reading the closed account's tokens: %v", err)
	}
	if left != nil {
		t.Fatalf("the close left the token %q of the closed account stored", left.Name)
	}

	// A token the close did not reach -- issued after it, or by a release
	// that did not revoke -- is refused by the resolver itself.
	late, _, err := app.Tokens.Issue(ctx, ana, "issued after the close", 0)
	if err != nil {
		t.Fatalf("issuing a token after the close: %v", err)
	}
	if _, err := resolver.ResolveToken(ctx, middleware.DigestToken(late)); !errors.Is(err, middleware.ErrUnknownToken) {
		t.Fatalf("a token reaching a closed account resolved with %v, want ErrUnknownToken", err)
	}
}

// TestAPasswordResetRevokesThePersonalAccessTokens: a reset is how somebody
// takes an account back, and a token minted by whoever held the old password
// went on acting as the account after it. Both resets -- the link a person
// follows and the operator's command -- now revoke the account's tokens in the
// reset's own transaction, and leave another account's tokens alone.
func TestAPasswordResetRevokesThePersonalAccessTokens(t *testing.T) {
	const password = "a password long enough to reset"
	for name, reset := range map[string]func(t *testing.T, app tests.Application, user *models.User){
		"through the link": func(t *testing.T, app tests.Application, user *models.User) {
			if _, err := app.Users.ResetPassword(context.Background(), appconfig.Tenant(), user.ID, user.Email,
				accounts.Fingerprint(user.Password), "another password long enough"); err != nil {
				t.Fatalf("resetting the password through the link: %v", err)
			}
		},
		"by an operator's command": func(t *testing.T, app tests.Application, user *models.User) {
			if _, err := app.Users.SetPassword(context.Background(), appconfig.Tenant(), user.Email,
				"another password long enough"); err != nil {
				t.Fatalf("setting the password: %v", err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			app := tests.Booted(t)
			ctx := context.Background()
			resolver := appmiddleware.NewPersonalAccessTokens(app.Tokens)

			ana := tests.Register(t, app, "Ana", "ana.resets@example.test", password)
			bea := tests.Register(t, app, "Bea", "bea.stays@example.test", password)
			anas, _, err := app.Tokens.Issue(ctx, ana.Subject(), "deploy script", 0)
			if err != nil {
				t.Fatalf("issuing Ana's token: %v", err)
			}
			beas, _, err := app.Tokens.Issue(ctx, bea.Subject(), "backup script", 0)
			if err != nil {
				t.Fatalf("issuing Bea's token: %v", err)
			}

			reset(t, app, ana)

			if _, err := resolver.ResolveToken(ctx, middleware.DigestToken(anas)); !errors.Is(err, middleware.ErrUnknownToken) {
				t.Errorf("a token issued before the reset resolved with %v, want ErrUnknownToken", err)
			}
			left, err := models.PersonalAccessTokens(app.DB).Where("user_id", "=", ana.ID).
				First(ctx, auth.SystemGrant(policies.PersonalAccessTokenList, appconfig.Tenant()))
			if err != nil {
				t.Fatalf("reading the reset account's tokens: %v", err)
			}
			if left != nil {
				t.Errorf("the reset left the token %q stored", left.Name)
			}
			if _, err := resolver.ResolveToken(ctx, middleware.DigestToken(beas)); err != nil {
				t.Errorf("another account's token was refused after Ana's reset: %v", err)
			}
		})
	}
}
