---
name: arandu-view
description: Write or change a page, layout, template, HTML fragment or component usage in an Arandu (Go) application -- anything under resources/views. Use when the request is to "add a page", "change the layout", "make a form", "render a list", "style this", "load this part with htmx", or when a fragment, hx-get, hx-target, @include or a partial is involved. The templates are .kyse.go files compiled to Go, and the syntax, the escaping rules and the Content-Security-Policy constraints are not the ones any other template engine uses. Covers @extends, @section, @foreach, the typed data struct, page against fragment, escaped versus raw interpolation, and why an inline style or an Alpine attribute will not work.
license: MIT
---

# Writing a view

## When to use

A page a person navigates to, a part of one that htmx replaces, a form, a
component call, the layout. The action that renders it is `arandu-http`; a JSON
answer is `arandu-api`.

## Before you start

- `resources/views/notes/` holds the example's four screens -- a listing
  through the `DataTable` component, a record with a named action's form, and
  the create and edit forms built from `Field`, `Textarea` and `Checkbox` --
  and `resources/views/partials/notes_table.kyse.go` is the listing's table,
  the one fragment the project ships. `resources/views/comments/` is what
  `aru make:module` writes for a nested resource, untouched.
- A view is Go: `resources/views/home.kyse.go` compiles, through
  `aru view:build`, to `storage/framework/views/home.go`, which is build output
  and is gitignored. An error points at the line you wrote.

## Contracts and imports

| piece | contract |
| --- | --- |
| page | `resources/views/<area>/<action>.kyse.go`: `//go:build kyse`, `@extends('layouts.app')`, and a data struct that embeds `view.Page` (`github.com/arandu-io/hesape/view`), asserted with `var _ view.Layout = XData{}` |
| fragment | `resources/views/partials/<name>.kyse.go`, no `@extends`; its page draws it with `@include('partials.<name>')`, handing the page's data over |
| chrome | `view.New(ctx, title)` in the controller: the title, the flash a rejected form left, the CSRF token the middleware issued |
| answering | `ctx.View("notes.index", data)` for the page, `ctx.Fragment(http.StatusOK, "partials.notes_table", data)` for the fragment |
| component | `github.com/arandu-io/kyse/components`, imported, never copied: an exported function taking its props struct |
| behaviour | `ui.js`, bound once on `document`, dispatching on `data-*` attributes; this project registers its own in `resources/js/custom.js` with `arandu.ui.define` and `arandu.ui.action` |

The directives are a closed set: `@extends` `@section`/`@endsection` `@yield`
`@include` `@if`/`@elseif`/`@else`/`@endif` `@foreach`/`@endforeach`
`@forelse`/`@empty`/`@endforelse` `@for`/`@endfor` `@while`/`@endwhile`
`@continue` `@break` `@go`/`@endgo` `@csrf` `@attributes`. `{{ }}` escapes,
`{!! !!}` does not, `{{-- --}}` never reaches the page.

## Procedure

1. **Write the `.kyse.go`**, its data struct in `@go`:

   ```go
   //go:build kyse

   package views

   import "github.com/arandu-io/hesape/view"

   @go
   // InvoicesData is what the controller hands this page.
   type InvoicesData struct {
   	view.Page
   	Invoices []InvoiceRow
   }

   // InvoiceRow is one invoice, already formatted by the controller.
   type InvoiceRow struct{ Reference string }

   var _ view.Layout = InvoicesData{}
   @endgo

   @extends('layouts.app')

   @section('content')
   	@foreach(.Invoices as invoice)
   		<p>{{ invoice.Reference }}</p>
   	@endforeach
   @endsection
   ```

   The build tag keeps the compiler out of the file: Go reads the constraint
   and never parses the markup below the package clause.
2. **Decide page or fragment.** Every navigation -- a direct visit, a link the
   layout's `hx-boost` turned into an htmx request, a history restore -- gets
   the whole document, and the answer never changes on `HX-Boosted`, so a
   reload shows the same page. A fragment is what one element asks for on
   purpose, with `hx-get` or `hx-post` and an `hx-target` naming the element it
   replaces; it lives in `partials/` and the page includes it.
