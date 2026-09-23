package feature_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arandu-io/arandu/tests"

	"github.com/arandu-io/hesape/config"
)

// Fixed browser assets and native GEO documents are real routes in the binary.
// The favicon remains an embedded public file; crawler and model-facing paths
// belong to the GEO module so disabling indexing fails closed instead of
// leaving a permissive stale robots.txt on disk.
func TestTheFixedPublicPathsAreServed(t *testing.T) {
	k := tests.Kernel(t, config.EnvDev)

	for _, tc := range []struct {
		path        string
		contentType string
	}{
		{"/favicon.ico", "image/x-icon"},
		{"/robots.txt", "text/plain"},
		{"/sitemap.xml", "application/xml"},
		{"/llms.txt", "text/plain"},
		{"/llms-full.txt", "text/plain"},
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

func TestGeoDefaultsFailClosedAndOwnTheirRoutes(t *testing.T) {
	k := tests.Kernel(t, config.EnvDev)
	for _, tc := range []struct {
		path string
		want string
	}{
		{"/robots.txt", "Disallow: /"},
		{"/sitemap.xml", "<urlset"},
		{"/llms.txt", "Discovery disabled"},
		{"/llms-full.txt", "Discovery disabled"},
	} {
		rec := httptest.NewRecorder()
		k.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), tc.want) {
			t.Fatalf("GET %s = %d %q", tc.path, rec.Code, rec.Body.String())
		}
		if tc.path == "/sitemap.xml" && strings.Contains(rec.Body.String(), "<loc>") {
			t.Fatal("the generated project indexes a page before GEO indexing is enabled")
		}

		matches := 0
		for _, route := range k.Routes() {
			if route.Pattern != tc.path {
				continue
			}
			matches++
			if route.Module != "geo" {
				t.Fatalf("%s belongs to %q, want geo", tc.path, route.Module)
			}
		}
		if matches != 1 {
			t.Fatalf("%s registered %d times", tc.path, matches)
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
