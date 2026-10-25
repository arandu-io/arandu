// Example resource. Remove with the list under "The example resource" in README.md.

package feature_test

import (
	"context"
	"encoding/json"
	"errors"
	"html"
	"net/http"
	"strings"
	"testing"
	"time"
	// The zone database, linked into this test binary, so the zone
	// TestANotesDatesAreShownInTheApplicationsTimeZone serves under loads on
	// a host that ships none.
	_ "time/tzdata"

	"github.com/arandu-io/framework/test"
	"github.com/arandu-io/hesape/auth"
	accounts "github.com/arandu-io/hesape/auth/account"
	"github.com/arandu-io/hesape/events"
	hnotifications "github.com/arandu-io/hesape/notifications"
	"github.com/arandu-io/hesape/queue"

	clients "github.com/arandu-io/arandu/app/Clients"
	appevents "github.com/arandu-io/arandu/app/Events"
	appjobs "github.com/arandu-io/arandu/app/Jobs"
	models "github.com/arandu-io/arandu/app/Models"
	notifications "github.com/arandu-io/arandu/app/Notifications"
	policies "github.com/arandu-io/arandu/app/Policies"
	services "github.com/arandu-io/arandu/app/Services"
	appconfig "github.com/arandu-io/arandu/config"
	factories "github.com/arandu-io/arandu/database/factories"
	seeders "github.com/arandu-io/arandu/database/seeders"
	"github.com/arandu-io/arandu/lang"
	"github.com/arandu-io/arandu/tests"
)

// notesFixture is the application with two members of one tenant, each with a
// browser signed in as them.
type notesFixture struct {
	server   *tests.Server
	app      tests.Application
	ana, bea *test.Client
	anaID    string
	beaID    string
	// members are the two accounts by id, as the factory stored them.
	members map[string]*models.User
}

func newNotesFixture(t *testing.T) notesFixture {
	t.Helper()
	server := tests.Serve(t)
	tenant := appconfig.Tenant()
	members, err := factories.Users(server.App.DB).Count(2).
		Create(context.Background(), auth.SystemGrant(accounts.ActionUserCreate, tenant))
	if err != nil {
		t.Fatalf("creating the members: %v", err)
	}
	ana, bea := members[0], members[1]
	return notesFixture{
		server:  server,
		app:     server.App,
		ana:     tests.SignIn(t, server, ana),
		bea:     tests.SignIn(t, server, bea),
		anaID:   ana.ID,
		beaID:   bea.ID,
		members: map[string]*models.User{ana.ID: ana, bea.ID: bea},
	}
}

// subject is the session subject of one of the two members, read from the
// account the factory stored: what the resolver answers for them.
func (f notesFixture) subject(t *testing.T, id string) auth.Subject {
	t.Helper()
	member, ok := f.members[id]
	if !ok {
		t.Fatalf("%s is not one of the fixture's members", id)
	}
	return member.Subject()
}

// write creates a note through the browser, as a person would, and answers
// its address.
func (f notesFixture) write(t *testing.T, client *test.Client, title string) string {
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
		auth.SystemGrant(policies.NoteView, appconfig.Tenant()), id)
	if err != nil {
		t.Fatalf("reading note %s: %v", id, err)
	}
	return note
}

func TestNotesAreWrittenReadChangedAndDeletedByTheirAuthor(t *testing.T) {
	f := newNotesFixture(t)

	address := f.write(t, f.ana, "Groceries")
	f.ana.Get(address).AssertOk().AssertSee("Groceries").AssertSee("Written in a test.")
	f.ana.Get("/notes").AssertOk().AssertSee("Groceries")
	if note := f.stored(t, address); note.UserID != f.anaID || note.TenantID != appconfig.Tenant() || !note.Pinned {
		t.Fatalf("stored note = %+v, want pinned, written by %s in the configured tenant", note, f.anaID)
	}

	f.ana.Get(address + "/edit").AssertOk().AssertSee("Groceries")
	f.ana.Put(address, map[string]string{"title": "Groceries for Sunday", "body": "Bread."}).
		AssertStatus(http.StatusSeeOther).AssertRedirect(address)
	f.ana.Get(address).AssertOk().AssertSee("Groceries for Sunday").AssertSee("Bread.")
	if note := f.stored(t, address); note.Pinned {
		t.Fatal("an unchecked box left the note pinned")
	}

	tests.FromPage(t, f.ana, address).Delete(address, nil).AssertStatus(http.StatusSeeOther).AssertRedirect("/notes")
	f.ana.Get(address).AssertStatus(http.StatusNotFound)
}

// count is how many notes the tenant holds, read past every policy.
func (f notesFixture) count(t *testing.T) int {
	t.Helper()
	found, err := models.Notes(f.app.DB).Get(context.Background(),
		auth.SystemGrant(policies.NoteList, appconfig.Tenant()))
	if err != nil {
		t.Fatalf("counting the notes: %v", err)
	}
	return len(found)
}

// untitled is a note the request refuses: the title is blank once trimmed.
var untitled = map[string]string{"title": "  ", "body": "Kept as typed."}

