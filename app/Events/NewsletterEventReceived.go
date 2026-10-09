// Example resource. Remove with the list under "The example resource" in README.md.

package events

import (
	hevents "github.com/arandu-io/hesape/events"
)

// NewsletterEventReceivedName is the event, in the vocabulary of the domain rather than of the
// database: "newsletter.event_received", never "newsletter_delivery.updated". A consumer that
// has to diff two rows to learn what happened is a consumer coupled to your
// schema.
const NewsletterEventReceivedName = "newsletter.event_received"

// NewsletterEventReceived is what the consumer receives.
//
// Facts that were true when it happened, serialized as JSON. An event that says
// "look it up" is an event that reads a row which has already changed.
//
// It is stored by NewsletterEventService when the newsletter provider reports
// on a digest, with the provider's delivery id as the aggregate id: a delivery
// the provider retries is stored again, and a listener that acts on it keys on
// that id, as every consumer of an at-least-once relay has to.
type NewsletterEventReceived struct {
	// Kind is what the provider reports, such as "digest.delivered". It is
	// not called Event, because Event is the method below.
	Kind string `json:"event"`
	// Digest is the key the digest was sent under, NewsletterDigest.Key.
	Digest string `json:"digest"`
}

// Event turns the payload into the record the outbox stores.
//
// There is no Dispatch and no Publish, and that is the whole design: the service
// that makes the change stores the event in the SAME transaction as the write,
// and the relay publishes it afterwards.
//
//	err := database.Transaction(ctx, s.db, func(ctx context.Context) error {
//		if _, err := newsletterDelivery.Save(ctx, g); err != nil {
//			return err
//		}
//		return s.outbox.Store(ctx, g, []events.Event{appevents.NewsletterEventReceived{
//			// the payload's fields
//		}.Event(newsletterDelivery.ID)})
//	})
//
// Store outside database.Transaction returns ErrNoTransaction on purpose. An
// event stored next to a row that then rolled back is worse than no event, and
// an event stored after the commit is one process crash away from being lost.
func (e NewsletterEventReceived) Event(aggregateID string) hevents.Event {
	return hevents.Event{
		Name:        NewsletterEventReceivedName,
		Aggregate:   "newsletter_delivery",
		AggregateID: aggregateID,
		Payload:     e,
	}
}

// arandu:begin custom
// Anything about this event that the fields do not say: a computed total, a
// MarshalJSON that renames a key for a consumer you do not control.
// arandu:end custom
