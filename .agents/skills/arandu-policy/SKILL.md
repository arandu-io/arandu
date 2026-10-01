---
name: arandu-policy
description: Authorization in an Arandu (Go) application. Use when writing or changing who may read or change a record, when a Model or service call will not compile, when something asks for a security.Grant, or when the request mentions "permissions", "roles", "who can access", "authorize", "multi-tenant", "tenant isolation", or "this method needs a Grant". Also use when tempted to remove a parameter to make code compile — here that parameter is the only thing making the query safe. Covers Policy, Grant, data.Tenant, re-authorizing the row, and SystemGrant.
license: MIT
---

# Authorization, and why it will not compile without it

`security.Grant` has only unexported fields. Nothing outside the package that
defines it can build one. Every read and write through a Model takes one — the
query terminals `First`, `Get`, `Value` and the entity's `Save` and `Delete`
all ask for it:

```go
found, err := models.Invoices(s.db).NewQuery().WhereKey(id).First(ctx, g) // no Grant, no compile
```

and the tenant is read off it with `data.Tenant(g)` — never from the path, the
body or a header. So a service that reaches the database without asking a
Policy has nothing to pass. That is the whole design: the safe path is not
documented, the unsafe path is absent.

## The procedure

**1. The Policy decides; `security.Authorize` issues.** A policy is a
`security.Policy[T]`, and its only method is `Can`: it returns `nil` to allow and
an error to deny. It never builds a Grant. `app/Policies/NotePolicy.go`, the
example resource, is the model to follow for a rule about the row — only a
note's author may change it; this is the shape:

```go
const (
	ActionInvoiceView security.Action = "invoice.view"
	ActionInvoiceList security.Action = "invoice.list"
)

type InvoicePolicy struct{}

var _ security.Policy[models.Invoice] = InvoicePolicy{}

func (InvoicePolicy) Can(_ context.Context, s security.Subject, a security.Action, inv models.Invoice) error {
	// Tenant isolation comes first and applies to every action.
	if inv.ID != "" && inv.TenantID != s.Tenant {
		return fmt.Errorf("invoice belongs to another tenant")
	}
	switch a {
	case ActionInvoiceView, ActionInvoiceList:
		if s.HasRole("member") {
			return nil
		}
	}
	return fmt.Errorf("no rule allows %s on invoice", a)
}
```

Actions are constants, tenant isolation is the first check, and there is no
default branch that allows — the function denies by falling through.

**2. The service asks, then reads.** `security.Authorize(ctx, policy, subject,
action, resource)` runs `Can` and, only when it returns `nil`, issues the Grant:

```go
func (s *InvoiceService) Get(ctx context.Context, actor security.Subject, id string) (*models.Invoice, error) {
	g, err := security.Authorize(ctx, s.policy, actor, policies.ActionInvoiceView, models.Invoice{})
	if err != nil {
		return nil, err
	}
	// A missing row is model.ErrModelNotFound, which the router answers 404.
	found, err := models.Invoices(s.db).FindOrFail(ctx, g, id)
	if err != nil {
		return nil, err
	}
	// The row is authorized too, now that it is in hand.
	if _, err := security.Authorize(ctx, s.policy, actor, policies.ActionInvoiceView, *found); err != nil {
		return nil, err
	}
	return found, nil
}
```

**3. Authorize the row as well as the action.** The first call answers "may this
caller look at invoices at all". The second, with the record in hand, answers
"may this caller look at *this* invoice". Skipping it means any user of the same
tenant sees the row, and `aru doctor` reports it as `resource-not-reauthorized`.
A write asks about the row it read, never about what the request says the row
is: `NoteService.Update` reads the note, then authorizes `NoteUpdate` against
that stored note, so the owner the policy compares is the stored one.

**4. Reads are not exempt.** `List`, `Find`, a read model, a projection, a
report, a dashboard and an export all require a Grant and all filter by the
tenant on it. "The read path can skip the policy" is a cross-tenant data leak
with a technical name.

## What to do when it will not compile

You are missing a Grant, and the answer is never to remove the parameter.

- **In a handler**: call the service, and let the service ask the Policy first.
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
hidden. Everything else — a Model query reachable with no Grant, a tenant chosen
by the caller — is a build that does not complete.

## The generated policy denies everything

`aru make:module` and `aru make:policy` write a policy that refuses every
action, with no allow-everything branch to delete later. Open it one action at a
time, deliberately. `aru doctor` reports `policy-never-opened` as a warning
rather than an error, because a fresh module is correctly closed and a project
that is red on day zero teaches people to ignore the report.
