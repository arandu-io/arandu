package feature_test

import (
	"context"
	"html"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/arandu-io/framework/modules/auth"

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
// checked below is behaviour: two applications over one store, and a boot that
// refuses the configurations that cannot deliver one.

// respServer is the RESP endpoint the two-instance proof counts in.
//
// REDIS_ADDRESS names a server that is already running; without it the test
// starts redis-server itself, on a port the operating system chose, and stops
// it afterwards. There is no third option and no fake: what is being proved is
// that two processes read one store, and a store that lives inside the test
// process proves the opposite of the thing.
func respServer(t *testing.T) string {
	t.Helper()

	if addr := os.Getenv("REDIS_ADDRESS"); addr != "" {
		return addr
	}

	binary, err := exec.LookPath("redis-server")
	if err != nil {
		t.Skip("no RESP server: set REDIS_ADDRESS to one, or install redis-server so this test can start its own")
	}

	// A port the operating system picked and then released, so two runs of the
	// suite never land on each other.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserving a port: %v", err)
	}
	address := listener.Addr().String()
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatalf("reading the reserved port: %v", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("releasing the reserved port: %v", err)
	}

	// Persistence off in both forms. Nothing written here outlives the test, and
	// a server left to snapshot would drop a dump file into whatever directory
	// `go test` happened to be run from.
	server := exec.Command(binary, "--bind", "127.0.0.1", "--port", port, "--save", "", "--appendonly", "no")
	if err := server.Start(); err != nil {
		t.Skipf("redis-server is on the path and did not start: %v", err)
	}
	t.Cleanup(func() {
		_ = server.Process.Kill()
		_ = server.Wait()
	})

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", address, 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return address
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("redis-server did not answer on %s within ten seconds", address)
	return ""
}

// sharedSessionEnv points the session at a RESP endpoint while leaving the
// cache in the process.
//
// That combination is the one this file exists for. The two settings are
// independent: what a cache loses to a restart is work, and what the sessions
// lose is everybody who was signed in, so a deployment that shares one and not
// the other is making a choice rather than a mistake.
func sharedSessionEnv(t *testing.T, address string) {
	t.Helper()

	sqliteEnv(t)
	t.Setenv("CACHE_STORE", "memory")
	t.Setenv("SESSION_DRIVER", "kv")
	t.Setenv("REDIS_URL", "redis://"+address)
	// A prefix per run, so a server that outlives one test -- the one
	// REDIS_ADDRESS names -- never answers this test with the last run's keys.
	t.Setenv("CACHE_PREFIX", "arandu-test-"+strconv.FormatInt(time.Now().UnixNano(), 36)+":")
}

// bootedInstance builds and boots one instance of the application.
//
// One instance, built the way the commands build it, and the test builds two of
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

// hiddenField reads one hidden input out of a rendered form.
var hiddenField = regexp.MustCompile(`<input type="hidden" name="_csrf" value="([^"]*)"`)

// signIn drives the sign-in form of one instance and returns the cookies a
// browser would be holding afterwards.
//
// Through the form and not through the service, because the service
// authenticates and writes no session at all: the handler is what rotates the
// id and puts it in the store, which is the write this test is about.
func signIn(t *testing.T, handler http.Handler, email, password string) []*http.Cookie {
	t.Helper()

	form := httptest.NewRecorder()
	handler.ServeHTTP(form, httptest.NewRequest(http.MethodGet, "/auth/login", nil))
	if form.Code != http.StatusOK {
		t.Fatalf("GET /auth/login = %d, want 200. Body:\n%s", form.Code, form.Body.String())
	}
	token := hiddenField.FindStringSubmatch(form.Body.String())
	if token == nil {
		t.Fatalf("the sign-in form carries no _csrf field, so nothing below can post to it:\n%s", form.Body.String())
	}

	body := url.Values{
		"email":    {email},
		"password": {password},
		"_csrf":    {html.UnescapeString(token[1])},
	}
	post := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(body.Encode()))
	post.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, post)
	if rec.Code < 300 || rec.Code > 399 {
		t.Fatalf("POST /auth/login = %d, want a redirect. Body:\n%s", rec.Code, rec.Body.String())
	}

	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("the sign-in set no cookie, so there is no session for anything to read")
	}
	return cookies
}

