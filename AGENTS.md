# Working in this repository

This is an Arandu application. Arandu is a Go framework where the architecture
is enforced by the compiler rather than by convention, so the fastest way to be
wrong here is to write what another framework would want.

Read `.agents/skills/` before writing code. Start with `arandu-feature`: it
classifies the change, walks the two tables below and names the family skill
of each step -- `arandu-module`, `arandu-http`, `arandu-api`, `arandu-view`,
`arandu-async`, `arandu-integrations`, `arandu-policy`, `arandu-doctor` and
`arandu-ecosystem`. An assistant whose client can start an MCP server connects
to `{"command": "aru", "args": ["mcp"]}` and asks it before guessing:
`arandu-mcp` says which tool answers what.

## Feature anatomy

The order a feature is built and checked in. It is not the order a request runs
through, and not every feature has every step: a task with no table gets no
migration, an API with no screen gets no view.

Every generator below exists in `aru` v0.66.0, the version the Dockerfile
builds with, and prints its wiring instead of editing `bootstrap/app.go` or
`routes/web.go`. The example column is the `notes` resource, which has every
step; `.agents/skills/notes` says which command wrote each file.

| # | step | file | generator | example in this repository |
| --- | --- | --- | --- | --- |
| 1 | specification, when there is one | `database/specs/<entity>.yaml` | `aru schema`, `aru generate` | none: notes was generated from flags |
| 2 | migration | `database/migrations/<id>.go` | `aru make:migration`, `aru make:model -m` | `database/migrations/2026_10_01_000001_create_notes_table.go`, `database/migrations/2026_10_09_000002_add_published_at_to_notes.go` |
| 3 | model, its query and the entity's rules | `app/Models/<Entity>.go`, and `<Entity>Query.go` beside it | `aru make:model`, `aru model:build` | `app/Models/Note.go`, with the `Publish` transition |
| 4 | factory and seeder | `database/factories/`, `database/seeders/` | `aru make:factory`, `aru make:seeder` | `database/factories/NoteFactory.go`, `database/seeders/NoteSeeder.go` |
| 5 | policy | `app/Policies/<Entity>Policy.go` | `aru make:policy` | `app/Policies/NotePolicy.go` |
| 6 | request | `app/Http/Requests/<Entity>Request.go` | `aru make:request` | `app/Http/Requests/NoteRequest.go` |
| 7 | service | `app/Services/<Entity>Service.go` | `aru make:module`, `aru make:service` | `app/Services/NoteService.go` |
| 8 | controller | `app/Http/Controllers/<Entity>Controller.go` | `aru make:controller` with `--resource`, `--singleton`, `--invokable`, `--parent`, `--action` | `app/Http/Controllers/NoteController.go` (`Publish` is `--action=publish`), `app/Http/Controllers/CommentController.go` (`--parent=notes`) |
| 9 | routes and their guard | `routes/web.go` | printed by the generators, pasted by you | the notes group in the custom block of `routes/web.go` |
| 10 | page, fragment, component or JSON Resource | `resources/views/**/*.kyse.go`, `app/Http/Resources/` | `aru make:module` writes the pages; `aru make:resource` | `resources/views/notes/`, `resources/views/partials/notes_table.kyse.go`, `app/Http/Resources/NoteResource.go` |
| 11 | tests and wiring | `tests/Feature`, `tests/Unit`, `bootstrap/app.go` | `aru make:test`; `aru make:module --tenant` writes the tenant test | `tests/Feature/Notes_test.go`, `tests/Feature/CommentTenantScope_test.go`, `tests/Feature/TenantScope_test.go` |

`aru make:module <name> --fields "..." --tenant` writes the migration, the
model and its query, the factory, the seeder, the policy, the request, the
service, the controller, the pages and the tests in one go, and prints the
lines of step 9 and of `bootstrap/app.go` instead of editing them; with
`--parent=<resource>` it nests the module under another. `aru model:build`
rewrites the query whenever the entity changes.

Beyond the eleven steps, each in the example:

