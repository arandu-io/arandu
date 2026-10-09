package feature_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/arandu-io/hesape/auth"

	"github.com/arandu-io/arandu/bootstrap"
	appconfig "github.com/arandu-io/arandu/config"
)

// SESSION_DRIVER says where session state is kept, and the proof that it does
// is not that the value parses.
//
// It parsed, it validated, it had an error message of its own -- and the
// bootstrap built the in-process backend whatever it said. A deployment that
// asked for shared sessions got one session store per replica, reported itself
// healthy, and signed half its visitors out on every request. So what is
// checked below is behaviour: the session lands in the store the connector
// opened, and the boot refuses the configurations that cannot deliver one.
//
// That two processes over one real server read one session is the connector's
// claim, and it is proved where the connector is, against a real server:
// TestSessionRoundTrip and TestSigningOutASubjectSpansReplicasAndStopsAtTheTenant
// in github.com/arandu-io/hesape/redis, with the byte-for-byte compatibility of
// the handler this application is given in TestBothHandlersStoreTheGoldenBytes.

// bootedInstance builds and boots one instance of the application.
//
// One instance, built the way the commands build it, and a test may build two of
// them: everything that is per-process -- the session backend among it -- is
// separate between the two, and everything they share, they share through the
// stores the configuration named.
func bootedInstance(t *testing.T) bootstrap.App {
	t.Helper()

	cfg, db, _ := openForTest(t)
	app, err := bootstrap.Build(cfg, db)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := app.Kernel.Boot(context.Background()); err != nil {
		t.Fatalf("Boot: %v", err)
	}
	t.Cleanup(func() { _ = app.Kernel.Shutdown() })
	return app
}

// TestTheSessionIsKeptByTheStoreTheConnectorOpened.
//
// SESSION_DRIVER=redis beside CACHE_STORE=memory, which is the combination this
// wiring exists for: the two settings are independent, because what a cache
// loses to a restart is work and what the sessions lose is everybody who was
// signed in.
//
// A rotation on one instance has to land in the shared store -- as the record
// the store's own handler keeps, payload still encoded, tenant and subject
// beside it -- and a second instance has to read it from there rather than
// from a backend of its own. Each instance opens the store for itself, so the
// only thing the two have in common is the server the endpoint names.
func TestTheSessionIsKeptByTheStoreTheConnectorOpened(t *testing.T) {
	server := borrowRESP(t)
	sqliteEnv(t)
	t.Setenv("CACHE_STORE", "memory")
	t.Setenv("SESSION_DRIVER", "redis")
	t.Setenv("REDIS_URL", server.url())

	if err := bootstrap.Dispatch("migrate", nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	first := bootedInstance(t)
	second := bootedInstance(t)

	const (
		email    = "ana@example.test"
		password = "a-long-enough-password"
	)
	user, err := first.Users.Register(context.Background(), bootstrap.Tenant(), "Ana", email, password)
	if err != nil {
		t.Fatalf("registering: %v", err)
	}

	// UI calls this exact Rotate seam only after every factor succeeds; keeping
	// the storage proof here avoids making the bare skeleton depend on UI that
	// is published later.
	response := httptest.NewRecorder()
	id, err := first.Sessions.Rotate(context.Background(), response, "", user.Subject())
	if err != nil {
		t.Fatalf("rotating the session on the first instance: %v", err)
	}
	if id == "" {
		t.Fatal("the first instance returned an empty session id")
	}

	stored, err := server.sessions.Read(context.Background(), id)
	if err != nil {
		t.Fatalf("the session is not in the shared store, so no other replica can read it: %v", err)
	}
	if stored.Tenant != user.TenantID || stored.SubjectID != user.ID {
		t.Errorf("the shared store indexes the session under %s in %s, want %s in %s",
			stored.SubjectID, stored.Tenant, user.ID, user.TenantID)
	}
	var subject auth.Subject
	if err := json.Unmarshal(stored.Payload, &subject); err != nil {
		t.Fatalf("the stored payload is not the subject as JSON: %v (%s)", err, stored.Payload)
	}
	if subject.ID != user.ID || subject.Tenant != user.TenantID {
		t.Errorf("the stored subject is %s in %s, want %s in %s", subject.ID, subject.Tenant, user.ID, user.TenantID)
	}

	cookies := response.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("the first instance wrote no session cookie")
	}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	loaded, err := second.Sessions.Load(context.Background(), request)
	if err != nil {
		t.Fatalf("loading the first instance's session on the second: %v", err)
	}
	if loaded.ID != user.ID || loaded.Tenant != user.TenantID {
		t.Fatalf("loaded subject = %s in %s, want %s in %s", loaded.ID, loaded.Tenant, user.ID, user.TenantID)
	}
}

// TestTheSessionReachesItsStoreWhateverTheCacheDefaultsTo.
//
// SESSION_DRIVER=redis beside CACHE_STORE=memory, which is the combination the
// nullable connection could not express: there was one connection, the cache
// decided whether it existed, and a session that wanted one where the cache
// wanted none had nothing to use.
//
// It is read off the health check because that is where a resolved store
// becomes visible from outside: the server does not answer, and a probe that
// stayed green would be a probe reporting a deployment that is not the one
// running.
func TestTheSessionReachesItsStoreWhateverTheCacheDefaultsTo(t *testing.T) {
	address, _ := unansweredRESP(t)

	sqliteEnv(t)
	t.Setenv("CACHE_STORE", "memory")
	t.Setenv("SESSION_DRIVER", "redis")
	t.Setenv("REDIS_URL", "redis://"+address)

	migrateBeforeTheSessionIsPointedAtIt(t)

	cfg, db, _ := openForTest(t)
	app, err := bootstrap.Build(cfg, db)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := app.Kernel.Boot(context.Background()); err != nil {
		t.Fatalf("Boot: %v", err)
	}
	t.Cleanup(func() { _ = app.Kernel.Shutdown() })

	if app.Cache == nil {
		t.Fatal("no store was opened: the session named the shared store and nothing resolved it")
	}

	rec := httptest.NewRecorder()
	app.Kernel.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/_arandu/health", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: every session of this deployment lives on a server that does not answer, and nothing said so", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "cache") {
		t.Errorf("the failing module is not named in %q", rec.Body.String())
	}
}

