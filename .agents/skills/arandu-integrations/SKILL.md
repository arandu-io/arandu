---
name: arandu-integrations
description: Reaching other systems from an Arandu (Go) application, and letting them reach it -- the client of an external API with its interface and fake, webhooks sent and received, and tools, resources and prompts an AI assistant calls over MCP. Use when the request is to "call the payment provider", "integrate with X", "send to Slack", "receive a webhook", "verify a signature", "expose this to Claude/an assistant/an agent", "add an MCP tool", or when an http.Client, an API key or app/Clients is involved. Covers aru make:client, make:mcp-tool, make:mcp-resource and make:mcp-prompt.
license: MIT
---

# Other systems

## When to use

A call that leaves the process, a request another system sends in, or a
capability handed to an assistant. The service that decides to make the call
is `arandu-module`; the job or listener that makes it later is `arandu-async`.

## Before you start

- Read the example's client: `app/Clients/NewsletterClient.go`, its fake
  `app/Clients/NewsletterFake.go`, the service that depends on it
  (`NoteService.SendDigest`) and `newsletter` at the end of `bootstrap/app.go`.
- Read the example's received webhook: `NewsletterWebhookController.Store`,
  `NewsletterEventService.Receive`, the `/webhooks/newsletter` route and the
  `middleware.CSRFExcept("/webhooks/")` passed to `CSRFProtect` in
  `bootstrap/app.go`.
- Ask `arandu-ecosystem` first whether the client belongs here at all: a client
  another project would reuse, and an engine that wraps another technology --
  a model runtime, OCR, a camera -- is a `github.com/hyz-is/arandu-*` module.

## Contracts and imports

| piece | contract |
| --- | --- |
| client | `app/Clients/<Vendor>Client.go`: a typed `<Vendor>Config` (its secret kept out of logs and JSON), a small `<Vendor>` interface the callers depend on, and `<Vendor>Client` built with the config and a `*client.Factory` from `github.com/arandu-io/hesape/http/client` |
| fake | `app/Clients/<Vendor>Fake.go`: the interface, answering what it was told and recording `Calls` |
| a request | only through the factory: `c.http.CreatePendingRequest().BaseURL(...)`, which carries a deadline, bounds the body and refuses an address inside the network |
| a caller | a service, a job or a listener, through the interface; never a controller, a model or a view |
| configuration | a `Credential` in `config/services.go`, read from the environment, named in `.env.example` |
| webhook received | `app/Http/Controllers/<Vendor>WebhookController.go`, `Store(ctx)`, under `/webhooks/`: it reads the raw body and checks `webhook.Verify(secrets, timestamp, deliveryID, body, signature)` from `github.com/arandu-io/hesape/webhook` -- the headers are `X-Arandu-Timestamp`, `X-Arandu-Delivery-ID` and `X-Arandu-Signature` -- and the timestamp's age, before it binds or stores anything; then a service records the delivery and the answer is 2xx |
| CSRF | `/webhooks/` is exempt by name, in `bootstrap/app.go`: `middleware.CSRFProtect(csrf, sessions.IDFromRequest, middleware.CSRFExcept("/webhooks/"))`. A route under it has no protection but its own signature check |
| webhook secret | a `Credential` in `config/services.go` (`NEWSLETTER_WEBHOOK_SECRET`), 32 bytes or more, named empty in `.env.example`; with none the controller answers 404 |
| webhook sent | `webhook.NewPublisher(manager, resolver)` is an `events.Publisher`: a listener in `listeners.Each` that delivers outbox events, signed, with retries |
| MCP | `app/Mcp/<Name>.go`: a tool with `Name`, `Description`, `Schema` and `Handle(ctx, r mcp.Request)`, calling a service with `r.Subject()`, from `github.com/arandu-io/mcp` |

## Procedure

