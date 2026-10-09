---
name: arandu-ecosystem
description: Decide whether a feature belongs in this application or in a shared Arandu module, before adding or changing authorization, roles, organizations or tenants, balances, credits or any spendable unit, tags or labels, rendered Markdown, or API documentation. Use when a request mentions permissions, groups, members, workspaces, wallet, ledger, credits, points, tags, categories, Markdown, OpenAPI or Swagger, or when about to create a table or package for something another project would also need.
license: MIT
---

# Using the shared Arandu modules

Five responsibilities already have an Arandu module. Writing a second one inside
this project is the mistake this procedure exists to stop: it works on day one,
and from then on two implementations drift.

| responsibility | module | it owns |
| --- | --- | --- |
| who may do what | `github.com/hyz-is/arandu-permission` | permission groups, actions, members of a group, the screens that administer them |
| anything with a balance | `github.com/hyz-is/arandu-wallet` | wallets, deposits, withdrawals, transfers, credits, refunds, the ledger and statements, idempotency keys |
| labels on records | `github.com/hyz-is/arandu-tags` | tags, their ordering, attaching tags to any record |
| Markdown a page shows | `github.com/hyz-is/arandu-markdown` | sanitized HTML from `hesape/str.Markdown`, the table of contents, plain text, reading time |
| OpenAPI for the HTTP routes | `github.com/hyz-is/arandu-swagger` | an OpenAPI 3.1 document built from the route table, Swagger UI served from the same origin |

## 1. Read what is already here

1. `AGENTS.md`, then the other skills in `.agents/skills/`.
2. `bootstrap/app.go`: the `Build` function constructs every collaborator, and
   the `Register(...)` list is everything the application was given. If a
   module is not in that list, the application does not have it.
3. The `notes` resource is the worked example of the mandatory path:
   `app/Http/Controllers/NoteController.go` takes the subject with
   `ctx.User()`, `app/Services/NoteService.go` asks the Policy with
   `security.Authorize`, and only the `Grant` that returns reaches
   `models.Notes(db)`.
4. `tests/Feature/TenantScope_test.go` is how this project proves a record of
   one tenant is invisible to another. Any new table gets the same proof.
5. `github.com/arandu-io/examples` is the public reference application: a
   category/post domain with Policies, Services, the generated queries,
   `PostRepository` for the read the query builder cannot say, and
   `tests/Feature/TenantScoping_test.go`. Read it before inventing a shape.

## 2. Answer these before writing a migration

1. Is this the login identity, or data an organization owns?
2. Which tenant owns each row? (It will come from `data.Tenant(g)`, never from
   a path, a body, a query string or a header.)
3. Which Policy action authorizes the read, and which the write? A read is
   authorized exactly like a write.
4. Is it access control? → arandu-permission.
5. Does any number in it go up and down and get spent? → arandu-wallet.
6. Is it a label a user attaches? → arandu-tags.
7. Is any field Markdown that a page renders? → arandu-markdown.
8. Does it expose an HTTP API someone else calls? → arandu-swagger.
9. Would the next project need the same thing? → it is a module, not this
   application's code (step 5).

Write the schema only once each answer is written down.

## 3. Organizations, members and permissions

The flow is fixed:

`login identity → membership → organization (the tenant) → subject → Policy → security.Grant → data.Tenant(g) → the row`

- A membership links a login to an organization. The organization is the
  tenant of everything it owns.
- What a member may do inside the organization is authorization, and it lives
  in arandu-permission: its groups, its actions (`permission.Actions()` plus
  this application's own), its member screens. Do not add a `role` column, an
  `is_admin` flag or an ACL table to this project.
- Application Policies still decide domain rules ("only the author edits a
  draft"); arandu-permission decides who holds which action.
- A guessed id from another tenant answers not found, never forbidden: a
  forbidden answer confirms the row exists.

## 4. Balances, credits and spendable units

If the feature has a balance, credits, tokens, points that can be spent,
deposits, withdrawals, transfers, holds, refunds or a statement, it goes through
arandu-wallet. Do not create a `balance` column, a `credits` table, a
`*_ledger` table or a running total on another entity: arandu-wallet writes every
movement to its ledger inside one transaction, undoes an operation with a
reversal or a refund instead of editing it, and replays an idempotency key
instead of moving the value twice.

This application may own the commercial side — the product catalog, prices,
the contract with a payment provider — and it calls the wallet to move value.
If the wallet lacks a primitive the feature needs, stop and name the gap; extend
the module rather than writing a local substitute.

## 5. Installing a module

Every module installs the same way, and nothing edits `bootstrap/app.go` for
you:

```bash
go get github.com/hyz-is/arandu-wallet
```

In `bootstrap/app.go`, construct it in `Build`, after the session store exists:

```go
	walletModule, err := wallet.New(wallet.Config{
		Tenant: cfg.Auth.Tenant,
		CSRF:   cfg.CSRF,
	}, db, sessions)
	if err != nil {
		return App{}, err
	}
```

Add `walletModule,` to the `Register(...)` list. Then:

```bash
aru migrate                          # the module's tables
aru vendor:publish --tag=view        # preview the screens it publishes
aru vendor:publish --tag=view --apply
aru view:build
```

Each module's README has its exact `Config` and the actions it declares; read it
rather than guessing field names. arandu-markdown adds no table and no route,
and arandu-swagger is registered last, after the modules whose routes it
documents, and stays disabled unless `Config.Enabled` is true.

## 6. When no module owns it

1. Name the capability and list the modules above you checked.
2. Say why none of them owns it, or which one should be extended.
3. If it is new and the next project would need it, start the module from
   `github.com/arandu-io/package-skeleton`, the template community modules are
   cloned from, and define its public contract before any application embeds
   it.
4. If it really belongs only to this application, generate it with
   `aru make:module` (see `arandu-module`) so it gets the entity, the generated
   query, the Policy, the Service and the tests in the shape everything else
   has.

## 7. Before calling it finished

- The gates, all of them, as `AGENTS.md` lists them:

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

- `aru doctor` has no `driver-not-linked`, `model-core-outside-models` or
  `model-query-stale` finding.
- A tenant-isolation test for every new table, read and write, in the shape of
  `tests/Feature/TenantScope_test.go`.
- The Policy is asked before anything touches the database, and the tenant in
  every query came from the Grant.
- Nothing in the diff is a second copy of what one of the five modules owns.
