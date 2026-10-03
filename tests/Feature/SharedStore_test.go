package feature_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arandu-io/hesape/cache"
	"github.com/arandu-io/hesape/session"

	"github.com/arandu-io/arandu/bootstrap"
)

// The shared store this application wires when a setting names the RESP server,
// checked without one.
//
// The project links SQLite and nothing else, so the RESP connector is not in
// this binary and a test that set CACHE_STORE=redis would stop at the boot, at
// the error naming the import -- which is right, and is checked in
// tests/Connectors. What these tests check is the wiring on the other side of
// that import: which settings open the store, what the connector is handed,
// and which consumer ends up over it. So a connector of this file's own stands
// in for the real one, registered under the name the real one registers, the
// way borrowMySQL lends a driver to the one dialect this project does not link.
//
// What it cannot stand in for is the server. Whether SET NX is atomic, whether
// two processes see one session, whether a TLS handshake happens on the wire --
// those are claims about a protocol, and they are proved against a real server
// where the connector is, in github.com/arandu-io/hesape/redis. A fake that
// answered them would prove the fake.

// respDriver is the value CACHE_STORE, SESSION_DRIVER and QUEUE_CONNECTION take
// to ask for the RESP server, and the name its connectors register under.
const respDriver = "redis"

var (
	linkRESPOnce sync.Once
	// fakeRESPLinked reports whether "redis" in the cache registry is the
	// connector below. It is false when the project has linked the real one:
	// registering a second connector under a taken name panics, by design.
	fakeRESPLinked bool
)

// linkRESP registers the fake cache connector unless a real one is linked, and
// reports whether the fake is the one answering. Either way a connector for
// "redis" is linked once it returns, which is all a test that only needs the
// boot to get past the link check asks of it.
func linkRESP() bool {
	linkRESPOnce.Do(func() {
		if slices.Contains(cache.Registered(), respDriver) {
			return
		}
		cache.Register(respConnector{})
		fakeRESPLinked = true
	})
	return fakeRESPLinked
}

// borrowRESP links the fake connector and starts a server for one test, or
// skips when the project has since linked the real connector: what the test
// reads back is the fake's own record of what it was handed, and a real store
// keeps none.
func borrowRESP(t *testing.T) *fakeRESP {
	t.Helper()
	if !linkRESP() {
		t.Skip("redis is linked to the real connector here, and this test reads what the fake one was handed: " +
			"what it checks of the wiring is checked against the real store in github.com/arandu-io/hesape/redis")
	}
	return startRESP(t)
}

// unansweredRESP returns the address of a RESP server that does not answer,
// and the fake behind it -- nil when the project links the real connector, in
// which case the address is a port nothing listens on and the real client is
// refused by the operating system.
func unansweredRESP(t *testing.T) (string, *fakeRESP) {
	t.Helper()
	if !linkRESP() {
		return "127.0.0.1:1", nil
	}
	server := startRESP(t)
	server.fail()
	return server.address, server
}

// respServers is every server a test started, by address. Opening an address no
// test started is what dialling a port nobody listens on is: a server that
// does not answer.
var respServers = struct {
	sync.Mutex
	byAddress map[string]*fakeRESP
}{byAddress: map[string]*fakeRESP{}}

// respSequence names each server, so two tests never share one.
var respSequence atomic.Int64

// fakeRESP is one server: the entries, the locks and the sessions every store
// opened over its address shares, and the endpoints those stores were opened
// over.
type fakeRESP struct {
	address string

	entries  *cache.ArrayStore
	sessions *session.ArrayHandler[json.RawMessage]

	mu         sync.Mutex
	down       bool
	lockHeld   bool
	noSessions bool
	endpoints  []cache.Endpoint
}

func startRESP(t *testing.T) *fakeRESP {
	t.Helper()
	server := newRESP("resp-" + strconv.FormatInt(respSequence.Add(1), 10) + ".test:6379")

	respServers.Lock()
	respServers.byAddress[server.address] = server
	respServers.Unlock()
	t.Cleanup(func() {
		respServers.Lock()
		delete(respServers.byAddress, server.address)
		respServers.Unlock()
	})
	return server
}

