---
name: arandu-api
description: Answering a program rather than a person in an Arandu (Go) application -- a JSON Resource, a JSON answer from the same routes the pages use, problem+json errors, bearer-token authentication and Idempotency-Key replay. Use when the request is to "add an API", "return JSON", "add an endpoint for the mobile app", "API tokens", "make this POST idempotent", "the error format of the API", or when ctx.JSON, ctx.WantsJSON, JsonResource, RequireToken or Idempotent is involved. Covers aru make:resource.
license: MIT
---

# Answering a program

## When to use

A client that sends `Accept: application/json` -- a mobile app, a script,
another service -- reading or changing a record. The routes it calls are the
ones `arandu-http` registers; this skill is about the answer and the guards an
API needs. A page is `arandu-view`.

## Before you start

- Read `app/Http/Resources/NoteResource.go` and the JSON branches of
  `NoteController.Index`, `Show`, `Store` and `Publish`.
- Read the `/api` group in the custom block of `routes/web.go`: `POST
  /api/notes` and `POST /api/notes/{id}/publish`, behind `RequireToken` and
  `Idempotent`, answered by the same two actions the browser's routes use.
- Read `app/Services/PersonalAccessTokenService.go`, which issues, revokes and
  resolves the tokens, and `app/Http/Middleware/PersonalAccessTokens.go`, the
  resolver `RequireToken` is given.
- There is no `routes/api.go`. A JSON client is answered by the same routes;
  a route only a program calls sits in `routes/web.go` behind the guard a
  program carries.

## Contracts and imports

| piece | contract |
| --- | --- |
| JSON Resource | `app/Http/Resources/<Entity>Resource.go`: a type with `ToArray() map[string]any` (the fields that may leave, by name) and `With() map[string]any` (what goes beside `data`), asserted with `var _ hhttp.JsonResource` |
| answer | `ctx.JSON(status, resource)` writes `{"data": ToArray(), ...With()}`; `ctx.TOON(status, resource)` is the same Resource for a payload going to a language model, chosen in code, never from a header |
| asking | `ctx.WantsJSON()` reads `Accept`; an action that answers both says `Vary: Accept` |
| errors | written by the router through `hesape/exception` as `application/problem+json` (RFC 9457): `validation.Errors` is 422 with an `errors` member keyed by field, a missing row 404, a refusal 403, a duplicate 409, an error with `HTTPStatus() int` that status |
| token | `middleware.RequireToken(d.Tokens)`, with `d.Tokens` the `middleware.TokenResolver` bootstrap builds: `appmiddleware.NewPersonalAccessTokens(tokens)`, which answers `middleware.ErrUnknownToken` for every token the service refuses -- unknown, revoked, expired, its account gone. A route behind it reads who is asking with `ctx.User()`, exactly as behind `RequireAuth` |
| issuing | `tokens.Issue(ctx, actor, name, lifetime)` on `*services.PersonalAccessTokenService` (`App.Tokens`) returns the token once, and stores only its SHA-256 as hex -- `middleware.DigestToken(token).String()`; `tokens.Revoke(ctx, actor, id)` deletes one. An account issues and revokes its own, and nothing else |
| CSRF | `CSRFProtect` leaves a request with `Authorization: Bearer` and no valid session cookie to the route's guard. A browser that reports it cross-site is still refused 403, and a session cookie riding along still needs the CSRF token (419) |
| idempotency | `middleware.Idempotent(d.Idempotency, ttl)`, mounted after the guard that puts the subject on the request; `d.Idempotency` is the cache store `CACHE_STORE` names, which can lock and is shared across replicas when it is the RESP one |

`hhttp` is `github.com/arandu-io/hesape/http`; `middleware` is
`github.com/arandu-io/framework/http/middleware`.

## Procedure

1. **Generate the Resource:** `aru make:resource Note` writes the Resource, its
   collection and the test that holds the answer to the listed fields. It lists
   every column but the tenant; delete what the answer must not carry, from
   `ToArray` and from the test's list.
2. **Answer from the action.** Before rendering, `if ctx.WantsJSON() { return
   ctx.JSON(http.StatusOK, resources.NewNoteResource(found)) }`, after adding
   `Vary: Accept`. A collection takes the page's next link through `With`.
3. **Let the router write every error.** Return the service's error; the JSON
   client gets the problem document, the browser its page or redirect.
4. **Give a program its own guard** when it does not carry the session cookie:
   a route in the `/api` group behind `RequireToken`, pointed at the action
   the browser's route already uses. Put `Idempotent` on a write a client may
   retry. The action answers a JSON client with the Resource -- `Store`
   answers 201 with `Location` -- and the policy decides as it does for the
   browser, about the account the token acts as.

