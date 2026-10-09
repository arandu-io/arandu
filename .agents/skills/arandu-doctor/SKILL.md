---
name: arandu-doctor
description: Read and act on aru doctor, the architecture checker of an Arandu (Go) application. Use when a doctor finding is reported, when a build passes but something feels unenforced, or before declaring any Arandu change finished — it is one of the gates. Also use when the request mentions "lint", "static analysis", "architecture check", "why is this failing", or a rule name such as grant-not-received, tenant-from-request, view-data-is-a-map or raw-output-is-not-a-component. Covers what each finding means, why it is never suppressed, and what the checker cannot see.
license: MIT
---

# Reading what the doctor says

`aru doctor` is this framework's architecture rules run as static analysis over
the parsed tree. Without it, mandatory architecture is documentation nobody
reads.

Every finding carries a file, a line, the rule name, what is wrong, and a **Why**
that says what breaks. The Why is the field that matters: a finding that only
says what is forbidden gets suppressed, and one that says what breaks gets
fixed.

```sh
aru doctor            # every finding
aru doctor --strict   # warnings fail too
```

## The procedure

**1. Read the Why, not the rule name.** The rule name tells you which check
fired. The Why tells you what a user of the application would experience. Fix
the second one.

**2. Fix the cause at the line it names.** Every finding points at real code.

**3. Never suppress.** There is no ignore comment and no allow-list, deliberately.
A finding you cannot fix is a design question, not a lint to silence. The one
marker the doctor reads is `//arandu:system-grant <reason>`, on or above a
`SystemGrant` call outside a seeder, a job or a command: it excuses
`system-grant-outside-scope` on that line and nothing else, and a marker with
no reason excuses nothing.

**4. Run it again until it is clean, then run the other gates.**

## The findings you will actually meet

**`grant-not-received`** — a repository method takes no `security.Grant`. Every
caller gets the row, whoever asked. Add the Grant as the parameter before the id,
start the method with `if err := g.Check(Action...); err != nil { return err }`
— a Grant that is taken and never checked is `grant-not-checked` — and take it
from the Policy.

**`tenant-from-request`** — the tenant is read from a path segment, a form
field or a query; `tenant-from-header` is the same finding for a header. A
tenant that arrives with the request is a tenant the caller chose. Read it with
`data.Tenant(g)`.

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

## Every rule

The whole set the doctor checks, with the severity it reports. An error fails
the run; a warning fails it only under `--strict`.

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
| `resource-not-reauthorized` | warning | a method that reads a row and does not authorize the row it read |
| `handler-reaches-data` | error | a controller, a middleware or a route file that uses the data package beyond `data.Query` |
| `handler-reaches-the-model` | error | a controller, a middleware or a route file that opens a query on a model |
| `controller-reaches-repository` | error | a controller, a middleware or a route file that imports `app/Repositories` |
| `tenant-from-request` | error | a tenant read from the request — a path value, a form field, a query parameter — or a Grant whose tenant came from one |
| `tenant-from-header` | error | a tenant read from a header |
| `system-grant-without-tenant` | error | a `SystemGrant` with an empty tenant |
| `system-grant-outside-scope` | warning | a `SystemGrant` outside a seeder, a job and a command |
| `sql-built-with-sprintf` | error | SQL assembled with `fmt.Sprintf` |
| `sql-built-by-concatenation` | error | SQL assembled by concatenating a value |
| `sql-without-tenant-scope` | error | a statement on a table other statements scope by `tenant_id`, without that filter |
| `sensitive-field-not-redacted` | warning | a type holding a secret that does not redact itself |
| `session-not-rotated` | error | a sign-in that authenticates and keeps the session id it arrived with |
| `view-data-is-a-map` | error | a view handed a map |
| `view-does-not-exist` | error | a view name that names no `.kyse.go` |
| `view-keeps-state-in-the-browser` | error | an `x-` or `@` attribute with a value in a view |
| `raw-output-is-not-a-component` | warning | `{!! x !!}` given a value rather than a component call |
| `permission-not-declared` | error | code that uses a permission `arandu.mod.toml` declares false |
| `permission-not-used` | warning | a permission `arandu.mod.toml` declares true and nothing uses |
| `outbox-not-registered` | error | code that stores domain events in a project that registers no outbox table |
| `migrations-not-linked` | warning | migrations in a package nothing imports, so `aru migrate` never sees them |
| `added-column-not-nullable` | warning | a column added to an existing table without `Nullable()` or a default |
| `rollback-does-nothing` | warning | a migration that declares neither a `Down` nor that it is irreversible |
| `driver-not-linked` | warning | an engine `.env.example` names whose connector the project does not import |
| `retired-module` | warning | an import of a module whose repository was deleted |
| `model-query-stale` | error | a generated query or factory missing, behind its entity, or left from one that is gone |
| `model-core-outside-models` | error | the model core used outside a package that declares entities |
| `test-is-not-run` | warning | a file named `...Test.go`, or a test function in a file whose name does not end in `_test.go` |
| `test-outside-the-tests-tree` | warning | a test outside `tests/` whose name does not end in `_internal_test.go` |
| `package-clause-is-capitalised` | warning | a package clause with a capital letter |
| `scaffolding-ships` | warning | a file outside the tests that imports test scaffolding |
| `profile-not-declared` | warning | `--profile=performance` on a project whose `arandu.mod.toml` does not list it |
| `join-across-aggregates` | error | `--profile=performance`: a statement that reads two tables |
| `transaction-across-aggregates` | error | `--profile=performance`: a transaction that writes two aggregates |

## What it cannot see, and why that matters

It reads the parsed tree, not the running program.

- SQL built from a variable, or held in a package constant, is not inspected. A
  clean report means no unscoped statement was **found**, not that none exists.
- Partition keys are not checked, because nothing in the code declares one.
- A build tag is invisible to it, so a file excluded from the compiler is still
  read.

Trust it as evidence, never as proof.
