---
name: arandu-doctor
description: Read and act on aru doctor, the architecture checker of an Arandu (Go) application. Use when a doctor finding is reported, when a build passes but something feels unenforced, or before declaring any Arandu change finished — it is one of the gates. Also use when the request mentions "lint", "static analysis", "architecture check", "why is this failing", or a rule name such as grant-not-received, tenant-from-request, view-data-is-a-map, raw-output-is-not-a-component, service-takes-http or input-read-by-hand. Covers what each finding means, why it is never suppressed, and what the checker cannot see.
license: MIT
---

# Reading what the doctor says

## When to use

`aru doctor` reported something, a build passes and something still feels
unenforced, or a change is about to be called finished -- the doctor is the
last gate. Fixing what a finding points at is the family skill of that code;
this skill says what the finding means.

## Before you start

`aru doctor` is this framework's architecture rules run as static analysis over
the parsed tree. Without it, mandatory architecture is documentation nobody
reads. Run the `aru` the Dockerfile pins -- an older one checks fewer rules, and
its clean report says less than it seems to.

## Contracts and imports

Every finding carries a file, a line, the rule name, what is wrong, and a
**Why** that says what breaks. An error fails the run; a warning fails it only
under `--strict`. There is no ignore comment and no allow-list. The one marker
the doctor reads is `//arandu:system-grant <reason>`, on or above an
`auth.SystemGrant` call outside a seeder, a job and a command: it excuses
`system-grant-outside-scope` on that line and nothing else, and a marker with
no reason excuses nothing.

## Every rule

The whole set the doctor checks, with the severity it reports, in the order
`aru doctor --list` prints them. An error fails the run; a warning fails it
only under `--strict`. `tests/Unit/DoctorSkill_test.go` fails when a row here
differs from the list it carries, and, when the `aru` on PATH is the release the
Dockerfile pins, when that list differs from what `aru doctor --list` prints.

