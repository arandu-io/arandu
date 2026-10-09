---
name: arandu-feature
description: Start here for any change to an Arandu (Go) application that adds or changes behaviour -- "add invoices", "let users publish a post", "send a weekly report", "call the payment provider", "expose this to an assistant", "add an API endpoint". It classifies the feature, walks the anatomy and the "where each kind of code lives" table in AGENTS.md, picks the aru generators, says what to wire by hand and what to test, and names the family skill for each step. Use it before writing any file.
license: MIT
---

# Building a feature

## When to use

Any request that adds or changes what the application does. It is the entry
to the other skills: it decides the shape of the feature, and each step names
the family skill that holds that step's rules. A one-line fix inside code that
already exists goes straight to the family skill of that code.

## Before you start

1. Read `AGENTS.md` whole: the anatomy table, the "Where each kind of code
   lives" table and "What does not exist here".
2. Read the example the steps point at. The `notes` resource is the whole
   anatomy in this repository: a model with a transition, a nested resource, a
   named action, a JSON Resource, an event with a listener, a notification, a
   scheduled job and an external client. `.agents/skills/notes` maps it.
3. Answer the ownership questions in `arandu-ecosystem` before any table: a
   module of the ecosystem may already own the capability.

## Contracts and imports

Each symbol has one import path, and `aru imports:catalog` prints it for the
framework version in `go.mod`:

| what | import |
| --- | --- |
| `Grant`, `Subject`, `Authorize`, `Tenant`, `SystemGrant` | `github.com/arandu-io/hesape/auth` |
| `Context`, the argument of every action | `github.com/arandu-io/hesape/http` |
| `Router`, `Indexer` and the other resource interfaces, `Invoker` | `github.com/arandu-io/framework/http` |
| the route guards, `RequireToken`, `Idempotent` | `github.com/arandu-io/framework/http/middleware` |
| `validation.Errors`, `Validatable` | `github.com/arandu-io/hesape/validation` |
| `database.DB`, `database.Transaction` | `github.com/arandu-io/hesape/database` |

## Procedure

**1. Classify the feature.** Write down which of these it is; most are two or
three at once.

| the feature | its first step | family skill |
| --- | --- | --- |
| a new kind of record, with a table | `aru make:module` | `arandu-module` |
| a record that lives under another | `aru make:module --parent=<resource>` | `arandu-module`, `arandu-http` |
| a verb beyond CRUD on a record: publish, approve, cancel | a rule in the entity, `aru make:controller --action` | `arandu-module`, `arandu-http` |
| a screen or a part of one | a view, and maybe a partial | `arandu-view` |
| an answer to a program rather than a person | `aru make:resource` | `arandu-api` |
| work after the response, on a schedule or in reaction to a fact | `aru make:job`, `aru make:event`, `aru make:listener`, `aru make:notification` | `arandu-async` |
| a call to another system, a webhook, an assistant | `aru make:client`, `aru make:mcp-tool` | `arandu-integrations` |
| who may do it | the policy | `arandu-policy` |
| roles, balances, tags, Markdown, API docs | a module of the ecosystem | `arandu-ecosystem` |

**2. Walk the anatomy in order**, skipping the steps the feature does not have:
specification, migration, model and its rules, factory and seeder, policy,
request, service, controller, routes, view or resource, tests and wiring. The
order is the order of dependence: each step compiles against the one before.

**3. Put each piece where the table says.** A rule of the entity in the
model's custom block, the order of a use case in the service, input conversion
in the controller, the answer in a view or a JSON Resource. A piece that seems
to belong in two rows belongs in the one that does not need the other's
collaborators.

**4. Generate, then finish by hand.** Every generator prints its wiring and
edits neither `bootstrap/app.go` nor `routes/web.go`. Paste what it printed,
read it, and write what it could not know in the custom blocks.

**5. Test the path.** A feature test that drives the feature the way its
caller does, the tenant isolation of every new table, and the answers the
family skill lists under "Acceptance test".

**6. Report what is missing.** A capability the framework or the generators do
not have is a gap to name in the report, with the file that would have used
it -- never a second way written in the application to cover it.

## Commands

