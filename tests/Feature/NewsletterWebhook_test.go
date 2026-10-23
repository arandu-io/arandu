// Example resource. Remove with the list under "The example resource" in README.md.

package feature_test

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/arandu-io/hesape/auth"
	"github.com/arandu-io/hesape/events"
	"github.com/arandu-io/hesape/webhook"

	appevents "github.com/arandu-io/arandu/app/Events"
	requests "github.com/arandu-io/arandu/app/Http/Requests"
	models "github.com/arandu-io/arandu/app/Models"
	services "github.com/arandu-io/arandu/app/Services"
	appconfig "github.com/arandu-io/arandu/config"
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
func receivedEvents(t *testing.T, app tests.Application) []events.Stored {
	t.Helper()
	pending, err := events.NewOutbox(app.DB).Pending(context.Background(), appconfig.Tenant(), 100)
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
	app := tests.Serve(t).App
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
	app := tests.Serve(t).App
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
	app := tests.Serve(t).App
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

// TestADeliveryIsStoredOnceUnderItsID: inside the tolerance a captured
// delivery still verifies, and a provider retries a delivery whose answer it
// never saw. Either way the second copy carries an id this tenant has already
// received: it is answered 200, so a retrying provider stops, and stores
// nothing.
func TestADeliveryIsStoredOnceUnderItsID(t *testing.T) {
	t.Setenv("NEWSLETTER_WEBHOOK_SECRET", newsletterSecret)
	app := tests.Serve(t).App
	body := `{"event":"digest.bounced","digest":"notes-digest-1"}`
	sent := signedDelivery(newsletterSecret, "delivery-once", body, time.Now())

	first := apiCall(t, app, http.MethodPost, "/webhooks/newsletter", body, sent...)
	if first.Code != http.StatusAccepted {
		t.Fatalf("the first delivery answered %d, want 202:\n%s", first.Code, first.Body)
	}
	changed := `{"event":"digest.delivered","digest":"notes-digest-2"}`
	for name, c := range map[string]struct {
		body    string
		headers []string
	}{
		"the same request replayed":          {body, sent},
		"signed again, a minute later":       {body, signedDelivery(newsletterSecret, "delivery-once", body, time.Now().Add(time.Minute))},
		"a different body under the same id": {changed, signedDelivery(newsletterSecret, "delivery-once", changed, time.Now())},
	} {
		again := apiCall(t, app, http.MethodPost, "/webhooks/newsletter", c.body, c.headers...)
		if again.Code != http.StatusOK {
			t.Errorf("%s: answered %d, want 200:\n%s", name, again.Code, again.Body)
		}
	}

	if stored := receivedEvents(t, app); len(stored) != 1 {
		t.Fatalf("%d newsletter events stored for one delivery id, want 1", len(stored))
	}
}

// TestADeliveryIDIsUniquePerTenant: the record of received deliveries is
// scoped by tenant like every other table, so an id one tenant received does
// not refuse the same id for another, and each tenant reads only its own.
func TestADeliveryIDIsUniquePerTenant(t *testing.T) {
	app := tests.Serve(t).App
	ctx := context.Background()
	in := requests.NewsletterEventRequest{Event: "digest.delivered", Digest: "notes-digest-1"}
	const other = "22222222-2222-4222-8222-222222222222"

	if err := services.NewNewsletterEventService(app.DB, appconfig.Tenant()).Receive(ctx, "shared-id", in); err != nil {
		t.Fatalf("the first tenant's delivery: %v", err)
	}
	if err := services.NewNewsletterEventService(app.DB, other).Receive(ctx, "shared-id", in); err != nil {
		t.Fatalf("the same delivery id for another tenant was refused: %v", err)
	}
	err := services.NewNewsletterEventService(app.DB, other).Receive(ctx, "shared-id", in)
	if !errors.Is(err, services.ErrNewsletterDeliveryReplayed) {
		t.Fatalf("the second copy for the same tenant returned %v, want ErrNewsletterDeliveryReplayed", err)
	}

	for _, tenant := range []string{appconfig.Tenant(), other} {
		//arandu:system-grant the test reads one tenant's record of deliveries to prove the scope
		g := auth.SystemGrant(services.NewsletterEventReceive, tenant)
		rows, err := models.NewsletterDeliveries(app.DB).Get(ctx, g)
		if err != nil {
			t.Fatalf("reading %s's deliveries: %v", tenant, err)
		}
		if len(rows) != 1 || rows[0].TenantID != tenant || rows[0].DeliveryID != "shared-id" {
			t.Errorf("tenant %s reads %d deliveries, want its own one", tenant, len(rows))
		}
	}
}
