package feature_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/arandu-io/arandu/bootstrap"

	"github.com/arandu-io/framework/arandutest"
	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/events"
	"github.com/arandu-io/framework/security"
)

// The relay against a real database. Everything it claims -- at-least-once,
// parking after repeated failures, the lag that tells a stopped relay from an
// idle one -- is a claim about rows, and a fake would prove none of it.

func storedEvents(t *testing.T, db *data.DB, names ...string) {
	t.Helper()
	outbox := events.NewOutbox(db)
	g := security.SystemGrant("invoice.pay", bootstrap.Tenant())

	err := data.Transaction(context.Background(), db, func(ctx context.Context) error {
		list := make([]events.Event, 0, len(names))
		for i, name := range names {
			list = append(list, events.Event{
				Name: name, Aggregate: "invoice", AggregateID: string(rune('a' + i)),
				Payload: map[string]string{"n": name},
			})
		}
		return outbox.Store(ctx, g, list)
	})
	if err != nil {
		t.Fatalf("storing: %v", err)
	}
}

func TestTheRelayPublishesWhatWasStored(t *testing.T) {
	db := migratedDB(t)
	outbox := events.NewOutbox(db)
	ctx := context.Background()

	storedEvents(t, db, "invoice.paid", "invoice.closed")

	var got arandutest.Collected
	arandutest.DrainOutbox(t, ctx, outbox, &got)

	if len(got.Events) != 2 {
		t.Fatalf("published %d events, want 2: %v", len(got.Events), got.Names())
	}
	// Order is the order they happened. Two events about one aggregate only make
	// sense in sequence.
	if got.Names()[0] != "invoice.paid" || got.Names()[1] != "invoice.closed" {
		t.Errorf("published in the wrong order: %v", got.Names())
	}
	// The Grant travelled with the event: who authorized it, which action.
	if got.Events[0].Action != "invoice.pay" || got.Events[0].TenantID != bootstrap.Tenant() {
		t.Errorf("the audit trail did not survive: %+v", got.Events[0])
	}

	// Published once. A second drain must not deliver them again -- at-least-once
	// is the guarantee, not the goal.
	var again arandutest.Collected
	arandutest.DrainOutbox(t, ctx, outbox, &again)
	if len(again.Events) != 0 {
		t.Errorf("a second pass republished %d events", len(again.Events))
	}
}

// TestAFailedPublishIsRetried: the event stays in the table, and the attempt is
// counted with the reason.
func TestAFailedPublishIsRetried(t *testing.T) {
	db := migratedDB(t)
	outbox := events.NewOutbox(db)
	ctx := context.Background()

	storedEvents(t, db, "invoice.paid")

	refuse := events.PublisherFunc(func(context.Context, events.Stored) error {
		return errors.New("the broker refused")
	})
	relay := events.NewRelay(outbox, refuse, events.RelayOptions{MaxAttempts: 3})

	if err := relay.Drain(ctx); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	pending, err := outbox.PendingAll(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("%d events pending after a failure, want 1", len(pending))
	}
	if pending[0].Attempts != 1 {
		t.Errorf("attempts = %d, want 1", pending[0].Attempts)
	}
	if pending[0].LastError != "the broker refused" {
		t.Errorf("the reason was not stored: %q", pending[0].LastError)
	}
}

