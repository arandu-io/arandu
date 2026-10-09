// Package middleware holds what this application puts in front of its routes,
// beside the guards the framework already has.
package middleware

import (
	"context"
	"errors"

	fmiddleware "github.com/arandu-io/framework/http/middleware"
	"github.com/arandu-io/hesape/auth"

	services "github.com/arandu-io/arandu/app/Services"
)

// PersonalAccessTokens is the resolver RequireToken asks who a bearer token
// acts as: the personal access tokens this application issued.
//
// It translates and decides nothing. The digest RequireToken computed is
// handed to the service as the hexadecimal it is stored in, and every token
// the service does not accept is the one ErrUnknownToken, so a missing token, a
// revoked one and an expired one get the same 401. Any other error is a store
// that could not answer, and RequireToken answers it 500 rather than telling a
// client with a good token that it has none.
type PersonalAccessTokens struct {
	tokens *services.PersonalAccessTokenService
}

// NewPersonalAccessTokens returns the resolver over the service that issued the
// tokens. bootstrap builds it and hands it to the routes.
func NewPersonalAccessTokens(tokens *services.PersonalAccessTokenService) PersonalAccessTokens {
	return PersonalAccessTokens{tokens: tokens}
}

// Compile-time proof that RequireToken takes it.
var _ fmiddleware.TokenResolver = PersonalAccessTokens{}

// ResolveToken answers the account the token with this digest acts as.
func (p PersonalAccessTokens) ResolveToken(ctx context.Context, digest fmiddleware.TokenDigest) (auth.Subject, error) {
	subject, err := p.tokens.Resolve(ctx, digest.String())
	if errors.Is(err, services.ErrTokenNotAccepted) {
		return auth.Subject{}, fmiddleware.ErrUnknownToken
	}
	return subject, err
}
