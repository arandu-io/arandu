# Skills

Procedures an assistant follows when working in this project, in the location
the tools agree on: `.agents/skills/<name>/SKILL.md`.

Each file opens with frontmatter carrying a `name` and a `description`. The
description is what a tool reads to decide whether the skill is relevant, so it
names the situation you are in rather than the topic it covers.

| skill | when it fires |
| --- | --- |
| `arandu-module` | adding an entity, a resource, a CRUD feature |
| `arandu-policy` | writing authorization, or a repository call that will not compile |
| `arandu-view` | writing a page, a layout or a fragment |
| `arandu-doctor` | `aru doctor` reported something, or a change is about to be called finished |

## Why these exist

This framework is in nobody's training set. A model asked to write Go here fills
the gap with the frameworks it does know and produces a service container, a
fillable model, a facade, a service provider — none of which exist, all of which
were considered and refused. `AGENTS.md` at the root lists what each of those
maps to.

The rest of the answer is that the project is built to be checked rather than
trusted: `aru schema` prints the schema a specification is written against,
`aru generate --check` validates before anything is written, and `aru doctor`
reports twenty-nine findings that each name a file, a line and what breaks. An
assistant that uses those is not guessing.

## Adding your own

A skill in this directory is yours and travels with the project. Keep it a
procedure rather than a description: a file that says "read the documentation"
never changes what anybody does.