// migrateBeforeTheSessionIsPointedAtIt builds the schema while the session is
// still in the process.
//
// Every migration command takes a lock, and the lock lives in the shared store
// as soon as one is named -- so a migrate against a server that does not answer
// is correctly refused. What this test is about is which store the session
// named, and it needs a migrated database to boot against rather than a
// migration.
func migrateBeforeTheSessionIsPointedAtIt(t *testing.T) {
	t.Helper()

	driver, url := os.Getenv("SESSION_DRIVER"), os.Getenv("REDIS_URL")
	t.Setenv("SESSION_DRIVER", string(appconfig.SessionMemory))
	t.Setenv("REDIS_URL", "")

	if err := bootstrap.Dispatch("migrate", nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	t.Setenv("SESSION_DRIVER", driver)
	t.Setenv("REDIS_URL", url)
}

// TestASessionOverAStoreThisProcessKeepsToItselfIsRefusedAtTheBoot.
//
// Load refuses SESSION_DRIVER=redis without REDIS_URL, and a configuration
// assembled in Go skips Load entirely -- which is every test in this repository
// and every consumer that builds the struct by hand. The refusal that matters
// is the one in the wiring, because it is the one nothing can go around.
//
// It has to refuse rather than fall back. An in-process backend satisfies the
// type it is handed to and none of what was asked for, and the deployment it
// produces reports itself healthy.
//
// A connector is linked first, so the boot gets past the question of whether
// one is in the binary and reaches the question this test asks.
func TestASessionOverAStoreThisProcessKeepsToItselfIsRefusedAtTheBoot(t *testing.T) {
	linkRESP()
	sqliteEnv(t)
	t.Setenv("CACHE_STORE", "memory")
	t.Setenv("REDIS_URL", "")

	cfg, db, _ := openForTest(t)
	cfg.Session.Driver = appconfig.SessionRedis

	if _, err := bootstrap.Build(cfg, db); err == nil {
		t.Fatal("the application started with its sessions in a store no other replica can read")
	} else {
		for _, want := range []string{"SESSION_DRIVER", `"redis"`, "REDIS_URL"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("the refusal does not name %s, and whoever reads it has to guess which two settings disagree: %v", want, err)
			}
		}
	}
}

// TestASharedStoreThatKeepsNoSessionsIsRefusedAtTheBoot.
//
// The connector is what decides whether the shared store keeps sessions, and a
// store that keeps none leaves nowhere to put them but this process -- the
// failure SESSION_DRIVER=redis was written to avoid. So the boot refuses, naming
// the setting, rather than starting with the in-process backend.
func TestASharedStoreThatKeepsNoSessionsIsRefusedAtTheBoot(t *testing.T) {
	server := borrowRESP(t)
	server.keepNoSessions()
	sqliteEnv(t)
	t.Setenv("CACHE_STORE", "memory")
	t.Setenv("SESSION_DRIVER", "redis")
	t.Setenv("REDIS_URL", server.url())

	cfg, db, _ := openForTest(t)
	if _, err := bootstrap.Build(cfg, db); err == nil {
		t.Fatal("the application started with its sessions in a store that cannot keep them")
	} else {
		for _, want := range []string{"SESSION_DRIVER", `"redis"`, "cannot keep sessions"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("the refusal does not name %s: %v", want, err)
			}
		}
	}
}

// TestAnUnknownSessionDriverIsRefusedAtTheBoot.
//
// The same door, from the other side: a driver nobody recognises must not fall
// through to the in-process backend. Falling through is how a typo becomes a
// fleet of replicas each holding its own sessions, with nothing in the log.
func TestAnUnknownSessionDriverIsRefusedAtTheBoot(t *testing.T) {
	sqliteEnv(t)
	t.Setenv("CACHE_STORE", "memory")

	cfg, db, _ := openForTest(t)
	cfg.Session.Driver = "kev"

	if _, err := bootstrap.Build(cfg, db); err == nil {
		t.Fatal("the application started on a session driver it does not implement")
	} else if !strings.Contains(err.Error(), "SESSION_DRIVER") || !strings.Contains(err.Error(), `"kev"`) {
		t.Errorf("the refusal names neither the setting nor the value: %v", err)
	}
}

// A retired spelling must not select shared or in-process sessions implicitly.
func TestRetiredSessionDriverIsRefusedAtTheBoot(t *testing.T) {
	sqliteEnv(t)
	t.Setenv("SESSION_DRIVER", "memory")
	t.Setenv("CACHE_STORE", "memory")
	cfg, db, _ := openForTest(t)
	cfg.Session.Driver = "kv"
	_, err := bootstrap.Build(cfg, db)
	if err == nil {
		t.Fatal("the retired session driver was accepted")
	}
	for _, want := range []string{"SESSION_DRIVER", `"kv"`, "memory", "redis"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("driver refusal %q does not contain %q", err, want)
		}
	}
}
