# Working in this repository

This is an Arandu application. Arandu is a Go framework where the architecture
is enforced by the compiler rather than by convention, so the fastest way to be
wrong here is to write what another framework would want.

Read `.agents/skills/` before writing code. Each skill is a procedure, and the
one you need is named by the situation you are in.

## The four gates

Nothing is finished until all four exit zero.

```sh
export GOWORK=off
aru model:build
aru view:build
gofmt -l $(find . -name '*.go' -not -path '*/testdata/*' -not -name '*.kyse.go')
go build ./...
go vet ./...
go test -race ./...
aru doctor
```

Both filters on `gofmt` are load-bearing. A `.kyse.go` is excluded from the
compiler by a build tag, and `gofmt` is the only tool in the chain that ignores
build tags — without the filter it reports a syntax error on every view.
`testdata/` holds fixtures that are invalid on purpose.

`aru model:build` and `aru view:build` write what the compiler then reads — the
query beside each entity and the compiled views — so they run first. `aru dev`
and `aru build` run both for you.

## What does not exist here

Reaching for one of these is the most common way to waste an afternoon. None of
them is missing by accident; each was considered and refused.

| A model reaches for | What is here instead |
| --- | --- |
| a service container, dependency injection | `bootstrap/app.go`, a list. Reading it tells you what every route was given |
| facades, static proxies | the collaborator, passed to the constructor |
| an ORM model with fillable fields, Active Record | an entity struct that embeds `model.Model`, with its table declared once beside it; every terminal takes a `security.Grant`, and a typed request decides which fields a write may touch |
| service providers, auto-discovery | `foundation.Module`: `Name()` and `Routes()`, registered explicitly |
| a routes file loaded by convention | `routes/web.go`, called |
| middleware that authorizes | a Policy that `security.Authorize` consults before it issues a `Grant`. Middleware answers "is there a session", puts the subject on the request, and stops |
| an ORM query builder on the model | `models.Notes(db)`, the typed query `aru model:build` generates beside the entity, finished by a terminal that takes the `Grant`; a repository with parameterised SQL only for a query the builder cannot say |
| a template engine with runtime lookup | `.kyse.go`, compiled to Go. A missing field is a build error |
| npm, a bundler, `node_modules` | nothing. There is no Node in this tree and there is no step that wants one |

## The two rules everything else follows from

**Authorization is a value.** `security.Grant` has only unexported fields.
Every read and write of the model takes one. A handler that reaches the
database without asking a Policy has nothing to pass, so it does not compile.

**The tenant comes from the Grant.** `data.Tenant(g)`, never from a path
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
