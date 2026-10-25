// Example resource. Remove with the list under "The example resource" in README.md.

package feature_test

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/arandu-io/hesape/auth"
	"github.com/arandu-io/hesape/support"

	models "github.com/arandu-io/arandu/app/Models"
	policies "github.com/arandu-io/arandu/app/Policies"
	appconfig "github.com/arandu-io/arandu/config"
	factories "github.com/arandu-io/arandu/database/factories"
	"github.com/arandu-io/arandu/tests"
)

// listedBodies reads the first cell of every row of a comments table: the
// comment's body, as the link that opens it.
var listedBodies = regexp.MustCompile(`(?s)data-column="body"[^>]*>\s*<a\s[^>]*>\s*(.*?)\s*</a>`)

// bodiesOn is the bodies of the comments a page lists, in order.
func bodiesOn(page string) []string {
	var out []string
	for _, match := range listedBodies.FindAllStringSubmatch(page, -1) {
		out = append(out, match[1])
	}
	return out
}

// seedComments stores comments under one note, one minute apart, written by
// one member.
func (f notesFixture) seedComments(t *testing.T, noteID, author, tenant string, bodies ...string) {
	t.Helper()
	support.FreezeTime()
	t.Cleanup(support.TravelBack)
	for _, body := range bodies {
		if _, err := factories.Comments(f.app.DB).State(func(row *models.Comment) {
			row.NoteID, row.UserID, row.Body = noteID, author, body
		}).CreateOne(context.Background(), auth.SystemGrant(policies.CommentCreate, tenant)); err != nil {
			t.Fatalf("creating the comment %q: %v", body, err)
		}
		support.Travel(time.Minute)
	}
}

// TestTheCommentsListingIsSearchedOrderedPagedAndHiddenByTheQuery: the comments
// of a note are a listing like the notes', declared once, and every control of
// its table is an address of the same listing.
func TestTheCommentsListingIsSearchedOrderedPagedAndHiddenByTheQuery(t *testing.T) {
	f := newNotesFixture(t)
	note := f.write(t, f.ana, "Groceries")
	other := f.write(t, f.ana, "Another note")
	noteID, otherID := strings.TrimPrefix(note, "/notes/"), strings.TrimPrefix(other, "/notes/")

	f.seedComments(t, noteID, f.beaID, appconfig.Tenant(), "Add coffee", "Add milk", "Buy bread", "Skip the sugar")
	f.seedComments(t, otherID, f.beaID, appconfig.Tenant(), "Add salt")
	address := note + "/comments"

	for _, tc := range []struct {
		query string
		want  []string
	}{
		// Newest first until asked otherwise, and only this note's.
		{"", []string{"Skip the sugar", "Buy bread", "Add milk", "Add coffee"}},
		{"?sort=created_at&dir=asc", []string{"Add coffee", "Add milk", "Buy bread", "Skip the sugar"}},
		{"?sort=body", []string{"Add coffee", "Add milk", "Buy bread", "Skip the sugar"}},
		{"?sort=body&dir=desc", []string{"Skip the sugar", "Buy bread", "Add milk", "Add coffee"}},
		{"?q=MILK", []string{"Add milk"}},
		{"?q=add", []string{"Add milk", "Add coffee"}},
		{"?q=salt", nil},
		{"?per_page=10&page=1&sort=body", []string{"Add coffee", "Add milk", "Buy bread", "Skip the sugar"}},
		// Whatever is not declared falls back to the defaults.
		{"?sort=user_id", []string{"Skip the sugar", "Buy bread", "Add milk", "Add coffee"}},
		{"?sort=note_id&dir=desc;", []string{"Skip the sugar", "Buy bread", "Add milk", "Add coffee"}},
		{"?per_page=100000&page=-4", []string{"Skip the sugar", "Buy bread", "Add milk", "Add coffee"}},
		{"?note_id=" + otherID, []string{"Skip the sugar", "Buy bread", "Add milk", "Add coffee"}},
	} {
		page := f.ana.Get(address + tc.query).AssertOk().GetContent()
		if got := bodiesOn(page); !slices.Equal(got, tc.want) {
			t.Errorf("%s: bodies = %v, want %v", tc.query, got, tc.want)
		}
		if strings.Contains(page, "Add salt") {
			t.Errorf("%s shows the comment of another note", tc.query)
		}
	}

	hidden := f.ana.Get(address + "?hide=created_at&hide=body").AssertOk().GetContent()
	if !regexp.MustCompile(`data-column="created_at"[^>]*\bhidden\b`).MatchString(hidden) ||
		regexp.MustCompile(`data-column="body"[^>]*\bhidden\b`).MatchString(hidden) {
		t.Error("hide= hid the column the listing did not declare hideable, or did not hide the one it did")
	}
}

