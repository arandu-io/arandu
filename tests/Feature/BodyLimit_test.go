package feature_test

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	fhttp "github.com/arandu-io/framework/http"
	"github.com/arandu-io/framework/http/middleware"
	"github.com/arandu-io/hesape/config"

	"github.com/arandu-io/arandu/tests"
)

// bodyProbeModule answers one POST by reading the whole body, and reports how
// much of it arrived. It counts how often it ran, so a refusal earlier in the
// pipeline is visible as a handler that never did.
type bodyProbeModule struct{ calls *atomic.Int64 }

func (bodyProbeModule) Name() string { return "body-probe" }

func (m bodyProbeModule) Routes(router *fhttp.Router) {
	router.Post("/body-probe", func(w http.ResponseWriter, r *http.Request) {
		m.calls.Add(1)
		read, err := io.Copy(io.Discard, r.Body)
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
		}
		_, _ = io.WriteString(w, strconv.FormatInt(read, 10))
	})
}

// TestAnOversizedBodyIsRefusedBeforeTheSessionOrCSRF.
//
// The body carries no CSRF token, so anything that reached the CSRF check would
// answer 419. 413 is the proof that the limit stood in front of it, and the
// probe never running is the proof that nothing behind it read the body.
func TestAnOversizedBodyIsRefusedBeforeTheSessionOrCSRF(t *testing.T) {
	t.Setenv("HTTP_MAX_BODY_BYTES", "1024")
	calls := &atomic.Int64{}
	k := tests.Kernel(t, config.EnvDev, bodyProbeModule{calls: calls})

	request := httptest.NewRequest(http.MethodPost, "/body-probe", bytes.NewReader(bytes.Repeat([]byte("a"), 2048)))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	k.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("a 2048-byte body under a 1024-byte limit answered %d, want 413", response.Code)
	}
	if calls.Load() != 0 {
		t.Fatal("the handler ran for a body the limit had refused")
	}

	// A body under the limit is not refused by it: the same request, small,
	// reaches the CSRF check and is turned away there for carrying no token.
	small := httptest.NewRequest(http.MethodPost, "/body-probe", strings.NewReader("name=a"))
	small.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	answer := httptest.NewRecorder()
	k.Handler().ServeHTTP(answer, small)
	if answer.Code != middleware.StatusCSRFExpired {
		t.Fatalf("a small body without a token answered %d, want %d", answer.Code, middleware.StatusCSRFExpired)
	}
}

// TestABodyThatDeclaresNoLengthIsCutOffAtTheLimit.
//
// A chunked body carries no Content-Length, so nothing can refuse it before it
// is read. What protects the process is that reading stops at the limit, and the
// handler is told why.
func TestABodyThatDeclaresNoLengthIsCutOffAtTheLimit(t *testing.T) {
	t.Setenv("HTTP_MAX_BODY_BYTES", "1024")
	calls := &atomic.Int64{}
	k := tests.Kernel(t, config.EnvDev, bodyProbeModule{calls: calls})

	page := httptest.NewRecorder()
	k.Handler().ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
	token := csrfTokenFromPage(t, page.Body.String())

	// io.MultiReader hides the length, so httptest leaves ContentLength at -1,
	// which is what a chunked request looks like to a handler.
	request := httptest.NewRequest(http.MethodPost, "/body-probe",
		io.MultiReader(bytes.NewReader(bytes.Repeat([]byte("a"), 64<<10))))
	request.Header.Set("X-CSRF-Token", token)
	request.Header.Set("Content-Type", "application/octet-stream")
	for _, cookie := range page.Result().Cookies() {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	k.Handler().ServeHTTP(response, request)

	if calls.Load() != 1 {
		t.Fatalf("the handler ran %d times, want 1", calls.Load())
	}
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("the probe answered %d, want 413: it read a body past the limit without being stopped", response.Code)
	}
	read, err := strconv.Atoi(response.Body.String())
	if err != nil {
		t.Fatalf("the probe answered %q", response.Body.String())
	}
	if read > 1024 {
		t.Fatalf("the handler read %d bytes of a body limited to 1024", read)
	}
}

// TestTheDefaultBodyLimitAdmitsAnOrdinaryForm keeps the default from being a
// limit nobody can submit a form under.
func TestTheDefaultBodyLimitAdmitsAnOrdinaryForm(t *testing.T) {
	t.Setenv("HTTP_MAX_BODY_BYTES", "")
	calls := &atomic.Int64{}
	k := tests.Kernel(t, config.EnvDev, bodyProbeModule{calls: calls})

	page := httptest.NewRecorder()
	k.Handler().ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
	token := csrfTokenFromPage(t, page.Body.String())

	request := httptest.NewRequest(http.MethodPost, "/body-probe", bytes.NewReader(bytes.Repeat([]byte("a"), 1<<20)))
	request.Header.Set("X-CSRF-Token", token)
	request.Header.Set("Content-Type", "application/octet-stream")
	for _, cookie := range page.Result().Cookies() {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	k.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK || response.Body.String() != strconv.Itoa(1<<20) {
		t.Fatalf("a 1 MiB body under the default limit answered %d with %q", response.Code, response.Body.String())
	}
}
