---
name: arandu-module
description: Add an entity, resource, model, table or CRUD feature to an Arandu (Go) application. Use when the request is to "create a model", "add a resource", "scaffold CRUD", "add invoices", "make a posts table", "generate a module", or any new domain object with a database table and rules about who may touch it. In Arandu the model writes a YAML specification and a deterministic generator writes the Go — never write the entity, policy, service or migration by hand. Covers aru schema, aru generate, aru make:module, the ten column types, the five actions, and the custom blocks that survive regeneration.
license: MIT
---

# Adding a module to an Arandu application

You do not write the Go. You write a specification and a deterministic generator
writes the model (the entity and its `Model` constructor, whose query terminals
take a Grant), the policy, the service, the request, the controller, the
migration, the screens and the tests. It writes no repository and no routes: the
route lines and the wiring are **printed** for you to paste.

This is not a preference. A new framework is in nobody's training set, so a
model asked for Go here fills the gap with the frameworks it does know and
produces a service container, a fillable model, a facade — none of which exist.
A specification is small enough to be right, and a wrong one fails validation
instead of becoming Go that does not compile.

## The reference

The project ships one module built this way, **notes**, and it is the shape to
copy when in doubt: `app/Models/Note.go`, `app/Policies/NotePolicy.go`,
`app/Services/NoteService.go`, `app/Http/Controllers/NoteController.go`,
`app/Http/Requests/NoteRequest.go`, `resources/views/notes/`,
`database/factories/NoteFactory.go`, `database/seeders/NoteSeeder.go` and
`tests/Feature/Notes_test.go`. It is marked as an example in every file, and
README.md lists what to delete when the project no longer wants it.

What it shows that the generator alone does not: an ownership rule in the
policy's custom block, the author set in the service from the subject and never
from the form, and feature tests that sign somebody in and prove a cross-tenant
read is 404 and another member's change is 403.

## The procedure

**1. Read the schema.** It is generated from the validator's own constants, so
it cannot drift from what the generator accepts.

```sh
aru schema
```

**2. Write the specification.** One file, six top-level properties, and the
schema refuses any property it does not know.

```yaml
# invoice.yaml
version: "1"
name: invoice
description: An invoice sent to a customer.
tenant: true
fields:
  - name: reference
    type: string
    required: true
    unique: true
  - name: total
    type: money
  - name: sent_at
    type: timestamp
permissions:
  view: [member, admin]
  create: [admin]
  update: [admin]
  delete: [admin]
```

**3. Check it before anything is written.**

```sh
aru generate invoice.yaml --check
```

Everything wrong with the document is reported at once. Fix all of it and check
again.

**4. Generate.**

```sh
aru generate invoice.yaml
```

The specification is saved beside the code it produced, in `database/specs/`,
so regenerating reads it back. `aru make:module` is the same generator driven by
flags instead of a file — `aru make:module invoice --fields
"reference:string!,total:money" --tenant [--force]` — and writes the same tree;
the flags have no way to say `permissions` or a description, so use the
specification when the module needs either.

**5. Wire it.** The generator prints three lines to paste: the controller field
in `routes.Deps`, the routes behind the sign-in guard in the custom block of
`routes/web.go` —

```go
r.Group("", middleware.RequireAuth(d.Sessions)).Resource("invoices", d.Invoice)
```

— and the constructor in the `routes.Deps` literal of `bootstrap/app.go`:

```go
Invoice: controllers.NewInvoiceController(services.NewInvoiceService(db)),
```

For a `tenant: true` module it also prints the line that claims the table in
`tests/Feature/TenantScope_test.go`; that suite is red until somebody has read
the queries and added it. The generator edits none of these files, on purpose:
a generator that changes the wiring behind you is a generator whose output
nobody can explain.

**6. Run the gates.**

```sh
export GOWORK=off
aru view:build && go build ./... && go vet ./... && go test -race ./... && aru doctor
```

## The two closed sets

Column types, and nothing else is accepted:

`string` `text` `int` `decimal` `money` `bool` `date` `timestamp` `uuid` `email`

`money` is stored in cents as an integer. `decimal` is never money — a
fractional binary number is the wrong shape for an amount and the schema says so.

Actions, and nothing else: `view` `create` `update` `delete` `list`.

They are closed because an open set is a type system, and a type system is a
language somebody maintains forever.

## What falls outside

A case the specification cannot express is written in Go, between the markers
the generator preserves:

```go
// arandu:begin custom
// Your code here survives the next `aru generate`.
// arandu:end custom
```

Do not widen the specification to fit one case. That is how a schema becomes a
language.

## What the generated code guarantees, and you must not undo

- Every service method takes the acting `security.Subject` and asks the Policy
  through `security.Authorize` before it touches a row, and every Model read and
  write — `FindOrFail`, `First`, `Get`, `SimplePaginate`, `Save`, `Delete` —
  takes the `security.Grant` that call issued. Removing it to make something
  compile is removing the only thing that makes the query safe.
- The controller takes the service and nothing else. Who is asking is
  `ctx.User()`, which the route guard put on the request; the input is
  `ctx.Bind` into the request struct's `form` tags; a page's chrome and its CSRF
  token are `view.New(ctx, title)`. No controller loads the session or issues a
  token.
- An action returns its error and the router answers it: `validation.Errors`
  back to the form, a missing row 404, a refusal 403, a duplicate key 409, and
  an error with an `HTTPStatus() int` method that status. Do not map them by
  hand.
- The tenant comes from `data.Tenant(g)`. Never from a path segment, a body, a
  query or a header.
- The generated policy denies every action, with no allow-everything branch to
  delete later. Open it deliberately, one action at a time.

If you find yourself wanting to query a Model without a Grant, stop. There is no
correct way to do it, and the compiler is what says so.