func TestANoteWithoutATitleComesBackWithTheMessage(t *testing.T) {
	f := newNotesFixture(t)

	// The rejected form is drawn on the next page a browser navigates to, and a
	// browser navigating says it wants HTML: that is what tells the page from
	// the fragments and assets the same page asks for. The Referer is the form
	// the browser posted from, and it is where the answer sends it back.
	browser := f.ana.WithHeader("Accept", "text/html")
	browser.Get("/notes/create").AssertOk()
	rejected := browser.WithHeader("Referer", "/notes/create").Post("/notes", untitled)
	browser.WithHeader("Referer", "")

	// 303 and nothing else. It tells the browser to GET the address it is sent
	// to, so the entry the history keeps is that GET: a reload asks for the
	// form again instead of posting it. A 307 or 308 would post it again, and a
	// 422 with the form in its body would leave the POST as the entry a reload
	// repeats.
	rejected.AssertStatus(http.StatusSeeOther).AssertRedirect("/notes/create")
	if got := rejected.Header("Cache-Control"); !strings.Contains(got, "no-store") {
		t.Errorf("the answer to a rejected form is cacheable (Cache-Control %q), and it carries what one person typed", got)
	}

	browser.Get("/notes/create").AssertOk().AssertSee("Kept as typed.").AssertSee("is required")

	// The reload: the same GET, which the flash has already been spent on.
	browser.Get("/notes/create").AssertOk().AssertDontSee("Kept as typed.").AssertDontSee("is required")
	browser.Get("/notes").AssertOk().AssertDontSee("Kept as typed.")
	if got := f.count(t); got != 0 {
		t.Fatalf("a rejected note and a reload left %d notes stored, want none", got)
	}
}

// TestARejectedNoteFromHTMXIsANavigationBack: the create form posts with
// hx-post, and htmx discards a 4xx body by default. What it does follow is
// HX-Redirect, as a full navigation -- so the answer is the same redirect back
// to the form, spelled the way htmx reads it, with no body to swap.
func TestARejectedNoteFromHTMXIsANavigationBack(t *testing.T) {
	f := newNotesFixture(t)

	f.ana.Get("/notes/create").AssertOk()
	rejected := f.ana.WithHeader("HX-Request", "true").WithHeader("Referer", "/notes/create").
		Post("/notes", untitled)
	f.ana.WithHeader("HX-Request", "").WithHeader("Referer", "")

	rejected.AssertStatus(http.StatusNoContent).AssertRedirect("/notes/create")
	if body := rejected.GetContent(); body != "" {
		t.Errorf("the HTMX answer to a rejected form has a body htmx would swap in before navigating: %q", body)
	}

	// The navigation htmx makes is an ordinary GET, and it finds the messages.
	f.ana.WithHeader("Accept", "text/html").Get("/notes/create").AssertOk().
		AssertSee("Kept as typed.").AssertSee("is required")
	if got := f.count(t); got != 0 {
		t.Fatalf("a rejected note left %d notes stored, want none", got)
	}
}

// TestARejectedNoteFromAJSONClientIsAProblemDocument: a client that asked for
// JSON has no form to go back to. It gets 422 and the messages by field, and
// nothing is left in the flash for a page nobody will load.
func TestARejectedNoteFromAJSONClientIsAProblemDocument(t *testing.T) {
	f := newNotesFixture(t)

	f.ana.Get("/notes/create").AssertOk()
	rejected := f.ana.WithHeader("Accept", "application/json").Post("/notes", untitled)
	f.ana.WithHeader("Accept", "")

	rejected.AssertStatus(http.StatusUnprocessableEntity)
	if got := rejected.Header("Content-Type"); !strings.HasPrefix(got, "application/problem+json") {
		t.Fatalf("Content-Type = %q, want application/problem+json", got)
	}
	var problem struct {
		Status int                 `json:"status"`
		Errors map[string][]string `json:"errors"`
	}
	if err := json.Unmarshal([]byte(rejected.GetContent()), &problem); err != nil {
		t.Fatalf("the body is not a problem document: %v\n%s", err, rejected.GetContent())
	}
	if problem.Status != http.StatusUnprocessableEntity || len(problem.Errors["title"]) == 0 {
		t.Fatalf("problem = %+v, want status 422 and a message for title", problem)
	}
	if _, typed := problem.Errors["body"]; typed {
		t.Errorf("a field that passed has a message: %v", problem.Errors["body"])
	}

	f.ana.WithHeader("Accept", "text/html").Get("/notes/create").AssertOk().
		AssertDontSee("Kept as typed.").AssertDontSee("is required")
	if got := f.count(t); got != 0 {
		t.Fatalf("a rejected note left %d notes stored, want none", got)
	}
}

