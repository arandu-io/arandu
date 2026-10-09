---
name: arandu-http
description: Controllers, requests and routes of an Arandu (Go) application -- the seven resource actions, a resource nested under another, a singleton, a single-action (invokable) controller, a named action such as publish or approve, the request struct and its validation, route guards and route names. Use when the request is to "add a route", "add an endpoint", "add an action", "publish/approve/cancel a record", "nest X under Y", "validate this form", "who can reach this page", or when ctx.Bind, ctx.User, r.Resource, r.ResourceAction, r.Singleton or r.Invokable is involved. Covers aru make:controller (--resource, --singleton, --invokable, --parent, --action), make:request and make:middleware.
license: MIT
---

# Controllers, requests and routes

## When to use

Anything between the router and the service: which address answers, behind
which guard, which controller method takes it, and how the input becomes a
request struct. What the answer looks like is `arandu-view` for a page and
`arandu-api` for a program; what the service does is `arandu-module`.

## Before you start

- Read `routes/web.go` -- its custom block is the whole table of what this
  application answers -- and `app/Http/Controllers/NoteController.go`.
- The service the controller calls exists first. A controller written before
  its service grows the service's work.

## Contracts and imports

| piece | contract |
| --- | --- |
| action | `func (c *X) Name(ctx *hhttp.Context) error`, with `hhttp "github.com/arandu-io/hesape/http"` |
| resource controller | the seven actions, each asserted: `var _ fhttp.Indexer = (*X)(nil)` and `Creator`, `Storer`, `Shower`, `Editor`, `Updater`, `Destroyer`, with `fhttp "github.com/arandu-io/framework/http"`; `Resource` registers only what is implemented |
| invokable controller | `Invoke(ctx *hhttp.Context) error`, asserted with `var _ fhttp.Invoker = (*X)(nil)` |
| request | `app/Http/Requests/<Entity>Request.go`: fields with `form:"..."` tags, `func (r X) Validate() validation.Errors`, `var _ validation.Validatable = X{}` |
| who is asking | `who, _ := ctx.User()`, put there by the route's guard |
| input | `ctx.Bind(&in)`, which reads only the tagged fields; a conversion error comes back as `validation.Errors` |
| path | `ctx.Param("id")` on a flat resource's member; `ctx.Param("note")` and `ctx.Param("comment")` under a nested one |
| an address | `ctx.URL("notes.show", id)` and `ctx.RedirectRoute("notes.show", id)`, by route name |
| an error | returned; the router answers `validation.Errors` with a redirect back to the form (a 422 problem document to a JSON client), a missing row 404, a refusal 403, a duplicate key 409, and an error with `HTTPStatus() int` that status |

The routes, in the custom block of `routes/web.go`:

| shape | registration | addresses and names |
| --- | --- | --- |
| resource | `r.Resource("notes", d.Note)` | `/notes`, `/notes/{id}`; `notes.index` ... `notes.destroy` |
| nested, shallow | `r.Resource("notes.comments", d.Comment)` | index, create, store under `/notes/{note}/comments`; show, edit, update, destroy at `/comments/{comment}`; `notes.comments.*` |
| named action | `r.ResourceAction("POST", "notes", "publish", d.Note.Publish)` | `POST /notes/{id}/publish`, `notes.publish` |
| singleton | `r.Singleton("settings", d.Settings)` | show, edit, update at `/settings`, no id |
| invokable | `r.Invokable("POST", "/reports", d.RunReport).Name("reports")` | one address |
| one action | `r.Action("GET", "/dashboard", d.Home.Index, guard).Name("dashboard")` | one address |

A guard goes on the group or the route -- `r.Group("", middleware.RequireAuth(d.Sessions))`
-- with `middleware "github.com/arandu-io/framework/http/middleware"`, never at
the top of an action.

## Procedure

1. **Pick the shape.** CRUD on a record is a resource. A record reached
   through another is nested, and the parent in the path is where the person
   navigated -- the service loads it through the Grant. A verb on one record is
   a named action of its resource. One thing with no id per account is a
   singleton. One operation that is not a record's is an invokable controller.
2. **Generate it.** `aru make:module` writes the resource controller with the
   module; one controller alone is `aru make:controller Note --resource`,
   `--parent=notes`, `--singleton` or `--invokable`, and `--action=publish`
   adds a named action. Each prints the route lines.
3. **Write the action thin:** read who is asking, bind the input, call the
   service, answer. `NoteController.Publish` is the whole shape of a named
   action:

   ```go
   func (c *NoteController) Publish(ctx *hhttp.Context) error {
   	who, _ := ctx.User()
   	published, err := c.svc.Publish(ctx.Ctx(), who, ctx.Param("id"))
   	if err != nil {
   		return err
   	}
   	return ctx.RedirectRoute("notes.show", published.ID)
   }
   ```

4. **Write the request's rules** in `Validate`, and leave calling it to the
   service, so a job and a command that call the same service pass the same
   validation.