1. **Generate the client:** `aru make:client Newsletter` writes the config, the
   interface, the client, the fake and a test that runs the client against a
   faked factory. Add each call to the interface, the client and the fake
   together, in their custom blocks, with types of the client's own so no
   caller reads the vendor's wire format.
2. **Make the call idempotent** when it changes something on the other side:
   carry a key that survives a retry, as `NewsletterDigest.Key` does, sent as
   `Idempotency-Key`.
3. **Depend on the interface.** The service takes `clients.Newsletter`, set
   with `WithNewsletter`; `bootstrap/app.go` builds the real client from the
   configuration, or leaves it nil when nothing is configured, so the
   application runs with no credential and a test passes the fake.
4. **Receive a webhook** under `/webhooks/<vendor>`, in a controller that
   reads the raw body, verifies the signature and the timestamp before
   anything else, binds the request from the same bytes, hands it to a service
   and answers 2xx. The service records the delivery -- the example stores an
   event in the outbox under the delivery id, which the relay hands to the
   listeners after the answer -- so a slow step never times the sender out. A
   job dispatched from the service is the other way to do the work later.
5. **Expose a capability to an assistant** with `aru make:mcp-tool ShowNote
   --service=Note` (`make:mcp-resource`, `make:mcp-prompt` for the other two
   kinds). It writes the type in `app/Mcp`, its test and the wiring; the module
   is taken first with `go get github.com/arandu-io/mcp@v0.4.0`.

## Commands

- `aru make:client <Vendor>`
- `aru make:mcp-tool <Name> --service=<Entity>`, `aru make:mcp-resource <Name> --service=<Entity>`, `aru make:mcp-prompt <Name>`

## Example

Code that calls an external system, and the two stand-ins a test builds for it
-- neither reaches the network:

```go compile
package example

import (
	"context"
	"net/http"

	"github.com/arandu-io/hesape/http/client"

	clients "<module>/app/Clients"
)

// Announce takes the client's interface, never the concrete type, so a test
// hands it the fake.
func Announce(ctx context.Context, newsletter clients.Newsletter, title string) error {
	return newsletter.SendDigest(ctx, clients.NewsletterDigest{
		Key:   "announce-" + title,
		Notes: []clients.NewsletterNote{{Title: title}},
	})
}

// Offline builds the fake, which records the call, and the real client over a
// faked factory, which records the request it would have sent.
func Offline() (*clients.NewsletterFake, *clients.NewsletterClient) {
	fake := &clients.NewsletterFake{}
	factory := client.NewFactory(nil).Fake(func(*http.Request) (*http.Response, error) {
		return client.NewResponseFromBytes(http.StatusAccepted, nil, nil).HTTPResponse(), nil
	})
	faked := clients.NewNewsletterClient(clients.NewsletterConfig{BaseURL: "https://newsletter.example.test"}, factory)
	return fake, faked
}
```

A received webhook, reduced to its order: the signature over the exact
bytes, then the request, then the service, then 202 -- as
`NewsletterWebhookController.Store` does it:

```go compile
package example

import (
	"bytes"
	"io"
	"net/http"

	"github.com/arandu-io/hesape/exception"
	hhttp "github.com/arandu-io/hesape/http"
	"github.com/arandu-io/hesape/webhook"

	requests "<module>/app/Http/Requests"
	services "<module>/app/Services"
)

// Receive refuses a delivery whose signature does not verify before it reads
// a single field of it.
func Receive(ctx *hhttp.Context, secrets webhook.SecretSet, events *services.NewsletterEventService) error {
	body, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		return err
	}
	delivery := ctx.Header("X-Arandu-Delivery-ID")
	if !webhook.Verify(secrets, ctx.Header("X-Arandu-Timestamp"), delivery, body, ctx.Header("X-Arandu-Signature")) {
		return exception.Abort(http.StatusUnauthorized, "this delivery is not signed by the provider")
	}
	ctx.Request.Body = io.NopCloser(bytes.NewReader(body))
	var in requests.NewsletterEventRequest
	if err := ctx.Bind(&in); err != nil {
		return err
	}
	if err := events.Receive(ctx.Ctx(), delivery, in); err != nil {
		return err
	}
	return ctx.Status(http.StatusAccepted)
}
```

