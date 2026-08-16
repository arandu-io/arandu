package feature_test

import (
	"context"
	"errors"
	"testing"

	"github.com/arandu-io/arandu/bootstrap"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/events"
	"github.com/arandu-io/framework/security"
)

// The outbox, against a real database. The guarantee is that the event and the
// row it describes commit together or not at all, and that is a claim about a
// transaction -- which is exactly the thing a fake cannot prove.

func migratedDB(t *testing.T) *data.DB {
	t.Helper()
	sqliteEnv(t)

	if err := bootstrap.Dispatch("migrate", nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	_, db, _ := openForTest(t)
	return db
}

func TestTheEventCommitsWithTheWrite(t *testing.T) {
	db := migratedDB(t)
	outbox := events.NewOutbox(db)
	ctx := context.Background()
	g := security.SystemGrant("customer.create", bootstrap.Tenant())

	err := data.Transaction(ctx, db, func(ctx context.Context) error {
		if _, err := db.ExecContext(ctx, `CREATE TABLE customer (id TEXT PRIMARY KEY)`); err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO customer (id) VALUES (?)`, "c-1"); err != nil {
			return err
		}
		return outbox.Store(ctx, g, []events.Event{{
			Name:        "customer.created",
			Aggregate:   "customer",
			AggregateID: "c-1",
			Payload:     map[string]string{"name": "Ana"},
		}})
	})
	if err != nil {
		t.Fatalf("Transaction: %v", err)
	}

	pending, err := outbox.Pending(ctx, bootstrap.Tenant(), 10)
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("%d events pending, want 1", len(pending))
	}

	stored := pending[0]
	if stored.Name != "customer.created" || stored.AggregateID != "c-1" {
		t.Errorf("stored = %+v", stored)
	}
	// The Grant is the audit trail: who authorized it, which action, which
	// tenant, without a second table.
	if stored.Action != "customer.create" || stored.TenantID != bootstrap.Tenant() {
		t.Errorf("the Grant did not reach the row: %+v", stored)
	}

	var payload struct {
		Name string `json:"name"`
	}
	if err := stored.Decode(&payload); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if payload.Name != "Ana" {
		t.Errorf("payload = %+v", payload)
	}
}

// TestARolledBackWriteStoresNoEvent is the half that matters most: without it,
// the rest of the system reacts to something that did not happen.
func TestARolledBackWriteStoresNoEvent(t *testing.T) {
	db := migratedDB(t)
	outbox := events.NewOutbox(db)
	ctx := context.Background()
	failed := errors.New("the rule said no")

	err := data.Transaction(ctx, db, func(ctx context.Context) error {
		if err := outbox.Store(ctx, security.SystemGrant("customer.create", bootstrap.Tenant()), []events.Event{
			{Name: "customer.created", Aggregate: "customer", AggregateID: "c-2"},
		}); err != nil {
			return err
		}
		return failed
	})
	if !errors.Is(err, failed) {
		t.Fatalf("err = %v, want the caller's error", err)
	}

	pending, err := outbox.Pending(ctx, bootstrap.Tenant(), 10)
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("%d events survived a rollback, want 0", len(pending))
	}
}

// TestPublishingMarksTheEvent: at-least-once delivery needs a way to say "this
// one left", or the relay republishes forever.
func TestPublishingMarksTheEvent(t *testing.T) {
	db := migratedDB(t)
	outbox := events.NewOutbox(db)
	ctx := context.Background()

	err := data.Transaction(ctx, db, func(ctx context.Context) error {
		return outbox.Store(ctx, security.SystemGrant("invoice.pay", bootstrap.Tenant()), []events.Event{
			{Name: "invoice.paid", Aggregate: "invoice", AggregateID: "i-1"},
		})
	})
	if err != nil {
		t.Fatalf("Transaction: %v", err)
	}

	pending, err := outbox.Pending(ctx, bootstrap.Tenant(), 10)
	if err != nil || len(pending) != 1 {
		t.Fatalf("Pending: %v, %d events", err, len(pending))
	}

	if err := outbox.MarkFailed(ctx, pending[0].ID, errors.New("the broker refused")); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}
	again, err := outbox.Pending(ctx, bootstrap.Tenant(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 1 || again[0].Attempts != 1 {
		t.Fatalf("a failed attempt was not counted: %+v", again)
	}

	if err := outbox.MarkPublished(ctx, pending[0].ID); err != nil {
		t.Fatalf("MarkPublished: %v", err)
	}
	after, err := outbox.Pending(ctx, bootstrap.Tenant(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 0 {
		t.Fatalf("%d events still pending after publishing, want 0", len(after))
	}
}

// TestAnotherTenantDoesNotSeeTheEvent: the tenant scope reaches the outbox too.
// A relay running for one tenant must not read another's events.
func TestAnotherTenantDoesNotSeeTheEvent(t *testing.T) {
	db := migratedDB(t)
	outbox := events.NewOutbox(db)
	ctx := context.Background()

	err := data.Transaction(ctx, db, func(ctx context.Context) error {
		return outbox.Store(ctx, security.SystemGrant("invoice.pay", bootstrap.Tenant()), []events.Event{
			{Name: "invoice.paid", Aggregate: "invoice", AggregateID: "i-1"},
		})
	})
	if err != nil {
		t.Fatalf("Transaction: %v", err)
	}

	other, err := outbox.Pending(ctx, "22222222-2222-4222-8222-222222222222", 10)
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	if len(other) != 0 {
		t.Fatalf("another tenant read %d events", len(other))
	}
}