func newRESP(address string) *fakeRESP {
	return &fakeRESP{
		address:  address,
		entries:  cache.NewArrayStore(),
		sessions: session.NewArrayHandler[json.RawMessage](),
	}
}

// respServerAt is the server listening on address, or one that does not answer.
func respServerAt(address string) *fakeRESP {
	respServers.Lock()
	defer respServers.Unlock()
	if server, found := respServers.byAddress[address]; found {
		return server
	}
	server := newRESP(address)
	server.down = true
	return server
}

// url is what REDIS_URL says to reach this server.
func (s *fakeRESP) url() string { return "redis://" + s.address }

// fail makes every call answer the way a server that is not there answers.
func (s *fakeRESP) fail() { s.mu.Lock(); s.down = true; s.mu.Unlock() }

// holdLock makes every lock answer that another process holds it.
func (s *fakeRESP) holdLock() { s.mu.Lock(); s.lockHeld = true; s.mu.Unlock() }

// keepNoSessions makes the stores opened over this server stores that keep no
// sessions: shared, and not a session.Keeper.
func (s *fakeRESP) keepNoSessions() { s.mu.Lock(); s.noSessions = true; s.mu.Unlock() }

// opened returns every endpoint a store was opened over, in order.
func (s *fakeRESP) opened() []cache.Endpoint {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.endpoints)
}

// keys returns the keys the server holds, sorted.
func (s *fakeRESP) keys() []string {
	out := make([]string, 0)
	for key := range s.entries.All() {
		out = append(out, key)
	}
	slices.Sort(out)
	return out
}

func (s *fakeRESP) reach() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.down {
		return fmt.Errorf("dial tcp %s: connect: connection refused", s.address)
	}
	return nil
}

// respConnector is the cache connector the fake registers as "redis".
type respConnector struct{}

func (respConnector) Driver() string { return respDriver }

// Open refuses an endpoint with no address, as the real connector does, and
// dials nothing, as the real connector does.
func (respConnector) Open(e cache.Endpoint) (cache.SharedStore, error) {
	if e.Address == "" {
		return nil, errors.New("the endpoint names no address, so there is no server to share the store on")
	}
	server := respServerAt(e.Address)

	server.mu.Lock()
	server.endpoints = append(server.endpoints, e)
	noSessions := server.noSessions
	server.mu.Unlock()

	store := &respStore{server: server}
	if noSessions {
		return sharedOnly{store}, nil
	}
	return store, nil
}

// respStore is one store opened over a fake server.
type respStore struct{ server *fakeRESP }

var (
	_ cache.SharedStore = (*respStore)(nil)
	_ session.Keeper    = (*respStore)(nil)
)

func (s *respStore) Get(ctx context.Context, key string) ([]byte, error) {
	if err := s.server.reach(); err != nil {
		return nil, err
	}
	return s.server.entries.Get(ctx, key)
}

func (s *respStore) Put(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if err := s.server.reach(); err != nil {
		return err
	}
	return s.server.entries.Put(ctx, key, value, ttl)
}

func (s *respStore) Add(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	if err := s.server.reach(); err != nil {
		return false, err
	}
	return s.server.entries.Add(ctx, key, value, ttl)
}

func (s *respStore) Forget(ctx context.Context, key string) error {
	if err := s.server.reach(); err != nil {
		return err
	}
	return s.server.entries.Forget(ctx, key)
}

func (s *respStore) Increment(ctx context.Context, key string, delta int64, ttl time.Duration) (int64, error) {
	if err := s.server.reach(); err != nil {
		return 0, err
	}
	return s.server.entries.Increment(ctx, key, delta, ttl)
}

func (s *respStore) Flush(ctx context.Context, prefix string) error {
	if err := s.server.reach(); err != nil {
		return err
	}
	return s.server.entries.Flush(ctx, prefix)
}

func (s *respStore) AcquireLock(ctx context.Context, key, token string, ttl time.Duration) (bool, error) {
	if err := s.server.reach(); err != nil {
		return false, err
	}
	s.server.mu.Lock()
	held := s.server.lockHeld
	s.server.mu.Unlock()
	if held {
		return false, nil
	}
	return s.server.entries.AcquireLock(ctx, key, token, ttl)
}

