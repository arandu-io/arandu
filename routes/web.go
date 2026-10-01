// Package routes is where this application declares what it answers.
//
// Two files: web.go for what a browser reaches, and
// console.go for what the command line does. There is no api.go -- the handler
// decides between a JSON body and an HTML fragment, and a second router for the
// same resources would be a second place to forget a policy (doc 28).
package routes

import (
	"github.com/arandu-io/framework/http"
	"github.com/arandu-io/framework/http/middleware"
	"github.com/arandu-io/framework/security"

	controllers "github.com/arandu-io/arandu/app/Http/Controllers"
	"github.com/arandu-io/arandu/public"
)

// Deps carries the controllers the routes dispatch to.
//
// A struct rather than a growing parameter list, and explicit rather than
// resolved from a container: reading bootstrap/app.go tells you what every route
// was given, which is the property a dependency container costs you.
type Deps struct {
	Home *controllers.HomeController

	// Note is the example resource. Remove it with the list under "The example
	// resource" in README.md.
	Note *controllers.NoteController

	// Sessions is what the route guards read: RequireAuth refuses a request
	// without a session, and LoadSubject lets a public page know who is looking.
	// Both put the subject on the request, and a handler reads it with
	// ctx.User() rather than loading the session again.
	//
	// A guard is not a second authorization path. It answers "is there a
	// session" and stops; whether this subject may touch this record is the
	// Policy's answer, and the Policy still runs.
	Sessions *security.SessionStore
}

// Web registers the browser-facing routes.
//
// Name() is what makes a route addressable by name,
// so a link is built from r.Table().URL("home") and a renamed path does not
// leave a dead href behind:
//
//	r.Get("/", handler).Name("home")
//	r.Resource("invoices", invoiceController)      // the seven REST routes
//
// The guards live in github.com/arandu-io/framework/http/middleware:
//
//	r.Action("GET", "/dashboard", ctrl.Index, middleware.RequireAuth(d.Sessions)).Name("dashboard")
//	admin := r.Group("/admin", middleware.RequireRole(d.Sessions, "admin"))
//
// Resource registers only the actions the controller implements, so a route that
// exists is a route that answers.
//
// The guard goes on the route and not at the top of the handler. A check written
// inside a controller is a check the next controller does not have, and it is
// written where nobody reading this table can see it -- this file is what says
// which addresses are open. The sign-in screen is guarded the same way, by the
// auth module that registers it, so somebody already signed in is not shown a
// form telling them they are not.
func Web(r *http.Router, d Deps) {
	// "/{$}" and not "/". This is the one place Go's router does not behave the
	// way it conventionally does: a pattern ending in a slash matches every path below
	// it, so "GET /" would answer for /anything -- including the 404s, and
	// including /_arandu/debug when the console is not mounted. The {$} anchors
	// the match to the end of the path, which is what Route::get('/') means.
	//
	// LoadSubject and not RequireAuth: the page is public, and it greets
	// somebody signed in by name.
	r.Action("GET", "/{$}", d.Home.Index, middleware.LoadSubject(d.Sessions)).Name("home")

	// The fixed favicon and brand names the browser asks for are embedded in the
	// binary. Crawler discovery documents are registered by the native GEO module.
	public.Routes(r)

	// arandu:begin custom
	// The routes of this application go here. `aru make:module` appends to this
	// block and leaves everything else in the file alone.

	// The example resource, behind the sign-in guard: the controller reads who
	// is asking from what the guard puts on the request. Remove it with the list
	// under "The example resource" in README.md.
	r.Group("", middleware.RequireAuth(d.Sessions)).Resource("notes", d.Note)
	// arandu:end custom
}
