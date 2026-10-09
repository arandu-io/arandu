package feature_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/arandu-io/framework/http/middleware"
	"github.com/arandu-io/hesape/auth"

	appmiddleware "github.com/arandu-io/arandu/app/Http/Middleware"
	policies "github.com/arandu-io/arandu/app/Policies"
	"github.com/arandu-io/arandu/bootstrap"
	factories "github.com/arandu-io/arandu/database/factories"
	"github.com/arandu-io/arandu/tests"
)

// tokenAccounts boots the application with two accounts of its tenant and
// answers them as the subjects their sessions would carry.
func tokenAccounts(t *testing.T) (bootstrap.App, auth.Subject, auth.Subject) {
	t.Helper()
	app := tests.Booted(t)
	members, err := factories.Users(app.DB).Count(2).
		Create(context.Background(), auth.SystemGrant(policies.ActionUserCreate, bootstrap.Tenant()))
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
