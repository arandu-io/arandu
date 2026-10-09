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
| webhook received | the signature checked with `webhook.Verify(secrets, timestamp, deliveryID, body, signature)` from `github.com/arandu-io/hesape/webhook`, then stored, then handed to a job; the answer is 2xx |
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
4. **Receive a webhook** in a controller that reads the raw body, verifies the
   signature, stores the delivery, dispatches a job and answers 2xx -- the
   processing is the job's, so a slow step never times the sender out.
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

## Extending it

New calls go in the custom blocks of the client, its interface and its fake --
all three, or the fake stops standing in. A field the config needs goes in the
config struct and in its `LogValue` only if it is not a secret.

## Wiring

- `config/services.go` and `.env.example`: the credential.
- `bootstrap/app.go`: the client built once, from the credential, with
  `client.NewFactory(nil)`, and handed to the services that call it.
- MCP: the server composed in `bootstrap/app.go` and mounted in
  `routes/web.go` with `r.Action("POST", "/mcp", mcp.Web(d.MCP), ...)` behind a
  guard; there is no `routes/ai.go`.

## Acceptance test

- The generated client test, and one per call: the request it builds against a
  faked factory -- method, path, headers, body -- and its answer to a failure.
- The caller tested with the fake: what it was asked, and what the caller does
  when the fake answers an error.
- No test reaches the network: the fake, or a factory faked with `Fake`,
  answers inside the process, and `PreventStrayRequests(true)` makes a request
  nothing stubbed an error.

## Limits

This project cannot receive a webhook, nor serve an MCP client holding a token,
yet: `middleware.CSRFProtect` runs on every write before routing, and refuses a
POST that carries neither a session cookie nor a CSRF token -- which is what a
sender's delivery and a token client's call are. The signature or the token is
the right guard for those routes; letting the pipeline hand them to it is a
framework decision, reported rather than worked around here. There is no MCP
example in this project, because requiring the mcp module is a decision for the
project that exposes itself.

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
