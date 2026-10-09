---
name: arandu-mcp
description: Connect a coding assistant to `aru mcp`, the MCP server the aru CLI runs for the project in the current directory, and ask it instead of guessing -- where a kind of code lives, the order a feature is built in, what aru doctor reports, which command does what, and a generator previewed before it writes. Use at the start of any session in an Arandu (Go) application whose client can start an MCP server, when the question is "where does this go", "what is the recipe for a webhook/job/nested resource", "run the doctor", "which aru command", or before running an aru make:* generator. This is the developer's server; an MCP server the application itself exposes is arandu-integrations.
license: MIT
metadata:
  audience: app
---

# Asking aru instead of guessing

## When to use

At the start of a session in this project, when the assistant's client can
start an MCP server over stdio, and again whenever a question has an answer
`aru` already holds: where a kind of code lives and what it may import, the
order a feature is built in, what the doctor reports, which command and flag
exist, what a generator would write. The answers come from the `aru` the
project runs, so they do not age the way a copied document does.

It is not the MCP server an application offers its own users' assistants --
tools in `app/Mcp`, mounted in `routes/web.go`. That one is
`arandu-integrations`.

## Before you start

- `aru` must be on PATH, at the release the Dockerfile pins (`ARU_VERSION`).
  An older one serves an older contract and fewer doctor rules.
- The server works on the project in the directory it is started in: start it
  at the root, where `go.mod` is.
- Read `arandu-feature` first. The tools answer the questions its procedure
  asks; they do not replace the procedure.

## Contracts and imports

The client starts the server the way it starts any stdio server:

```json
{"command": "aru", "args": ["mcp"]}
```

Standard output carries protocol frames and nothing else; logs go to standard
error. The tools, and when to call each:

| tool | arguments | call it to |
| --- | --- | --- |
| `where_does_it_go` | `kind`, one of the contract's cards: controller, request, service, policy, model, repository, client, job, event, listener, mail, notification, command, middleware, migration, factory, seeder, enum, resource, view, fragment, mcp-tool, webhook, invokable-controller | learn the path a kind of code lives at, its imports, its signature, who calls it, what it may and may not do and its generator -- before writing the file |
| `feature_recipe` | `recipe`: crud, action, invokable, nested, report, job, command, listener, webhook, integration, mcp | get the order a kind of feature is built in, and the cards of each step |
| `doctor` | `rule` to keep one rule's findings, `profile` (conventional or performance) | read every finding with its rule, severity, file, line, why it matters and the card whose shape answers it -- after a change, before calling it finished |
| `generate` | `command` (a `make:*`), `arguments` as typed after it, `apply` | preview a generator: without `apply` it lists the files it would write and writes nothing; with `apply=true` it writes them and returns the wiring to paste |
| `project_map` | none | read this project's features, its routes with method, pattern and name, every file classified by what it declares, the edges between them -- routes-to, authorizes, persists, renders, tested-by -- and the doctor's findings |
| `commands` | `prefix` such as `make:` | list the aru commands with their usage and flags |
| `imports_catalog` | `package`, `moved_only` | the import path each framework symbol is named by, for the version `go.mod` requires |

## Procedure

1. **Connect.** Add the server to the client's configuration with the line
   above, at the project root, and check it lists the seven tools.
2. **Ask the recipe.** Classify the feature as `arandu-feature` says, then call
   `feature_recipe` with the matching recipe and follow its steps in order.
3. **Ask where each piece goes.** Before writing a file, call
   `where_does_it_go` with its kind, and write it at that path with those
   imports and that signature. When the card and this repository's example
   disagree, the card is the contract and the example is reported.
4. **Generate preview-first.** Call `generate` without `apply`, read the files
   it lists, and only then call it again with `apply=true`. Paste the wiring it
   returns into `bootstrap/app.go` and `routes/web.go` yourself -- the tool
   never edits them, and refuses a generator whose plan would.
5. **Run the doctor** through the `doctor` tool after the change, fix each
   finding by moving the code to where its card says, and run it again until
   it reports nothing. Then run every gate below in a terminal.

## Commands

- `aru mcp`, the server itself; the client starts it, a person does not
- `aru doctor`, `aru imports:catalog` and the generators, `aru make:module`
  and the rest, are what the tools run, and each works the same in a terminal

## Example

What `where_does_it_go` answers for `service` -- validate, load with the read
Grant, authorize, apply the entity's rule, persist with the Grant -- written
against the example resource, as the card's signature says:

```go compile
package example

import (
	"context"

	"github.com/arandu-io/hesape/auth"
	"github.com/arandu-io/hesape/database"

	models "<module>/app/Models"
	policies "<module>/app/Policies"
)

// Unpin is a use case in the shape the service card gives: the row is read
// under a read Grant, the policy decides about that row, and the write is
// made with the Grant the policy issued for it.
func Unpin(ctx context.Context, db *database.DB, actor auth.Subject, id string) error {
	var policy policies.NotePolicy
	read, err := auth.Authorize(ctx, policy, actor, policies.NoteView, models.Note{})
	if err != nil {
		return err
	}
	note, err := models.Notes(db).FindOrFail(ctx, read, id)
	if err != nil {
		return err
	}
	write, err := auth.Authorize(ctx, policy, actor, policies.NoteUpdate, *note)
	if err != nil {
		return err
	}
	note.Pinned = false
	_, err = note.Save(ctx, write)
	return err
}
```

## Do not

- Call `generate` with `apply=true` before reading its preview.
- Ask the tool to write wiring, run a migration or edit `bootstrap/` or
  `routes/`: it refuses, and working around it in a shell is the edit the
  refusal exists to keep in a person's hands.
- Copy a card's text into a document or a skill of this project: ask the tool
  again next time, so the answer follows the `aru` that is pinned.
- Treat the server as the application's own MCP surface. Tools an
  application offers its users live in `app/Mcp` and call services as who is
  asking -- `arandu-integrations`.

## Extending it

Nothing here is extended by the project. A card that answers the wrong path,
a recipe that is missing or a tool that is needed is a change to `aru`,
reported with what was asked and what came back.

## Wiring

None in the project. The client's configuration holds the one line; the
project's `bootstrap/app.go`, `routes/web.go` and `go.mod` do not change.

## Acceptance test

- The client lists `doctor`, `project_map`, `where_does_it_go`,
  `feature_recipe`, `commands`, `imports_catalog` and `generate`.
- `where_does_it_go` with `kind=service` answers `app/Services/<Entity>Service.go`.
- `generate` without `apply` writes nothing: `git status` is clean afterwards.
- The `doctor` tool and `aru doctor` in a terminal report the same findings.

## Limits

The server reads the project on disk; it does not run the application, so a
route that exists only at runtime is not in `project_map`. The `doctor` tool
is the doctor, with its limits: evidence, not proof. `generate` runs only the
`make:*` commands; migrations, `aru view:build`, `aru model:build` and the
gates are run in a terminal.

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
