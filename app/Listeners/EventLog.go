// Package listeners is what this application does with what it observes.
//
// A listener is an ordinary struct with its collaborators as fields, built and
// handed over in bootstrap/app.go. Nothing here reaches for a global, and
// nothing registers itself: a listener nobody wired is a listener that does not
// run, and that is visible in the wiring rather than in a registry.
package listeners

import (
	"context"

	"github.com/arandu-io/framework/events"
	"github.com/arandu-io/framework/observability"
)

// EventLog is what this application does with a domain event once it has
// committed: it writes it down.
//
// It implements events.Publisher, which is the whole of what the relay needs.
// An application with somewhere else to send events replaces this one value in
// bootstrap/app.go and changes nothing else -- a webhook, a queue, a broker and
// an in-process handler are all this shape from here.
//
// Writing the event down is the floor rather than a placeholder. The three
// events a fresh project stores are auth.user.registered, auth.email.verified
// and auth.password.reset, and each is something somebody has to be able to
// answer a question about months later. A line carrying the tenant and the
// action that authorized the write is that answer, and there is nowhere cheaper
// to put it.
//
// # Delivery is at-least-once
//
// The relay can hand the same event over twice: a publish that succeeded and a
// mark that did not look identical from in here. So whatever a listener does has
// to be safe to do again, and a repeated line is the cheapest shape of that --
// an upsert keyed by Stored.ID and a call carrying an idempotency key are the
// others. Nothing checks it, because a repeat is a property of what the code
// does rather than of what it looks like.
type EventLog struct {
	// A listener's collaborators are fields, set where it is wired. This one has
	// none: the logger arrives on the context, and reaching for a global instead
	// is what makes a listener no test can pin.
}

// NewEventLog returns the listener.
func NewEventLog() *EventLog { return &EventLog{} }

// The relay is built with this value, so a signature that stops matching the
// interface fails here rather than at the line that wires it.
var _ events.Publisher = (*EventLog)(nil)

// Publish writes one committed event down, with the audit trail its row carries.
//
// The three fields after the identity are the ones that cannot be reconstructed
// afterwards: the tenant the event belongs to, the subject whose Grant
// authorized the write, and the action that Grant was minted for. They were
// sealed into the row inside the transaction that produced it, and this is where
// they leave the database.
//
// The payload is deliberately not among them. It is whatever JSON the producer
// stored, and a log aggregator is the wrong home for it. A listener that needs
// it decodes it with Stored.Decode into a struct declared beside itself, never
// one shared with the producer: a consumer compiled against the producer's type
// is a consumer that has to be deployed with it.
//
// An error here means "not delivered": the relay keeps the event, counts the
// attempt with the reason, and parks it once the attempts run out. Returning nil
// over a failure loses the event silently, which is the one outcome the outbox
// exists to prevent. Nothing here can fail, so nothing here returns one.
func (l *EventLog) Publish(ctx context.Context, e events.Stored) error {
	observability.Log(ctx).Info("domain event published",
		"event", e.Name,
		"id", e.ID,
		"aggregate", e.Aggregate,
		"aggregate_id", e.AggregateID,
		"tenant", e.TenantID,
		"authorized_by", e.AuthorizedBy,
		"action", e.Action,
	)
	return nil
}
