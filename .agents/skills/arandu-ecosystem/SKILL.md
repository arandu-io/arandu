---
name: arandu-ecosystem
description: Decide whether a feature belongs in this application or in a shared Arandu module, before adding or changing authorization, roles, organizations or tenants, balances, credits or any spendable unit, tags or labels, rendered Markdown, API documentation, Brazilian documents and money (CPF, CNPJ, BRL), or a helper such as a slug or a currency format. Use when a request mentions permissions, groups, members, workspaces, wallet, ledger, credits, points, tags, categories, Markdown, OpenAPI or Swagger, CPF/CNPJ, or when about to create a table, a package or a helper something else would also need.
license: MIT
---

# Using what the ecosystem already owns

## When to use

Before the first table, package or helper of a feature: something else may own
it already. Writing a second one here works on day one, and from then on two
implementations drift. A capability this application really owns goes on to
`arandu-module`.

## Before you start

1. `AGENTS.md`, then `arandu-feature`.
2. `bootstrap/app.go`: what `Build` constructs and passes to `k.Register` is
   everything the application was given. A module not registered there is a
   module the application does not have.
3. `github.com/arandu-io/examples`, the public reference application, before
   inventing a shape.

## Contracts and imports

| responsibility | owner | it owns |
| --- | --- | --- |
| who may do what inside an organization | `github.com/hyz-is/arandu-permission` | permission groups, actions, members of a group, the screens that administer them; it fills the subject's roles, and this application's policies still decide |
| anything with a balance | `github.com/hyz-is/arandu-wallet` | wallets, deposits, withdrawals, transfers, credits, holds, refunds, the ledger and statements, idempotency keys |
| labels on records | `github.com/hyz-is/arandu-tags` | tags, their ordering, attaching tags to any record |
| Markdown a page shows | `github.com/hyz-is/arandu-markdown` | sanitized HTML, the table of contents, plain text, reading time |
| OpenAPI for the HTTP routes | `github.com/hyz-is/arandu-swagger` | an OpenAPI 3.1 document built from the route table, Swagger UI served from the same origin |
| CPF, CNPJ, CEP, BRL, Brazilian dates | `github.com/hyz-is/arandu-br` | validation rules and pure formatting functions |
| a helper | the package functions of `github.com/arandu-io/hesape`: `support`, `str`, `number`, `collections`, `pagination` | slugs, currency, plurals, collections -- called by package name, never global |

The flow of an organization's data is fixed:
`login identity -> membership -> organization (the tenant) -> subject -> Policy -> auth.Grant -> auth.Tenant(g) -> the row`.

## Procedure

1. **Answer these before a migration**, in writing:
   1. Is this the login identity, or data an organization owns?
   2. Which tenant owns each row? It comes from `auth.Tenant(g)`.
   3. Which policy action authorizes the read, and which the write?
   4. Is it access control? arandu-permission.
   5. Does any number in it go up and down and get spent? arandu-wallet.
   6. Is it a label a user attaches? arandu-tags.
   7. Is a field Markdown a page renders? arandu-markdown.
   8. Does it expose an HTTP API someone else calls? arandu-swagger.
   9. Is it a Brazilian document or amount? arandu-br.
   10. Would the next project need the same thing? Then it is a module (step 4).
2. **Use the owner.** A role is arandu-permission's, never a `role` column or an
   ACL table; a balance is arandu-wallet's, never a `balance` column, a
   `credits` table or a `*_ledger`. This application may own the commercial
   side -- a catalog, prices, a contract with a payment provider -- and calls
   the wallet to move value.
3. **Install a module** the same way for all of them, from the latest release
   in `hyz-is`, never with a `replace` to a local checkout:

   ```bash
   go get github.com/hyz-is/arandu-wallet@<tag>
   ```

   Construct it in `Build`, after the session store exists, with the tenant
   from `cfg.Auth.Tenant` and the CSRF issuer `Build` already made; add it to
   the `Register(...)` list; then `aru migrate`, and preview what it publishes
   with `aru vendor:publish --tag=view` before `--apply`. Each module's README
   has its exact `Config`.
4. **When no module owns it**, stop and report: name the capability, the
   modules checked, why none owns it, and whether one should be extended. A new
   reusable capability starts from `github.com/arandu-io/package-skeleton` as a
   `hyz-is/arandu-*` module, with its public contract defined before any
   application embeds it. Only what belongs to this application alone is
   generated here.

## Commands

- `aru migrate`, for the tables a module brings
- `aru vendor:publish --tag=view`, then `aru vendor:publish --tag=view --apply`, then `aru view:build`
- `aru about`, what the project is wired with
- `aru skills:sync`, the skills a required `hyz-is` module hands out, with `--apply` to write them

## Example

The helpers are package functions of hesape, called by name -- never written
again here:

```go compile
package example

import (
	"github.com/arandu-io/hesape/number"
	"github.com/arandu-io/hesape/str"
)

// Listing is how a record is addressed and priced on a page: a slug for the
// address and a currency for the amount, held in cents.
func Listing(title string, cents int64) (slug, price string) {
	return str.Slug(title, "-"), number.CurrencyFromCents(cents, "USD")
}
```

## Do not

- Create a `role`, `is_admin`, `balance` or `credits` column, a tags table, a
  Markdown renderer or a hand-kept OpenAPI file.
- Write a `Slugify`, a CPF validator or a BRL formatter in the application:
  `helper-reimplemented`.
- Point `go.mod` at a local checkout with `replace`, or work around a module's
  defect in the application: fix it in the module's repository.
- Take an organization id from a form as the tenant. It is navigation intent;
  the tenant comes from the Grant.

## Extending it

A module's screens are published into the project with `aru vendor:publish`
and edited there, inside their custom blocks. A primitive a module lacks is
added to the module, by its repository and a release -- never as a local
substitute.

## Wiring

Explicit, in `bootstrap/app.go`: constructed in `Build`, registered in the
`Register(...)` list -- arandu-swagger last, after the modules whose routes it
documents. Migrations run through `aru migrate`, never at boot.

## Acceptance test

- Every new table with a tenant column claimed in
  `tests/Feature/TenantScope_test.go`, with a cross-tenant test.
- Nothing in the diff is a second copy of what an owner above holds.
- `aru doctor` reports no `driver-not-linked`, `helper-reimplemented` or
  `skills-missing`.

## Limits

This skeleton requires none of the `hyz-is` modules: a project takes the ones
it needs. The list above is the set that exists today; a capability not in it
is a report, not a license to build it here.

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
