package feature_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/events"
	"github.com/arandu-io/framework/observability"
	"github.com/arandu-io/framework/security"

	appevents "github.com/arandu-io/arandu/app/Events"
	"github.com/arandu-io/arandu/app/Policies"
	"github.com/arandu-io/arandu/bootstrap"
)

// The relay THIS application wires, from the write to the listener.
//
// Relay_test.go proves what a relay does, against an outbox a test filled in
// itself and a publisher it built for the occasion. Every one of those passes
// over an application that wires no relay at all, which is the state this
// skeleton was in: events.NewModule() brings the outbox table and starts
// nothing, so the auth module wrote a row for every registration into a table no
// process ever read. Nothing failed, which is how it survived.
//
// Nothing below starts the loop, and nothing has to. Start belongs to
// kernel.Background and is called by Kernel.Run, never by Kernel.Boot, so a
// booted-but-not-served application publishes exactly when a test says so. That
// is what makes "published once" a countable claim instead of a race with a
// ticker.

func TestTheApplicationWiresARelayAndItPublishesWhatAuthStored(t *testing.T) {
	app, db := bootedApp(t)
	if app.Relay == nil {
		t.Fatal("the application wired no relay: every event it stores would sit in the outbox unread")
	}

	ctx := context.Background()

	// A registration through the service the application wired, not a row this
	// test wrote: UserService.Register stores the event inside the same
	// transaction as the user, which is the write the outbox exists for.
	if _, err := app.Users.Register(ctx, bootstrap.Tenant(), "Ana", "ana@example.test", "a-long-enough-password"); err != nil {
		t.Fatalf("Register: %v", err)
	}

	outbox := events.NewOutbox(db)
	pending, err := outbox.PendingAll(ctx, 10)
	if err != nil {
		t.Fatalf("PendingAll: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("the registration stored %d events, want 1", len(pending))
	}

	// The listener is this application's own, and it writes the event down. So
	// the log is where a test watches it work, rather than substituting a
	// publisher and proving nothing about the one that is wired.
	seen := &publishedEvents{}
	logged := observability.WithLogger(ctx, slog.New(seen))

	if err := app.Relay.Drain(logged); err != nil {
		t.Fatalf("Drain: %v", err)
	}

	published := seen.all()
	if len(published) != 1 {
		t.Fatalf("the listener was handed %d events, want 1: %v", len(published), published)
	}
	e := published[0]

	if e["event"] != appevents.UserRegistered {
		t.Errorf("the listener was handed %q, want %s", e["event"], appevents.UserRegistered)
	}
	if e["id"] == "" {
		t.Error("the event arrived with no id, which is what a consumer deduplicates on")
	}
	// The Grant was sealed into the row inside the transaction that wrote it, and
	// this is the far end of that: it survived the commit, the poll and the
	// publish. The tenant is what scopes the event and the action is what says
	// which authorization produced it, and they are two claims rather than one.
	if e["tenant"] != bootstrap.Tenant() {
		t.Errorf("the event arrived under tenant %q, want %q", e["tenant"], bootstrap.Tenant())
	}
	if e["action"] != string(policies.ActionUserCreate) {
		t.Errorf("the event arrived with action %q, want %s", e["action"], policies.ActionUserCreate)
	}
	// Registering is authorized for a declared guest, which has no id. An event
	// arriving with a subject here would mean the row carried somebody else's.
	if e["authorized_by"] != "" {
		t.Errorf("the event arrived authorized by %q, want the guest's absent subject", e["authorized_by"])
	}

	// Marked, so the backlog is gone. A pass that published and did not mark is
	// the failure the next assertion is about.
	if left, err := outbox.PendingAll(ctx, 10); err != nil || len(left) != 0 {
		t.Fatalf("%d events are still pending after a pass (%v)", len(left), err)
	}

	// Published once. At-least-once is what a consumer has to tolerate, not what
	// this settles for: a second pass over a marked row delivers nothing.
	if err := app.Relay.Drain(logged); err != nil {
		t.Fatalf("second Drain: %v", err)
	}
	if again := seen.all(); len(again) != 1 {
		t.Errorf("a second pass delivered %d more events", len(again)-1)
	}
}

// TestTheProbeFailsWhileNothingIsDrainingTheOutbox.
//
// The relay is registered WITH the module and not merely built beside it, and
// this is what tells those two apart from outside the process. Health and
// Diagnose both answer nothing when the module holds no relay, so an application
// that built one and still registered events.NewModule() would report itself
// healthy with a backlog behind it -- which is precisely the state the probe
// exists to catch, because a relay that stopped looks exactly like a relay with
// nothing to do.
func TestTheProbeFailsWhileNothingIsDrainingTheOutbox(t *testing.T) {
	app, db := bootedApp(t)
	handler := app.Kernel.Handler()

	probe := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/_arandu/health", nil))
		return rec
	}

	if rec := probe(); rec.Code != http.StatusOK {
		t.Fatalf("the probe answered %d with an empty outbox: %s", rec.Code, rec.Body.String())
	}

	// Older than the threshold, and stored rather than inserted: OccurredAt is a
	// field of the event, so the age is the fixture and the write is still the
	// one production makes.
	ctx := context.Background()
	g := security.SystemGrant("invoice.pay", bootstrap.Tenant())
	err := data.Transaction(ctx, db, func(ctx context.Context) error {
		return events.NewOutbox(db).Store(ctx, g, []events.Event{{
			Name:        "invoice.paid",
			Aggregate:   "invoice",
			AggregateID: "i-1",
			OccurredAt:  time.Now().UTC().Add(-5 * time.Minute),
		}})
	})
	if err != nil {
		t.Fatalf("storing: %v", err)
	}

	rec := probe()
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("the probe answered %d with a five-minute backlog, want 503", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "events") {
		t.Errorf("the probe does not name the module holding the backlog: %q", rec.Body.String())
	}

	if err := app.Relay.Drain(observability.WithLogger(ctx, slog.New(&publishedEvents{}))); err != nil {
		t.Fatalf("Drain: %v", err)
	}
	if rec := probe(); rec.Code != http.StatusOK {
		t.Errorf("the probe still fails after the outbox was drained: %s", rec.Body.String())
	}
}