| what | generator | example |
| --- | --- | --- |
| a domain event, stored by the write | `aru make:event` | `app/Events/NotePublished.go`, stored by `NoteService.Publish` |
| a listener, run by the relay | `aru make:listener` | `app/Listeners/NotifyNoteAuthor.go`, in `listeners.Each` in `bootstrap/app.go` |
| a notification | `aru make:notification` | `app/Notifications/NotePublished.go`, over the database channel |
| a job and its schedule | `aru make:job` | `app/Jobs/SendNotesDigest.go`, scheduled in `app/Providers/AppServiceProvider.go` |
| an external client and its fake | `aru make:client` | `app/Clients/NewsletterClient.go`, `app/Clients/NewsletterFake.go` |
| a received webhook, verified before anything else | `aru make:controller`, `aru make:event` | `app/Http/Controllers/NewsletterWebhookController.go` at `/webhooks/newsletter`, exempt from CSRF by `middleware.CSRFExcept("/webhooks/")` in `bootstrap/app.go` |
| a write a program makes with a bearer token | none: a route in the `/api` group | `POST /api/notes` in `routes/web.go`, behind `RequireToken` and `Idempotent`; the tokens are `app/Services/PersonalAccessTokenService.go` |
| a tool, resource or prompt of an MCP server | `aru make:mcp-tool`, `aru make:mcp-resource`, `aru make:mcp-prompt` | none: this project does not require the mcp module |
| a console command | `aru make:command` | none |

## Where each kind of code lives

| kind of code | where | called by | must not |
| --- | --- | --- | --- |
| turning the HTTP input into a value | the controller, with `ctx.Bind` into the request | the router | read the form field by field, decode a body by hand |
| validating the request | `Validate()` on the request, in `app/Http/Requests` | the service | be called by the controller |
| an invariant or a transition of the entity | the custom block of `app/Models/<Entity>.go` | the service | reach the database, the network, an implicit clock or a Grant |
| a query scope | a method on `*<Entity>Query`, in the custom block of `app/Models/<Entity>.go` | the service, a repository | skip the Grant; `<Entity>Query.go` is generated and never edited |
| authorization | `app/Policies`, asked through `auth.Authorize` | the service | be skipped on a read |
| a use case | `app/Services`, one service per aggregate | a controller, a job, a listener, a command | take an HTTP type, live in a subpackage, run a worker of its own |
| a complex query, a report, a read model | `app/Repositories` | the service | exist for plain CRUD |
| a client of an external system | `app/Clients/<Vendor>Client.go`, with its interface and a fake | a service, a job, a listener | reach a model, a Grant or the session |
| an engine that wraps another technology, a client another project would reuse | a `github.com/hyz-is/arandu-*` module | a service, a job | live in `app/Services` |
| background work, a loop, a retry | `app/Jobs`, run by the worker or the scheduler | a service, the scheduler, a listener | be a goroutine started in a constructor or in `Boot` |
| a reaction to something that happened | `app/Listeners`, through the outbox, listed in `listeners.Each` | the relay | call a controller |
| telling a person something | `app/Notifications`, sent through the `Notifier` `bootstrap/app.go` builds | a listener, a service | write its own row or send mail by hand |
| a closed set of values | `app/Enums` (`aru make:enum`) | anything | — |
| a console command | `app/Console/Commands` (`aru make:command`), registered in `routes/console.go` | the console | put its logic in `bootstrap/` |
| a JSON answer about the domain | a `JsonResource`, in `app/Http/Resources` | a controller, with `ctx.JSON` | `json.NewEncoder` on the ResponseWriter |
| an API error | a problem document, written by the router | the router | a format of its own |
| a rejected form | the `validation.Errors` the service returns; the router answers it | the router | a 422 written by a controller |
| a page | a view with `@extends('layouts.app')` and a struct that embeds `view.Page` | a controller, with `ctx.View` | take a map as its data |
| an htmx fragment | `resources/views/partials/`, included by its page with `@include` | a controller, with `ctx.Fragment`, for the request whose `HX-Target` names it | depend on `HX-Boosted` |
| a tool, resource or prompt for an assistant | `app/Mcp/` | the MCP server mounted in `routes/web.go` | reach a model or a client |
| a helper | the package functions of `github.com/arandu-io/hesape` | anything | be written again here |
| CPF, CNPJ, BRL | the `github.com/hyz-is/arandu-br` module | a request, a view | be written again here |

