// Example resource. Remove with the list under "The example resource" in README.md.

package unit_test

import (
	"context"
	"errors"
	"testing"

	"github.com/arandu-io/hesape/auth"
	"github.com/arandu-io/hesape/database/model"

	models "github.com/arandu-io/arandu/app/Models"
	policies "github.com/arandu-io/arandu/app/Policies"
	services "github.com/arandu-io/arandu/app/Services"
)

// notePersistent is the Model-first persistence boundary this module
// depends on. The proofs below fail at compile time if the entity stops
// embedding the Hesape model -- nothing else satisfies model.Entity -- or if its
// generated query can be run without a Grant.
type notePersistent interface {
	model.Entity
	Save(context.Context, auth.Grant) (bool, error)
}

var (
	_ notePersistent                                                                              = (*models.Note)(nil)
	_ func(*models.NoteQuery, context.Context, auth.Grant, ...any) (models.NoteCollection, error) = (*models.NoteQuery).Get
)

// TestEveryNoteReadRequiresAuthorization needs no database: the service
// authorizes each read before it asks the Model for a query. A nil handle turns
// an accidental query-before-policy into an immediate test failure.
func TestEveryNoteReadRequiresAuthorization(t *testing.T) {
	svc := services.NewNoteService(nil)
	ctx := context.Background()
	var anonymous auth.Subject

	calls := map[string]func() error{
		"Get": func() error {
			_, err := svc.Get(ctx, anonymous, "id")
			return err
		},
		"List": func() error {
			_, _, err := svc.List(ctx, anonymous, 1)
			return err
		},
		"Delete": func() error {
			return svc.Delete(ctx, anonymous, "id")
		},
	}

	for name, call := range calls {
		t.Run(name+" with no subject", func(t *testing.T) {
			if err := call(); !errors.Is(err, auth.ErrForbidden) {
				t.Fatalf("error = %v, want ErrForbidden", err)
			}
		})
	}
}

// TestTheNotePolicyDeniesWhatItDoesNotKnow is the property that keeps a
// policy safe as it grows: an action nobody wrote a rule for is refused, rather
// than falling through to allowed.
//
// It uses an action that will never be opened, so it keeps passing after you open
// the real ones -- a test that breaks when you do what the generator told you to
// do is a test people delete.
func TestTheNotePolicyDeniesWhatItDoesNotKnow(t *testing.T) {
	admin := auth.Subject{ID: "a1", Tenant: "t1", Roles: []string{"admin", "staff"}}

	err := (policies.NotePolicy{}).Can(context.Background(), admin,
		"note.action_that_does_not_exist", models.Note{})

	if err == nil {
		t.Fatal("an action with no rule was allowed: the policy falls through to allowed")
	}
}

// arandu:begin custom
// Tests for the rules you wrote go here, and survive regeneration.
// arandu:end custom