// publishedEvents keeps the lines the listener wrote.
//
// It reads the attributes and not the rendered text: a check against a formatted
// sentence passes over a missing tenant and fails over a reworded message, which
// is the wrong answer both ways round.
//
// Nothing else logs on this path -- the relay writes only on a failure, and the
// data layer writes nothing -- so an unexpected record is a surprise the counts
// above are meant to report rather than hide.
type publishedEvents struct {
	mu      sync.Mutex
	records []map[string]string
}

// Enabled accepts every level. A handler that filtered would be a second place
// for an assertion to come up empty without saying why.
func (p *publishedEvents) Enabled(context.Context, slog.Level) bool { return true }

// Handle keeps one record's attributes, flattened to strings.
func (p *publishedEvents) Handle(_ context.Context, r slog.Record) error {
	attrs := make(map[string]string, r.NumAttrs())
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.String()
		return true
	})

	p.mu.Lock()
	defer p.mu.Unlock()
	p.records = append(p.records, attrs)
	return nil
}

// WithAttrs returns the handler unchanged: the listener passes its attributes to
// Info inline, so there is nothing to accumulate, and a copy would be a second
// place the records could land.
func (p *publishedEvents) WithAttrs([]slog.Attr) slog.Handler { return p }

// WithGroup returns the handler unchanged, for the reason WithAttrs does.
func (p *publishedEvents) WithGroup(string) slog.Handler { return p }

// all is what was recorded, oldest first.
//
// A copy under the lock, because the relay publishes on the caller's goroutine
// here and on its own under Run, and a test that read the slice directly would
// be the one place in this file that stopped being true in production.
func (p *publishedEvents) all() []map[string]string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]map[string]string(nil), p.records...)
}