// TestTheNotesTableIsAnsweredAloneOnlyToTheElementThatAsksForIt.
//
// The listing's address has two representations. The table alone goes to the
// one request that asks for it: htmx naming the table's element as the target
// it will replace. Every other request -- a direct visit, a boosted link, a
// history restore, an htmx request aimed at another element -- gets the whole
// document, because each of them replaces the page, and a reload of any of
// them has to show the same thing.
func TestTheNotesTableIsAnsweredAloneOnlyToTheElementThatAsksForIt(t *testing.T) {
	f := newNotesFixture(t)
	f.write(t, f.ana, "Groceries")

	const table = `id="notes-index"`
	// asks loads the listing with the headers a request of that kind carries.
	// The client's assertions stop the test, so this runs on the test's own
	// goroutine rather than in a subtest.
	asks := func(kind string, headers map[string]string) string {
		t.Helper()
		for name, value := range headers {
			f.ana.WithHeader(name, value)
		}
		answer := f.ana.Get("/notes")
		for name := range headers {
			f.ana.WithHeader(name, "")
		}
		answer.AssertOk().AssertSee("Groceries")
		vary := answer.Header("Vary")
		for _, header := range []string{"HX-Request", "HX-Target"} {
			if !strings.Contains(vary, header) {
				t.Errorf("%s: Vary = %q, and the answer depends on %s: "+
					"a cache that is not told can hand one representation to a request for the other", kind, vary, header)
			}
		}
		return answer.GetContent()
	}

	for _, request := range []struct {
		kind    string
		headers map[string]string
	}{
		{"a direct visit", nil},
		{"a boosted link", map[string]string{"HX-Request": "true", "HX-Boosted": "true"}},
		{"a history restore", map[string]string{"HX-Request": "true", "HX-History-Restore-Request": "true"}},
		{"an htmx request for another element", map[string]string{"HX-Request": "true", "HX-Target": "main"}},
		{"the table's target named without htmx", map[string]string{"HX-Target": "notes-index"}},
	} {
		page := asks(request.kind, request.headers)
		if !strings.Contains(page, "<html") || !strings.Contains(page, table) {
			t.Errorf("%s was not answered with the whole page around the table:\n%s", request.kind, page)
		}
	}

	fragment := strings.TrimSpace(asks("the table's own request", map[string]string{"HX-Request": "true", "HX-Target": "notes-index"}))
	if strings.Contains(fragment, "<html") || !strings.HasPrefix(fragment, "<div") || !strings.Contains(fragment, table) {
		t.Errorf("the table's own request was not answered with the table alone:\n%s", fragment)
	}
}

func TestAnotherMembersNoteIsReadButNotChanged(t *testing.T) {
	f := newNotesFixture(t)
	address := f.write(t, f.bea, "Bea's plan")

	f.ana.Get("/notes").AssertOk().AssertSee("Bea&#39;s plan")
	// The page offers none of the three changes the policy would refuse, and
	// the edit form is refused rather than drawn for a submission that would
	// be.
	f.ana.Get(address).AssertOk().AssertSee("Bea&#39;s plan").
		AssertDontSee(address + "/edit").AssertDontSee(`hx-delete="` + address).AssertDontSee(address + "/publish")
	f.ana.Get(address + "/edit").AssertStatus(http.StatusForbidden)
	f.bea.Get(address).AssertOk().
		AssertSee(address + "/edit").AssertSee(`hx-delete="` + address).AssertSee(address + "/publish")
	f.bea.Get(address + "/edit").AssertOk()
	ana := tests.FromPage(t, f.ana, address)
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
	}).CreateOne(context.Background(), auth.SystemGrant(policies.NoteCreate, elsewhere))
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
	ana := tests.FromPage(t, f.ana, "/notes")
	ana.Put(address, map[string]string{"title": "Moved"}).AssertStatus(http.StatusNotFound)
	ana.Delete(address, nil).AssertStatus(http.StatusNotFound)

	still, err := models.Notes(f.app.DB).Find(context.Background(), auth.SystemGrant(policies.NoteView, elsewhere), theirs.ID)
	if err != nil || still == nil || still.Title != "Another tenant's secret" {
		t.Fatalf("the other tenant's note is now %+v (%v)", still, err)
	}
}

// TestEveryNotePageDrawsTheAppName: NoteController hands view.New a title and
// nothing else, and every page it draws carries the configured name as its
// brand, put on the request before the route ran. A guest is sent to sign in
// before any of them renders, so there is no guest half to check.
func TestEveryNotePageDrawsTheAppName(t *testing.T) {
	f := newNotesFixture(t)
	address := f.write(t, f.ana, "Groceries")
	for _, page := range []string{"/notes", "/notes/create", address, address + "/edit"} {
		assertDrawsTheAppName(t, page, f.ana.Get(page).AssertOk().GetContent())
	}
}

func TestNotesNeedSomebodySignedIn(t *testing.T) {
	guest := tests.Serve(t).Client()
	guest.Get("/notes").AssertRedirect("/auth/login")
	guest.Get("/notes/create").AssertRedirect("/auth/login")
}