func (s *respStore) ReleaseLock(ctx context.Context, key, token string) error {
	if err := s.server.reach(); err != nil {
		return err
	}
	return s.server.entries.ReleaseLock(ctx, key, token)
}

func (s *respStore) Ping(context.Context) error { return s.server.reach() }

func (s *respStore) Close() error { return nil }

func (s *respStore) Sessions() session.Handler[json.RawMessage] { return s.server.sessions }

// sharedOnly is a shared store that keeps no sessions. Embedding the interface
// promotes its methods and nothing else, so Sessions is not among them.
type sharedOnly struct{ cache.SharedStore }

// TestTheSharedStoreIsOpenedOverTheEndpointTheEnvironmentNames.
//
// Every field of REDIS_URL is a setting somebody wrote down, and one that stops
// at the configuration is a setting that silently does nothing: a password the
// connection never sends, a database number nobody selects, a prefix two
// applications on one server do not get. And the store is opened once per
// process -- the cache, the lock and the health check over one store, not three
// beside each other.
func TestTheSharedStoreIsOpenedOverTheEndpointTheEnvironmentNames(t *testing.T) {
	server := borrowRESP(t)
	sqliteEnv(t)
	t.Setenv("CACHE_STORE", "redis")
	t.Setenv("REDIS_URL", "redis://:secret@"+server.address+"/2")
	t.Setenv("CACHE_PREFIX", "shop:cache:")

	if err := bootstrap.Dispatch("migrate", nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	before := len(server.opened())
	app := bootedInstance(t)
	opened := server.opened()

	if app.Cache == nil {
		t.Fatal("CACHE_STORE named the shared store and the application holds none")
	}
	if got := len(opened) - before; got != 1 {
		t.Fatalf("one application opened the store %d times, want once: its consumers have to share one store", got)
	}

	e := opened[len(opened)-1]
	if e.Address != server.address || e.Password != "secret" || e.Database != 2 || e.Prefix != "shop:cache:" {
		t.Errorf("the connector was handed address=%q password=%q database=%d prefix=%q, want what REDIS_URL and CACHE_PREFIX say",
			e.Address, e.Password, e.Database, e.Prefix)
	}
	if e.TLS != nil {
		t.Error("redis:// asked for no encryption and the connector was handed a TLS configuration")
	}

	// A store that answers is a healthy one, which is the other side of
	// TestTheSharedStoreIsOnTheHealthCheck.
	rec := httptest.NewRecorder()
	app.Kernel.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/_arandu/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d with the shared store answering, want 200. Body:\n%s", rec.Code, rec.Body.String())
	}
}

// TestTheQueuePauseIsWrittenWhereTheWorkerReadsIt.
//
// `aru queue:pause` is one process and the worker is another, so the flag has
// to land in a store both can see -- the shared one -- and the command has to
// refuse when there is none, rather than record the pause inside itself and
// report success against a worker that never sees it.
func TestTheQueuePauseIsWrittenWhereTheWorkerReadsIt(t *testing.T) {
	t.Run("over the shared store", func(t *testing.T) {
		server := borrowRESP(t)
		sqliteEnv(t)
		t.Setenv("CACHE_STORE", "redis")
		t.Setenv("REDIS_URL", server.url())
		if err := bootstrap.Dispatch("migrate", nil); err != nil {
			t.Fatalf("migrate: %v", err)
		}

		if err := bootstrap.Dispatch("queue:pause", []string{"database:default"}); err != nil {
			t.Fatalf("queue:pause: %v", err)
		}
		if !slices.ContainsFunc(server.keys(), func(key string) bool {
			return strings.Contains(key, "paused") && strings.HasSuffix(key, "database:default")
		}) {
			t.Errorf("the pause is not in the shared store, so no worker will see it. It holds: %v", server.keys())
		}
	})

	t.Run("with nothing shared", func(t *testing.T) {
		sqliteEnv(t)
		t.Setenv("CACHE_STORE", "memory")
		t.Setenv("REDIS_URL", "")
		if err := bootstrap.Dispatch("migrate", nil); err != nil {
			t.Fatalf("migrate: %v", err)
		}

		if err := bootstrap.Dispatch("queue:pause", []string{"database:default"}); err == nil {
			t.Fatal("queue:pause reported a pause with no store any worker can read")
		}
	})
}
