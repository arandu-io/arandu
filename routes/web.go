// Package routes is where this application declares what it answers.
//
// Two files, the same two Laravel has: web.go for what a browser reaches, and
// console.go for what the command line does. There is no api.go -- the handler
// decides between a JSON body and an HTML fragment, and a second router for the
// same resources would be a second place to forget a policy (doc 28).
package routes

import (
	"github.com/arandu-io/framework/httpx"

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
}

// Web registers the browser-facing routes.
//
// The vocabulary is Laravel's. Name() is what makes a route addressable by name,
// so a link is built from r.Table().URL("home") and a renamed path does not
// leave a dead href behind:
//
//	r.Get("/", handler).Name("home")
//	r.Action("GET", "/dashboard", ctrl.Index).Name("dashboard")
//	r.Resource("invoices", invoiceController)      // the seven of Laravel
//	admin := r.Group("/admin", middleware.RequireRole("admin"))
//
// Resource registers only the actions the controller implements, so a route that
// exists is a route that answers.
func Web(r *httpx.Router, d Deps) {
	// "/{$}" and not "/". This is the one place Go's router does not behave the
	// way Laravel's does: a pattern ending in a slash matches every path below
	// it, so "GET /" would answer for /anything -- including the 404s, and
	// including /_arandu/debug when the console is not mounted. The {$} anchors
	// the match to the end of the path, which is what Route::get('/') means.
	r.Action("GET", "/{$}", d.Home.Index).Name("home")

	// The fixed names the outside world asks for: /favicon.ico, which the layout
	// links, and /robots.txt, which a crawler fetches without being told to.
	// They are embedded in the binary and there is no document root -- see the
	// public package. Without this line the icon in the tab is a 404.
	public.Routes(r)

	// arandu:begin custom
	// The routes of this application go here. `aru make:module` appends to this
	// block and leaves everything else in the file alone.
	// arandu:end custom
}
