package feature_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/arandu-io/arandu/bootstrap"
)

// The limit the pipeline declares, per minute and per caller.
//
// Written out here rather than read from anywhere, because a test that asked
// the code what the limit was would agree with it whatever it became. If this
// number and bootstrap/app.go disagree, one of them is a decision somebody took
// without the other.
const requestsPerMinute = 300

// countedRequest asks one instance for a page that costs nothing to render and
// returns what it answered.
//
// Any route does: the throttle is global middleware, so it counts before the
// router has chosen anything. The cheapest one keeps the loop below to the
// thing being measured.
func countedRequest(t *testing.T, handler http.Handler) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/robots.txt", nil))
	return rec
}

// TestTwoInstancesOverOneStoreAreOneBudget.
//
// The whole reason the counter moved out of the process. Counting in memory
// meant N replicas allowed N times the limit, on the endpoints a limit is put
// there for -- and nothing about the deployment said so, because each replica
// was enforcing the number it was configured with.
//
// One instance spends the budget and the other is the one refused. There is no
// weaker version of this: a single instance going over its own limit passes
// just as well against a counter that never left the process.
func TestTwoInstancesOverOneStoreAreOneBudget(t *testing.T) {
	address := respServer(t)

	sqliteEnv(t)
	t.Setenv("CACHE_STORE", "redis")
	t.Setenv("REDIS_URL", "redis://"+address)
	t.Setenv("CACHE_PREFIX", "arandu-test-"+strconv.FormatInt(time.Now().UnixNano(), 36)+":")

	if err := bootstrap.Dispatch("migrate", nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	first := bootedInstance(t)
	second := bootedInstance(t)

	// The budget, spent entirely on the first instance. Both are asked as the
	// same anonymous caller, which is what makes them one caller: the key is the
	// peer address, and httptest hands every request the same one.
	for i := range requestsPerMinute {
		if rec := countedRequest(t, first.Kernel.Handler()); rec.Code != http.StatusOK {
			t.Fatalf("request %d of the budget answered %d, and the budget is %d", i+1, rec.Code, requestsPerMinute)
		}
	}

	rec := countedRequest(t, second.Kernel.Handler())
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("the second instance answered %d after the first spent the whole budget: "+
			"the two replicas are counting apart, and the limit is worth %d times what it says", rec.Code, 2)
	}

	// The refusal is one a person can act on. htmx swaps no 4xx, so without
	// HX-Refresh somebody presses the button, the screen does not change, and
	// the limit reads as a broken page. The header is added to the answer that
	// needs it and to no other, so it is asked for the way a browser would ask.
	boosted := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/robots.txt", nil)
	request.Header.Set("HX-Request", "true")
	second.Kernel.Handler().ServeHTTP(boosted, request)

	if boosted.Code != http.StatusTooManyRequests {
		t.Fatalf("the boosted request answered %d, want 429", boosted.Code)
	}
	if boosted.Header().Get("HX-Refresh") == "" {
		t.Error("the refusal carries no HX-Refresh, so over the limit is a button that does nothing")
	}

	if got := rec.Header().Get("Retry-After"); got == "" {
		t.Error("the refusal says to wait and does not say how long")
	}
	if got := rec.Header().Get("X-RateLimit-Limit"); got != strconv.Itoa(requestsPerMinute) {
		t.Errorf("X-RateLimit-Limit = %q, want %d", got, requestsPerMinute)
	}
}

// TestTheLimitIsCountedAndReportedOnEveryAnswer.
//
// The headers are not decoration: a client that has to back off reads them, and
// they are the only way to tell "this application has a limit" from "this
// application had one and it never counted". This one needs no server, so the
// budget being counted at all is checked wherever the suite runs.
func TestTheLimitIsCountedAndReportedOnEveryAnswer(t *testing.T) {
	sqliteEnv(t)
	t.Setenv("CACHE_STORE", "memory")
	t.Setenv("REDIS_URL", "")

	if err := bootstrap.Dispatch("migrate", nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	app := bootedInstance(t)

	first := countedRequest(t, app.Kernel.Handler())
	if first.Code != http.StatusOK {
		t.Fatalf("the first request answered %d", first.Code)
	}
	if got := first.Header().Get("X-RateLimit-Limit"); got != strconv.Itoa(requestsPerMinute) {
		t.Fatalf("X-RateLimit-Limit = %q, want %d: nothing counted this request", got, requestsPerMinute)
	}

	remaining, err := strconv.Atoi(first.Header().Get("X-RateLimit-Remaining"))
	if err != nil {
		t.Fatalf("X-RateLimit-Remaining = %q: %v", first.Header().Get("X-RateLimit-Remaining"), err)
	}
	if remaining != requestsPerMinute-1 {
		t.Fatalf("X-RateLimit-Remaining = %d after one request, want %d", remaining, requestsPerMinute-1)
	}

	// It goes down. A counter reporting the same number on every answer is a
	// counter that is not counting, and the header alone cannot tell them apart.
	second := countedRequest(t, app.Kernel.Handler())
	next, err := strconv.Atoi(second.Header().Get("X-RateLimit-Remaining"))
	if err != nil {
		t.Fatalf("X-RateLimit-Remaining = %q: %v", second.Header().Get("X-RateLimit-Remaining"), err)
	}
	if next != remaining-1 {
		t.Fatalf("X-RateLimit-Remaining went %d then %d: the budget is not being spent", remaining, next)
	}
}
