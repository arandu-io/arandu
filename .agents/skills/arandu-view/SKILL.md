---
name: arandu-view
description: Write or change a page, layout, template, HTML fragment or component usage in an Arandu (Go) application — anything under resources/views. Use when the request is to "add a page", "change the layout", "make a form", "render a list", "style this", or when an HTMX fragment is involved. The templates are .kyse.go files compiled to Go, and the syntax, the escaping rules and the Content-Security-Policy constraints are not the ones any other template engine uses. Covers @extends, @section, @foreach, the typed data struct, escaped versus raw interpolation, and why an inline style or an Alpine attribute will not work.
license: MIT
---

# Writing a view

A view is Go. You write `resources/views/home.kyse.go`; `aru view:build`
compiles it to `storage/framework/views/home.go`, which is build output and is
gitignored. An error points at the line you wrote, not at generated code.

## The shape of the file

```go
//go:build kyse

package views

import (
	"github.com/arandu-io/hesape/view"
	"github.com/arandu-io/kyse/components"
)

@go
// InvoicesData is what the controller hands this page.
type InvoicesData struct {
	view.Page

	// Invoices is what the list draws, already formatted by the controller.
	Invoices []InvoiceRow
}

// InvoiceRow is one invoice, as the page shows it.
type InvoiceRow struct {
	Reference string
}

// Compile-time proof that this page fits the layout it extends.
var _ view.Layout = InvoicesData{}
@endgo

@extends('layouts.app')

@section('content')
	<h1>{{ .PageTitle() }}</h1>

	@foreach(.Invoices as invoice)
		<p>{{ invoice.Reference }}</p>
	@endforeach

	{!! components.Button(components.ButtonProps{Label: "New"}) !!}
@endsection
```

The build tag is what keeps the compiler out of the file: Go reads the
constraint, leaves the file out, and never parses the markup below the package
clause, which it would reject.

`resources/views/notes/` is the example resource's four screens — a listing
through the `DataTable` component, a record, and the create and edit forms
built from `Field`, `Textarea` and `Checkbox` — and the place to look for a
working page of each kind. `resources/views/partials/notes_table.kyse.go` is
the listing's table, the one fragment the project ships.

## The page the controller hands over

The controller fills `view.Page` with `view.New(ctx, title)`: the title, the
messages and the typed input a rejected form left behind, and the CSRF token
the middleware issued for this request. `@csrf` and the layout's `hx-headers`
read that token off the page. A controller never issues one itself; a page
drawn outside the middleware that protects forms is the only one that needs
`WithToken`.

## A page, or a fragment

Every navigation is answered with the whole document: a direct visit, a link
the layout's `hx-boost` turned into an htmx request, a history restore. The
answer never changes on `HX-Boosted`, so a reload shows the same page.

A fragment is what one element asks for on purpose: `hx-get` or `hx-post`, with
an `hx-target` naming the element it replaces.

- Its view lives in `resources/views/partials/` and has no `@extends`.
- Its page draws it with `@include('partials.<name>')`, which hands the page's
  data over unchanged, so the page and the fragment come from one template.
- The controller answers it with
  `ctx.Fragment(http.StatusOK, "partials.<name>", data)` when `ctx.IsHTMX()` and
  `ctx.Header("HX-Target")` names that element, and with the page otherwise.
  Both answers carry `Vary: HX-Request, HX-Target`.
- `bootstrap/app.go` links the partials package with a blank import, as it
  links the layouts: a view nobody imports is never registered.

`NoteController.Index` and `partials/notes_table.kyse.go` are the example.

A rejected form is not a fragment. The action returns the `validation.Errors`,
the router sends the browser back to the form -- htmx follows that as a
navigation -- and the page after it shows the messages through `view.New`.
There is no 422 to swap.

## The procedure

