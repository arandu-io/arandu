// Example resource. Remove with the list under "The example resource" in README.md.

package feature_test

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	frameevents "github.com/arandu-io/framework/events"
	"github.com/arandu-io/hesape/events"
	"github.com/arandu-io/hesape/webhook"

	appevents "github.com/arandu-io/arandu/app/Events"
	"github.com/arandu-io/arandu/bootstrap"
	"github.com/arandu-io/arandu/tests"
)

// newsletterSecret is what the tests configure the provider's secret as. It is
// a test value, not a credential: nothing outside this process knows it.
const newsletterSecret = "test-only-newsletter-webhook-secret-0123456789"

// signedDelivery is the headers the newsletter provider sends with a delivery,
// signed with secret the way github.com/arandu-io/hesape/webhook signs one.
func signedDelivery(secret, id, body string, at time.Time) []string {
	timestamp := strconv.FormatInt(at.Unix(), 10)
	return []string{
		"X-Arandu-Delivery-ID", id,
		"X-Arandu-Timestamp", timestamp,
		"X-Arandu-Signature", webhook.Sign([]byte(secret), timestamp, id, []byte(body)),
	}
}

// receivedEvents reads what the deliveries stored in the outbox.
func receivedEvents(t *testing.T, app bootstrap.App) []events.Stored {
	t.Helper()
	pending, err := frameevents.NewOutbox(app.DB).Pending(context.Background(), bootstrap.Tenant(), 100)
	if err != nil {
		t.Fatalf("reading the outbox: %v", err)
	}
	var found []events.Stored
	for _, e := range pending {
		if e.Name == appevents.NewsletterEventReceivedName {
			found = append(found, e)
		}
	}
	return found
}

// TestASignedDeliveryIsStoredWithNoCSRFToken: the provider posts with no
// session and no CSRF token, the route is exempt from the CSRF check by name,
// and the signature is what admits the delivery -- which is stored for the
// listeners and answered 202.
func TestASignedDeliveryIsStoredWithNoCSRFToken(t *testing.T) {
	t.Setenv("NEWSLETTER_WEBHOOK_SECRET", newsletterSecret)
	app := tests.Booted(t)
	body := `{"event":"digest.delivered","digest":"notes-digest-1"}`

	answer := apiCall(t, app, http.MethodPost, "/webhooks/newsletter", body,
		signedDelivery(newsletterSecret, "delivery-1", body, time.Now())...)
	if answer.Code != http.StatusAccepted {
		t.Fatalf("a signed delivery answered %d, want 202:\n%s", answer.Code, answer.Body)
	}

	stored := receivedEvents(t, app)
	if len(stored) != 1 {
		t.Fatalf("%d newsletter events stored, want 1", len(stored))
	}
	var payload appevents.NewsletterEventReceived
	if err := stored[0].Decode(&payload); err != nil {
		t.Fatalf("decoding the event: %v", err)
	}
	if stored[0].AggregateID != "delivery-1" || payload.Kind != "digest.delivered" || payload.Digest != "notes-digest-1" {
		t.Fatalf("event = %+v, payload = %+v, want delivery-1 reporting notes-digest-1 delivered", stored[0], payload)
	}
}

// TestADeliveryThatIsNotTheProvidersIsRefusedBeforeAnythingIsStored: a wrong
// secret, a changed body, a missing signature and a replay outside the
// tolerance are all 401, and none of them stores anything -- the signature is
// checked before the body is bound.
func TestADeliveryThatIsNotTheProvidersIsRefusedBeforeAnythingIsStored(t *testing.T) {
	t.Setenv("NEWSLETTER_WEBHOOK_SECRET", newsletterSecret)
	app := tests.Booted(t)
	body := `{"event":"digest.bounced","digest":"notes-digest-1"}`

	for name, c := range map[string]struct {
		body    string
		headers []string
	}{
		"signed with another secret": {body, signedDelivery("somebody-else-entirely-0123456789-abcdef", "d-1", body, time.Now())},
		"body changed after signing": {`{"event":"digest.delivered","digest":"notes-digest-1"}`, signedDelivery(newsletterSecret, "d-2", body, time.Now())},
		"no signature at all":        {body, []string{"X-Arandu-Delivery-ID", "d-3", "X-Arandu-Timestamp", strconv.FormatInt(time.Now().Unix(), 10)}},
		"replayed an hour later":     {body, signedDelivery(newsletterSecret, "d-4", body, time.Now().Add(-time.Hour))},
	} {
		answer := apiCall(t, app, http.MethodPost, "/webhooks/newsletter", c.body, c.headers...)
		if answer.Code != http.StatusUnauthorized {
			t.Errorf("%s: answered %d, want 401:\n%s", name, answer.Code, answer.Body)
		}
	}
	if stored := receivedEvents(t, app); len(stored) != 0 {
		t.Fatalf("refused deliveries stored %d events", len(stored))
	}
}

// TestTheWebhookDoesNotExistWithoutASecret: with no secret configured, no
// delivery can be verified, and the route answers every one 404 -- a correctly
// signed one included, since there is nothing to sign it with.
func TestTheWebhookDoesNotExistWithoutASecret(t *testing.T) {
	t.Setenv("NEWSLETTER_WEBHOOK_SECRET", "")
	app := tests.Booted(t)
	body := `{"event":"digest.delivered","digest":"notes-digest-1"}`

	answer := apiCall(t, app, http.MethodPost, "/webhooks/newsletter", body,
		signedDelivery("", "delivery-1", body, time.Now())...)
	if answer.Code != http.StatusNotFound {
		t.Fatalf("a delivery with no secret configured answered %d, want 404:\n%s", answer.Code, answer.Body)
	}
	if stored := receivedEvents(t, app); len(stored) != 0 {
		t.Fatalf("an unconfigured webhook stored %d events", len(stored))
	}
}