| rule | severity | what it reports |
| --- | --- | --- |
| `file-does-not-parse` | error | a Go file the doctor cannot read, which leaves the rest of the report incomplete |
| `repository-without-policy` | error | an entity with a repository and no policy |
| `grant-not-received` | error | an exported repository method that reaches the database and takes no Grant |
| `grant-not-checked` | error | a method that takes a Grant and never calls `Check` on it |
| `grant-check-discarded` | error | a `Check` whose answer is thrown away, as in `_ = g.Check(...)` |
| `policy-never-opened` | warning | a policy that denies every action |
| `action-not-a-constant` | error | an action built from a value rather than named as a constant |
| `enum-rule-not-derived` | error or warning | an `enum` rule that lists cases its type does not declare (error), or repeats the ones it does (warning) |
| `handler-reaches-data` | error | a controller, a middleware or a route file that uses the data package beyond `data.Query` |
| `handler-reaches-the-model` | error | a controller, a middleware or a route file that opens a query on a model |
| `controller-reaches-repository` | error | a controller, a middleware or a route file that imports `app/Repositories` |
| `tenant-from-request` | error | a tenant read from the request — a path value, a form field, a query parameter — or a Grant whose tenant came from one |
| `tenant-from-header` | error | a tenant read from a header |
| `system-grant-without-tenant` | error | a `SystemGrant` with an empty tenant |
| `system-grant-outside-scope` | warning | a `SystemGrant` outside a seeder, a job and a command |
| `sql-built-with-sprintf` | error | SQL assembled with `fmt.Sprintf` |
| `sql-built-by-concatenation` | error | SQL assembled by concatenating a value |
| `sensitive-field-not-redacted` | warning | a type holding a secret that does not redact itself |
| `session-not-rotated` | error | a sign-in that authenticates and keeps the session id it arrived with |
| `view-data-is-a-map` | error | a view handed a map |
| `view-does-not-exist` | error | a view name that names no `.kyse.go` |
| `permission-not-declared` | error | code that uses a permission `arandu.mod.toml` declares false |
| `permission-not-used` | warning | a permission `arandu.mod.toml` declares true and nothing uses |
| `view-keeps-state-in-the-browser` | error | an `x-` or `@` attribute with a value in a view |
| `sql-without-tenant-scope` | error | a statement on a table other statements scope by `tenant_id`, without that filter |
| `outbox-not-registered` | error | code that stores domain events in a project that registers no outbox table |
| `resource-not-reauthorized` | warning | a method that reads a row and does not authorize the row it read |
| `raw-output-is-not-a-component` | warning | `{!! x !!}` given a value rather than a component call |
| `retired-module` | warning | an import of a module whose repository was deleted |
| `import-not-canonical` | warning | a framework symbol named through a bridge package rather than through the path `aru imports:catalog` gives it |
| `test-is-not-run` | warning | a file named `...Test.go`, or a test function in a file whose name does not end in `_test.go` |
| `test-outside-the-tests-tree` | warning | a test outside `tests/` whose name does not end in `_internal_test.go` |
| `package-clause-is-capitalised` | warning | a package clause with a capital letter |
| `scaffolding-ships` | warning | a file outside the tests that imports test scaffolding |
| `skills-out-of-date` | warning | a skill under `.agents/skills` copied from the skeleton or a `hyz-is` module that is behind what that origin hands out at the version the project pins |
| `skills-missing` | warning | a skill the skeleton or a required `hyz-is` module hands out that this project does not have |
| `migrations-not-linked` | warning | migrations in a package nothing imports, so `aru migrate` never sees them |
| `added-column-not-nullable` | warning | a column added to an existing table without `Nullable()` or a default |
| `rollback-does-nothing` | warning | a migration that declares neither a `Down` nor that it is irreversible |
| `driver-not-linked` | warning | an engine `.env.example` names whose connector the project does not import |
| `profile-not-declared` | warning | `--profile=performance`: a project whose `arandu.mod.toml` does not list that profile |
| `join-across-aggregates` | error | `--profile=performance`: a statement that reads two tables |
| `transaction-across-aggregates` | error | `--profile=performance`: a transaction that writes two aggregates |
| `model-query-stale` | error | a generated query or factory missing, behind its entity, or left from one that is gone |
| `model-core-outside-models` | error | the model core used outside a package that declares entities |
| `input-read-by-hand` | warning | a controller that reads the form, the query or the body field by field instead of `ctx.Bind` into the request |
| `validate-called-by-controller` | warning | a controller that calls `Validate()` on the request, or `validation.Validate`, which is the service's call |
| `json-written-by-hand` | warning | a controller that encodes JSON onto the response writer, or writes its own status with it, instead of a resource and `ctx.JSON` |
| `invalid-form-answered-by-hand` | warning | a controller that answers a rejected form with a 422 of its own instead of returning the `validation.Errors` |
| `session-loaded-in-controller` | warning | a controller outside `app/Http/Controllers/Auth` that loads the session instead of reading `ctx.User()` |
| `redirect-to-literal-path` | warning | a redirect in a controller to a path written as a literal rather than a named route |
| `html-template-in-app` | warning | a file under `app/` that imports `html/template` |
| `service-takes-http` | warning | a file under `app/Services` that names the request, the response writer, the HTTP context, the session, a cookie or `template.HTML` |
| `service-subpackage` | warning | a directory under `app/Services` that holds Go |
| `service-file-too-large` | warning | a file under `app/Services` longer than 600 lines |
| `controller-too-many-actions` | warning | a controller type with more than 12 handler methods |
| `operation-chosen-by-form-field` | warning | a handler that switches on a submitted field to call one service method or another |
| `client-outside-clients` | warning | a request to another system — an `http.Client`, `http.Get`, `http.NewRequest`, the hesape HTTP client — made under `app/` outside `app/Clients` |
| `model-rule-touches-io` | warning | a method in the custom block of a model that reaches the database, the network or the clock |
| `fragment-without-partial` | warning | `ctx.Fragment` given a view outside `resources/views/partials` |
| `helper-reimplemented` | warning | a function under `app/` that rewrites a helper the catalog already has, such as a slug, a CPF or a BRL formatter |
| `raw-sql-outside-repository` | warning | a statement run outside `app/Repositories` and `database/` |
| `generated-not-wired` | warning | a `New...` constructor of a controller or a service that nothing else names |
| `subject-built-by-hand` | warning | a `Subject` literal outside the tests and `database/` that writes its own roles or actions |

