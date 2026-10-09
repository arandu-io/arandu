<p align="center">
  <img src=".github/logo.svg" alt="Arandu" width="180">
</p>

<h1 align="center">arandu-io/arandu</h1>

<p align="center">The project skeleton `aru new` clones — a running application from the first commit.</p>

<p align="center">
<a href="https://github.com/arandu-io/arandu/actions/workflows/ci.yml"><img src="https://github.com/arandu-io/arandu/actions/workflows/ci.yml/badge.svg" alt="Build Status"></a>
<a href="https://pkg.go.dev/github.com/arandu-io/arandu"><img src="https://pkg.go.dev/badge/github.com/arandu-io/arandu.svg" alt="Go Reference"></a>
<a href="https://github.com/arandu-io/arandu/tags"><img src="https://img.shields.io/github/v/tag/arandu-io/arandu?label=version" alt="Latest Version"></a>
<a href="LICENSE.md"><img src="https://img.shields.io/github/license/arandu-io/arandu" alt="License"></a>
</p>


## About the skeleton

> **Note:** this is what a new project starts from. The framework it runs on is
> [arandu-io/framework](https://github.com/arandu-io/framework).

You do not clone it by hand:

```sh
aru new my-app
cd my-app && aru dev
```

That gives you a Go framework for web applications, services and APIs, built
around development speed, a single compiled binary instead of a JavaScript
bundle, and authorization the compiler charges for: a repository call with no
`Grant` does not compile.

## What it delivers

- **A conventional tree** — `app/Http/Controllers`, `app/Models`,
  `app/Policies`, `app/Repositories`, `app/Services`, `app/Jobs`,
  `app/Events`, `app/Listeners`, `app/Mail`, `bootstrap/`, `config/`,
  `database/`, `resources/views/`, `routes/`, `storage/`, `public/` — so
  nothing about where a file lives has to be learned.
- **A mandatory `app/Policies/`** — elsewhere a policy directory is a habit an
  organised team keeps; here `aru doctor` fails a repository whose entity has no
  policy, and the policy denies by default with no allow-all branch.
- **A binary, not a toolchain** — it runs with `git clone && aru dev`. No
  `node_modules`, no `package.json`, no JavaScript lockfile, and no Node
  installed: the view compiler and every script are embedded in the binary.
- **`bootstrap/app.go`** — the one place the application is wired. `aru
  make:module` prints the lines a generated module needs and never edits this
  file itself: a generator that rewrites your wiring behind your back is a
  generator whose output nobody can account for.

## The example resource

A fresh project carries one small, complete feature, **notes**, so the first
thing you read is a whole module that works rather than an empty directory. It
has every step of the feature anatomy in `AGENTS.md`, each written by the `aru`
generator for it and opened by hand where the generator stops --
`.agents/skills/notes` names the command behind every file:

- **notes**, by `aru make:module note --fields "title:string!,body:text,pinned:bool" --tenant`:
  `NoteController` reads who is asking with `ctx.User()`, binds the form with
  `ctx.Bind` and returns every error to the router; `NoteService` validates,
  asks `NotePolicy` for a `Grant` and reads and writes through
  `models.Notes(db)`; anybody signed in to the tenant reads and writes notes,
  and only a note's author changes, deletes or publishes one;
- **comments nested under a note**, by `aru make:module comment --tenant --parent=notes`:
  the service loads the note under the note's own policy and lists by it;
- **publishing**, a named action of the resource (`aru make:controller --action=publish`)
  over a transition of the entity, `Note.Publish`, which stores `note.published`
  in the same transaction (`aru make:event`);
- **a JSON answer** from the same routes through `NoteResource` (`aru make:resource`),
  with problem documents for every refusal;
- **a notification** to the author's bell menu (`aru make:notification`), sent by
  a listener of the outbox (`aru make:listener`);
- **a nightly digest**, a scheduled task that enqueues a job (`aru make:job`)
  which hands the published notes to a newsletter client (`aru make:client`) --
  the fake in the tests, nothing at all when no `NEWSLETTER_API_URL` is set.

`tests/Feature/Notes_test.go` and `tests/Feature/Comments_test.go` prove each
path, including another tenant's note (404) and another member's (403).

`aru migrate` creates the tables and `aru db:seed` writes six notes in
development, by two accounts nobody can sign in as. To use it in a browser,
publish the sign-in screens with `go run github.com/arandu-io/ui@latest auth`,
make your own account with
`aru db:seed UserSeeder -e you@example.com -p <a-long-password>`, sign in, and
open `/notes`: the seeded notes are there to read, and refused to change,
because you did not write them.

### Removing it

There is no command for it. Delete these files:

```text
.agents/skills/notes/SKILL.md
app/Clients/NewsletterClient.go
app/Clients/NewsletterFake.go
app/Events/NotePublished.go
app/Http/Controllers/CommentController.go
app/Http/Controllers/NoteController.go
app/Http/Requests/CommentRequest.go
app/Http/Requests/NoteRequest.go
app/Http/Resources/NoteResource.go
app/Jobs/SendNotesDigest.go
app/Listeners/NotifyNoteAuthor.go
app/Models/Comment.go
app/Models/CommentQuery.go        (generated; aru model:build also removes it once Comment.go is gone)
app/Models/Note.go
app/Models/NoteQuery.go           (generated; aru model:build also removes it once Note.go is gone)
app/Notifications/NotePublished.go
app/Policies/CommentPolicy.go
app/Policies/NotePolicy.go
app/Services/CommentService.go
app/Services/NoteService.go
database/factories/CommentFactory.go
database/factories/NoteFactory.go
database/migrations/2026_10_01_000001_create_notes_table.go
database/migrations/2026_10_09_000001_create_comments_table.go
database/migrations/2026_10_09_000002_add_published_at_to_notes.go
database/seeders/NoteSeeder.go
resources/views/comments/         (the directory, four views)
resources/views/notes/            (the directory, four views)
resources/views/partials/notes_table.kyse.go
storage/framework/views/comments/ (the directory, compiled output)
storage/framework/views/notes/    (the directory, compiled output)
storage/framework/views/partials/notes_table.go   (compiled output)
tests/Feature/CommentTenantScope_test.go
tests/Feature/Comments_test.go
tests/Feature/Notes_test.go
tests/Unit/Comment_test.go
tests/Unit/NewsletterClient_test.go
tests/Unit/Note_test.go
tests/Unit/NotePublishedNotification_test.go
tests/Unit/NoteResource_test.go
```

and these lines, each marked with a comment naming this section:

```text
routes/web.go                        Note    *controllers.NoteController
routes/web.go                        Comment *controllers.CommentController
routes/web.go                        notes := r.Group("", middleware.RequireAuth(d.Sessions)), and the three lines that use it
bootstrap/app.go                     Notes *services.NoteService, in App, and Notes: notes where Build returns App
bootstrap/app.go                     listeners.NewNotifyNoteAuthor(notifier),
bootstrap/app.go                     notes := services.NewNoteService(db).WithNewsletter(newsletter(cfg.Services.Newsletter))
bootstrap/app.go                     Note:    controllers.NewNoteController(notes),
bootstrap/app.go                     Comment: controllers.NewCommentController(services.NewCommentService(db, notes)),
bootstrap/app.go                     _ ".../storage/framework/views/partials"   (once no other partial is left)
bootstrap/app.go                     func newsletter, at the end of the file
bootstrap/background.go              w.Handle(appjobs.SendNotesDigestName, appjobs.NewSendNotesDigestHandler(app.Notes))
app/Providers/AppServiceProvider.go  the notes.digest task in Schedule
config/services.go                   Newsletter Credential, and its entry in loadServices
.env.example                         NEWSLETTER_API_URL= and NEWSLETTER_TOKEN=
database/seeders/seeders.go          NoteSeeder{},
database/seeders/DatabaseSeeder.go   return NoteSeeder{}.Run(ctx, d)   (becomes: return nil)
tests/Feature/TenantScope_test.go    "notes": "...", and "comments": "...",
```

then the imports the deletions leave unused, which `go build` names. What
stays is the application's own: the notifier and its table, the listener list,
the scheduler's tenants and the queue the provider schedules on.

A database that already ran the migrations keeps the tables: in development run
`aru migrate:fresh` after deleting the files; anywhere else add a migration
that drops `comments` and `notes` and the `published_at` it added. Then
`aru model:build`, `aru view:build`, `go test ./...` and `aru doctor`.

`aru doctor` checks this tree against the architecture rules — from a
repository missing its policy to a tenant read off the request instead of the
`Grant` — and CI runs it on every push, without `--strict`: an error fails the
build, a warning stays the to-do it is, and a new project is never red for code
the generator wrote.

12,831 lines of Go outside the tests and 11,800 in them, across 59 test files,
the example resource included, counted with `wc -l` over the tracked `.go`
files — small on purpose: it is what a project starts from, not what it grows
into.

## The rest of Arandu

`aru` is the command line that clones and drives this skeleton;
[`arandu-io/framework`](https://github.com/arandu-io/framework) is what it
runs on; `hesape` is the component collection the framework is built from;
`examples` is a complete application, read-worthy end to end, built the same
way `aru new` starts one.

## Learning Arandu

The API reference is generated from the doc comments and lives on
[pkg.go.dev](https://pkg.go.dev/github.com/arandu-io/arandu). Every exported
symbol carries one, and that is deliberate: it is the documentation that cannot
drift from the code, because it sits in the same file.

The CLI documents itself. `aru help` lists every command, and each one explains
what it writes and what to do with it. `aru doctor` explains what it found and
what breaks, not which rule was violated.

The guide is published at [arandu.io/docs](https://arandu.io/docs), and the
site is itself an Arandu application. Where the guide and a doc comment
disagree, the doc comment sits next to the code and is the one to trust.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Before opening a pull request, the three
commands under "Before you open a pull request" have to pass — CI runs them, and
then the binary, `aru doctor`, the image and `govulncheck` on top.

## Security Vulnerabilities

Please review [our security policy](SECURITY.md) on how to report a
vulnerability. Never open a public issue for one.

## License

Open-sourced software licensed under the [MIT license](LICENSE.md).