The controller also refuses a timestamp more than five minutes from its clock,
which this reduction leaves out: the signature covers the timestamp, so a
captured delivery replayed later is refused even though it still verifies.

An MCP tool, as `aru make:mcp-tool ShowNote --service=Note` writes it. It is
not compiled here, because this project does not require the mcp module:

```go
func (t ShowNote) Handle(ctx context.Context, r mcp.Request) (mcp.Response, error) {
	id, _ := r.String("id")
	found, err := t.svc.Get(ctx, r.Subject(), id)
	if err != nil {
		return mcp.Response{}, err
	}
	return mcp.JSON(resources.NewNoteResource(found)), nil
}
```

## Do not

- Make an HTTP call outside `app/Clients`, or with an `http.Client` of your
  own: `client-outside-clients`. The factory is what bounds it.
- Hand a client a model, a Grant or the session (`client-reaches-the-model`):
  it answers about the other system, and the service decides about ours.
- Put a vendor's SDK behind `app/Services` with a `*Port` or `*Adapter` name:
  the client is the one shape for an external system.
- Let an MCP tool reach a model, a repository or a client: it calls a service
  as who is asking, exactly as a controller does.
- Put a secret in code or in a test. A test gets the fake or a faked factory,
  and a credential comes from the environment.
- Bind, validate or store a webhook's body before its signature verified, or
  exempt a path from CSRF anywhere but the `CSRFExcept` in `bootstrap/app.go`.
  A route under `/webhooks/` that skips the check is a form any site can post.

## Extending it

New calls go in the custom blocks of the client, its interface and its fake --
all three, or the fake stops standing in. A field the config needs goes in the
config struct and in its `LogValue` only if it is not a secret.

## Wiring

- `config/services.go` and `.env.example`: the credential.
- `bootstrap/app.go`: the client built once, from the credential, with
  `client.NewFactory(nil)`, and handed to the services that call it.
- A received webhook: the controller built in `bootstrap/app.go` with its
  secret and its service, handed to the routes through `Deps`, and its route
  under `/webhooks/` in the custom block of `routes/web.go`, with no guard. The
  `CSRFExcept("/webhooks/")` is already there; a vendor under another prefix
  is a second argument to it, never a second exemption elsewhere.
- MCP: the server composed in `bootstrap/app.go` and mounted in
  `routes/web.go` with `r.Action("POST", "/mcp", mcp.Web(d.MCP), ...)` behind a
  guard; there is no `routes/ai.go`. Behind `RequireToken` a client holding a
  personal access token reaches it -- see `arandu-api`.

## Acceptance test

- The generated client test, and one per call: the request it builds against a
  faked factory -- method, path, headers, body -- and its answer to a failure.
- The caller tested with the fake: what it was asked, and what the caller does
  when the fake answers an error.
- No test reaches the network: the fake, or a factory faked with `Fake`,
  answers inside the process, and `PreventStrayRequests(true)` makes a request
  nothing stubbed an error.
- A received webhook, as `tests/Feature/NewsletterWebhook_test.go` does: a
  delivery signed with `webhook.Sign` and no CSRF token answers 2xx and is
  recorded; another secret, a changed body, no signature and an old timestamp
  answer 401 with nothing recorded; no secret configured answers 404.

## Limits

The example verifies the wire format `github.com/arandu-io/hesape/webhook`
sends, which is what another Arandu application delivers. A vendor that signs
another way -- another header, another string signed -- is verified with that
vendor's scheme in its own controller, still before anything else. A delivery
the provider retries is recorded again; whatever acts on it keys on the
delivery id, as every consumer of an at-least-once relay has to.

There is no MCP example in this project, because requiring the mcp module is a
decision for the project that exposes itself.

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
