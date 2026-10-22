package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/arandu-io/hesape/auth"
	"github.com/arandu-io/hesape/auth/account"

	models "github.com/arandu-io/arandu/app/Models"
)

// ErrAccountGone is what CurrentSubject answers for an account that can no
// longer act: one that does not exist in the tenant, one that was closed, and
// one Hooks.Admit refuses. It is one error for the three, so nothing that
// repeats it can tell a caller which.
var ErrAccountGone = errors.New("account: gone, closed or refused")

// CurrentSubject answers the subject of the account userID in tenant as it is
// stored now: the one User.Subject builds from the row, with the roles and the
// verification the row holds at this moment and not at any earlier one.
//
// It is how anything that already knows who somebody is -- a session read on
// every request, a bearer token -- learns what they may do today. The account
// is held to the rules a sign-in is held to, so a closed account, or one
// Hooks.Admit refuses, answers ErrAccountGone exactly as a missing one does.
// Any other error is the read failing, and the caller must fail with it rather
// than fall back on a subject it kept from before.
func CurrentSubject(ctx context.Context, users *account.Users, tenant, userID string) (auth.Subject, error) {
	found, err := users.FindForAuthentication(ctx, tenant, userID)
	if errors.Is(err, account.ErrUserNotFound) {
		return auth.Subject{}, ErrAccountGone
	}
	if err != nil {
		return auth.Subject{}, err
	}
	if err := users.Admit(ctx, found); err != nil {
		return auth.Subject{}, ErrAccountGone
	}
	// The account rules hand back the *User this application's users query
	// built; User.Subject is the one place a subject is made from it.
	user, ok := found.(*models.User)
	if !ok {
		return auth.Subject{}, fmt.Errorf("account: the account is a %T, not a *models.User", found)
	}
	return user.Subject(), nil
}