| step | command |
| --- | --- |
| specification | `aru schema`, `aru generate <spec.yaml> --check`, `aru generate <spec.yaml>` |
| a whole resource | `aru make:module <name> --fields "..." --tenant [--parent=<resource>]` |
| one artifact | `aru make:model`, `aru make:migration`, `aru make:policy`, `aru make:request`, `aru make:service`, `aru make:controller`, `aru make:resource`, `aru make:job`, `aru make:event`, `aru make:listener`, `aru make:notification`, `aru make:client`, `aru make:mcp-tool`, `aru make:command`, `aru make:enum`, `aru make:mail`, `aru make:middleware`, `aru make:factory`, `aru make:seeder`, `aru make:test` |
| the generated query | `aru model:build` |
| the views | `aru view:build` |
| what is wired | `aru route:list`, `aru schedule:list`, `aru about` |
| the architecture check | `aru doctor` |

`aru <command> --help` prints its usage line with every flag it takes.

## Example

Every step of a feature leaves an artifact the next step relies on, and the
compiler holds each of them to its contract. This file is the checklist of the
example's steps, written as the assertions the compiler checks:

```go compile
package feature

import (
	fhttp "github.com/arandu-io/framework/http"
	"github.com/arandu-io/hesape/auth"
	hevents "github.com/arandu-io/hesape/events"
	hhttp "github.com/arandu-io/hesape/http"
	"github.com/arandu-io/hesape/queue"
	"github.com/arandu-io/hesape/validation"

	clients "<module>/app/Clients"
	controllers "<module>/app/Http/Controllers"
	requests "<module>/app/Http/Requests"
	resources "<module>/app/Http/Resources"
	appjobs "<module>/app/Jobs"
	listeners "<module>/app/Listeners"
	models "<module>/app/Models"
	policies "<module>/app/Policies"
)

// The notes feature, step by step.
var (
	_ auth.Policy[models.Note] = policies.NotePolicy{}              // the policy
	_ validation.Validatable   = requests.NoteRequest{}             // the request
	_ fhttp.Indexer            = (*controllers.NoteController)(nil) // the controller
	_ fhttp.Indexer            = (*controllers.CommentController)(nil)
	_ hhttp.JsonResource       = resources.NoteResource{}               // the answer to a program
	_ hevents.Publisher        = (*listeners.NotifyNoteAuthor)(nil)     // the reaction to note.published
	_ queue.Handler            = (*appjobs.SendNotesDigestHandler)(nil) // the nightly job
	_ clients.Newsletter       = (*clients.NewsletterClient)(nil)       // the external system
	_ clients.Newsletter       = (*clients.NewsletterFake)(nil)         // and its fake
	_                          = (*models.Note).Publish                 // the transition is the entity's
)
```

## Do not

- Start from the controller. A controller written first grows the rules the
  entity and the service should hold, because it is the only file there is.
- Reach for what another framework would use: a service container, a facade, a
  routes file per audience, a base controller with `validate()`. `AGENTS.md`
  says what is here instead.
- Write a second way to do what exists. A local role table, a balance column,
  an HTTP call from a service, a JSON encoder in a controller -- each has an
  owner in the table.

## Extending it

What a generator does not cover is written between `// arandu:begin custom`
and `// arandu:end custom` in the file it wrote, and survives the next run with
`--force`. A file finished by hand outside those markers -- as the example's
controller is -- is changed by hand from then on, and its skill says so.

## Wiring

All of it is by hand, in two files, from what the generators print:

- `bootstrap/app.go` builds every collaborator once, in `Build`, and hands it
  down: services to controllers, the notifier to listeners, the clients to
  services, the queue to the provider that schedules.
- `routes/web.go` holds the field on `Deps` and the route lines, in the custom
  block, behind the guard the route needs.
- `bootstrap/background.go` registers job handlers; `app/Providers` declares
  scheduled tasks; `routes/console.go` declares commands.

## Acceptance test

The feature is done when:

1. a feature test drives it as its caller does -- a browser through
   `tests.SignedIn`, a JSON client with `Accept: application/json`, the
   scheduler through `RunNow`, the relay through `Drain`;
2. every new table with `tenant_id` is claimed in
   `tests/Feature/TenantScope_test.go` and has a cross-tenant test that reads
   and writes;
3. the gates below pass, with `aru doctor` reporting nothing new.

## Limits

This skill orders the work; it does not hold the rules of any step. Where a
family skill and this one seem to disagree, the family skill is right about its
step. Where a generator, a family skill and the code disagree, the code is
right and the other two are wrong -- report it.

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