// TestTheCommentsListingHasOnePagerAndPagesIt: the table's pager is the only
// pager, and it steps through the note's comments.
func TestTheCommentsListingHasOnePagerAndPagesIt(t *testing.T) {
	f := newNotesFixture(t)
	note := f.write(t, f.ana, "Groceries")
	bodies := make([]string, 0, 30)
	for i := 1; i <= 30; i++ {
		bodies = append(bodies, fmt.Sprintf("Comment %02d", i))
	}
	f.seedComments(t, strings.TrimPrefix(note, "/notes/"), f.beaID, appconfig.Tenant(), bodies...)
	address := note + "/comments"

	first := f.ana.Get(address + "?sort=body").AssertOk().GetContent()
	if got := bodiesOn(first); len(got) != 25 || got[0] != "Comment 01" || got[24] != "Comment 25" {
		t.Errorf("the first page = %d rows, want the first 25 comments", len(got))
	}
	if got := strings.Count(first, `data-part="pagination"`); got != 1 {
		t.Errorf("the page draws %d pagers of the table, want 1", got)
	}
	if strings.Contains(first, `rel="next"`) || strings.Contains(first, `<nav data-part="root"`) {
		t.Error("the page draws a second pager next to the table's")
	}
	next := tagWith(first, `data-step="next"`)
	for _, want := range []string{`href="` + address + `?page=2&amp;sort=body"`, `hx-target="#comments-index"`} {
		if !strings.Contains(next, want) {
			t.Errorf("the next step has no %s: %s", want, next)
		}
	}

	second := f.ana.Get(address + "?sort=body&page=2").AssertOk().GetContent()
	if got := bodiesOn(second); len(got) != 5 || got[0] != "Comment 26" {
		t.Errorf("the second page = %v, want the last five comments", got)
	}
}

// TestTheCommentsTableIsAnsweredAloneToItsOwnRequest: the table alone goes to
// the request that names its element, the whole page to every other, and the
// answer says which headers it depends on.
func TestTheCommentsTableIsAnsweredAloneToItsOwnRequest(t *testing.T) {
	f := newNotesFixture(t)
	note := f.write(t, f.ana, "Groceries")
	f.comment(t, f.bea, note, "Add coffee.")
	address := note + "/comments"

	whole := f.ana.Get(address).AssertOk()
	if body := whole.GetContent(); !strings.Contains(body, "<html") || !strings.Contains(body, `id="comments-index"`) {
		t.Error("a direct visit was not answered with the whole page around the table")
	}
	for _, header := range []string{"HX-Request", "HX-Target"} {
		if !strings.Contains(whole.Header("Vary"), header) {
			t.Errorf("Vary = %q, and the answer depends on %s", whole.Header("Vary"), header)
		}
	}

	alone := f.ana.WithHeader("HX-Request", "true").WithHeader("HX-Target", "comments-index").Get(address)
	f.ana.WithHeader("HX-Request", "").WithHeader("HX-Target", "")
	fragment := strings.TrimSpace(alone.AssertOk().GetContent())
	if strings.Contains(fragment, "<html") || !strings.HasPrefix(fragment, "<div") ||
		!strings.Contains(fragment, `id="comments-index"`) || !strings.Contains(fragment, "Add coffee.") {
		t.Errorf("the table's own request was not answered with the table alone:\n%s", fragment)
	}
}

// TestACommentOfAnotherNoteOrTenantIsNeverListed: the note in the path is the
// one the service loaded under its own policy, so a note of another tenant is
// not found, and the comments of a note of this tenant stay under it.
func TestACommentOfAnotherNoteOrTenantIsNeverListed(t *testing.T) {
	f := newNotesFixture(t)
	foreign, err := factories.Notes(f.app.DB).State(func(n *models.Note) { n.UserID = f.anaID }).
		CreateOne(context.Background(), auth.SystemGrant(policies.NoteCreate, foreignTenant))
	if err != nil {
		t.Fatalf("creating a note in another tenant: %v", err)
	}
	f.seedComments(t, foreign.ID, f.anaID, foreignTenant, "A secret of the other tenant")

	f.ana.Get("/notes/" + foreign.ID + "/comments").AssertStatus(http.StatusNotFound)
	f.ana.Get("/notes/" + foreign.ID + "/comments?q=secret").AssertStatus(http.StatusNotFound)

	// A note of this tenant lists only its own, whatever the query asks.
	note := f.write(t, f.ana, "Groceries")
	f.comment(t, f.bea, note, "Add coffee.")
	for _, query := range []string{"", "?q=secret", "?note_id=" + foreign.ID, "?tenant_id=" + foreignTenant} {
		page := f.ana.Get(note + "/comments" + query).AssertOk().GetContent()
		if strings.Contains(page, "secret of the other tenant") {
			t.Errorf("%s shows a comment of another tenant", query)
		}
	}
}

// TestTheEditEntryOfACommentIsOfferedToItsAuthorOnly: a row's menu offers the
// edit screen to whoever the policy would let in.
func TestTheEditEntryOfACommentIsOfferedToItsAuthorOnly(t *testing.T) {
	f := newNotesFixture(t)
	note := f.write(t, f.ana, "Groceries")
	address := f.comment(t, f.bea, note, "Add coffee.")

	if page := f.bea.Get(note + "/comments").AssertOk().GetContent(); !strings.Contains(page, address+"/edit") {
		t.Error("the author of a comment is not offered its edit screen")
	}
	if page := f.ana.Get(note + "/comments").AssertOk().GetContent(); strings.Contains(page, address+"/edit") {
		t.Error("another member is offered the edit screen of a comment it would be refused")
	}
}

// TestNobodyReadsTheCommentsListingWithoutSigningIn: no query string is a way
// past the guard.
func TestNobodyReadsTheCommentsListingWithoutSigningIn(t *testing.T) {
	guest := tests.Serve(t).Client()
	for _, query := range []string{"", "?q=a", "?sort=body&dir=desc", "?sort=password"} {
		guest.Get("/notes/any/comments" + query).AssertRedirect("/auth/login")
	}
}