// TestTheExampleNotesAreSeededInDevelopmentOnly runs the root seeder the way a
// developer does after migrating: in development it writes the example notes,
// by authors nobody can sign in as, once however often it runs. Told it is not
// development, it writes nothing.
func TestTheExampleNotesAreSeededInDevelopmentOnly(t *testing.T) {
	sqliteEnv(t)
	if err := tests.Dispatch("migrate", nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cfg, db, _ := openForTest(t)
	ctx := context.Background()
	notes := func() []*models.Note {
		t.Helper()
		found, err := models.Notes(db).Get(ctx, auth.SystemGrant(policies.NoteList, appconfig.Tenant()))
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
		if err := tests.Dispatch("db:seed", nil); err != nil {
			t.Fatalf("db:seed: %v", err)
		}
	}
	seeded := notes()
	if len(seeded) != 6 {
		t.Fatalf("two runs of the root seeder in development wrote %d notes, want 6", len(seeded))
	}
	for _, note := range seeded {
		author, err := models.Users(db).Find(ctx, auth.SystemGrant(accounts.ActionUserView, appconfig.Tenant()), note.UserID)
		if err != nil || author == nil {
			t.Fatalf("note %s has no author in the tenant: %v", note.ID, err)
		}
		if author.Password != factories.UnusablePassword {
			t.Fatalf("the seeded author %s has a password somebody could type", author.Email)
		}
	}
}

// published is the note.published events waiting in the outbox of the
// configured tenant: what the relay will hand the listeners.
func (f notesFixture) published(t *testing.T) []events.Stored {
	t.Helper()
	pending, err := events.NewOutbox(f.app.DB).Pending(context.Background(), appconfig.Tenant(), 100)
	if err != nil {
		t.Fatalf("reading the outbox: %v", err)
	}
	var found []events.Stored
	for _, e := range pending {
		if e.Name == appevents.NotePublishedName {
			found = append(found, e)
		}
	}
	return found
}

func TestTheAuthorPublishesANoteOnceWithItsEvent(t *testing.T) {
	f := newNotesFixture(t)
	address := f.write(t, f.ana, "Groceries")

	// The page offers the action while the note is a draft.
	f.ana.Get(address).AssertOk().AssertSee("Draft").AssertSee(address + "/publish")

	tests.FromPage(t, f.ana, address).Post(address+"/publish", nil).
		AssertStatus(http.StatusSeeOther).AssertRedirect(address)

	note := f.stored(t, address)
	if !note.Published() {
		t.Fatalf("the note was not published: %+v", note)
	}
	f.ana.Get(address).AssertOk().AssertDontSee("Draft").AssertDontSee(address + "/publish")

	// The row and its event were written together: one event, about this
	// note, sealed with the Grant the policy issued for the publication.
	stored := f.published(t)
	if len(stored) != 1 {
		t.Fatalf("%d note.published events in the outbox, want 1", len(stored))
	}
	var payload appevents.NotePublished
	if err := stored[0].Decode(&payload); err != nil {
		t.Fatalf("decoding the event: %v", err)
	}
	if stored[0].AggregateID != note.ID || payload.Author != f.anaID || payload.Title != "Groceries" ||
		stored[0].Action != string(policies.NotePublish) {
		t.Fatalf("event = %+v, payload = %+v, want this note's publication by %s", stored[0], payload, f.anaID)
	}

	// A second publication is a conflict with the row's state, and stores
	// nothing more.
	tests.FromPage(t, f.ana, address).Post(address+"/publish", nil).AssertStatus(http.StatusConflict)
	if got := len(f.published(t)); got != 1 {
		t.Fatalf("publishing twice left %d events, want 1", got)
	}
}

func TestAnotherMembersNoteIsNotPublished(t *testing.T) {
	f := newNotesFixture(t)
	address := f.write(t, f.bea, "Bea's plan")

	tests.FromPage(t, f.ana, address).Post(address+"/publish", nil).AssertStatus(http.StatusForbidden)

	if note := f.stored(t, address); note.Published() {
		t.Fatal("a refused publication published the note")
	}
	if got := len(f.published(t)); got != 0 {
		t.Fatalf("a refused publication stored %d events", got)
	}
}

func TestANoteOfAnotherTenantIsNotPublished(t *testing.T) {
	f := newNotesFixture(t)
	const elsewhere = "22222222-2222-4222-8222-222222222222"
	theirs, err := factories.Notes(f.app.DB).State(func(n *models.Note) { n.UserID = f.anaID }).
		CreateOne(context.Background(), auth.SystemGrant(policies.NoteCreate, elsewhere))
	if err != nil {
		t.Fatalf("creating a note in another tenant: %v", err)
	}

	tests.FromPage(t, f.ana, "/notes").Post("/notes/"+theirs.ID+"/publish", nil).AssertStatus(http.StatusNotFound)

	still, err := models.Notes(f.app.DB).Find(context.Background(), auth.SystemGrant(policies.NoteView, elsewhere), theirs.ID)
	if err != nil || still.Published() {
		t.Fatalf("the other tenant's note is now %+v (%v)", still, err)
	}
}

// TestPublishingIsNeverAGet: the action changes state, so the route answers
// POST and nothing else. A GET that published would be fired by a browser
// prefetching the link.
func TestPublishingIsNeverAGet(t *testing.T) {
	f := newNotesFixture(t)
	address := f.write(t, f.ana, "Groceries")

	f.ana.Get(address + "/publish").AssertStatus(http.StatusMethodNotAllowed)
	if f.stored(t, address).Published() {
		t.Fatal("a GET published the note")
	}
}

// asJSON sends one request as a client that asked for JSON, and puts the
// client back the way it was.
func asJSON(client *test.Client, send func(*test.Client) *test.Response) *test.Response {
	answer := send(client.WithHeader("Accept", "application/json"))
	client.WithHeader("Accept", "")
	return answer
}

// decodeJSON reads an answer that must be JSON of the given media type.
func decodeJSON(t *testing.T, answer *test.Response, mediaType string, into any) {
	t.Helper()
	if got := answer.Header("Content-Type"); !strings.HasPrefix(got, mediaType) {
		t.Fatalf("Content-Type = %q, want %s", got, mediaType)
	}
	if err := json.Unmarshal([]byte(answer.GetContent()), into); err != nil {
		t.Fatalf("the body is not JSON: %v\n%s", err, answer.GetContent())
	}
}

// TestAJSONClientIsAnsweredThroughTheNoteResource: the same routes answer a
// client that asked for JSON, through the JSON Resource, with the fields it
// lists -- the tenant is not among them -- and the Vary that says the answer
// depends on Accept.
func TestAJSONClientIsAnsweredThroughTheNoteResource(t *testing.T) {
	f := newNotesFixture(t)
	address := f.write(t, f.ana, "Groceries")
	id := strings.TrimPrefix(address, "/notes/")

	shown := asJSON(f.ana, func(c *test.Client) *test.Response { return c.Get(address) }).AssertOk()
	if vary := shown.Header("Vary"); !strings.Contains(vary, "Accept") {
		t.Errorf("Vary = %q: the record answers HTML or JSON by Accept", vary)
	}
	var one struct {
		Data map[string]any `json:"data"`
	}
	decodeJSON(t, shown, "application/json", &one)
	if one.Data["id"] != id || one.Data["title"] != "Groceries" || one.Data["published_at"] != nil {
		t.Errorf("data = %v, want the draft note %s", one.Data, id)
	}
	if _, leaked := one.Data["tenant_id"]; leaked {
		t.Error("the tenant left in the answer, and the resource does not list it")
	}

	listed := asJSON(f.ana, func(c *test.Client) *test.Response { return c.Get("/notes") }).AssertOk()
	var page struct {
		Data struct {
			Notes []map[string]any `json:"notes"`
		} `json:"data"`
	}
	decodeJSON(t, listed, "application/json", &page)
	if len(page.Data.Notes) != 1 || page.Data.Notes[0]["id"] != id {
		t.Errorf("notes = %v, want the one note", page.Data.Notes)
	}

	browser := tests.FromPage(t, f.ana, address)
	publishedNow := asJSON(browser, func(c *test.Client) *test.Response {
		return c.Post(address+"/publish", nil)
	}).AssertOk()
	decodeJSON(t, publishedNow, "application/json", &one)
	if one.Data["published_at"] == nil {
		t.Errorf("data = %v, want the published note", one.Data)
	}
}

// TestAJSONClientIsRefusedWithProblemDocuments: what the router answers a page
// with a status, it answers a JSON client with a problem document carrying the
// same status -- a missing row, a refusal and a conflict with the row's state.
func TestAJSONClientIsRefusedWithProblemDocuments(t *testing.T) {
	f := newNotesFixture(t)
	beas := f.write(t, f.bea, "Bea's plan")
	anas := f.write(t, f.ana, "Groceries")
	const elsewhere = "22222222-2222-4222-8222-222222222222"
	theirs, err := factories.Notes(f.app.DB).State(func(n *models.Note) { n.UserID = f.anaID }).
		CreateOne(context.Background(), auth.SystemGrant(policies.NoteCreate, elsewhere))
	if err != nil {
		t.Fatalf("creating a note in another tenant: %v", err)
	}

	browser := tests.FromPage(t, f.ana, anas)
	browser.Post(anas+"/publish", nil).AssertStatus(http.StatusSeeOther)

	for _, refused := range []struct {
		kind   string
		status int
		send   func(*test.Client) *test.Response
	}{
		{"another tenant's note", http.StatusNotFound, func(c *test.Client) *test.Response {
			return c.Get("/notes/" + theirs.ID)
		}},
		{"another member's note published", http.StatusForbidden, func(c *test.Client) *test.Response {
			return c.Post(beas+"/publish", nil)
		}},
		{"a note published twice", http.StatusConflict, func(c *test.Client) *test.Response {
			return c.Post(anas+"/publish", nil)
		}},
	} {
		answer := asJSON(browser, refused.send).AssertStatus(refused.status)
		var problem struct {
			Status int    `json:"status"`
			Title  string `json:"title"`
		}
		decodeJSON(t, answer, "application/problem+json", &problem)
		if problem.Status != refused.status || problem.Title == "" {
			t.Errorf("%s: problem = %+v, want status %d with a title", refused.kind, problem, refused.status)
		}
	}
}

// account is a recipient by id, the way the bell menu of a signed-in account
// is read back.
type account string

func (a account) NotifiableID() string                     { return string(a) }
func (account) NotifiableType() string                     { return "user" }
func (account) RouteFor(hnotifications.ChannelName) string { return "" }

// TestTheAuthorOfAPublishedNoteIsToldThroughTheOutbox: the publication stores
// its event and tells nobody; the application's relay hands the committed
// event to its listeners, and NotifyNoteAuthor stores the row the author's
// bell menu draws -- read back here as the author, under the notifications
// policy.
func TestTheAuthorOfAPublishedNoteIsToldThroughTheOutbox(t *testing.T) {
	f := newNotesFixture(t)
	address := f.write(t, f.ana, "Groceries")
	tests.FromPage(t, f.ana, address).Post(address+"/publish", nil).AssertStatus(http.StatusSeeOther)

	ctx := context.Background()
	store := hnotifications.NewTableStore(f.app.DB)
	bell := func(who string) []hnotifications.Record {
		t.Helper()
		g, err := auth.Authorize(ctx, hnotifications.Policy{}, f.subject(t, who),
			hnotifications.ActionList, hnotifications.Record{})
		if err != nil {
			t.Fatalf("authorizing the bell menu of %s: %v", who, err)
		}
		rows, err := store.For(ctx, g, account(who), 10)
		if err != nil {
			t.Fatalf("reading the bell menu of %s: %v", who, err)
		}
		return rows
	}

	if got := bell(f.anaID); len(got) != 0 {
		t.Fatalf("the author was told before the relay ran: %v", got)
	}
	if err := f.app.Relay.Drain(ctx); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	rows := bell(f.anaID)
	if len(rows) != 1 || rows[0].Key != notifications.NotePublishedKey {
		t.Fatalf("the author's bell menu holds %v, want one %s", rows, notifications.NotePublishedKey)
	}
	if id := strings.TrimPrefix(address, "/notes/"); !strings.Contains(string(rows[0].Data), id) {
		t.Errorf("the stored row %s does not name the note %s", rows[0].Data, id)
	}
	if got := bell(f.beaID); len(got) != 0 {
		t.Errorf("another member was told about a note they did not write: %v", got)
	}
}

// TestTheNightlyDigestIsScheduledQueuedAndSentWithoutTheNetwork walks the
// digest end to end: the schedule the application declares, run now on the
// path the scheduler takes, enqueues the job; the worker runs it under the
// Grant it rebuilt from the row; and the service hands the newsletter the
// notes published in the window -- the fake, so nothing leaves the process
// and no credential is needed.
func TestTheNightlyDigestIsScheduledQueuedAndSentWithoutTheNetwork(t *testing.T) {
	f := newNotesFixture(t)
	ctx := context.Background()

	// The schedule as the binary lists it: the digest, nightly at 02:00, on
	// one replica.
	listed, err := tests.Run("schedule:list")
	if err != nil {
		t.Fatalf("schedule:list: %v", err)
	}
	if !scheduledRow(listed, appjobs.SendNotesDigestName, "0 2 * * *", "true") {
		t.Fatalf("the digest is not listed nightly at 02:00 on one replica:\n%s", listed)
	}

	published := f.write(t, f.ana, "Groceries")
	tests.FromPage(t, f.ana, published).Post(published+"/publish", nil).AssertStatus(http.StatusSeeOther)
	f.write(t, f.ana, "A draft")

	if err := tests.Dispatch("schedule:run", []string{appjobs.SendNotesDigestName, "--tenant=" + appconfig.Tenant()}); err != nil {
		t.Fatalf("running the digest task: %v", err)
	}
	if pending, err := f.app.Queue.PendingSize(ctx, ""); err != nil || pending != 1 {
		t.Fatalf("%d jobs queued (%v), want the digest", pending, err)
	}

	newsletter := &clients.NewsletterFake{}
	w := queue.NewWorker(f.app.Queue, queue.WorkerOptions{Sleep: time.Millisecond})
	w.Handle(appjobs.SendNotesDigestName, appjobs.NewSendNotesDigestHandler(
		services.NewNoteService(f.app.DB).WithNewsletter(newsletter)))
	if err := w.RunNextJob(ctx); err != nil {
		t.Fatalf("running the digest job: %v", err)
	}

	if len(newsletter.Digests) != 1 {
		t.Fatalf("the newsletter was handed %d digests (calls %v), want 1", len(newsletter.Digests), newsletter.Calls)
	}
	digest := newsletter.Digests[0]
	if len(digest.Notes) != 1 || digest.Notes[0].Title != "Groceries" || digest.Key == "" {
		t.Fatalf("digest = %+v, want the one published note, keyed by the job", digest)
	}
}

// scheduledRow reports whether the schedule:list table has a row for the task
// id with the spec and the singleton column given.
func scheduledRow(listed, id, spec, singleton string) bool {
	for _, line := range strings.Split(listed, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), id+" ") &&
			strings.Contains(line, spec) && strings.Contains(line, " "+singleton+" ") {
			return true
		}
	}
	return false
}

