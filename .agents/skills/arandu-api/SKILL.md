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
  `NoteController.Index`, `Show` and `Publish`.
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
| token | `middleware.RequireToken(resolver)`, with `resolver` a `middleware.TokenResolver`: `ResolveToken(ctx, middleware.TokenDigest) (auth.Subject, error)`, answering `middleware.ErrUnknownToken` for a token it does not know. The application stores `middleware.DigestToken(token)`, never the token |
| idempotency | `middleware.Idempotent(store, ttl)`, mounted after the guard that puts the subject on the request; `store` is a cache store that can lock, shared across replicas |

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
   a group behind `RequireToken`. Put `Idempotent` after the guard on a write a
   client may retry. Read "Limits" before a token client writes.

## Commands

- `aru make:resource <Name>` writes `app/Http/Resources/<Name>Resource.go` and `tests/Unit/<Name>Resource_test.go`
- `aru route:list` shows which routes a token group holds

## Example

Reads for a token client, answered through the Resource, and a write a
signed-in client may retry under `Idempotency-Key`:

```go compile
package example

import (
	"net/http"
	"time"

	fhttp "github.com/arandu-io/framework/http"
	"github.com/arandu-io/framework/http/middleware"
	"github.com/arandu-io/framework/security"
	hhttp "github.com/arandu-io/hesape/http"

	controllers "<module>/app/Http/Controllers"
	resources "<module>/app/Http/Resources"
	services "<module>/app/Services"
)

// NoteAPI answers programs that hold a token.
type NoteAPI struct {
	notes *services.NoteService
}

// Show answers one note through its Resource. A missing note and a refusal are
// returned, and the router writes the problem document.
func (a *NoteAPI) Show(ctx *hhttp.Context) error {
	who, _ := ctx.User()
	found, err := a.notes.Get(ctx.Ctx(), who, ctx.Param("id"))
	if err != nil {
		return err
	}
	return ctx.JSON(http.StatusOK, resources.NewNoteResource(found))
}

// APIRoutes puts each guard before what depends on the subject it carries.
func APIRoutes(r *fhttp.Router, tokens middleware.TokenResolver, store middleware.IdempotencyStore,
	sessions *security.SessionStore, a *NoteAPI, note *controllers.NoteController) {
	api := r.Group("/api", middleware.RequireToken(tokens))
	api.Action("GET", "/notes/{id}", a.Show).Name("api.notes.show")

	// A retry of the same publication, with the same Idempotency-Key, is
	// answered from the store and publishes nothing twice.
	signedIn := r.Group("", middleware.RequireAuth(sessions))
	signedIn.ResourceAction("POST", "notes", "publish", note.Publish, middleware.Idempotent(store, 24*time.Hour))
}
```

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
token group needs the resolver and the store built in `bootstrap/app.go` and
handed to the routes through `Deps`; the store is the shared cache when there
are replicas, because a key one process remembers is run again by the next.

## Acceptance test

- The Resource's unit test: the keys answered are exactly the listed ones.
- A feature test as a JSON client: `200` with the fields and `Vary: Accept`,
  and a problem document (`Content-Type: application/problem+json`, `status`,
  `title`) for a missing row, a refusal, a conflict and a rejected input --
  `TestAJSONClientIsAnsweredThroughTheNoteResource` and
  `TestAJSONClientIsRefusedWithProblemDocuments`.
- For a token group: no token and an unknown token answer the same 401.
- For an idempotent write: a retry with the same key answers
  `Idempotent-Replayed: true` and changes nothing twice.

## Limits

A token client cannot write here yet. `bootstrap/app.go` puts
`middleware.CSRFProtect` on every request, before routing, and it refuses a
write that carries neither a session cookie nor a CSRF token -- which is what a
client holding a bearer token sends. Reads behind `RequireToken` work; a write
from a token client needs the pipeline to leave token-authenticated routes to
their own guard, a decision for the framework rather than a hole for this
application to cut, so it is reported, not worked around.

This project issues no tokens: the example's JSON client signs in with the
session like a browser, and sends the CSRF token in `X-CSRF-Token`. A resolver needs a table of digests and a policy over
who may issue them, which is application code to write -- and an
`arandu-ecosystem` question first. The generated Resource has no knowledge of
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
