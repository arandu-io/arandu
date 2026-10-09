// Example resource. Remove with the list under "The example resource" in README.md.

package services

import (
	"context"

	frameevents "github.com/arandu-io/framework/events"
	"github.com/arandu-io/hesape/auth"
	"github.com/arandu-io/hesape/database"
	"github.com/arandu-io/hesape/events"

	appevents "github.com/arandu-io/arandu/app/Events"
	requests "github.com/arandu-io/arandu/app/Http/Requests"
)

// NewsletterEventReceive is the action an event the newsletter provider
// reported is stored under. Nobody asks a policy for it: the provider is not an
// account, and the signature the controller verified is what admitted it.
const NewsletterEventReceive auth.Action = "newsletter.event.receive"

// NewsletterEventService records what the newsletter provider reports about
// the digests NoteService sent it.
//
// It records and does nothing else. The event goes to the outbox, in a
// transaction, and the relay hands it to the listeners in listeners.Each once
// it is committed -- so the request that delivered it is answered as soon as
// the row is written, and whatever the application does about a bounced digest
// runs afterwards, retried by the relay, never inside the provider's timeout.
type NewsletterEventService struct {
	db     *database.DB
	outbox *events.Outbox
	// tenant is the tenant the digests are sent for: the one the scheduler
	// expands the nightly task to.
	tenant string
}

// NewNewsletterEventService wires the service for the tenant the digests are
// sent for.
func NewNewsletterEventService(db *database.DB, tenant string) *NewsletterEventService {
	return &NewsletterEventService{db: db, outbox: frameevents.NewOutbox(db), tenant: tenant}
}

// Receive validates the event and stores it, under the delivery id the
// provider gave it.
func (s *NewsletterEventService) Receive(ctx context.Context, deliveryID string, in requests.NewsletterEventRequest) error {
	if errs := in.Validate(); errs.Any() {
		return errs
	}
	//arandu:system-grant a webhook delivery has no subject; its signature was verified with this deployment's secret, and the tenant is the one the digests are sent for
	g := auth.SystemGrant(NewsletterEventReceive, s.tenant)
	return database.Transaction(ctx, s.db, func(ctx context.Context) error {
		return s.outbox.Store(ctx, g, []events.Event{appevents.NewsletterEventReceived{
			Kind:   in.Event,
			Digest: in.Digest,
		}.Event(deliveryID)})
	})
}
