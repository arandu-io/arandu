package controllers

import (
	"context"

	"github.com/arandu-io/framework/http"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/view"

	"github.com/arandu-io/arandu/storage/framework/views"
)

// UserNames is the projection the landing page needs from application users.
// Keeping the interface here lets the published UI replace this controller
// without coupling the request layer to a concrete service constructor.
type UserNames interface {
	PublicNames(context.Context, security.Subject, []string) (map[string]string, error)
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
	appName string

	// sessions and csrf are what the chrome is drawn from: who is signed in, and
	// the token every write of this session carries. They arrive through the
	// constructor for the same reason appName does, and they are the same two
	// every controller `aru make:module` writes takes -- a screen is allowed to
	// know about a token and a cookie.
	sessions *security.SessionStore
	csrf     *security.CSRF

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
// The starter kit replaces this file together with the layout and the pages
// that extend it. Its publisher must keep this constructor aligned with
// bootstrap/app.go, or regenerating authentication leaves the project unable to
// compile.
func NewHomeController(appName string, sessions *security.SessionStore, csrf *security.CSRF, people UserNames, tenant string) *HomeController {
	return &HomeController{
		appName: appName, sessions: sessions, csrf: csrf,
		people: people, tenant: tenant,
	}
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
// view.Page is the chrome the layout draws, embedded rather than repeated. The
// navigation draws a link only for what answers. The published authentication
// UI owns the sign-in and sign-out routes; registration stays off until its
// handler is published as well.
func (c *HomeController) Index(ctx *http.Context) error {
	// Who is signed in, from the session cookie and never from the request. An
	// error here is the anonymous case -- no cookie, a forged one, or a session
	// that expired -- and the guest half of the navigation is what gets drawn.
	subject, err := c.sessions.Load(ctx.Ctx(), ctx.Request)
	signedIn := err == nil

	// The token reaches the markup twice: the hidden field of the sign-out form
	// and the hx-headers attribute on <body>. A page rendered without one answers
	// 200 and then refuses the next write with 419, which reads like a broken
	// session rather than a missing field.
	token, err := c.csrf.Issue(c.sessions.IDFromRequest(ctx.Request))
	if err != nil {
		return err
	}

	// The name to greet, from the id the session carries. One lookup by primary
	// key for the person who is signed in, and the id is the fallback: a header
	// is not worth a 500, and a guest never reaches the lookup at all.
	name := subject.ID
	if signedIn && c.people != nil {
		// PublicNames authorizes against the reader rather than taking a tenant
		// string, so the policy sees who is asking.
		reader := security.Subject{ID: subject.ID, Tenant: c.tenant}
		if names, err := c.people.PublicNames(ctx.Ctx(), reader, []string{subject.ID}); err == nil && names[subject.ID] != "" {
			name = names[subject.ID]
		}
	}

	return ctx.View("home", views.HomeData{
		Page: view.Page{
			Title:         c.appName,
			AppName:       c.appName,
			Token:         token,
			Authenticated: signedIn,
			UserName:      name,
			HomeURL:       "/",
			LoginURL:      "/auth/login",
			LogoutURL:     "/auth/logout",
		},
		Name: "world",
		Features: []views.Feature{
			{
				Title: "Authorization the compiler enforces",
				Body:  "No repository is reachable without a security.Grant, and no Grant exists without a Policy having answered.",
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
