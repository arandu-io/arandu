package feature_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/arandu-io/arandu/bootstrap"
)

// What the cache configuration is for: a deployment that names a shared store
// gets a process holding a connection to it, and one that names none gets no
// connection at all.
//
// The health check is where the difference shows, and it is the difference
// between a probe that reports the deployment and a probe that reports half of
// it.

// probeHealth boots the application the commands boot and asks it the question a
// load balancer asks.
func probeHealth(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()

	migrateWithoutTheStore(t)

	cfg, db, _ := openForTest(t)
	app, err := bootstrap.Build(cfg, db)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := app.Kernel.Boot(context.Background()); err != nil {
		t.Fatalf("Boot: %v", err)
	}
	t.Cleanup(func() { _ = app.Kernel.Shutdown() })

	rec := httptest.NewRecorder()
	app.Kernel.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/_arandu/health", nil))
	return rec
}

// migrateWithoutTheStore builds the schema before the store is pointed at
// something that does not answer.
//
// Every migration command takes a lock, and the lock lives in the cache, so a
// migrate against a dead store is correctly refused -- see
// MigrateIsolated_test.go. What these tests are about is the health probe, and
// they need a migrated database to boot against rather than a migration.
func migrateWithoutTheStore(t *testing.T) {
	t.Helper()

	store, url := os.Getenv("CACHE_STORE"), os.Getenv("REDIS_URL")
	t.Setenv("CACHE_STORE", "memory")
	t.Setenv("REDIS_URL", "")

	if err := bootstrap.Dispatch("migrate", nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	t.Setenv("CACHE_STORE", store)
	t.Setenv("REDIS_URL", url)
}

// TestTheSharedStoreIsOnTheHealthCheck.
//
// A deployment that named a shared store depends on it: the sessions, the rate
// limit and the lock that keeps a scheduled task on one replica all live there.
// A configuration nothing reads leaves the probe green while every replica runs
// on its own, which is the outage that reports itself as healthy.
func TestTheSharedStoreIsOnTheHealthCheck(t *testing.T) {
	sqliteEnv(t)
	t.Setenv("CACHE_STORE", "redis")
	// Port 1 is reserved and nothing listens on it, so the connection is refused
	// at once rather than left to time out.
	t.Setenv("REDIS_URL", "redis://127.0.0.1:1")

	rec := probeHealth(t)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: the configured store does not answer, and nothing said so", rec.Code)
	}
	// The name is the one the bootstrap registers the connection under. It has
	// to appear: a 503 that does not say which module is down sends whoever is
	// on call to read every one of them.
	if !strings.Contains(rec.Body.String(), "cache") {
		t.Errorf("the failing module is not named in %q", rec.Body.String())
	}
}

// TestTheInProcessStoreOpensNoConnection.
//
// The other half of the same guarantee. CACHE_STORE=memory is the single
// replica caching inside itself, and a connection opened anyway would fail a
// probe over a server the deployment never asked for -- with REDIS_URL set,
// because the session configuration reads the same variable.
func TestTheInProcessStoreOpensNoConnection(t *testing.T) {
	sqliteEnv(t)
	t.Setenv("CACHE_STORE", "memory")
	t.Setenv("REDIS_URL", "redis://127.0.0.1:1")

	rec := probeHealth(t)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200. Body:\n%s", rec.Code, rec.Body.String())
	}
}
