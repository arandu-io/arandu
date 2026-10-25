//go:build e2e

// Package e2e_test holds this application's journeys: the server tests.Serve
// boots on a port of its own, over a database on the engine the test
// environment names, driven in a real browser by a driver of tests.Drivers.
// Each one is written in the user's words -- roles, labels, visible text --
// and the page waits by itself for htmx to settle and the network to go idle,
// and fails by itself on an uncaught exception, a console error, a 5xx and a
// CSP violation.
//
//	aru test --e2e    go test -tags e2e ./tests/E2E/...
package e2e_test

import (
	"context"
	"testing"

	"github.com/arandu-io/framework/test/browser"
	"github.com/arandu-io/hesape/auth"
	accounts "github.com/arandu-io/hesape/auth/account"

	models "github.com/arandu-io/arandu/app/Models"
	policies "github.com/arandu-io/arandu/app/Policies"
	appconfig "github.com/arandu-io/arandu/config"
	factories "github.com/arandu-io/arandu/database/factories"
	"github.com/arandu-io/arandu/tests"
)

// TestANoteIsWrittenChangedAndPublishedByItsAuthorOnly is the example
// resource end to end: Ana writes a note, edits it and publishes it, each
// write landing on a page that says so in a toast; the listing pages through
// the table's own pager without leaving the page; and Bea, a member of the same tenant, sees
// the note without the controls to change it and is refused its edit screen.
// The note and the listing are walked through every width and scheme, and
// neither may be wider than the screen it is drawn on.
func TestANoteIsWrittenChangedAndPublishedByItsAuthorOnly(t *testing.T) {
	server := tests.Serve(t)
	tenant := appconfig.Tenant()
	members, err := factories.Users(server.App.DB).Count(2).
		Create(context.Background(), auth.SystemGrant(accounts.ActionUserCreate, tenant))
	if err != nil {
		t.Fatalf("creating the members: %v", err)
	}
	ana, bea := members[0], members[1]
	// A listing longer than one page, so the pager has a page to fetch.
	if _, err := factories.Notes(server.App.DB).Count(25).State(func(n *models.Note) { n.UserID = ana.ID }).
		Create(context.Background(), auth.SystemGrant(policies.NoteCreate, tenant)); err != nil {
		t.Fatalf("creating the notes: %v", err)
	}

	page := tests.Open(t, server)
	page.SetCookies(tests.SignIn(t, server, ana).Cookies()...)

	page.Visit("notes.create")
	page.Fill("Title", "Groceries")
	page.Fill("Body", "Bread and coffee.")
	page.ClickRole("button", "Save")
	page.ExpectText("Note created.")
	page.ExpectText("Groceries")
	page.ExpectText("Draft")

	page.ClickRole("link", "Edit")
	page.Fill("Title", "Groceries for Sunday")
	page.ClickRole("button", "Save")
	page.ExpectText("Note saved.")
	page.ExpectText("Groceries for Sunday")
	address := page.URL()

	page.ClickRole("button", "Publish")
	page.ExpectText("Note published.")
	page.ExpectNoText("Draft")
	page.ExpectHidden(browser.Role("button", "Publish"))
	page.Responsive()

	page.Visit("notes.index")
	page.Responsive()
	page.ClickRole("link", "Go to next page")
	page.ExpectURL("/notes?page=2")
	page.ExpectVisible(browser.Role("link", "Go to previous page"))
	page.ExpectHidden(browser.Role("link", "Go to next page"))

	page.SetCookies(tests.SignIn(t, server, bea).Cookies()...)
	page.Visit(address)
	page.ExpectText("Groceries for Sunday")
	page.ExpectHidden(browser.Role("link", "Edit"))
	page.ExpectHidden(browser.Role("button", "Delete"))
	page.Visit(address + "/edit")
	page.ExpectText("Forbidden")
	page.ExpectNoText("Groceries for Sunday")
}
