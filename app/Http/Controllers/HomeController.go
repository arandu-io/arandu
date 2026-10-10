package controllers

import (
	"context"

	"github.com/arandu-io/framework/http"
	"github.com/arandu-io/hesape/auth"
	hhttp "github.com/arandu-io/hesape/http"
	"github.com/arandu-io/hesape/view"

	"github.com/arandu-io/arandu/storage/framework/views"
)

// UserNames is the projection the landing page needs from application users.
// Keeping the interface here lets the published UI replace this controller
// without coupling the request layer to a concrete service constructor.
type UserNames interface {
	PublicNames(context.Context, auth.Subject, []string) (map[string]string, error)
}

// HomeController answers the landing page.
//
// It is the smallest complete example of the shape: a struct, a constructor that
// takes what it needs, and one method per route that returns an error.
type HomeController struct {
	Controller

	// appName is what the page is titled. It comes from the configuration, and
	// it arrives through the constructor rather than through a global read: a
	// controller that reads the environment is a controller no test can pin.
	// The brand is not drawn from it: view.New reads that off the request.
	appName string

	// people and tenant are how the id in a session becomes a name to greet. A
	// session carries an id and not a name on purpose: a name kept in one stays
	// wrong after somebody changes theirs.
	//
	// The tenant is whose rows are read. It comes from the configuration,
	// through bootstrap/app.go, and never from the request.
	//
	// The application owns the user service. This controller asks only for the
	// projection it renders, which is also the seam the published UI keeps.
	people UserNames
	tenant string
}

// NewHomeController returns the controller. `bootstrap` builds it and hands it
// to the routes.
//
// There is no session store and no CSRF issuer here: the route puts who is
// signed in on the request, and the middleware that protects forms puts the
// token there, so a screen reads both off the request it is answering.
//
// The starter kit replaces this file together with the layout and the pages
// that extend it. Its publisher must keep this constructor aligned with
// bootstrap/app.go, or regenerating authentication leaves the project unable to
// compile.
func NewHomeController(appName string, people UserNames, tenant string) *HomeController {
	return &HomeController{appName: appName, people: people, tenant: tenant}
}

// Compile-time proof that this controller answers GET / the way Resource and the
// route table expect. It costs nothing and catches a renamed method.
var _ http.Indexer = (*HomeController)(nil)

// Index renders the landing page.
//
// The data is views.HomeData, the struct the view itself declares. Hand it
// anything else and the build fails, naming both sides -- which is the whole
// reason the view is compiled instead of interpreted.
//
// view.Page is the chrome the layout draws, embedded rather than repeated.
// view.New fills the title, the flash and the CSRF token the middleware issued
// for this request -- the token every write from this page carries, in the
// sign-out form and in hx-headers. It fills the brand with the application
// name the framework puts on every request, so this controller passes the name
// only as the page's title. It also fills the navigation's addresses
// from the route table, by name, and none of them is written here: the
// published authentication UI owns the sign-in and sign-out routes, and in a
// project without it those addresses are empty and the layout draws no link to
// a screen that does not exist.
func (c *HomeController) Index(ctx *hhttp.Context) error {
	// Who is signed in, put on the request by the route's LoadSubject from the
	// session cookie and never from the request body. No subject is the
	// anonymous case -- no cookie, a forged one, or a session that expired --
	// and the guest half of the navigation is what gets drawn.
	subject, signedIn := ctx.User()

	// The name to greet, from the id the session carries. One lookup by primary
	// key for the person who is signed in, and the id is the fallback: a header
	// is not worth a 500, and a guest never reaches the lookup at all.
	name := subject.ID
	if signedIn && c.people != nil {
		// PublicNames authorizes against the reader rather than taking a tenant
		// string, so the policy sees who is asking.
		reader := auth.Subject{ID: subject.ID, Tenant: c.tenant}
		if names, err := c.people.PublicNames(ctx.Ctx(), reader, []string{subject.ID}); err == nil && names[subject.ID] != "" {
			name = names[subject.ID]
		}
	}

	page := view.New(ctx, c.appName)
	page.Authenticated = signedIn
	page.UserName = name

	return ctx.View("home", views.HomeData{
		Page: page,
		Name: "world",
		Features: []views.Feature{
			{
				Title: "Authorization the compiler enforces",
				Body:  "No repository is reachable without an auth.Grant, and no Grant exists without a Policy having answered.",
			},
			{
				Title: "One view, one runtime, one build",
				Body:  "kyse for markup, HTMX for interaction, Go for everything else. No Node, no bundler, no lockfile.",
			},
			{
				Title: "The tenant comes from the Grant",
				Body:  "Never from a path, a body, a query or a header. Cache, session, storage and locks are prefixed by it.",
			},
			{
				Title: "Debug that names the probable cause",
				Body:  "The error page shows your frames expanded, the queries of the request and what the modules report.",
			},
		},
	})
}
