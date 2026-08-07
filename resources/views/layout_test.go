package views

import (
	"bytes"
	"strings"
	"testing"

	"github.com/arandu-io/framework/view"
)

// TestTheLayoutRendersAnyPageThatFitsIt is the guard on the property the whole
// view layer rests on: pages carry different data and share one layout.
//
// It was not always true. `aru make:auth` replaced layouts/app with one typed by
// its own struct, and from that moment every other page in the project answered
//
//	view "layouts.app" takes AuthPage and got views.InvoicesIndexData.
//	The controller and the view disagree about the data
//
// The layout renders with the Layout interface, and a page fits by embedding
// Page. The type below is not HomeData and was never seen by the layout, which
// is exactly the point.
func TestTheLayoutRendersAnyPageThatFitsIt(t *testing.T) {
	type reportData struct {
		Page
		Rows []string
	}

	var buf bytes.Buffer
	data := reportData{
		Page: Page{
			Title:    "Quarterly report",
			AppName:  "Arandu",
			HomeURL:  "/",
			LoginURL: "/auth/login",
		},
		Rows: []string{"one"},
	}
	if err := view.RenderInto(&buf, "layouts.app", data, nil); err != nil {
		t.Fatalf("a page that is not the one the skeleton ships does not render: %v", err)
	}

	for _, want := range []string{
		"<title>Quarterly report</title>",
		`href="/"`,
		"Arandu",
		`href="/auth/login"`,
	} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("the layout drew no %s:\n%s", want, buf.String())
		}
	}
}

// TestTheLayoutDrawsTheSignedInHalf: the other branch of the navigation, which
// no test would reach through the landing page alone.
func TestTheLayoutDrawsTheSignedInHalf(t *testing.T) {
	var buf bytes.Buffer
	data := Page{
		Title:         "Dashboard",
		AppName:       "Arandu",
		Token:         "a-token",
		Authenticated: true,
		UserName:      "Ana",
		LogoutURL:     "/auth/logout",
	}
	if err := view.RenderInto(&buf, "layouts.app", data, nil); err != nil {
		t.Fatalf("RenderInto: %v", err)
	}

	body := buf.String()
	if !strings.Contains(body, "Ana") {
		t.Error("the navigation does not name the person signed in")
	}
	if !strings.Contains(body, `action="/auth/logout"`) {
		t.Error("there is no sign-out form")
	}
	// The token reaches the markup twice, and both are load-bearing: the hidden
	// field the form posts, and the header every hx- request carries.
	if strings.Count(body, "a-token") != 2 {
		t.Errorf("the CSRF token appears %d times, want the hidden field and hx-headers:\n%s",
			strings.Count(body, "a-token"), body)
	}
}