1. Write the `.kyse.go`.
2. `aru view:build` — it names the line if the template is wrong.
3. `go build ./...` — it names the line if the Go is wrong.
4. `aru doctor` — it names the line if the escaping is wrong.

That is the loop while writing. Before calling it finished, run the gates, all
of them, as `AGENTS.md` lists them.

## Directives

`@extends` `@section`/`@endsection` `@yield` `@include`
`@if`/`@elseif`/`@else`/`@endif` `@foreach`/`@endforeach`
`@forelse`/`@empty`/`@endforelse` `@for`/`@endfor` `@while`/`@endwhile`
`@continue` `@break` `@go`/`@endgo` `@csrf` `@attributes`

That is the whole set, and it is closed: what does not fit is written in Go,
inside `@go`.

`{{ }}` escapes. `{!! !!}` does not. `{{-- --}}` is stripped and never reaches
the page.

## The rules that will bite you

**The data is a struct that embeds `view.Page`. Never a map.** A typo in a map
key is a blank space on a page that answered 200; a typo in a field name is a
build error. `aru doctor` fails a map.

**`{!! !!}` accepts only a `template.HTML`, and the compiler enforces it.** The
value is assigned to a `template.HTML` before it is written, so a component, an
icon, a field typed as markup or a constant the view spells out compiles, and
a `string` that arrived as data stops the build at the line of the `.kyse.go`.
A component is entitled to skip escaping only because it escaped everything it
interpolated when it was generated. Do not convert a string to `template.HTML`
to get past the error: a value somebody typed has been through nothing, and the
conversion is stored cross-site scripting. Write `{{ x }}` or return it from a
component.

**There is no Alpine, and no `x-` attribute does anything.** Alpine is not
served, and pages run under `script-src 'self'` with no `unsafe-eval`, so an
`x-on:click`, an `x-data` or an `@click` would be dead markup at best. `@` at
the start of a line opens a directive, so the compiler refuses an `@click`
written there; inside a tag it compiles, and `aru doctor` fails it, and every
`x-` attribute with a value, as `view-keeps-state-in-the-browser`.
Client behaviour comes from `ui.js`, which is bound once on `document` and
dispatches on `data-*` attributes: the attribute carries data or the name of a
registered behaviour (`data-kyse-behavior`, `data-kyse-on-click`), never code.
This project registers its own behaviours by name in `resources/js/custom.js`,
with `arandu.ui.define` and `arandu.ui.action`.

**No expression goes into an attribute the browser runs.** An event handler
(`on*`), `hx-on*` and the `x-`/`:` families hold code, and the compiler refuses
interpolation into them. Dynamic data travels in an ordinary `data-*`
attribute, where the escaper can see it. This is what keeps the escaping
guaranteed, and it is also what the security policy requires: a string compiled
into a function at run time would not execute.

**A style attribute is refused too, by the browser.** `style-src 'self'` drops
`style="..."` as surely as it drops an inline script. Use a class.

**A loop binding is an ordinary Go name.** `@foreach(.Rows as row)` is fine, and
so is `s`, `err` or `d`. What the compiler refuses is a Go predeclared
identifier — `nil`, `len`, `string` — and it says so with the file and the line.

## Components

Imported, never copied. A component is an exported Go function taking its own
props struct, so a component that does not exist and a field that does not exist
are both build errors.

```sh
go get github.com/arandu-io/kyse/components
```

Do not write a component in the application when the library has one. Do not
copy one out of the library either — a copy stops receiving fixes.

## What is not here

No `node_modules`, no `package.json`, no bundler, no CDN. CSS is Tailwind
through a standalone binary the CLI downloads and pins. If you are about to add
a JavaScript dependency, you are about to be wrong: `resources/` holds one
script, `resources/js/custom.js`, which `resources/js/js.go` embeds and the
layout loads after `ui.js`. It is served as written, with no bundler between,
and `tests/Unit/resources_test.go` fails if any other `.js` appears under
`resources/` or if that one goes missing.