Each symbol is imported from one path: `aru imports:catalog` prints it for the
framework version in `go.mod`, and `aru doctor` reports the other spelling as
`import-not-canonical`. `Grant`, `Subject`, `Authorize` and `Tenant` come from
`github.com/arandu-io/hesape/auth`; `Router` and the resource interfaces from
`github.com/arandu-io/framework/http`; the `Context` an action takes from
`github.com/arandu-io/hesape/http`.

## The gates

Nothing is finished until each of these eight commands exits zero and `gofmt`
prints nothing. `task check` runs the same eight, in this order.

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

`aru model:build --check` fails when a generated query or factory is behind its
entity; `aru model:build` without the flag rewrites them. `aru view:build`
writes the compiled views the compiler then reads, so it runs before anything
that compiles. `aru dev` and `aru build` run both for you.

The `aru` in these commands is the release the Dockerfile pins, v0.66.0. An
older one reports fewer rules than the ones this project is held to, so a clean
`aru doctor` from it says less than it seems to.
`go run github.com/arandu-io/aru@v0.66.0 <command>` runs that release without
installing it.

Both filters on `gofmt` are load-bearing. A `.kyse.go` is excluded from the
compiler by a build tag, and `gofmt` is the only tool in the chain that ignores
build tags — without the filter it reports a syntax error on every view.
`testdata/` holds fixtures that are invalid on purpose.

CI runs these too, except `aru model:build --check`, whose question
`aru doctor` also asks as `model-query-stale`. It then builds the container
image and runs `govulncheck`, which need Docker and the network.

## What does not exist here

Reaching for one of these is the most common way to waste an afternoon. None of
them is missing by accident; each was considered and refused.

| A model reaches for | What is here instead |
| --- | --- |
| a service container, dependency injection | `bootstrap/app.go`, a list. Reading it tells you what every route was given |
| facades, static proxies | the collaborator, passed to the constructor |
| an ORM model with fillable fields, Active Record | an entity struct that embeds `model.Model`, with its table declared once beside it; every terminal takes an `auth.Grant`, and a typed request decides which fields a write may touch |
| service providers, auto-discovery | `foundation.Module`: `Name()` and `Routes()`, registered explicitly |
| a routes file loaded by convention, an `api.go` beside `web.go` | `routes/web.go`, called; a JSON client is answered by the same routes |
| middleware that authorizes | a Policy that `auth.Authorize` consults before it issues a `Grant`. Middleware answers "is there a session", puts the subject on the request, and stops |
| an ORM query builder on the model | `models.Notes(db)`, the typed query `aru model:build` generates beside the entity, finished by a terminal that takes the `Grant`; a repository with parameterised SQL only for a query the builder cannot say |
| a base controller with `validate()` and an "invalid" response | nothing: the action returns the `validation.Errors` the service returned, and the router answers a page with a redirect back to the form and a JSON client with a 422 problem document |
| a template engine with runtime lookup | `.kyse.go`, compiled to Go. A missing field is a build error |
| npm, a bundler, `node_modules` | nothing. There is no Node in this tree and there is no step that wants one |

## The two rules everything else follows from

**Authorization is a value.** `auth.Grant` has only unexported fields. Every
read and write of the model takes one. A handler that reaches the database
without asking a Policy has nothing to pass, so it does not compile.

**The tenant comes from the Grant.** `auth.Tenant(g)`, never from a path
segment, a body, a query or a header.

## Writing code

Prefer generating it. `aru make:module` writes a model, a migration, a policy, a
request, a service, a controller, views and tests, all shaped correctly, and
prints the three lines of wiring to paste. The `notes` resource is the example
to read first. What
the generator does not cover goes between `// arandu:begin custom` and
`// arandu:end custom`, which survives regeneration.

A model is two declarations in `app/Models`: a struct that embeds `model.Model`,
and `var noteTable = model.NewTable(model.TableSpec{...})` beside it. `aru
model:build` reads them and writes `NoteQuery.go` — the constructor
`models.Notes(db)`, the typed `NoteQuery` and `NoteCollection` — and re-renders
the factory in `database/factories`. Run it after changing an entity and never
edit what it writes. Every read and write starts at the constructor:
`models.Notes(db).Where("pinned", true).Get(ctx, g)`. A local scope is a method
on `*NoteQuery` in the custom block of `Note.go`; a relation is registered with
`noteTable.Relate` in an `init` function. `.agents/skills/arandu-module` has
the details.

Comments, identifiers, error messages, log lines, CLI output and test names are
in English.
