package feature_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arandu-io/arandu/tests"

	"github.com/arandu-io/framework/config"
)

// The two URLs nobody writes a link to and every client asks for anyway.
//
// A browser requests /favicon.ico on its own, and the layout links it by name;
// a crawler requests /robots.txt before anything else. Having the files on disk
// is not enough to answer either: there is no document root here, so a file that
// is not embedded and routed does not exist.

func TestTheFixedPublicPathsAreServed(t *testing.T) {
	k := tests.Kernel(t, config.EnvDev)

	for _, tc := range []struct {
		path        string
		contentType string
	}{
		{"/favicon.ico", "image/x-icon"},
		{"/robots.txt", "text/plain"},
	} {
		rec := httptest.NewRecorder()
		k.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))

		if rec.Code != http.StatusOK {
			t.Errorf("GET %s answered %d, want 200: the file is not embedded or not routed", tc.path, rec.Code)
			continue
		}
		if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, tc.contentType) {
			t.Errorf("GET %s served %q, want %s", tc.path, got, tc.contentType)
		}
		if rec.Body.Len() == 0 {
			t.Errorf("GET %s served an empty body", tc.path)
		}
	}
}

// TestTheLayoutLinksAFaviconThatExists closes the loop the layout opens. It
// emits <link rel="icon" href="/favicon.ico">, and a page that asks for a file
// the application does not answer for is a 404 on every single request.
func TestTheLayoutLinksAFaviconThatExists(t *testing.T) {
	k := tests.Kernel(t, config.EnvDev)

	page := httptest.NewRecorder()
	k.Handler().ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
	href := `href="/favicon.ico"`
	if !strings.Contains(page.Body.String(), href) {
		t.Skipf("the layout no longer links %s; nothing to close the loop on", href)
	}

	icon := httptest.NewRecorder()
	k.Handler().ServeHTTP(icon, httptest.NewRequest(http.MethodGet, "/favicon.ico", nil))
	if icon.Code != http.StatusOK {
		t.Fatalf("the layout links /favicon.ico and the application answers %d", icon.Code)
	}
}
