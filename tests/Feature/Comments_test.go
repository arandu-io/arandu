// Example resource. Remove with the list under "The example resource" in README.md.

package feature_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/arandu-io/hesape/arandutest"
	"github.com/arandu-io/hesape/auth"

	models "github.com/arandu-io/arandu/app/Models"
	policies "github.com/arandu-io/arandu/app/Policies"
	"github.com/arandu-io/arandu/bootstrap"
	factories "github.com/arandu-io/arandu/database/factories"
	"github.com/arandu-io/arandu/tests"
)

// comment writes a comment under the note at noteAddress through the browser,
// as a person would, and answers the comment's own address.
func (f notesFixture) comment(t *testing.T, client *arandutest.Client, noteAddress, body string) string {
	t.Helper()
	client.Get(noteAddress + "/comments/create").AssertOk()
	created := client.Post(noteAddress+"/comments", map[string]string{"body": body}).
		AssertStatus(http.StatusSeeOther)
	address := created.Header("Location")
	if !strings.HasPrefix(address, "/comments/") {
		t.Fatalf("storing a comment redirected to %q, want the comment's own page", address)
	}
	return address
}

// storedComment reads one comment straight from the table, past every policy.
func (f notesFixture) storedComment(t *testing.T, address string) *models.Comment {
	t.Helper()
	id := strings.TrimPrefix(address, "/comments/")
	found, err := models.Comments(f.app.DB).Find(context.Background(),
		auth.SystemGrant(policies.CommentView, bootstrap.Tenant()), id)
	if err != nil {
		t.Fatalf("reading comment %s: %v", id, err)
	}
	return found
}

func TestACommentIsWrittenUnderTheNoteAndChangedByItsAuthor(t *testing.T) {
	f := newNotesFixture(t)
	note := f.write(t, f.ana, "Groceries")

	address := f.comment(t, f.bea, note, "Add coffee.")
	stored := f.storedComment(t, address)
	if stored.NoteID != strings.TrimPrefix(note, "/notes/") || stored.UserID != f.beaID || stored.TenantID != bootstrap.Tenant() {
		t.Fatalf("stored comment = %+v, want it under %s, written by %s in the configured tenant", stored, note, f.beaID)
	}

	// The note's page links to the listing, and the listing shows the comment.
	f.ana.Get(note).AssertOk().AssertSee(note + "/comments")
	f.ana.Get(note + "/comments").AssertOk().AssertSee("Add coffee.")
	f.ana.Get(address).AssertOk().AssertSee("Add coffee.")

	// Another member reads it and may not change it; its author may.
	ana := fromPage(t, f.ana, address)
	ana.Put(address, map[string]string{"body": "Ana's now"}).AssertStatus(http.StatusForbidden)
	ana.Delete(address, nil).AssertStatus(http.StatusForbidden)

	f.bea.Get(address + "/edit").AssertOk().AssertSee("Add coffee.")
	f.bea.Put(address, map[string]string{"body": "Add coffee and milk."}).
		AssertStatus(http.StatusSeeOther).AssertRedirect(address)
	f.ana.Get(address).AssertOk().AssertSee("Add coffee and milk.")

	fromPage(t, f.bea, address).Delete(address, nil).
		AssertStatus(http.StatusSeeOther).AssertRedirect(note + "/comments")
	f.bea.Get(address).AssertStatus(http.StatusNotFound)
}

// TestTheCommentsOfANoteAreOnlyItsOwn: the listing is filtered by the note the
// service loaded, so a comment under one note never shows under another.
func TestTheCommentsOfANoteAreOnlyItsOwn(t *testing.T) {
	f := newNotesFixture(t)
	groceries := f.write(t, f.ana, "Groceries")
	chores := f.write(t, f.ana, "Chores")

	f.comment(t, f.ana, groceries, "Add coffee.")
	f.comment(t, f.ana, chores, "Water the plants.")

	f.ana.Get(groceries + "/comments").AssertOk().AssertSee("Add coffee.").AssertDontSee("Water the plants.")
	f.ana.Get(chores + "/comments").AssertOk().AssertSee("Water the plants.").AssertDontSee("Add coffee.")
}