3. **Answer it from the controller** with the fragment only when `ctx.IsHTMX()`
   and `ctx.Header("HX-Target")` names that element, the page otherwise, and
   `Vary: HX-Request, HX-Target` on both.
4. **Build, compile, check:** `aru view:build` names the template's line,
   `go build ./...` the Go's, `aru doctor` the escaping's.

A rejected form is not a fragment. The action returns the `validation.Errors`,
the router sends the browser back to the form -- htmx follows that as a
navigation -- and the page after it shows the messages through `view.New`. The
sign-in screens of `go run github.com/arandu-io/ui@v0.20.0 auth` follow the same
rule: a rejected sign-in is a redirect back to its screen, and a password never
comes back.

## Commands

- `aru view:build`, which also builds the stylesheet with the pinned Tailwind
- `aru make:module` writes the four screens of a resource; `aru make:mail` writes a mail's two views
- `aru dev` rebuilds the views on every change
- `go get github.com/arandu-io/kyse/components`

## Example

The controller side of one address with two representations, as
`NoteController.Index` writes it:

```go compile
package example

import (
	"net/http"

	hhttp "github.com/arandu-io/hesape/http"
	"github.com/arandu-io/hesape/view"

	views "<module>/storage/framework/views/notes"
)

// Listing answers the page to every navigation, and the table alone to the
// one request that names the table as the element it replaces.
func Listing(ctx *hhttp.Context, rows []views.NoteRow) error {
	data := views.NotesIndexData{
		Page:   view.New(ctx, "Notes"),
		Notes:  rows,
		NewURL: ctx.URL("notes.create"),
	}
	ctx.Response.Header().Add("Vary", "HX-Request, HX-Target")
	if ctx.IsHTMX() && ctx.Header("HX-Target") == views.NotesTableID {
		return ctx.Fragment(http.StatusOK, "partials.notes_table", data)
	}
	return ctx.View("notes.index", data)
}
```

## Do not

- Hand a view a map: a typo in a key is a blank space on a page that answered
  200. `aru doctor` fails it as `view-data-is-a-map`.
- Convert a string to `template.HTML` to get it through `{!! !!}`: a value
  somebody typed has been through nothing, and that is stored cross-site
  scripting. `{!! !!}` takes a component call; `raw-output-is-not-a-component`.
- Write an `x-` attribute, an `@click` or Alpine: it is not served, the pages
  run under `script-src 'self'` with no `unsafe-eval`, and `aru doctor` fails
  it as `view-keeps-state-in-the-browser`.
- Interpolate into an attribute the browser runs (`on*`, `hx-on*`): the
  compiler refuses it. Data travels in a `data-*` attribute.
- Write `style="..."`: `style-src 'self'` drops it. Use a class.
- Answer a fragment from a view outside `partials/` (`fragment-without-partial`),
  or decide by `HX-Boosted`.
- Add a JavaScript dependency, a `package.json` or a CDN. `resources/js/custom.js`
  is the one script, served as written; `tests/Unit/resources_test.go` fails on
  any other.

## Extending it

Anything a page needs in Go goes in the custom block inside its `@go`, and
survives regeneration of the screens. A new component belongs in the kyse
library, not in the application; a project's own behaviour is a named action
in `resources/js/custom.js`.

## Wiring

A page needs nothing: the controller that renders it imports its compiled
package. A directory of views that no controller imports -- the layouts, the
partials -- is linked with a blank import in `bootstrap/app.go`, or its views
are never registered and the first request says so.

## Acceptance test

- A feature test that loads each page and sees what it draws.
- For an address with a fragment, the matrix of
  `TestTheNotesTableIsAnsweredAloneOnlyToTheElementThatAsksForIt`: a direct
  visit, a boosted link, a history restore, an htmx request for another
  element and the target named without htmx all get the whole document; the
  element's own request gets the fragment alone; every answer carries the
  `Vary`.
- For a page with no fragment, the shape of `TestTheCommentPagesAreWholeDocuments`.

## Limits

`ctx.View` takes the view's name as a string, so a missing view compiles; it is
`aru doctor`'s `view-does-not-exist` and the tests that find it. The escaping is
guaranteed for what passes through `{{ }}` and the components; a value a
controller turned into `template.HTML` is trusted from then on.

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