// landingPage asks one instance for the front page, as the holder of these
// cookies.
func landingPage(t *testing.T, handler http.Handler, cookies []*http.Cookie) string {
	t.Helper()

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, c := range cookies {
		r.AddCookie(c)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want 200. Body:\n%s", rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

// TestASessionWrittenByOneInstanceIsReadByTheOther.
//
// The proof the whole wiring exists for, and the only one that could not pass
// while the defect was there: two applications, one store, a sign-in on the
// first and the front page drawn for that person by the second. With the
// session backend built in the process, the second instance draws the guest
// navigation, which is exactly what half the requests behind a load balancer
// used to get.
func TestASessionWrittenByOneInstanceIsReadByTheOther(t *testing.T) {
	address := respServer(t)
	sharedSessionEnv(t, address)

	if err := bootstrap.Dispatch("migrate", nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	first := bootedInstance(t)
	second := bootedInstance(t)

	const (
		email    = "ana@example.test"
		password = "a-long-enough-password"
	)
	if _, err := first.Auth.Register(context.Background(), bootstrap.Tenant(), auth.RegisterRequest{
		Name:                 "Ana",
		Email:                email,
		Password:             password,
		PasswordConfirmation: password,
	}); err != nil {
		t.Fatalf("registering: %v", err)
	}

	// The assertion can tell the two states apart. Without this the test would
	// pass just as well against a page that always drew the signed-in half.
	if guest := landingPage(t, second.Kernel.Handler(), nil); !strings.Contains(guest, "Sign in") {
		t.Fatalf("the anonymous front page does not draw the guest navigation, so nothing below distinguishes anything:\n%s", guest)
	}

	cookies := signIn(t, first.Kernel.Handler(), email, password)
	page := landingPage(t, second.Kernel.Handler(), cookies)

	if !strings.Contains(page, "Sign out") {
		t.Fatalf("the second instance drew the guest navigation for a person the first one signed in: " +
			"the session did not reach a store both of them read")
	}
	if !strings.Contains(page, "Ana") {
		t.Errorf("the second instance read a session and not the subject in it:\n%s", page)
	}
}

// TestTheSessionReachesItsStoreWhateverTheCacheDefaultsTo.
//
// SESSION_DRIVER=kv beside CACHE_STORE=memory, which is the combination the
// nullable connection could not express: there was one connection, the cache
// decided whether it existed, and a session that wanted one where the cache
// wanted none had nothing to use.
//
// It is read off the health check because that is where a resolved store
// becomes visible from outside: the endpoint does not answer, and a probe that
// stayed green would be a probe reporting a deployment that is not the one
// running.
func TestTheSessionReachesItsStoreWhateverTheCacheDefaultsTo(t *testing.T) {
	sqliteEnv(t)
	t.Setenv("CACHE_STORE", "memory")
	t.Setenv("SESSION_DRIVER", "kv")
	// Port 1 is reserved and nothing listens on it, so the connection is refused
	// at once rather than left to time out.
	t.Setenv("REDIS_URL", "redis://127.0.0.1:1")

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
		t.Fatal("no connection was opened: the session named the shared store and nothing resolved it")
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
// as soon as one is named -- so a migrate against an endpoint that does not
// answer is correctly refused. What this test is about is which store the
// session named, and it needs a migrated database to boot against rather than a
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
// Load refuses SESSION_DRIVER=kv without REDIS_URL, and a configuration
// assembled in Go skips Load entirely -- which is every test in this repository
// and every consumer that builds the struct by hand. The refusal that matters
// is the one in the wiring, because it is the one nothing can go around.
//
// It has to refuse rather than fall back. An in-process backend satisfies the
// type it is handed to and none of what was asked for, and the deployment it
// produces reports itself healthy.
func TestASessionOverAStoreThisProcessKeepsToItselfIsRefusedAtTheBoot(t *testing.T) {
	sqliteEnv(t)
	t.Setenv("CACHE_STORE", "memory")
	t.Setenv("REDIS_URL", "")

	cfg, db, _ := openForTest(t)
	cfg.Session.Driver = appconfig.SessionKV

	if _, err := bootstrap.Build(cfg, db); err == nil {
		t.Fatal("the application started with its sessions in a store no other replica can read")
	} else {
		for _, want := range []string{"SESSION_DRIVER", `"kv"`, `"redis"`, "REDIS_URL"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("the refusal does not name %s, and whoever reads it has to guess which two settings disagree: %v", want, err)
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