// TestTheCommentsOfANoteOfAnotherTenantAreNotFound: the note in the path is
// navigation, and the service loads it under the note's policy and the Grant's
// tenant. Another tenant's note is not found, so nothing is listed under it and
// nothing is written under it.
func TestTheCommentsOfANoteOfAnotherTenantAreNotFound(t *testing.T) {
	f := newNotesFixture(t)
	const elsewhere = "22222222-2222-4222-8222-222222222222"
	theirs, err := factories.Notes(f.app.DB).State(func(n *models.Note) {
		n.Title = "Another tenant's secret"
		n.UserID = f.anaID
	}).CreateOne(context.Background(), auth.SystemGrant(policies.NoteCreate, elsewhere))
	if err != nil {
		t.Fatalf("creating a note in another tenant: %v", err)
	}
	theirComment, err := factories.Comments(f.app.DB).State(func(c *models.Comment) {
		c.NoteID = theirs.ID
		c.UserID = f.anaID
		c.Body = "Another tenant's comment"
	}).CreateOne(context.Background(), auth.SystemGrant(policies.CommentCreate, elsewhere))
	if err != nil {
		t.Fatalf("creating a comment in another tenant: %v", err)
	}
	listing := "/notes/" + theirs.ID + "/comments"

	f.ana.Get(listing).AssertStatus(http.StatusNotFound)
	f.ana.Get(listing + "/create").AssertOk() // the empty form reads nothing
	ana := fromPage(t, f.ana, "/notes")
	ana.Post(listing, map[string]string{"body": "Slipped in."}).AssertStatus(http.StatusNotFound)
	f.ana.Get("/comments/" + theirComment.ID).AssertStatus(http.StatusNotFound)
	ana.Put("/comments/"+theirComment.ID, map[string]string{"body": "Moved"}).AssertStatus(http.StatusNotFound)

	found, err := models.Comments(f.app.DB).Where("note_id", theirs.ID).
		Get(context.Background(), auth.SystemGrant(policies.CommentList, elsewhere))
	if err != nil {
		t.Fatalf("reading the other tenant's comments: %v", err)
	}
	if len(found) != 1 || found[0].Body != "Another tenant's comment" {
		t.Fatalf("the other tenant's note now has %+v under it", found)
	}
}

// TestTheCommentPagesAreWholeDocuments: no comment screen has a fragment, so
// a direct visit, a boosted link and a history restore all get the page, layout
// and title included, and a reload shows what was on screen.
func TestTheCommentPagesAreWholeDocuments(t *testing.T) {
	f := newNotesFixture(t)
	note := f.write(t, f.ana, "Groceries")
	address := f.comment(t, f.ana, note, "Add coffee.")

	for _, page := range []string{note + "/comments", note + "/comments/create", address, address + "/edit"} {
		for _, request := range []struct {
			kind    string
			headers map[string]string
		}{
			{"a direct visit", nil},
			{"a boosted link", map[string]string{"HX-Request": "true", "HX-Boosted": "true"}},
			{"a history restore", map[string]string{"HX-Request": "true", "HX-History-Restore-Request": "true"}},
		} {
			for name, value := range request.headers {
				f.ana.WithHeader(name, value)
			}
			answer := f.ana.Get(page)
			for name := range request.headers {
				f.ana.WithHeader(name, "")
			}
			answer.AssertOk()
			if body := answer.GetContent(); !strings.Contains(body, "<html") || !strings.Contains(body, "<title>") {
				t.Errorf("%s of %s was not answered with the whole document", request.kind, page)
			}
		}
	}
}

// TestEveryCommentPageDrawsTheAppName: CommentController hands view.New a
// title and nothing else, and every page it draws carries the configured name
// as its brand. A guest is sent to sign in before any of them renders.
func TestEveryCommentPageDrawsTheAppName(t *testing.T) {
	f := newNotesFixture(t)
	note := f.write(t, f.ana, "Groceries")
	address := f.comment(t, f.ana, note, "Add coffee.")
	for _, page := range []string{note + "/comments", note + "/comments/create", address, address + "/edit"} {
		assertDrawsTheAppName(t, page, f.ana.Get(page).AssertOk().GetContent())
	}
}

func TestCommentsNeedSomebodySignedIn(t *testing.T) {
	app := tests.Booted(t)
	guest := arandutest.NewClient(t, app.Kernel.Handler())
	guest.Get("/notes/any/comments").AssertRedirect("/auth/login")
	guest.Get("/comments/any").AssertRedirect("/auth/login")
}