## Commands

- `aru make:resource <Name>` writes `app/Http/Resources/<Name>Resource.go` and `tests/Unit/<Name>Resource_test.go`
- `aru route:list` shows which routes a token group holds

## Example

A token for a program, issued as the account it acts as, and the routes it
calls -- the same lines as the `/api` group in `routes/web.go`:

```go compile
package example

import (
	"context"
	"time"

	fhttp "github.com/arandu-io/framework/http"
	"github.com/arandu-io/framework/http/middleware"
	"github.com/arandu-io/hesape/auth"

	controllers "<module>/app/Http/Controllers"
	services "<module>/app/Services"
)

// IssueForScript gives the signed-in account a token for a script, valid for
// ninety days. The token is in the first value and nowhere else: show it once.
func IssueForScript(ctx context.Context, tokens *services.PersonalAccessTokenService, who auth.Subject) (string, error) {
	token, _, err := tokens.Issue(ctx, who, "deploy script", 90*24*time.Hour)
	return token, err
}

// APIRoutes puts the guard first and Idempotent after it, on the actions the
// browser's routes already answer.
func APIRoutes(r *fhttp.Router, tokens middleware.TokenResolver, store middleware.IdempotencyStore,
	note *controllers.NoteController) {
	api := r.Group("/api", middleware.RequireToken(tokens))
	api.Action("POST", "/notes", note.Store,
		middleware.Idempotent(store, 24*time.Hour)).Name("api.notes.store")
	api.Action("POST", "/notes/{id}/publish", note.Publish,
		middleware.Idempotent(store, 24*time.Hour)).Name("api.notes.publish")
}
```

The client sends `Authorization: Bearer <token>` and `Accept:
application/json`, and `Idempotency-Key` on a write it may retry. No cookie,
no CSRF token.

## Do not

- Encode JSON onto the response writer or write a status by hand:
  `json-written-by-hand`. Every domain answer passes through a Resource.
- Hand `ctx.JSON` the entity, or a map: the entity answers every field it will
  ever have, the tenant and the next secret column included.
- Write an error format of your own. Two formats are two things every client
  parses.
- Read a token from a query string or a cookie, or fall back to the session on
  an API route: a cookie is what CSRF reaches.
- Mount `Idempotent` before the guard: it panics without a subject rather than
  keep one caller's answer where another could replay it.

## Extending it

A computed field, or one only some readers see, goes in the custom block of
`ToArray`. Metadata about the answer -- the next page, a total -- goes in
`With`. A second representation of the same record is a second Resource, not a
flag on the first.

## Wiring

`aru make:resource` needs no wiring: the controller imports the Resource. A
token route needs nothing new either: `bootstrap/app.go` already builds the
token service (`App.Tokens`), its resolver and the idempotency store, and hands
the last two to the routes as `Deps.Tokens` and `Deps.Idempotency`. The store
is the cache store `CACHE_STORE` names -- set it to the shared one when there
are replicas, because a key one process remembers is run again by the next.

## Acceptance test

- The Resource's unit test: the keys answered are exactly the listed ones.
- A feature test as a JSON client: `200` with the fields and `Vary: Accept`,
  and a problem document (`Content-Type: application/problem+json`, `status`,
  `title`) for a missing row, a refusal, a conflict and a rejected input --
  `TestAJSONClientIsAnsweredThroughTheNoteResource` and
  `TestAJSONClientIsRefusedWithProblemDocuments`.
- For a token route, as `tests/Feature/NotesAPI_test.go` does: a write with
  the token and no session answers 2xx and is the token's account in its
  tenant; another member's record is still 403; an unknown and a revoked token
  answer the same 401 with `WWW-Authenticate: Bearer`; a write the browser
  reports cross-site is 403, and one riding a session cookie without the CSRF
  token is 419.
- For an idempotent write: a retry with the same key answers
  `Idempotent-Replayed: true` and changes nothing twice.

## Limits

This project issues tokens through the service and has no screen for it: a
page that lists an account's tokens, shows a new one once and revokes them is
application code to write, on the service that is already here. A token acts
as its account in the tenant every sign-in of the deployment belongs to; an
application whose accounts live in several tenants decides how a token's
tenant is told apart -- never from the request -- and that is an
`arandu-ecosystem` question first.

A token carries the account's stored roles and no narrower scope of its own;
the policies decide every record. The generated Resource has no knowledge of
who reads it; a field only some readers may see is a decision written in the
custom block.

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