// TestAGrantIssuedToViewANoteCannotSaveIt: NotePolicy says which of its
// actions write, so the Grant auth.Authorize issues for note.view is one that
// reads. The author's own Grant to view the note -- a decision the policy
// allows -- is refused by Save and by Delete, and the row is as it was. The
// Grant for note.update, about the same row, saves it.
func TestAGrantIssuedToViewANoteCannotSaveIt(t *testing.T) {
	f := newNotesFixture(t)
	address := f.write(t, f.ana, "Groceries")
	ctx := context.Background()
	ana := f.subject(t, f.anaID)
	note := f.stored(t, address)

	viewing, err := auth.Authorize(ctx, policies.NotePolicy{}, ana, policies.NoteView, *note)
	if err != nil {
		t.Fatalf("authorizing the author to view the note: %v", err)
	}
	note.Title = "Changed under a view"
	if _, err := note.Save(ctx, viewing); !errors.Is(err, auth.ErrReadGrant) {
		t.Fatalf("Save under the note.view Grant = %v, want auth.ErrReadGrant", err)
	}
	if _, err := note.Delete(ctx, viewing); !errors.Is(err, auth.ErrReadGrant) {
		t.Fatalf("Delete under the note.view Grant = %v, want auth.ErrReadGrant", err)
	}
	if stored := f.stored(t, address); stored == nil || stored.Title != "Groceries" {
		t.Fatalf("a refused write left the note as %+v", stored)
	}

	updating, err := auth.Authorize(ctx, policies.NotePolicy{}, ana, policies.NoteUpdate, *note)
	if err != nil {
		t.Fatalf("authorizing the author to update the note: %v", err)
	}
	if _, err := note.Save(ctx, updating); err != nil {
		t.Fatalf("Save under the note.update Grant: %v", err)
	}
	if stored := f.stored(t, address); stored.Title != "Changed under a view" {
		t.Fatalf("the note.update Grant left the title %q", stored.Title)
	}
}

