// Example resource. Remove with the list under "The example resource" in README.md.

package feature_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/arandutest"

	models "github.com/arandu-io/arandu/app/Models"
	policies "github.com/arandu-io/arandu/app/Policies"
	"github.com/arandu-io/arandu/bootstrap"
	factories "github.com/arandu-io/arandu/database/factories"
	seeders "github.com/arandu-io/arandu/database/seeders"
	"github.com/arandu-io/arandu/tests"
)

// notesFixture is the application with two members of one tenant, each with a
// browser signed in as them.
type notesFixture struct {
	app      bootstrap.App
	ana, bea *arandutest.Client
	anaID    string
	beaID    string
}

func newNotesFixture(t *testing.T) notesFixture {
	t.Helper()
	app := tests.Booted(t)
	tenant := bootstrap.Tenant()
	members, err := factories.Users(app.DB).Count(2).
		Create(context.Background(), security.SystemGrant(policies.ActionUserCreate, tenant))
	if err != nil {
		t.Fatalf("creating the members: %v", err)
	}
	ana, bea := members[0].ID, members[1].ID
	return notesFixture{
		app:   app,
		ana:   tests.SignedIn(t, app, security.Subject{ID: ana, Tenant: tenant}),
		bea:   tests.SignedIn(t, app, security.Subject{ID: bea, Tenant: tenant}),
		anaID: ana,
		beaID: bea,
	}
}

// write creates a note through the browser, as a person would, and answers
// its address.
func (f notesFixture) write(t *testing.T, client *arandutest.Client, title string) string {
	t.Helper()
	client.Get("/notes/create").AssertOk()
	created := client.Post("/notes", map[string]string{"title": title, "body": "Written in a test.", "pinned": "1"}).
		AssertStatus(http.StatusSeeOther)
	address := created.Header("Location")
	if !strings.HasPrefix(address, "/notes/") || address == "/notes/create" {
		t.Fatalf("storing a note redirected to %q, want the note's own page", address)
	}
	return address
}

// stored reads one note straight from the table, past every policy, to see
// what a refused request left behind.
func (f notesFixture) stored(t *testing.T, address string) *models.Note {
	t.Helper()
	id := strings.TrimPrefix(address, "/notes/")
	note, err := models.Notes(f.app.DB).Find(context.Background(),
		security.SystemGrant(policies.NoteView, bootstrap.Tenant()), id)
	if err != nil {
		t.Fatalf("reading note %s: %v", id, err)
	}
	return note
}

// fromPage loads a page and makes every later request of the client carry the
// token the page holds for its htmx requests, as the browser does. The show
// page deletes with hx-delete, which sends no form: the token travels in the
// header the layout's hx-headers names.
func fromPage(t *testing.T, client *arandutest.Client, address string) *arandutest.Client {
	t.Helper()
	page := client.Get(address).AssertOk()
	return client.WithHeader("X-CSRF-Token", csrfTokenFromPage(t, page.GetContent()))
}

func TestNotesAreWrittenReadChangedAndDeletedByTheirAuthor(t *testing.T) {
	f := newNotesFixture(t)

	address := f.write(t, f.ana, "Groceries")
	f.ana.Get(address).AssertOk().AssertSee("Groceries").AssertSee("Written in a test.")
	f.ana.Get("/notes").AssertOk().AssertSee("Groceries")
	if note := f.stored(t, address); note.UserID != f.anaID || note.TenantID != bootstrap.Tenant() || !note.Pinned {
		t.Fatalf("stored note = %+v, want pinned, written by %s in the configured tenant", note, f.anaID)
	}

	f.ana.Get(address + "/edit").AssertOk().AssertSee("Groceries")
	f.ana.Put(address, map[string]string{"title": "Groceries for Sunday", "body": "Bread."}).
		AssertStatus(http.StatusSeeOther).AssertRedirect(address)
	f.ana.Get(address).AssertOk().AssertSee("Groceries for Sunday").AssertSee("Bread.")
	if note := f.stored(t, address); note.Pinned {
		t.Fatal("an unchecked box left the note pinned")
	}

	fromPage(t, f.ana, address).Delete(address, nil).AssertStatus(http.StatusSeeOther).AssertRedirect("/notes")
	f.ana.Get(address).AssertStatus(http.StatusNotFound)
}

func TestANoteWithoutATitleComesBackWithTheMessage(t *testing.T) {
	f := newNotesFixture(t)

	// The rejected form is drawn on the next page a browser navigates to, and a
	// browser navigating says it wants HTML: that is what tells the page from
	// the fragments and assets the same page asks for.
	f.ana.WithHeader("Accept", "text/html")
	f.ana.Get("/notes/create").AssertOk()
	f.ana.Post("/notes", map[string]string{"title": "  ", "body": "Kept as typed."}).AssertStatus(http.StatusSeeOther)
	f.ana.Get("/notes/create").AssertOk().AssertSee("Kept as typed.").AssertSee("is required")
	f.ana.Get("/notes").AssertOk().AssertDontSee("Kept as typed.")
}

