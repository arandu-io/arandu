// Example resource. Remove with the list under "The example resource" in README.md.

package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/arandu-io/hesape/http/client"
)

// NewsletterConfig is what the Newsletter client needs, typed. bootstrap/app.go
// reads it from the configuration once and hands it over; nothing here looks a
// setting up again.
type NewsletterConfig struct {
	// BaseURL is where Newsletter answers, with its scheme.
	BaseURL string
	// Token is the credential every request carries, as a bearer token.
	Token string
	// Timeout bounds each attempt. Zero leaves the client's own deadline.
	Timeout time.Duration
}

// LogValue implements slog.LogValuer, so a config passed to a log call, or
// dumped on the debug page, records where the client points and never the
// token.
func (c NewsletterConfig) LogValue() slog.Value {
	return slog.GroupValue(slog.String("base_url", c.BaseURL), slog.Duration("timeout", c.Timeout))
}

// MarshalJSON writes the same fields LogValue does, for the same reason: the
// token stays out of anything encoded.
func (c NewsletterConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"base_url": c.BaseURL, "timeout": c.Timeout.String()})
}

// Newsletter is what a service, a job or a listener depends on: the calls
// this application makes to Newsletter, and nothing else. It is small on
// purpose and declared here, beside the one implementation, so the code that
// calls Newsletter names this interface and a test hands it NewsletterFake.
type Newsletter interface {
	Status(ctx context.Context) (NewsletterStatus, error)

	// arandu:begin custom
	// The next call to Newsletter goes here, and on the client and the fake.

	// SendDigest hands the newsletter one digest to send to its readers.
	SendDigest(ctx context.Context, digest NewsletterDigest) error
	// arandu:end custom
}

// NewsletterStatus is what Status answers: a type of this client's own, so the code
// that calls it never reads Newsletter's wire format.
type NewsletterStatus struct {
	OK bool `json:"ok"`
}

// NewsletterClient talks to Newsletter over hesape/http/client, which is the only way
// a request leaves this application: every attempt carries a deadline, reads a
// bounded body, and refuses an address inside the network.
//
// It holds no Grant, no model and no session. It is reached by the service
// that decided to call it -- after the policy -- and answers with its own
// types, so what Newsletter changes on its side changes this file and no other.
type NewsletterClient struct {
	config NewsletterConfig
	http   *client.Factory
}

// NewNewsletterClient builds the client with its configuration and the factory its
// requests are made from: client.NewFactory(nil) in bootstrap/app.go, and the
// same factory faked in a test that wants to see the request.
func NewNewsletterClient(config NewsletterConfig, http *client.Factory) *NewsletterClient {
	return &NewsletterClient{config: config, http: http}
}

// Compile-time proof that the client is what the code calling it depends on.
var _ Newsletter = (*NewsletterClient)(nil)

// Status asks Newsletter whether it is answering.
func (c *NewsletterClient) Status(ctx context.Context) (NewsletterStatus, error) {
	res, err := c.request().Get(ctx, "/status", nil)
	if err != nil {
		return NewsletterStatus{}, fmt.Errorf("newsletter: %w", err)
	}
	if res.Failed() {
		return NewsletterStatus{}, fmt.Errorf("newsletter: status answered %d", res.Status())
	}
	var out NewsletterStatus
	if err := res.Object(&out); err != nil {
		return NewsletterStatus{}, fmt.Errorf("newsletter: reading the status: %w", err)
	}
	return out, nil
}

// request is every call's starting point: the base address, the credential and
// the deadline, set once rather than at each call.
func (c *NewsletterClient) request() *client.PendingRequest {
	r := c.http.CreatePendingRequest().BaseURL(c.config.BaseURL).AcceptJSON()
	if c.config.Token != "" {
		r = r.WithToken(c.config.Token, "Bearer")
	}
	if c.config.Timeout > 0 {
		r = r.Timeout(c.config.Timeout)
	}
	return r
}

// arandu:begin custom

// NewsletterDigest is one digest: the notes published in a window, as the
// newsletter receives them. It is a type of this client's own, so the code that
// calls it never builds the newsletter's wire format.
type NewsletterDigest struct {
	// Key identifies this digest across retries, and travels as the
	// Idempotency-Key header: a digest sent twice is sent once.
	Key string `json:"-"`
	// Notes are the published notes, newest first.
	Notes []NewsletterNote `json:"notes"`
}

// NewsletterNote is one note of a digest.
type NewsletterNote struct {
	Title       string    `json:"title"`
	PublishedAt time.Time `json:"published_at"`
}

// SendDigest posts the digest to the newsletter.
func (c *NewsletterClient) SendDigest(ctx context.Context, digest NewsletterDigest) error {
	r := c.request()
	if digest.Key != "" {
		r = r.WithHeader("Idempotency-Key", digest.Key)
	}
	res, err := r.Post(ctx, "/digests", digest)
	if err != nil {
		return fmt.Errorf("newsletter: %w", err)
	}
	if res.Failed() {
		return fmt.Errorf("newsletter: the digest answered %d", res.Status())
	}
	return nil
}

// arandu:end custom