## Procedure

1. **Read the Why, not the rule name.** The rule name says which check fired;
   the Why says what a user of the application would experience. Fix the
   second one.
2. **Fix the cause at the line it names**, by moving the code to the row of
   "Where each kind of code lives" in `AGENTS.md` that names its kind -- not by
   reshaping it until the rule stops matching.
3. **Never suppress.** A finding you cannot fix is a design question, and the
   answer goes in the report.
4. **Run it again until it is clean**, then the other gates.

### The findings you will meet most

**`grant-not-received`** — a repository method takes no `auth.Grant`. Every
caller gets the row, whoever asked. Add the Grant as the parameter before the id,
start the method with `if err := g.Check(Action...); err != nil { return err }`
— a Grant that is taken and never checked is `grant-not-checked` — and take it
from the Policy.

**`tenant-from-request`** — the tenant is read from a path segment, a form
field or a query; `tenant-from-header` is the same finding for a header. A
tenant that arrives with the request is a tenant the caller chose. Read it with
`auth.Tenant(g)`.

**`repository-without-policy`** — a repository is reachable with no Policy
deciding. Write the Policy; the generator writes one that denies everything, and
you open it action by action.

**`resource-not-reauthorized`** — a method authorized the action and then read
one row without authorizing the row. The first call answers "may this caller
look at all"; the second answers "may this caller look at *this*". Skipping the
second means any user of the same tenant sees the row.

**`view-data-is-a-map`** — a view was handed a map. A typo in a key is then a
blank space on a page that answered 200. Declare a struct that embeds
`view.Page`.

**`raw-output-is-not-a-component`** — `{!! x !!}` was given a value rather than a
call. The raw form escapes nothing, so a value that ever comes from a person is
stored cross-site scripting. Write `{{ x }}`, which escapes, or return it from a
component function.

**`policy-never-opened`** — a policy denies every action. On a new module this is
correct and expected; it is a warning so that a fresh project is not red on day
zero.

**`retired-module`** — an import names a module that no longer exists. The line
says what replaced it.

**`import-not-canonical`** — a file names a symbol through a framework bridge
package rather than through the path the symbol lives at: `security.Grant` for
`auth.Grant`, `data.DB` for `database.DB`, `fhttp.Context` for
`hhttp.Context`. The bridge only aliases or forwards, so the code is correct
today, and it stops compiling when the bridges are removed in v1.0.0. The line
names each symbol and its path; `aru imports:catalog` prints the whole table for
the framework version in `go.mod`. Import that path, and keep the framework
import only for what the framework declares itself, such as `Router` and
`SessionStore`. It is a warning because nothing is wrong yet.

**`model-query-stale`** — a `<Entity>Query.go`, or a factory `aru model:build`
renders, is missing, behind its entity, or left over from one that is gone. A
build compiles what is on disk, so the application would run against the query
of an entity that is not the one in the source. Run `aru model:build`; in a
pipeline that calls `go build` directly, `aru model:build --check` asks the same
question.

**`model-core-outside-models`** — the model core is used outside a package that
declares entities, which here is `app/Models`: a `model.NewTable`, or a method
called on a `*model.Table`, a `*model.Builder` or what `Base()` returns. The
core hands back untyped rows, and a query written on it is a second way to reach
the table, one the generated query does not describe. Call the generated
constructor, `models.Notes(db)`, or write the query as a method on `*NoteQuery`
in the custom block of the entity's file.

