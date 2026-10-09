// Example resource. Remove with the list under "The example resource" in README.md.

package controllers

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/arandu-io/hesape/exception"
	hhttp "github.com/arandu-io/hesape/http"
	"github.com/arandu-io/hesape/webhook"

	requests "github.com/arandu-io/arandu/app/Http/Requests"
	services "github.com/arandu-io/arandu/app/Services"
)

// The headers a signed delivery carries, as github.com/arandu-io/hesape/webhook
// sends them: the signature is computed over the timestamp, the delivery id and
// the body, in that order.
const (
	webhookSignatureHeader = "X-Arandu-Signature"
	webhookTimestampHeader = "X-Arandu-Timestamp"
	webhookDeliveryHeader  = "X-Arandu-Delivery-ID"
)

// webhookTolerance is how far a delivery's timestamp may be from this clock.
// The signature covers the timestamp, so a captured delivery replayed later
// than this is refused even though its signature still verifies.
const webhookTolerance = 5 * time.Minute

// NewsletterWebhookController receives what the newsletter provider posts back
// about the digests it was sent: POST /webhooks/newsletter.
//
// The route carries no session and no CSRF token. bootstrap/app.go exempts
// /webhooks/ from the CSRF check by name, and that is safe only because of what
// this controller does first: it verifies the signature over the raw body, with
// the secret this deployment shares with the provider, before it binds,
// validates or stores anything. A delivery whose signature does not verify
// changes nothing, whatever it says.
//
// With no secret configured the route answers 404 to every delivery: an
// endpoint nobody can sign for does not exist yet.
type NewsletterWebhookController struct {
	Controller

	secrets webhook.SecretSet
	events  *services.NewsletterEventService
	// now is the clock the timestamp is compared with, so a test can move it.
	now func() time.Time
}

// NewNewsletterWebhookController returns the controller over the secret the
// provider signs with -- empty when none is configured -- and the service that
// records what it reports.
func NewNewsletterWebhookController(secret string, events *services.NewsletterEventService) *NewsletterWebhookController {
	c := &NewsletterWebhookController{events: events, now: time.Now}
	if secret != "" {
		c.secrets = webhook.SecretSet{Current: []byte(secret)}
	}
	return c
}

// Store verifies one delivery, records it and answers 202.
//
// The body is read whole first, because the signature is over the exact bytes
// the provider sent; the body limit of the pipeline bounds it. Only then is it
// bound into the request, from those same bytes. What the event means is the
// listeners' business, after the relay hands it over -- this answers as soon as
// the event is stored.
func (c *NewsletterWebhookController) Store(ctx *hhttp.Context) error {
	if len(c.secrets.Current) == 0 {
		return exception.Abort(http.StatusNotFound, "")
	}

	body, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		return err
	}
	timestamp := ctx.Header(webhookTimestampHeader)
	delivery := ctx.Header(webhookDeliveryHeader)
	if delivery == "" || !c.fresh(timestamp) ||
		!webhook.Verify(c.secrets, timestamp, delivery, body, ctx.Header(webhookSignatureHeader)) {
		return exception.Abort(http.StatusUnauthorized, "this delivery is not signed by the newsletter provider")
	}

	ctx.Request.Body = io.NopCloser(bytes.NewReader(body))
	var in requests.NewsletterEventRequest
	if err := ctx.Bind(&in); err != nil {
		return err
	}
	if err := c.events.Receive(ctx.Ctx(), delivery, in); err != nil {
		return err
	}
	return ctx.Status(http.StatusAccepted)
}

// fresh reports whether the timestamp, in Unix seconds, is within the
// tolerance of this clock, on either side.
func (c *NewsletterWebhookController) fresh(timestamp string) bool {
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	gap := c.now().Sub(time.Unix(seconds, 0))
	return gap <= webhookTolerance && gap >= -webhookTolerance
}
