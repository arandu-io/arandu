package feature_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/arandu-io/hesape/config"

	"github.com/arandu-io/arandu/tests"
)

// remainingFor makes one anonymous request from peer, claiming to be forwarded
// for forwardedFor, and returns what the throttle says is left of the budget it
// was counted in.
//
// The throttle keys an anonymous request on its address, so two requests that
// report the same remaining-minus-one were counted in the same bucket -- which
// is the observable difference between an address that was believed and one
// that was not.
func remainingFor(t *testing.T, handler http.Handler, peer, forwardedFor string) int {
	t.Helper()

	request := httptest.NewRequest(http.MethodGet, "/robots.txt", nil)
	request.RemoteAddr = peer
	if forwardedFor != "" {
		request.Header.Set("X-Forwarded-For", forwardedFor)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("GET /robots.txt answered %d", response.Code)
	}
	remaining, err := strconv.Atoi(response.Header().Get("X-RateLimit-Remaining"))
	if err != nil {
		t.Fatalf("X-RateLimit-Remaining = %q: nothing counted this request", response.Header().Get("X-RateLimit-Remaining"))
	}
	return remaining
}

func proxyEnv(t *testing.T, trusted string) {
	t.Helper()
	t.Setenv("CACHE_STORE", "memory")
	t.Setenv("REDIS_URL", "")
	t.Setenv("TRUSTED_PROXIES", trusted)
}

// TestAForwardedHeaderFromAnUntrustedPeerIsIgnored is the default: nobody is
// trusted, so a client that writes X-Forwarded-For is still counted under the
// address it connected from, and cannot hand itself a fresh budget per request.
func TestAForwardedHeaderFromAnUntrustedPeerIsIgnored(t *testing.T) {
	proxyEnv(t, "")
	handler := tests.Kernel(t, config.EnvDev).Handler()

	first := remainingFor(t, handler, "192.0.2.10:40000", "198.51.100.1")
	second := remainingFor(t, handler, "192.0.2.10:40001", "198.51.100.2")
	if second != first-1 {
		t.Fatalf("remaining went %d then %d: two requests from one peer were counted apart "+
			"because each named a different X-Forwarded-For", first, second)
	}
}

// TestAForwardedHeaderIsIgnoredFromAPeerOutsideTheList: listing a proxy trusts
// that proxy, not every peer that sends the header.
func TestAForwardedHeaderIsIgnoredFromAPeerOutsideTheList(t *testing.T) {
	proxyEnv(t, "10.20.0.0/16")
	handler := tests.Kernel(t, config.EnvDev).Handler()

	first := remainingFor(t, handler, "192.0.2.20:40000", "198.51.100.11")
	second := remainingFor(t, handler, "192.0.2.20:40001", "198.51.100.12")
	if second != first-1 {
		t.Fatalf("remaining went %d then %d: a peer outside TRUSTED_PROXIES chose its own bucket", first, second)
	}
}

// TestATrustedProxyIsBelieved is the deployment behind a load balancer: every
// request arrives from the balancer, and each visitor is counted under the
// address the balancer reports rather than all of them under the balancer's.
func TestATrustedProxyIsBelieved(t *testing.T) {
	proxyEnv(t, "10.30.0.0/16")
	handler := tests.Kernel(t, config.EnvDev).Handler()

	first := remainingFor(t, handler, "10.30.0.5:40000", "198.51.100.21")
	other := remainingFor(t, handler, "10.30.0.5:40001", "198.51.100.22")
	if other != first {
		t.Fatalf("remaining went %d then %d: two visitors behind a trusted proxy shared one budget", first, other)
	}
	again := remainingFor(t, handler, "10.30.0.6:40002", "198.51.100.21")
	if again != first-1 {
		t.Fatalf("the first visitor's second request left %d, want %d: it was not counted under its own address", again, first-1)
	}
}
