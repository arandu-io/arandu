---
name: arandu-policy
description: Authorization in an Arandu (Go) application. Use when writing or changing who may read or change a record, when a Model or service call will not compile, when something asks for an auth.Grant, or when the request mentions "permissions", "roles", "who can access", "only the author", "authorize", "multi-tenant", "tenant isolation", or "this method needs a Grant". Also use when tempted to remove a parameter to make code compile -- here that parameter is the only thing making the query safe. Covers Policy, Grant, auth.Authorize, auth.Tenant, re-authorizing the row, SystemGrant and aru make:policy.
license: MIT
---

# Authorization, and why it will not compile without it

## When to use

Who may take an action on a record, the tenant a query reads, a Grant a call
asks for, and the one escape hatch for work with nobody asking. Which roles a
member holds inside an organization is `arandu-ecosystem` (arandu-permission);
this skill is the decision about a record.

## Before you start

- Read `app/Policies/NotePolicy.go`, the example's rules, and the `Get` and
  `Publish` methods of `app/Services/NoteService.go`, which ask it.
- `auth.Grant` has only unexported fields: nothing outside its package builds
  one. Every read and write through a model takes one. A service that reaches
  the database without asking a Policy has nothing to pass, and does not
  compile. The safe path is not documented; the unsafe path is absent.

## Contracts and imports

All from `github.com/arandu-io/hesape/auth`:

| symbol | contract |
| --- | --- |
| `Action` | a named constant, entity first: `NoteView auth.Action = "note.view"` |
| `Policy[T]` | `Can(ctx, s auth.Subject, a auth.Action, x T) error`: nil allows, an error denies; it never builds a Grant |
| `Authorize` | `auth.Authorize(ctx, policy, subject, action, resource) (auth.Grant, error)`: runs `Can`, and only when it answers nil issues the Grant |
| `Grant` | what every terminal of a generated query, `Save` and `Delete` take; `g.Check(action)` asks whether it was issued for an action |
| `Tenant` | `auth.Tenant(g)`, the only source of a tenant |
| `SystemGrant` | `auth.SystemGrant(action, tenant)`, for work with no subject: a seeder, a job, a command, `main.go` |
| `Subject` | who is asking: `ID`, `Tenant`, `Roles`, `HasRole` |

## Procedure

1. **Name the action** as a constant in the policy's file, beside the five
   the generator writes -- the example added `NotePublish`.
2. **Decide in `Can`**, inside the custom block: tenant isolation first, the
   zero subject refused, then one case per action, and nothing that allows by
   default -- the function denies by falling through.
3. **Ask from the service, twice for one row.** Once with the zero value, to
   ask "may this caller look at all", and once with the row in hand, "may this
   caller look at *this* row". A write asks about the row it read, never about
   what the request says the row is, so `NoteService.Publish` authorizes
   `NotePublish` against the stored note and its stored author.
4. **Read and write with the Grant that came back.** Reads are not exempt: a
   list, a report, an export and a projection all take a Grant and filter by
   its tenant.
5. **For work with nobody asking**, use `auth.SystemGrant` in a seeder, a job, a
   command or `main.go`. Anywhere else it carries its reason on the line
   before: `//arandu:system-grant <reason>`, as the example's listener does.

## Commands

- `aru make:policy <module>` writes a policy that denies every action
- `aru make:module` writes one with the module
- `aru action:list` lists the actions the source declares, with the constant and the line of each

## Example

A rule about the row, and the double authorization that reads it:

```go compile
package example

import (
	"context"
	"fmt"

	"github.com/arandu-io/hesape/auth"
	"github.com/arandu-io/hesape/database"

	models "<module>/app/Models"
)

// NotePin is pinning one note.
const NotePin auth.Action = "note.pin"

// PinPolicy decides who pins a note: its author, in its tenant.
type PinPolicy struct{}

var _ auth.Policy[models.Note] = PinPolicy{}

// Can answers nil to allow and an error to deny, and denies by falling through.
func (PinPolicy) Can(_ context.Context, s auth.Subject, a auth.Action, n models.Note) error {
	if n.ID != "" && n.TenantID != s.Tenant {
		return fmt.Errorf("note belongs to another tenant")
	}
	if s.ID == "" || s.Tenant == "" {
		return fmt.Errorf("pinning needs somebody signed in")
	}
	if a == NotePin && (n.ID == "" || n.OwnedBy(s.ID)) {
		return nil
	}
	return fmt.Errorf("no rule allows %s on note", a)
}

// Pin reads the row through a Grant, authorizes the row it read, and writes
// with the Grant that decision issued.
func Pin(ctx context.Context, db *database.DB, actor auth.Subject, id string) error {
	g, err := auth.Authorize(ctx, PinPolicy{}, actor, NotePin, models.Note{})
	if err != nil {
		return err
	}
	note, err := models.Notes(db).FindOrFail(ctx, g, id)
	if err != nil {
		return err
	}
	g, err = auth.Authorize(ctx, PinPolicy{}, actor, NotePin, *note)
	if err != nil {
		return err
	}
	note.Pinned = true
	_, err = note.Save(ctx, g)
	return err
}
```

## Do not

- Remove a Grant parameter to make something compile. There is no correct way
  to query a model without one.
- Read the tenant from a path, a body, a query string or a header:
  `tenant-from-request`, `tenant-from-header`. It is `auth.Tenant(g)`.
- Authorize in middleware, or in the controller. A guard answers "is there a
  session" and stops; the policy runs in the service, for every caller of it.
- Skip the second authorization on a row you read: `resource-not-reauthorized`.
- Answer another tenant's row with 403. A guessed id is not found -- a 403
  confirms the row exists -- and the tenant scope of every query already makes
  it so.
- Add a role column, an `is_admin` flag or an ACL table: roles are
  arandu-permission's (`arandu-ecosystem`), and the policy reads them off the
  subject.

## Extending it

A rule goes in the custom block of `Can`; an action is a constant beside the
generated ones. A rule that depends on another record -- the note a comment
belongs to -- is decided by loading that record through its own service and
policy, as `CommentService` loads the note, never by reading its table here.

## Wiring

A policy is a value: the service holds it as a field and passes it to
`auth.Authorize`. Nothing registers it, and nothing in `bootstrap/app.go`
changes.

## Acceptance test

- A unit test that an action nobody wrote a rule for is refused, which the
  generator writes and keeps passing as the policy opens.
- A unit test that every read of the service is refused for the zero subject
  before the database is touched -- the service runs with a nil database.
- Feature tests: another member's change answered 403, another tenant's row
  404, for every action that takes an id.

## Limits

`SystemGrant` exists. What catches a handler using it is `aru doctor`
(`system-grant-outside-scope`), not the type system, and the marker excuses the
line it is on and nothing else. A table with no tenant column is invisible to
the tenant test, and its isolation rests on every query of it taking a Grant.

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
