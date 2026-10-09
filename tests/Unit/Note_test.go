// Example resource. Remove with the list under "The example resource" in README.md.

package unit_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/arandu-io/hesape/auth"
	"github.com/arandu-io/hesape/database/model"

	clients "github.com/arandu-io/arandu/app/Clients"
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

// TestANoteIsPublishedOnceAtTheTimeItIsGiven is the entity's own rule, with no
// database and no clock: the time is an argument, and a second publication is
// refused with the status the router answers it with.
func TestANoteIsPublishedOnceAtTheTimeItIsGiven(t *testing.T) {
	at := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	var n models.Note

	if n.Published() {
		t.Fatal("a new note is published")
	}
	if err := n.Publish(at); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if !n.Published() || !n.PublishedAt.Equal(at) {
		t.Fatalf("PublishedAt = %v, want %v", n.PublishedAt, at)
	}

	err := n.Publish(at.Add(time.Hour))
	if !errors.Is(err, models.ErrNoteAlreadyPublished) {
		t.Fatalf("a second Publish = %v, want ErrNoteAlreadyPublished", err)
	}
	var status interface{ HTTPStatus() int }
	if !errors.As(err, &status) || status.HTTPStatus() != 409 {
		t.Errorf("the refusal answers %v, want 409", err)
	}
	if !n.PublishedAt.Equal(at) {
		t.Errorf("a refused Publish moved PublishedAt to %v", n.PublishedAt)
	}
}

// TestTheDigestReadsOnlyUnderAListGrant needs no database: a project with no
// newsletter sends nothing, and a Grant for anything but note.list is refused
// before the Model is asked for a query.
func TestTheDigestReadsOnlyUnderAListGrant(t *testing.T) {
	ctx := context.Background()
	since := time.Now().Add(-24 * time.Hour)

	if err := services.NewNoteService(nil).SendDigest(ctx, auth.SystemGrant(policies.NoteList, "t1"), since, "k"); err != nil {
		t.Fatalf("with no newsletter the digest is not a failure: %v", err)
	}

	withNewsletter := services.NewNoteService(nil).WithNewsletter(&clients.NewsletterFake{})
	if err := withNewsletter.SendDigest(ctx, auth.SystemGrant(policies.NoteView, "t1"), since, "k"); err == nil {
		t.Fatal("a Grant for note.view read the digest")
	}
}

// arandu:end custom