5. **Register the route** in the custom block, behind its guard, and give it a
   name if the generator did not.

## Commands

- `aru make:controller <Name> --resource`, `--singleton`, `--invokable`, with `--parent=<resource>` and `--action=<name>`
- `aru make:request <Name> --fields "title:string!,body:text"`
- `aru make:middleware <Name>`
- `aru route:list`, the table the router built

`aru make:controller --force` rewrites the file and keeps only its custom block:
on a controller finished by hand, as `NoteController` is, run it in a scratch
copy and move the new method into the custom block.

## Example

A single-action controller and the routes of every shape, as they compile
against this project:

```go compile
package example

import (
	fhttp "github.com/arandu-io/framework/http"
	"github.com/arandu-io/framework/http/middleware"
	"github.com/arandu-io/framework/security"
	hhttp "github.com/arandu-io/hesape/http"

	controllers "<module>/app/Http/Controllers"
	services "<module>/app/Services"
)

// PublishNote is one operation with one address: an invokable controller.
type PublishNote struct {
	notes *services.NoteService
}

// NewPublishNote takes the service it calls, built in bootstrap/app.go.
func NewPublishNote(notes *services.NoteService) *PublishNote {
	return &PublishNote{notes: notes}
}

var _ fhttp.Invoker = (*PublishNote)(nil)

// Invoke publishes the note the form names in its path.
func (c *PublishNote) Invoke(ctx *hhttp.Context) error {
	who, _ := ctx.User()
	published, err := c.notes.Publish(ctx.Ctx(), who, ctx.Param("id"))
	if err != nil {
		return err
	}
	return ctx.RedirectRoute("notes.show", published.ID)
}

// Routes registers each shape behind the sign-in guard.
func Routes(r *fhttp.Router, sessions *security.SessionStore, note *controllers.NoteController,
	comment *controllers.CommentController, publish *PublishNote) {
	signedIn := r.Group("", middleware.RequireAuth(sessions))
	signedIn.Resource("notes", note)
	signedIn.ResourceAction("POST", "notes", "publish", note.Publish)
	signedIn.Resource("notes.comments", comment)
	signedIn.Invokable("POST", "/notes/{id}/publish-now", publish).Name("notes.publish-now")
}
```

## Do not

- Read the form field by field, parse it, or decode a body by hand:
  `input-read-by-hand`. `ctx.Bind` into the request is the one conversion.
- Call `Validate()` in the controller: `validate-called-by-controller`.
- Answer a rejected form with a 422 of your own -- htmx discards its body and a
  reload posts again -- or redirect to a path written as a literal:
  `invalid-form-answered-by-hand`, `redirect-to-literal-path`.
- Load the session in a controller outside `app/Http/Controllers/Auth`:
  `session-loaded-in-controller`. `ctx.User()` is who is asking.
- Reach a model, a repository or a client from a controller:
  `handler-reaches-the-model`, `controller-reaches-repository`.
- Choose the operation by a form field (`operation-chosen-by-form-field`), put a
  state change behind GET, or grow a controller past twelve actions
  (`controller-too-many-actions`): a verb is a named action or its own
  controller.

## Extending it

Actions beyond the seven go in the controller's custom block, and their route
lines in the custom block of `routes/web.go`. Rules a request needs beyond the
generated ones -- a range, a format, a field that depends on another -- go in
the custom block of `Validate`.

## Wiring

- `routes/web.go`: the controller's field on `Deps`, and the route lines in the
  custom block, behind a guard.
- `bootstrap/app.go`: the constructor in the `routes.Deps` literal, with the
  service built once and shared -- the example builds `notes` once and hands it
  to both controllers that need it.
- A middleware of the application is constructed in `bootstrap/app.go` and
  put on the route or the group that needs it.

## Acceptance test

A feature test through the router, signed in with `tests.SignedIn`:

- the success, with the status and the `Location` it redirects to;
- another member's record (403) and another tenant's (404), for every action
  that takes an id, under the nested resource and the named action too;
- a rejected form: 303 back with the messages, 204 with `HX-Redirect` to htmx,
  422 problem document to JSON;
- a guest redirected to the sign-in page, and a GET on a state-changing route
  answered 405.

`tests/Feature/Notes_test.go` and `tests/Feature/Comments_test.go` hold each.

## Limits

`ctx.View(name, ...)` and route names are strings: a typo compiles, and is found
by `aru doctor` (`view-does-not-exist`) and by the tests, not by the compiler.
The router answers the errors it knows; a domain error it should answer with
another status carries `HTTPStatus() int` rather than being mapped in the
action.

## Gates

Run them all, in this order, as `AGENTS.md` lists them:

```sh
export GOWORK=off
aru model:build --check
aru view:build
gofmt -l $(find . -name '*.go' -not -path '*/testdata/*' -not -name '*.kyse.go')
go vet ./...
bash tests/test-layout-guard.sh
go test -race ./...
go build ./...
aru doctor
```

<!-- arandu:begin custom -->
<!-- arandu:end custom -->
