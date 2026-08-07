// Package controllers holds this application's controllers.
//
// The directory is app/Http/Controllers, in CamelCase, exactly where a Laravel
// developer looks for it. The package name follows Go and stays lowercase.
//
// A controller reads the request, calls a service and renders. It never reaches
// the database: a controller holding a repository is a controller that skipped
// the service, and therefore skipped the policy, and `aru doctor` refuses it.
package controllers

import (
	"net/http"

	"github.com/arandu-io/framework/httpx"
	"github.com/arandu-io/framework/validation"
)

// Controller is the type every controller in this directory embeds.
//
// It carries no dependencies, and that is deliberate: Laravel's base controller
// grew authorize(), validate() and dispatch() because there was nowhere else to
// put them, and each one hid a collaborator. Here it holds only what is about
// answering a request, and a controller's collaborators are fields it declares
// and the constructor sets.
type Controller struct{}

// Invalid answers a failed validation with the fragment HTMX swaps back in.
//
// The status is 422, not 200. A form that answers 200 with the errors in it
// tells the browser, the logs and every dashboard that the write succeeded --
// and HTMX still swaps the fragment either way, so nothing looks wrong until
// somebody asks why the success rate is 100%.
func (Controller) Invalid(ctx *httpx.Context, view string, data any) error {
	return ctx.Fragment(http.StatusUnprocessableEntity, view, data)
}

// Validated reports whether a request passed validation, and answers the caller
// when it did not.
//
//	if errs := req.Validate(); !c.Validated(errs) { ... }
//
// It exists so the check reads the same in every controller. What it does not do
// is decide the response: the view and its data are the page's business, and a
// base class that picked them would be a second router.
func (Controller) Validated(errs validation.Errors) bool { return len(errs) == 0 }
