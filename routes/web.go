// Package routes is where this application declares what it answers.
//
// Two files: web.go for what a browser reaches, and
// console.go for what the command line does. There is no api.go -- the handler
// decides between a JSON body and an HTML fragment, and a second router for the
// same resources would be a second place to forget a policy (doc 28).
package routes

import (
	"time"

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

	// Note is the example resource, and Comment the resource nested under it.
	// NewsletterWebhook receives what the newsletter provider reports about
	// the digests. Remove the three with the list under "The example resource"
	// in README.md.
	Note              *controllers.NoteController
	Comment           *controllers.CommentController
	NewsletterWebhook *controllers.NewsletterWebhookController

	// Sessions is what the route guards read: RequireAuth refuses a request
	// without a session, and LoadSubject lets a public page know who is looking.
	// Both put the subject on the request, and a handler reads it with
	// ctx.User() rather than loading the session again.
	//
	// A guard is not a second authorization path. It answers "is there a
	// session" and stops; whether this subject may touch this record is the
	// Policy's answer, and the Policy still runs.
	Sessions *security.SessionStore

	// Tokens is what RequireToken asks who a bearer token acts as: the
	// personal access tokens this application issued. A route a program calls
	// with a token sits behind it instead of behind the session, and the
	// Policy still decides every record, exactly as it does for a browser.
	Tokens middleware.TokenResolver

	// Idempotency is where Idempotent keeps the answer to a write that carried
	// an Idempotency-Key, and the lock that keeps two copies of it from running
	// at once. It is the cache store CACHE_STORE names: the in-process one for
	// a single replica, the shared one when there are several.
	Idempotency middleware.IdempotencyStore
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
	// The routes of this application go here. `aru make:module` edits nothing
	// in this file: it prints the line to paste in this block, and the field it
	// needs in Deps above.

	// The example resource, behind the sign-in guard: the controllers read who
	// is asking from what the guard puts on the request. Remove these lines,
	// down to the webhook, with the list under "The example resource" in
	// README.md.
	//
	// notes.publish is a named action on one note: POST /notes/{id}/publish.
	// notes.comments nests shallow: the listing, the form and the store answer
	// under /notes/{note}/comments, and the record at /comments/{comment}. The
	// note in the path is where the person navigated, never whose data it is.
	notes := r.Group("", middleware.RequireAuth(d.Sessions))
	notes.Resource("notes", d.Note)
	notes.ResourceAction("POST", "notes", "publish", d.Note.Publish)
	notes.Resource("notes.comments", d.Comment)

	// The same two writes for a program holding a personal access token, and
	// answered by the same actions: the guard is the token instead of the
	// session, and a retry that carries the same Idempotency-Key is answered
	// from the store and writes nothing twice. A bearer request with no
	// session cookie is left to RequireToken by the CSRF check, because the
	// header is not something a browser attaches by itself.
	api := r.Group("/api", middleware.RequireToken(d.Tokens))
	api.Action("POST", "/notes", d.Note.Store,
		middleware.Idempotent(d.Idempotency, 24*time.Hour)).Name("api.notes.store")
	api.Action("POST", "/notes/{id}/publish", d.Note.Publish,
		middleware.Idempotent(d.Idempotency, 24*time.Hour)).Name("api.notes.publish")

	// What the newsletter provider posts back. No guard: the controller
	// verifies the provider's signature before anything else, and /webhooks/
	// is exempt from the CSRF check in bootstrap/app.go for that reason.
	r.Action("POST", "/webhooks/newsletter", d.NewsletterWebhook.Store).Name("webhooks.newsletter")
	// arandu:end custom
}