func TestAnotherMembersNoteIsReadButNotChanged(t *testing.T) {
	f := newNotesFixture(t)
	address := f.write(t, f.bea, "Bea's plan")

	f.ana.Get("/notes").AssertOk().AssertSee("Bea&#39;s plan")
	f.ana.Get(address).AssertOk().AssertSee("Bea&#39;s plan")
	f.ana.Get(address + "/edit").AssertOk()
	ana := fromPage(t, f.ana, address)
	ana.Put(address, map[string]string{"title": "Ana's now"}).AssertStatus(http.StatusForbidden)
	ana.Delete(address, nil).AssertStatus(http.StatusForbidden)

	if note := f.stored(t, address); note.Title != "Bea's plan" || note.UserID != f.beaID {
		t.Fatalf("a refused change left the note as %+v", note)
	}
}

func TestANoteOfAnotherTenantIsNotFound(t *testing.T) {
	f := newNotesFixture(t)
	const elsewhere = "22222222-2222-4222-8222-222222222222"
	theirs, err := factories.Notes(f.app.DB).State(func(n *models.Note) {
		n.Title = "Another tenant's secret"
		n.UserID = f.anaID
	}).CreateOne(context.Background(), security.SystemGrant(policies.NoteCreate, elsewhere))
	if err != nil {
		t.Fatalf("creating a note in another tenant: %v", err)
	}
	address := "/notes/" + theirs.ID

	// Even with the id in hand, and even written under this member's id, the
	// row is in another tenant: every read is scoped by the Grant, so there is
	// nothing to authorize and nothing to show.
	f.ana.Get("/notes").AssertOk().AssertDontSee("Another tenant")
	f.ana.Get(address).AssertStatus(http.StatusNotFound)
	f.ana.Get(address + "/edit").AssertStatus(http.StatusNotFound)
	ana := fromPage(t, f.ana, "/notes")
	ana.Put(address, map[string]string{"title": "Moved"}).AssertStatus(http.StatusNotFound)
	ana.Delete(address, nil).AssertStatus(http.StatusNotFound)

	still, err := models.Notes(f.app.DB).Find(context.Background(), security.SystemGrant(policies.NoteView, elsewhere), theirs.ID)
	if err != nil || still == nil || still.Title != "Another tenant's secret" {
		t.Fatalf("the other tenant's note is now %+v (%v)", still, err)
	}
}

func TestNotesNeedSomebodySignedIn(t *testing.T) {
	app := tests.Booted(t)
	guest := arandutest.NewClient(t, app.Kernel.Handler())
	guest.Get("/notes").AssertRedirect("/auth/login")
	guest.Get("/notes/create").AssertRedirect("/auth/login")
}

// TestTheExampleNotesAreSeededInDevelopmentOnly runs the root seeder the way a
// developer does after migrating: in development it writes the example notes,
// by authors nobody can sign in as, once however often it runs. Told it is not
// development, it writes nothing.
func TestTheExampleNotesAreSeededInDevelopmentOnly(t *testing.T) {
	sqliteEnv(t)
	if err := bootstrap.Dispatch("migrate", nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cfg, db, _ := openForTest(t)
	ctx := context.Background()
	notes := func() []*models.Note {
		t.Helper()
		found, err := models.Notes(db).Get(ctx, security.SystemGrant(policies.NoteList, bootstrap.Tenant()))
		if err != nil {
			t.Fatalf("reading the seeded notes: %v", err)
		}
		return found
	}

	production := seeders.Deps{DB: db, Tenant: cfg.Auth.Tenant}
	if _, err := seeders.Run(ctx, production, nil); err != nil {
		t.Fatalf("seeding outside development: %v", err)
	}
	if got := len(notes()); got != 0 {
		t.Fatalf("seeding outside development wrote %d notes, want none", got)
	}

	for range 2 {
		if err := bootstrap.Dispatch("db:seed", nil); err != nil {
			t.Fatalf("db:seed: %v", err)
		}
	}
	seeded := notes()
	if len(seeded) != 6 {
		t.Fatalf("two runs of the root seeder in development wrote %d notes, want 6", len(seeded))
	}
	for _, note := range seeded {
		author, err := models.Users(db).Find(ctx, security.SystemGrant(policies.ActionUserView, bootstrap.Tenant()), note.UserID)
		if err != nil || author == nil {
			t.Fatalf("note %s has no author in the tenant: %v", note.ID, err)
		}
		if author.Password != factories.UnusablePassword {
			t.Fatalf("the seeded author %s has a password somebody could type", author.Email)
		}
	}
}
