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

The build tag is what keeps the compiler out of the file. Everything before the
first tag is Go the compiler would reject; the tag is why it never sees it.

## The procedure

1. Write the `.kyse.go`.
2. `aru view:build` — it names the line if the template is wrong.
3. `go build ./...` — it names the line if the Go is wrong.
4. `aru doctor` — it names the line if the escaping is wrong.

## Directives

`@extends` `@section`/`@endsection` `@yield` `@if`/`@elseif`/`@else`/`@endif`
`@foreach`/`@endforeach` `@forelse`/`@empty`/`@endforelse` `@for`/`@endfor`
`@go`/`@endgo` `@csrf`

`{{ }}` escapes. `{!! !!}` does not. `{{-- --}}` is stripped and never reaches
the page.

## The rules that will bite you

**The data is a struct that embeds `view.Page`. Never a map.** A typo in a map
key is a blank space on a page that answered 200; a typo in a field name is a
build error. `aru doctor` fails a map.

**`{!! !!}` is for a call, not a value.** It is entitled to skip escaping only
because a component function escaped everything it interpolated when it was
generated. A field that already holds markup has been through nothing, and the
first time one of them comes from a person it is stored cross-site scripting.
Write `{{ x }}` or return it from a component.

**There is no Alpine, and no `x-` attribute does anything.** Alpine is not
served, and pages run under `script-src 'self'` with no `unsafe-eval`, so an
`x-on:click`, an `x-data` or an `@click` would be dead markup at best. `@` also
starts a directive, so the compiler refuses the `@click` shorthand outright.
Client behaviour comes from `ui.js`, which is bound once on `document` and
dispatches on `data-*` attributes: the attribute carries data or the name of a
registered behaviour (`data-kyse-behavior`, `data-kyse-on-click`), never code.

**No expression goes into an attribute the browser runs.** An event handler
(`on*`), `hx-on*` and the `x-`/`:` families hold code, and the compiler refuses
interpolation into them. Dynamic data travels in an ordinary `data-*`
attribute, where the escaper can see it. This is what keeps the escaping
guaranteed, and it is also what the security policy requires: a string compiled
into a function at run time would not execute.

**A style attribute is refused too.** `style-src 'self'` drops `style="..."` as
surely as it drops an inline script. Use a class.

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
a JavaScript dependency, you are about to be wrong: `resources/` holds no `.js`
at all, and a test fails the build if one appears.
