//go:build e2e

package e2e_test

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/arandu-io/framework/test/browser"
	"github.com/arandu-io/hesape/auth"
	accounts "github.com/arandu-io/hesape/auth/account"
	"github.com/arandu-io/hesape/support"

	models "github.com/arandu-io/arandu/app/Models"
	policies "github.com/arandu-io/arandu/app/Policies"
	appconfig "github.com/arandu-io/arandu/config"
	factories "github.com/arandu-io/arandu/database/factories"
	"github.com/arandu-io/arandu/tests"
)

// rowTitle is the title a row of the notes table says, as the text of the
// table puts it on screen.
var rowTitle = regexp.MustCompile(`Note \d\d`)

// listedRows reads the table the way a person does: the text of the region,
// from which the titles of the rows in view come out top to bottom. A column
// that is hidden, or a menu that is closed, has no text to read.
func listedRows(page *browser.Page) []string {
	var text string
	page.Evaluate(`document.getElementById('notes-index').innerText`, &text)
	return rowTitle.FindAllString(text, -1)
}

// expectRows fails unless the table lists exactly these titles, in this order.
// It reads once: the address changes only after the swap has settled, and every
// call follows an expectation of the address.
func expectRows(t *testing.T, page *browser.Page, want ...string) {
	t.Helper()
	if got := listedRows(page); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("the table lists %v, want %v", got, want)
	}
}

// TestTheNotesTableIsWorkedByItsAddress is the listing end to end, in a real
// browser: a person searches, orders, filters by a facet and turns it off,
// pages, changes the size of the page, hides a column and ticks a row, and
// at every step the address says what is on screen -- so a reload shows the
// same table and the history walks back through it.
func TestTheNotesTableIsWorkedByItsAddress(t *testing.T) {
	server := tests.Serve(t)
	tenant := appconfig.Tenant()
	members, err := factories.Users(server.App.DB).Count(1).
		Create(context.Background(), auth.SystemGrant(accounts.ActionUserCreate, tenant))
	if err != nil {
		t.Fatalf("creating the member: %v", err)
	}
	ana := members[0]

	// Thirty notes, the newest being the highest number. Every even one says
	// "alpha" in its body and every third one is pinned.
	support.FreezeTime()
	t.Cleanup(support.TravelBack)
	for i := 1; i <= 30; i++ {
		body := "beta"
		if i%2 == 0 {
			body = "alpha"
		}
		if _, err := factories.Notes(server.App.DB).State(func(n *models.Note) {
			n.UserID, n.Title, n.Body, n.Pinned = ana.ID, fmt.Sprintf("Note %02d", i), body, i%3 == 0
		}).CreateOne(context.Background(), auth.SystemGrant(policies.NoteCreate, tenant)); err != nil {
			t.Fatalf("creating note %d: %v", i, err)
		}
		support.Travel(time.Minute)
	}

	page := tests.Open(t, server)
	page.SetCookies(tests.SignIn(t, server, ana).Cookies()...)

	page.Visit("notes.index")
	page.Responsive()
	expectRows(t, page, titlesFrom(30, 6)...)
	page.ExpectText("0 of 30 row(s) selected.")
	page.ExpectText("Page 1 of 2")

	// Search: one request, on a pause in typing, and the address says so.
	page.Fill("Search Notes", "alpha")
	page.ExpectURL("/notes?q=alpha")
	expectRows(t, page, evens(30, 2)...)

	// Order by the title, ascending and then descending, from its menu.
	page.ClickRole("button", "Title, not sorted")
	page.ClickRole("menuitem", "Asc")
	page.ExpectURL("/notes?q=alpha&sort=title")
	expectRows(t, page, reversed(evens(30, 2))...)
	page.ClickRole("button", "Title, sorted ascending")
	page.ClickRole("menuitem", "Desc")
	page.ExpectURL("/notes?dir=desc&q=alpha&sort=title")
	expectRows(t, page, evens(30, 2)...)

	// A facet on, with the count of what it matches, and off again.
	page.ClickRole("button", "Pinned")
	page.CheckIn(browser.Role("checkbox", "Pinned 5"), true)
	page.ExpectURL("/notes?dir=desc&pinned=1&q=alpha&sort=title")
	expectRows(t, page, "Note 30", "Note 24", "Note 18", "Note 12", "Note 06")
	page.CheckIn(browser.Role("checkbox", "Pinned 5"), false)
	page.ExpectURL("/notes?dir=desc&q=alpha&sort=title")
	expectRows(t, page, evens(30, 2)...)

	// Reset drops the search and the facets and keeps the order.
	page.ClickRole("link", "Reset")
	page.ExpectURL("/notes?dir=desc&sort=title")
	expectRows(t, page, titlesFrom(30, 6)...)
	page.ExpectText("Page 1 of 2")

	// The pager steps on the same address, and the size of the page keeps the
	// first row in view: row 26 is on the third page of tens.
	page.ClickRole("link", "Go to next page")
	page.ExpectURL("/notes?dir=desc&page=2&sort=title")
	expectRows(t, page, titlesFrom(5, 1)...)
	page.ClickRole("button", "Rows per page 25")
	page.ClickRole("menuitemradio", "10")
	page.ExpectURL("/notes?dir=desc&page=3&per_page=10&sort=title")
	expectRows(t, page, titlesFrom(10, 1)...)
	page.ExpectText("Page 3 of 3")

	// A row ticked stays ticked across a page, because the browser keeps the
	// keys and the footer counts them against the whole list.
	page.CheckIn(browser.Role("checkbox", "Select note Note 10"), true)
	page.ExpectText("1 of 30 row(s) selected.")
	page.ClickRole("link", "Go to previous page")
	page.ExpectURL("/notes?dir=desc&page=2&per_page=10&sort=title")
	expectRows(t, page, titlesFrom(20, 11)...)
	page.ExpectText("1 of 30 row(s) selected.")

	// A column hidden from its header menu travels in the address.
	page.ClickRole("button", "Body, not sorted")
	page.ClickRole("menuitem", "Hide")
	page.ExpectURL("/notes?dir=desc&hide=body&page=2&per_page=10&sort=title")
	expectRows(t, page, titlesFrom(20, 11)...)
	page.ExpectHidden(browser.Role("button", "Body, not sorted"))
	page.ExpectText("1 of 30 row(s) selected.")

	// A reload asks for the address, and gets the table the person left.
	page.Evaluate(`location.reload()`, nil)
	page.ExpectURL("/notes?dir=desc&hide=body&page=2&per_page=10&sort=title")
	// What was ticked lived in the page that is gone, so the count starting
	// over says the table on screen is the new document's.
	page.ExpectText("0 of 30 row(s) selected.")
	page.ExpectText("Page 2 of 3")
	expectRows(t, page, titlesFrom(20, 11)...)
	page.ExpectHidden(browser.Role("button", "Body, not sorted"))

	// Back walks the history one state at a time.
	page.Evaluate(`history.back()`, nil)
	page.ExpectURL("/notes?dir=desc&page=2&per_page=10&sort=title")
	page.ExpectVisible(browser.Role("button", "Body, not sorted"))
	page.ExpectText("Page 2 of 3")
	expectRows(t, page, titlesFrom(20, 11)...)
	page.Evaluate(`history.back()`, nil)
	page.ExpectURL("/notes?dir=desc&page=3&per_page=10&sort=title")
	page.ExpectText("Page 3 of 3")
	expectRows(t, page, titlesFrom(10, 1)...)

	page.Responsive()
}