Three more checks run only under `--profile=performance`:
`profile-not-declared`, `join-across-aggregates` and
`transaction-across-aggregates`. What they report is correct code on the
conventional profile, and each says so in its own first lines. The ones above
run on every profile.

### The structural warnings

Nineteen rules, from `input-read-by-hand` to `subject-built-by-hand` at the end
of the table above, report code that works and lives in the wrong place: a
controller that reads the form field by field, validates, writes JSON or a 422
by hand, loads the session or redirects to a literal path; a service that takes
an HTTP type, sits in a subpackage, or grows past 600 lines; a controller past
12 actions or one that picks the operation from a form field; markup outside a
view, a client outside `app/Clients`, a model rule that reaches the database,
the network or the clock, a fragment that is not a partial, a helper written
again, SQL outside a repository, a constructor nothing wires, and a `Subject`
that writes its own roles. All are warnings.

The correction for each is the row of "Where each kind of code lives" in
`AGENTS.md` that names the kind of code the finding is about: move the code
there, do not reshape it until the rule stops matching. A count rule
(`service-file-too-large`, `controller-too-many-actions`) says where to look,
not that the file is wrong. Each rule reads one function, file or call by name,
so a clean report means none of these shapes was found, not that none exists.

The sign-in screens `go run github.com/arandu-io/ui@v0.20.0 auth` publishes
still report `input-read-by-hand` and `html-template-in-app` in
`app/Http/Controllers/Auth`. Those are the kit's to fix, and republishing a kit
release that fixes them brings the fix. `session-loaded-in-controller` does not
read that directory at all, because signing in is where a session is first
loaded.

## Commands

- `aru doctor`, every finding; `aru doctor --strict`, warnings fail too
- `aru doctor --list`, every rule with its severity
- `aru doctor --profile=performance`, three more checks for the performance profile
- `aru imports:catalog`, the import path of each framework symbol
- `aru model:build`, the fix for `model-query-stale`

## Example

The one marker the doctor reads, on a call that has a reason to be outside a
seeder, a job and a command:

```go compile
package example

import (
	"context"

	"github.com/arandu-io/hesape/auth"
	"github.com/arandu-io/hesape/database"

	models "<module>/app/Models"
	policies "<module>/app/Policies"
)

// PublishedCount answers how many notes a tenant has published, for an
// operator's report that runs with no subject behind it.
func PublishedCount(ctx context.Context, db *database.DB, tenant string) (int, error) {
	//arandu:system-grant the operator's report runs with no subject; the tenant is the one the operator named
	g := auth.SystemGrant(policies.NoteList, tenant)
	published, err := models.Notes(db).WhereNotNull("published_at").Get(ctx, g)
	return len(published), err
}
```

## Do not

- Reshape code until a rule stops matching. A finding is about where the code
  lives; code moved to the right place stops matching because it is right.
- Treat a warning as noise because it is not an error. Structural rules start
  as warnings so a new project is not red on day zero, not because they are
  optional.
- Write a marker without a reason, or put one on a line it does not excuse.
- Run an older `aru` and call the report clean.

## Extending it

A project adds no rules and suppresses none. A rule that is wrong about correct
code is a bug of the doctor, reported with the file and the line; a rule that
should exist is a proposal to the framework.

## Wiring

None. The doctor reads the tree as it is; CI runs it on every push, without
`--strict`.

## Acceptance test

`aru doctor` prints no finding on the change. `tests/Unit/DoctorSkill_test.go`
holds the table above to the rule list, and to what `aru doctor --list` prints
when the `aru` on PATH is the pinned release.

## Limits

It reads the parsed tree, not the running program.

- SQL built from a variable, or held in a package constant, is not inspected. A
  clean report means no unscoped statement was **found**, not that none exists.
- Partition keys are not checked, because nothing in the code declares one.
- A build tag is invisible to it, so a file excluded from the compiler is still
  read.
- Each structural rule reads one function, file or call by name: a clean report
  means none of those shapes was found, not that none exists.

Trust it as evidence, never as proof.

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