// TestAnEventThatKeepsFailingIsParked: a relay stuck on one event stops
// delivering everything behind it, which turns one bad payload into an outage
// of every other event.
func TestAnEventThatKeepsFailingIsParked(t *testing.T) {
	db := migratedDB(t)
	outbox := events.NewOutbox(db)
	ctx := context.Background()

	storedEvents(t, db, "invoice.paid")

	refuse := events.PublisherFunc(func(context.Context, events.Stored) error {
		return errors.New("the payload is malformed")
	})
	relay := events.NewRelay(outbox, refuse, events.RelayOptions{MaxAttempts: 3})

	for range 3 {
		if err := relay.Drain(ctx); err != nil {
			t.Fatalf("Drain: %v", err)
		}
	}

	if pending, err := outbox.PendingAll(ctx, 10); err != nil || len(pending) != 0 {
		t.Fatalf("the event is still being retried: %v, %d pending", err, len(pending))
	}

	parked, err := outbox.Parked(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(parked) != 1 {
		t.Fatalf("%d events parked, want 1", len(parked))
	}
	if parked[0].LastError != "the payload is malformed" {
		t.Errorf("the parked event does not say why: %q", parked[0].LastError)
	}

	// A parked event must not block what came after it.
	storedEvents(t, db, "invoice.closed")
	var got arandutest.Collected
	arandutest.DrainOutbox(t, ctx, outbox, &got)
	if len(got.Events) != 1 || got.Events[0].Name != "invoice.closed" {
		t.Errorf("the parked event blocked the queue: %v", got.Names())
	}
}

// TestRetryPutsAParkedEventBackInLine: without it the only way out of a dead
// letter queue is SQL by hand, which is how it becomes a table nobody touches.
func TestRetryPutsAParkedEventBackInLine(t *testing.T) {
	db := migratedDB(t)
	outbox := events.NewOutbox(db)
	ctx := context.Background()

	storedEvents(t, db, "invoice.paid")

	refuse := events.PublisherFunc(func(context.Context, events.Stored) error {
		return errors.New("the consumer was down")
	})
	relay := events.NewRelay(outbox, refuse, events.RelayOptions{MaxAttempts: 1})
	if err := relay.Drain(ctx); err != nil {
		t.Fatal(err)
	}

	parked, err := outbox.Parked(ctx, 10)
	if err != nil || len(parked) != 1 {
		t.Fatalf("Parked: %v, %d", err, len(parked))
	}

	// The operator fixed the consumer.
	if err := outbox.Retry(ctx, parked[0].ID); err != nil {
		t.Fatalf("Retry: %v", err)
	}

	var got arandutest.Collected
	arandutest.DrainOutbox(t, ctx, outbox, &got)
	if len(got.Events) != 1 {
		t.Fatalf("the retried event was not published: %v", got.Names())
	}
	if got.Events[0].Attempts != 0 {
		t.Errorf("the attempt count was not reset: %d", got.Events[0].Attempts)
	}
}

// TestTheLagTellsAStoppedRelayFromAnIdleOne is the number the health check is
// built on. Without it, the first sign that publishing stopped is a customer
// asking why they never got the email.
func TestTheLagTellsAStoppedRelayFromAnIdleOne(t *testing.T) {
	db := migratedDB(t)
	outbox := events.NewOutbox(db)
	ctx := context.Background()

	// Nothing pending: zero, not an error and not a large number.
	if lag, err := outbox.Lag(ctx); err != nil || lag != 0 {
		t.Fatalf("an empty outbox reports lag %s (%v)", lag, err)
	}

	storedEvents(t, db, "invoice.paid")

	lag, err := outbox.Lag(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if lag <= 0 || lag > time.Minute {
		t.Fatalf("lag = %s, want a small positive duration", lag)
	}

	var got arandutest.Collected
	arandutest.DrainOutbox(t, ctx, outbox, &got)

	// Published, so the backlog is gone and the lag with it.
	if lag, err := outbox.Lag(ctx); err != nil || lag != 0 {
		t.Fatalf("after draining, lag = %s (%v)", lag, err)
	}
}

// TestParkedEventsDoNotCountAsLag: a parked event waits forever by definition,
// and counting it would make the health check fail until somebody deletes a row.
func TestParkedEventsDoNotCountAsLag(t *testing.T) {
	db := migratedDB(t)
	outbox := events.NewOutbox(db)
	ctx := context.Background()

	storedEvents(t, db, "invoice.paid")
	refuse := events.PublisherFunc(func(context.Context, events.Stored) error {
		return errors.New("no")
	})
	if err := events.NewRelay(outbox, refuse, events.RelayOptions{MaxAttempts: 1}).Drain(ctx); err != nil {
		t.Fatal(err)
	}

	if lag, err := outbox.Lag(ctx); err != nil || lag != 0 {
		t.Fatalf("a parked event counts as lag: %s (%v)", lag, err)
	}
}
