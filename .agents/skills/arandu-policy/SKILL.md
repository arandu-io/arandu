---
name: arandu-policy
description: Authorization in an Arandu (Go) application. Use when writing or changing who may read or change a record, when a repository call will not compile, when something asks for a security.Grant, or when the request mentions "permissions", "roles", "who can access", "authorize", "multi-tenant", "tenant isolation", or "this method needs a Grant". Also use when tempted to remove a parameter to make code compile — here that parameter is the only thing making the query safe. Covers Policy, Grant, data.Tenant, re-authorizing the row, and SystemGrant.
license: MIT
---

# Authorization, and why it will not compile without it

`security.Grant` has only unexported fields. Nothing outside the `security`
package can build one. Every repository method takes one before the id:

```go
func (r *InvoiceRepository) Find(
	ctx context.Context,
	g   security.Grant,   // no Grant, no compile
	id  string,
) (*models.Invoice, error) {
	tenant := data.Tenant(g)   // never from the path, the body or a header
	...
}
```

So a handler that reaches the database without asking a Policy has nothing to
pass. That is the whole design: the safe path is not documented, the unsafe path
is absent.

## The procedure

**1. The Policy decides and issues.** It is the only thing that produces a Grant.

```go
func (p *InvoicePolicy) View(ctx context.Context, s security.Subject, id string) (security.Grant, error) {
	if !s.HasRole("member") {
		return security.Grant{}, security.ErrForbidden
	}
	return security.Authorize(s, "invoice.view", id)
}
```

**2. The handler asks, then reads.**

```go
g, err := p.policy.View(ctx, subject, id)
if err != nil {
	return err
}
invoice, err := p.repo.Find(ctx, g, id)
```

**3. Authorize the row as well as the action.** The first call answers "may this
caller look at invoices at all". The second, with the record in hand, answers
"may this caller look at *this* invoice". Skipping it means any user of the same
tenant sees the row, and `aru doctor` reports it as `resource-not-reauthorized`.

**4. Reads are not exempt.** `List`, `Find`, a read model, a projection, a
report, a dashboard and an export all require a Grant and all filter by the
tenant on it. "The read path can skip the policy" is a cross-tenant data leak
with a technical name.

## What to do when it will not compile

You are missing a Grant, and the answer is never to remove the parameter.

- **In a handler**: ask the Policy first.
- **In a test**: build the subject and go through the Policy, so the test proves
  the refusal as well as the success.
- **In a scheduler, a migration or a queue worker**: there is no subject, and
  `security.SystemGrant` is the named escape hatch for exactly that. It is
  exported and auditable on purpose. `aru doctor` reports a *handler* that
  reaches for it, because a request always has a subject.

If none of those fits, the design is wrong rather than the compiler. Say so
instead of working around it.

## The honest limit

`SystemGrant` exists. What catches a handler using it is a lint, not the type
system, and that distinction is stated on the project's own site rather than
hidden. Everything else — a repository reachable with no Grant, a tenant chosen
by the caller — is a build that does not complete.

## The generated policy denies everything

`aru make:module` and `aru make:policy` write a policy that refuses every
action, with no allow-everything branch to delete later. Open it one action at a
time, deliberately. `aru doctor` reports `policy-never-opened` as a warning
rather than an error, because a fresh module is correctly closed and a project
that is red on day zero teaches people to ignore the report.