// toastSays reports whether the page draws sentence as the toast inside the
// layout's #toaster tray, and not only somewhere on the page.
func toastSays(page, sentence string) bool {
	start := strings.Index(page, `id="toaster"`)
	if start < 0 {
		return false
	}
	tray := page[start:]
	if end := strings.Index(tray, "</footer>"); end >= 0 {
		tray = tray[:end]
	}
	return strings.Contains(tray, `class="toast`) && strings.Contains(tray, sentence)
}

// TestAWriteThatWorkedSaysSoOnThePageItLandsOn: Store, Update and Destroy
// leave a sentence with ctx.With before redirecting, and the layout draws it
// as a toast in #toaster on the page the browser lands on -- once: a reload
// of that page draws no toast.
func TestAWriteThatWorkedSaysSoOnThePageItLandsOn(t *testing.T) {
	f := newNotesFixture(t)
	// A browser navigating says it wants HTML, and the flash is drawn on that
	// navigation rather than spent on an asset or a fragment.
	browser := f.ana.WithHeader("Accept", "text/html")
	defer f.ana.WithHeader("Accept", "")

	lands := func(step string, answer *test.Response, sentence string) {
		t.Helper()
		answer.AssertStatus(http.StatusSeeOther)
		address := answer.Header("Location")
		if page := browser.Get(address).AssertOk().GetContent(); !toastSays(page, sentence) {
			t.Fatalf("%s: the page at %s draws no toast saying %q:\n%s", step, address, sentence, page)
		}
		if page := browser.Get(address).AssertOk().GetContent(); strings.Contains(page, sentence) {
			t.Fatalf("%s: a reload of %s says %q again", step, address, sentence)
		}
	}

	browser.Get("/notes/create").AssertOk()
	created := browser.Post("/notes", map[string]string{"title": "Groceries", "body": "Bread."})
	address := created.Header("Location")
	lands("store", created, "Note created.")

	lands("update", browser.Put(address, map[string]string{"title": "Groceries for Sunday"}), "Note saved.")

	lands("destroy", tests.FromPage(t, browser, address).Delete(address, nil), "Note deleted.")
}

