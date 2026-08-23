---
name: arandu-module
description: Add an entity, resource, model, table or CRUD feature to an Arandu (Go) application. Use when the request is to "create a model", "add a resource", "scaffold CRUD", "add invoices", "make a posts table", "generate a module", or any new domain object with a database table and rules about who may touch it. In Arandu the model writes a YAML specification and a deterministic generator writes the Go — never write the entity, repository, policy or migration by hand. Covers aru schema, aru generate, the ten column types, the five actions, and the custom blocks that survive regeneration.
license: MIT
---

# Adding a module to an Arandu application

You do not write the Go. You write a specification and a deterministic generator
writes the entity, the policy, the repository, the service, the request, the
routes and the tests.

This is not a preference. A new framework is in nobody's training set, so a
model asked for Go here fills the gap with the frameworks it does know and
produces a service container, a fillable model, a facade — none of which exist.
A specification is small enough to be right, and a wrong one fails validation
instead of becoming Go that does not compile.

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

**5. Wire it.** The generator prints the lines to paste into `bootstrap/app.go`.
It does not edit that file, on purpose: a generator that changes the wiring
behind you is a generator whose output nobody can explain.

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

- Every repository method takes `security.Grant` before the id. Removing it to
  make something compile is removing the only thing that makes the query safe.
- The tenant comes from `data.Tenant(g)`. Never from a path segment, a body, a
  query or a header.
- The generated policy denies every action, with no allow-everything branch to
  delete later. Open it deliberately, one action at a time.

If you find yourself wanting to reach a repository without a Grant, stop. There
is no correct way to do it, and the compiler is what says so.
