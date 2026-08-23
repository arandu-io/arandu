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

## What does not exist here

Reaching for one of these is the most common way to waste an afternoon. None of
them is missing by accident; each was considered and refused.

| A model reaches for | What is here instead |
| --- | --- |
| a service container, dependency injection | `bootstrap/app.go`, a list. Reading it tells you what every route was given |
| facades, static proxies | the collaborator, passed to the constructor |
| an ORM model with fillable fields, Active Record | an entity struct and a repository that takes a `security.Grant` |
| service providers, auto-discovery | `foundation.Module`: `Name()` and `Routes()`, registered explicitly |
| a routes file loaded by convention | `routes/web.go`, called |
| middleware that authorizes | a Policy that issues a `Grant`. Middleware answers "is there a session" and stops |
| an ORM query builder on the model | SQL in the repository, parameterised |
| a template engine with runtime lookup | `.kyse.go`, compiled to Go. A missing field is a build error |
| npm, a bundler, `node_modules` | nothing. There is no Node in this tree and there is no step that wants one |

## The two rules everything else follows from

**Authorization is a value.** `security.Grant` has only unexported fields.
Every repository method takes one before the id. A handler that reaches the
database without asking a Policy has nothing to pass, so it does not compile.

**The tenant comes from the Grant.** `data.Tenant(g)`, never from a path
segment, a body, a query or a header.

## Writing code

Prefer generating it. `aru make:module` writes an entity, a policy, a
repository, a service, a request, routes and tests, all shaped correctly. What
the generator does not cover goes between `// arandu:begin custom` and
`// arandu:end custom`, which survives regeneration.

Comments, identifiers, error messages, log lines, CLI output and test names are
in English.