// TestTheNotePagesSayTheirLinesThroughTheCatalogue: the pages say their words
// with .Trans and a key of lang/en.json, and the provider hands the
// application the translator built from it. A page drawn without one shows
// the key itself, so no key of the catalogue may appear on any note page.
func TestTheNotePagesSayTheirLinesThroughTheCatalogue(t *testing.T) {
	raw, err := lang.Files.ReadFile("en.json")
	if err != nil {
		t.Fatalf("reading lang/en.json: %v", err)
	}
	var catalogue map[string]string
	if err := json.Unmarshal(raw, &catalogue); err != nil {
		t.Fatalf("lang/en.json is not a JSON object of lines: %v", err)
	}
	if len(catalogue) == 0 {
		t.Fatal("lang/en.json holds no line")
	}

	f := newNotesFixture(t)
	address := f.write(t, f.ana, "Groceries")
	for _, page := range []string{"/notes", "/notes/create", address, address + "/edit"} {
		body := f.ana.Get(page).AssertOk().GetContent()
		for key := range catalogue {
			if strings.Contains(body, key) {
				t.Errorf("%s shows the key %q instead of its line: the request carries no translator", page, key)
			}
		}
	}
}

// TestTheListingIsPagedByTheTablesPager: a listing longer than one page draws
// the table's own pager -- a real address for the next page, which htmx fetches
// into the table's element -- and the second page steps back to the first. The
// steps with nowhere to go are disabled and carry no address.
func TestTheListingIsPagedByTheTablesPager(t *testing.T) {
	f := newNotesFixture(t)
	empty := f.ana.Get("/notes").AssertOk().GetContent()
	if next := tagWith(empty, `data-step="next"`); next != "" {
		t.Fatalf("an empty listing draws a pager: %s", next)
	}

	if _, err := factories.Notes(f.app.DB).Count(26).State(func(n *models.Note) { n.UserID = f.anaID }).
		Create(context.Background(), auth.SystemGrant(policies.NoteCreate, appconfig.Tenant())); err != nil {
		t.Fatalf("creating the notes: %v", err)
	}

	first := f.ana.Get("/notes").AssertOk().GetContent()
	for _, want := range []string{`href="/notes?page=2"`, `hx-get="/notes?page=2"`, `hx-target="#notes-index"`, `hx-push-url="true"`, "Page 1 of 2"} {
		if !strings.Contains(first, want) {
			t.Errorf("the first page of 26 notes has no %s in its pager", want)
		}
	}
	if previous := tagWith(first, `data-step="previous"`); !strings.Contains(previous, `aria-disabled="true"`) || strings.Contains(previous, "href=") {
		t.Errorf("the first page steps back to a page before it: %s", previous)
	}

	second := f.ana.WithHeader("HX-Request", "true").WithHeader("HX-Target", "notes-index").Get("/notes?page=2")
	f.ana.WithHeader("HX-Request", "").WithHeader("HX-Target", "")
	table := second.AssertOk().GetContent()
	next, previous := tagWith(table, `data-step="next"`), tagWith(table, `data-step="previous"`)
	if strings.Contains(table, "<html") || !strings.Contains(table, "Page 2 of 2") ||
		!strings.Contains(next, `aria-disabled="true"`) || strings.Contains(next, "href=") ||
		!strings.Contains(previous, `hx-get="/notes"`) {
		t.Errorf("the second page, asked for by the pager, is not the table alone stepping back only:\n%s", table)
	}
}

