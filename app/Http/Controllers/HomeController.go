package controllers

import (
	"github.com/arandu-io/framework/httpx"

	"github.com/arandu-io/arandu/resources/views"
)

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
}

// NewHomeController returns the controller. `bootstrap` builds it and hands it
// to the routes.
func NewHomeController(appName string) *HomeController {
	return &HomeController{appName: appName}
}

// Compile-time proof that this controller answers GET / the way Resource and the
// route table expect. It costs nothing and catches a renamed method.
var _ httpx.Indexer = (*HomeController)(nil)

// Index renders the landing page.
//
// The data is views.HomeData, the struct the view itself declares. Hand it
// anything else and the build fails, naming both sides -- which is the whole
// reason the view is compiled instead of interpreted.
func (c *HomeController) Index(ctx *httpx.Context) error {
	return ctx.View("home", views.HomeData{
		Title: c.appName,
		Name:  "world",
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
