---
name: notes
description: Work with the Note module of this Arandu application. Use when the request mentions notes, when a notes route is involved, or when reading or changing note records. Covers what the module exposes, which roles may take which action, and the rule that the Service authorizes before it reaches the Hesape Model.
license: MIT
---

> Example resource. Remove with the list under "The example resource" in README.md.

# The Note module

It is what `aru make:module note --fields "title:string!,body:text,pinned:bool"
--tenant` writes, plus edits made by hand. No specification of it is kept --
there is no `database/specs/` here -- so there is nothing to generate it again
from, and it is not regenerated. `--force` would keep what sits between the
`// arandu:begin custom` and `// arandu:end custom` markers and drop every edit
outside them: the `user_id` author column in `Note.go` and in the migration, the
two lines of `NoteService.Create` that set it, and the example header of each
file. The build would then stop at `Note.OwnedBy`, which the policy's ownership
rule calls. Change these files directly.

## What it is made of

| file | what it holds |
| --- | --- |
| `app/Models/Note.go` | the entity and its table, with custom blocks for settings and local scopes |
| `app/Models/NoteQuery.go` | `Notes`, the typed query and the collection, written by `aru model:build` and never by hand |
| `app/Policies/NotePolicy.go` | who may do what: the rule `security.Authorize` asks before it issues a Grant |
| `app/Services/NoteService.go` | the domain, and the only caller of the Model entry point on a request's path |
| `app/Http/Controllers/NoteController.go` | the actions the routes dispatch to |
| `app/Http/Requests/NoteRequest.go` | the input contract of create and update, with its form tags. Authorization stays in the Policy |
| `resources/views`, under the resource | the four screens, which share one row struct |
| `tests/Unit/Note_test.go` | that reads authorize before the Model is queried |

## Its fields

| field | type |
| --- | --- |
| `title` | `string` |
| `body` | `text` |
| `pinned` | `bool` |

Every query uses the Model's `tenant_id` scope. Builder terminals take the
Grant, and its tenant never comes from a path segment, a body, a query or a header.

## Reaching a record

There is one way, and the compiler is what says so.

```go
g, err := security.Authorize(ctx, policy, subject, action, models.Note{})
if err != nil {
    return err
}
record, err := models.Notes(db).FindOrFail(ctx, g, id)
```

Every terminal of `NoteQuery` takes `security.Grant`, and nothing outside the
security package can build one. The Service owns the database handle, authorizes first,
and then spends that Grant on the Model. A Controller has neither dependency and
cannot grow a second persistence path.

Reads are not exempt. `List`, `FindOrFail`, a report and an export all require a Grant.

## A request, end to end

```go
var in requests.NoteRequest
if err := ctx.Bind(&in); err != nil {
    return err
}
who, _ := ctx.User()
created, err := c.svc.Create(ctx.Ctx(), who, in)
if err != nil {
    return err
}
return ctx.RedirectRoute("notes.show", created.ID)
```

- **Who is asking** is `ctx.User()`, which the sign-in guard on the routes puts
  on the request. There is no session lookup in the controller.
- **The input** is `ctx.Bind` into `NoteRequest`: only the fields with a
  `form` tag are read, trimmed and converted. A new field is one line there,
  one in the model and one in the service's `fill`, a new migration that adds
  the column, and its input on the create and edit forms.
- **An error is returned, never mapped.** The router answers it:
  `validation.Errors` goes back to the form with the messages and what was typed,
  a missing row is 404, a refusal is 403, and an error with an `HTTPStatus() int`
  method is that status.
- **A screen's page** is `view.New(ctx, title)`: the title, what a rejected form
  left in the flash, and the CSRF token the middleware issued. The controller
  takes the service and nothing else.
- **The listing** is the Model's `SimplePaginate`, newest first, and the key
  is one the Model generates on insert.

## What the policy allows

The generated policy denied every action; this example opened it, inside the
custom block of `app/Policies/NotePolicy.go`:

| action | who |
| --- | --- |
| `note.list`, `note.view`, `note.create` | anybody signed in to the tenant |
| `note.update`, `note.delete` | the note's author, `user_id`, and nobody else |

The author is set by `NoteService.Create` from `ctx.User()`, never from the
form. A note of another tenant is not found at all (404), because every read is
scoped by the Grant's tenant; another member's note is found and refused (403).
`tests/Feature/Notes_test.go` proves each of those.

## Before calling a change finished

The gates, all of them, as `AGENTS.md` lists them:

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