// TestANotesDatesAreShownInTheApplicationsTimeZone: every date a page shows
// is in APP_TIMEZONE, the zone the application renders dates in, whatever
// zone the database handed it back in and whatever zone the server runs in.
//
// The zone is one that is neither UTC nor the zone of the machine running
// the test, so a date left in either is caught here: the note page once
// showed when it was created in UTC and when it was published in the
// server's time, one beside the other.
func TestANotesDatesAreShownInTheApplicationsTimeZone(t *testing.T) {
	zone := zoneOfItsOwn(t)
	t.Setenv("APP_TIMEZONE", zone.String())
	f := newNotesFixture(t)
	address := f.write(t, f.ana, "Groceries")
	tests.FromPage(t, f.ana, address).Post(address+"/publish", nil).AssertStatus(http.StatusSeeOther)
	comment := f.comment(t, f.ana, address, "Add coffee.")

	note := f.stored(t, address)
	if note.PublishedAt == nil {
		t.Fatalf("the note was not published: %+v", note)
	}
	// Published a moment after it was written, and stored as that instant: a
	// clock read in the server's zone is stored hours off by a column that
	// keeps no offset.
	if gap := note.PublishedAt.Sub(note.CreatedAt); gap < 0 || gap > time.Minute {
		t.Errorf("the note is stored published %s after it was created (created %s, published %s), want within a minute",
			gap, note.CreatedAt.UTC().Format(time.RFC3339), note.PublishedAt.UTC().Format(time.RFC3339))
	}
	page := f.ana.Get(address).AssertOk().GetContent()
	for term, at := range map[string]time.Time{"Created": note.CreatedAt, "Published": *note.PublishedAt} {
		if got, want := definition(t, page, term), at.In(zone).Format("2006-01-02 15:04"); got != want {
			t.Errorf("the note page shows %s %q, want %q: the instant %s in %s",
				term, got, want, at.UTC().Format(time.RFC3339), zone)
		}
	}

	stored := f.storedComment(t, comment)
	page = f.ana.Get(comment).AssertOk().GetContent()
	if got, want := definition(t, page, "Created"), stored.CreatedAt.In(zone).Format("2006-01-02 15:04"); got != want {
		t.Errorf("the comment page shows Created %q, want %q: the instant %s in %s",
			got, want, stored.CreatedAt.UTC().Format(time.RFC3339), zone)
	}
}

// zoneOfItsOwn answers a zone whose offset now is neither UTC's nor the
// local zone's. A half-hour offset comes first, so a date left in another
// zone differs in its minutes as well as in its hour.
func zoneOfItsOwn(t *testing.T) *time.Location {
	t.Helper()
	now := time.Now()
	_, local := now.Zone()
	for _, name := range []string{"Asia/Kolkata", "Asia/Kathmandu", "Pacific/Kiritimati"} {
		zone, err := time.LoadLocation(name)
		if err != nil {
			t.Fatalf("loading %s: %v", name, err)
		}
		if _, offset := now.In(zone).Zone(); offset != 0 && offset != local {
			return zone
		}
	}
	t.Fatal("no candidate zone differs from both UTC and the local zone")
	return nil
}

// definition answers the text of the <dd> that follows the <dt> holding term
// on a page, and fails the test when the page has no such term.
func definition(t *testing.T, page, term string) string {
	t.Helper()
	start := strings.Index(page, ">"+term+"</dt>")
	if start < 0 {
		t.Fatalf("the page has no %q term", term)
	}
	rest := page[start:]
	open := strings.Index(rest, "<dd")
	if open < 0 {
		t.Fatalf("the %q term has no definition", term)
	}
	rest = rest[open:]
	body := strings.IndexByte(rest, '>')
	end := strings.Index(rest, "</dd>")
	if body < 0 || end < body {
		t.Fatalf("the definition of %q is not terminated", term)
	}
	return strings.TrimSpace(html.UnescapeString(rest[body+1 : end]))
}