// titlesFrom is the titles "Note NN" from high down to low, the order the
// newest-first default lists them in.
func titlesFrom(high, low int) []string {
	var titles []string
	for n := high; n >= low; n-- {
		titles = append(titles, fmt.Sprintf("Note %02d", n))
	}
	return titles
}

// evens is the even-numbered titles from high down to low.
func evens(high, low int) []string {
	var titles []string
	for n := high; n >= low; n -= 2 {
		titles = append(titles, fmt.Sprintf("Note %02d", n))
	}
	return titles
}

// reversed is the titles the other way round.
func reversed(titles []string) []string {
	out := make([]string, len(titles))
	for i, title := range titles {
		out[len(titles)-1-i] = title
	}
	return out
}

// TestTheCommentsTableIsWorkedByItsAddress: the comments of a note are the
// same table over the same listing -- searched, ordered and paged by the
// address, with one pager -- and a row's menu reaches the comment.
func TestTheCommentsTableIsWorkedByItsAddress(t *testing.T) {
	server := tests.Serve(t)
	tenant := appconfig.Tenant()
	members, err := factories.Users(server.App.DB).Count(1).
		Create(context.Background(), auth.SystemGrant(accounts.ActionUserCreate, tenant))
	if err != nil {
		t.Fatalf("creating the member: %v", err)
	}
	ana := members[0]
	note, err := factories.Notes(server.App.DB).State(func(n *models.Note) { n.UserID = ana.ID }).
		CreateOne(context.Background(), auth.SystemGrant(policies.NoteCreate, tenant))
	if err != nil {
		t.Fatalf("creating the note: %v", err)
	}

	support.FreezeTime()
	t.Cleanup(support.TravelBack)
	for i := 1; i <= 30; i++ {
		body := "plain"
		if i%5 == 0 {
			body = "starred"
		}
		if _, err := factories.Comments(server.App.DB).State(func(c *models.Comment) {
			c.NoteID, c.UserID, c.Body = note.ID, ana.ID, fmt.Sprintf("Comment %02d %s", i, body)
		}).CreateOne(context.Background(), auth.SystemGrant(policies.CommentCreate, tenant)); err != nil {
			t.Fatalf("creating comment %d: %v", i, err)
		}
		support.Travel(time.Minute)
	}

	page := tests.Open(t, server)
	page.SetCookies(tests.SignIn(t, server, ana).Cookies()...)
	index := "/notes/" + note.ID + "/comments"

	page.Visit(index)
	page.Responsive()
	page.ExpectText("Page 1 of 2")

	page.Fill("Search Comments", "starred")
	page.ExpectURL(index + "?q=starred")
	page.ExpectText("Comment 30 starred")
	page.ExpectText("Comment 05 starred")
	page.ExpectNoText("Comment 29 plain")

	page.ClickRole("link", "Reset")
	page.ExpectURL(index)
	page.ClickRole("link", "Go to next page")
	page.ExpectURL(index + "?page=2")
	page.ExpectText("Comment 01 plain")
	page.ExpectText("Page 2 of 2")
	page.Responsive()
}
