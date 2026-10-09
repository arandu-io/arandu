---
name: notes
description: Work with the example resource of this Arandu application -- notes, the comments nested under them, publishing a note, the JSON answers, the notification its author gets, the nightly digest and the newsletter client. Use when the request mentions notes or comments, when a notes route is involved, or when reading the example to copy its shape. It maps each step of the feature anatomy to the file that shows it and the aru command that wrote it.
license: MIT
---

> Example resource. Remove with the list under "The example resource" in README.md.

# The example resource

It is the anatomy of a feature, whole, in one module: each step of the table in
`AGENTS.md` has a file here, written by the generator named beside it and
finished by hand where the generator stops. Copy its shape; the family skills
hold the rules.

## What wrote what

| step | file | written by | finished by hand |
| --- | --- | --- | --- |
| migration | `database/migrations/2026_10_01_000001_create_notes_table.go` | `aru make:module note --fields "title:string!,body:text,pinned:bool" --tenant` | the `user_id` author column |
| a column added | `database/migrations/2026_10_09_000002_add_published_at_to_notes.go` | `aru make:migration add_published_at_to_notes --table=notes --fields "published_at:timestamp"` | nothing |
| model and its rules | `app/Models/Note.go`, `app/Models/NoteQuery.go` | `aru make:module`, `aru model:build` | `OwnedBy`, `Published`, `Publish` and `ErrNoteAlreadyPublished` in the custom block |
| factory and seeder | `database/factories/NoteFactory.go`, `database/seeders/NoteSeeder.go` | `aru make:factory Note`, `aru make:seeder Note` | notes by two authors nobody can sign in as |
| policy | `app/Policies/NotePolicy.go` | `aru make:module` | the rules below, and `NotePublish` |
| request | `app/Http/Requests/NoteRequest.go` | `aru make:module` | nothing |
| service | `app/Services/NoteService.go` | `aru make:module` | the author from the subject, `Publish`, `SendDigest`, `WithNewsletter` |
| controller | `app/Http/Controllers/NoteController.go` | `aru make:module`; `Publish` by `aru make:controller Note --resource --action=publish` | the table fragment, the JSON branches, the publish button's address |
| routes | the custom block of `routes/web.go` | printed by each generator | one sign-in group for all three lines |
| views | `resources/views/notes/`, `resources/views/partials/notes_table.kyse.go` | `aru make:module` | the table moved to a partial, the publish form |
| JSON Resource | `app/Http/Resources/NoteResource.go` | `aru make:resource Note` | the next page's link |
| event | `app/Events/NotePublished.go` | `aru make:event NotePublished --aggregate=note --event-name=note.published --fields "title:string,author:string"` | nothing |
| listener | `app/Listeners/NotifyNoteAuthor.go` | `aru make:listener NotifyNoteAuthor --event=note.published` | decoding the payload and sending the notification |
| notification | `app/Notifications/NotePublished.go` | `aru make:notification NotePublished --channels=database` | the note's id and title |
| job and schedule | `app/Jobs/SendNotesDigest.go`, `AppServiceProvider.Schedule` | `aru make:job SendNotesDigest --event-name=notes.digest --fields "since:timestamp"` | the handler calls `NoteService.SendDigest`; the task by hand |
| client | `app/Clients/NewsletterClient.go`, `app/Clients/NewsletterFake.go` | `aru make:client Newsletter` | `SendDigest` on the client, the interface and the fake |
| nested resource | `app/Models/Comment.go` and the rest of the comment files | `aru make:module comment --fields "body:text!" --tenant --parent=notes` | the `user_id` author, the opened policy, the note key as text, the listing's index |

No specification is kept -- there is no `database/specs/` -- so there is
nothing to generate the module from again. `--force` would keep what sits
between the `// arandu:begin custom` and `// arandu:end custom` markers and drop
every edit outside them. Change these files directly.

## What the policies allow

| action | who |
| --- | --- |
| `note.list`, `note.view`, `note.create` | anybody signed in to the tenant |
| `note.update`, `note.delete`, `note.publish` | the note's author, `user_id`, and nobody else |
| `comment.list`, `comment.view`, `comment.create` | anybody signed in to the tenant who can read the note |
| `comment.update`, `comment.delete` | the comment's author |

The author is set by the service from `ctx.User()`, never from the form. A note
of another tenant is not found at all (404); another member's note is found and
refused (403); a note published twice is a conflict (409).

## The paths it proves

- **A browser**: `tests/Feature/Notes_test.go` and `tests/Feature/Comments_test.go`
  write, read, change, delete and publish through the router, and prove the
  403, the 404 and the 409.
- **A fragment**: the listing's table answered alone only to the request that
  names it, and the whole page to every navigation.
- **A JSON client**: the same routes through `NoteResource`, and problem
  documents for each refusal.
- **The outbox**: publishing stores `note.published` in the write's
  transaction; draining the relay stores the author's notification.
- **The scheduler and the worker**: the nightly task enqueues the digest, and
  the worker sends it to `NewsletterFake` -- no network, no credential.

## Before calling a change finished

The gates, all of them, as `AGENTS.md` lists them:

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
