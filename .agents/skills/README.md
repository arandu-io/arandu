# Skills

Procedures an assistant follows when working in this project.

They live in `.agents/skills/<name>/SKILL.md`, which is the path the coding
assistants read from — Cursor, Codex, Cline, Copilot, Gemini CLI, Amp, OpenCode,
Warp, Zed and the rest all look there. It is one directory rather than a file
per vendor, so a skill written once is read by whatever this project is being
written with.

Each file opens with frontmatter carrying a `name` and a `description`. The
description is what a tool reads to decide whether the skill is relevant, so it
names the situation you are in rather than the topic it covers.

Start with `arandu-feature`: it classifies the change and names the family
skill of each step. Each family skill has the same sections, in the same order
-- when to use, before you start, contracts and imports, procedure, commands,
a compiling example, what not to do, how to extend it, wiring, the acceptance
test, limits and the gates -- and holds the rules of its family and no other's.

| skill | when it fires |
| --- | --- |
| `arandu-feature` | any change that adds or changes behaviour: the entry |
| `arandu-module` | an entity, a table, a column, a transition, the generated query, a service |
| `arandu-http` | a controller, a request, a route: resource, nested, singleton, invokable, named action |
| `arandu-api` | a JSON answer, a JSON Resource, problem+json, tokens, idempotency |
| `arandu-view` | a page, a layout, a fragment, a component |
| `arandu-async` | a job, a scheduled task, an event, a listener, a notification, a mail, a command |
| `arandu-integrations` | a client of another system, a webhook, an MCP tool |
| `arandu-policy` | authorization, a Grant, the tenant, `SystemGrant` |
| `arandu-doctor` | `aru doctor` reported something, or a change is about to be called finished |
| `arandu-ecosystem` | permissions, organizations, balances, tags, Markdown, API docs, CPF/CNPJ, a helper -- before a new table or package |
| `arandu-mcp` | the assistant can start `aru mcp`: ask it where code goes, the recipe, the doctor, and preview a generator |
| `notes` | the example resource: what each of its files shows and which command wrote it |

`tests/Unit/Skills_test.go` holds them to that: the sections, the gate block
identical to the one in `AGENTS.md`, every block fenced as `go compile` built
against this module, and -- when the `aru` on PATH is the release the
Dockerfile pins -- every `aru` command and flag they name present in its usage.

## Why these exist

This framework is in nobody's training set. A model asked to write Go here fills
the gap with the frameworks it does know and produces a service container, a
fillable model, a facade, a service provider — none of which exist, all of which
were considered and refused. `AGENTS.md` at the root lists what each of those
maps to.

The rest of the answer is that the project is built to be checked rather than
trusted: `aru schema` prints the schema a specification is written against,
`aru generate --check` validates before anything is written, and `aru doctor`
reports findings that each name a file, a line and what breaks. An
assistant that uses those is not guessing.

## Adding your own

A skill in this directory is yours and travels with the project. Keep it a
procedure rather than a description: a file that says "read the documentation"
never changes what anybody does.
