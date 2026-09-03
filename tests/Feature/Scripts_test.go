package feature_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arandu-io/hesape/config"
	"github.com/arandu-io/hesape/view"

	"github.com/arandu-io/arandu/tests"
)

// TestTheOnlyScriptsServedAreTheEmbeddedOnes pins the other half of the claim:
// every <script> a page emits points at the content-addressed assets the
// framework embeds. A tag pointing anywhere else is a 404 or a CDN, and the CSP
// is script-src 'self'.
func TestTheOnlyScriptsServedAreTheEmbeddedOnes(t *testing.T) {
	k := tests.Kernel(t, config.EnvDev)

	rec := httptest.NewRecorder()
	k.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	body := rec.Body.String()

	for rest := body; ; {
		i := strings.Index(rest, "<script")
		if i < 0 {
			break
		}
		rest = rest[i+len("<script"):]
		end := strings.Index(rest, ">")
		if end < 0 {
			break
		}
		tag := rest[:end]
		rest = rest[end:]

		src, ok := attribute(tag, "src")
		if !ok {
			t.Errorf("the page carries an inline <script>, which the CSP refuses: %q", tag)
			continue
		}
		if !strings.HasPrefix(src, "/_arandu/assets/") {
			t.Errorf("<script src=%q> does not point at an embedded asset", src)
		}
	}
}

// TestThisProjectsScriptIsServedAfterTheFrameworks pins the other end of the
// customisation layer: resources/js/custom.js, which is where an application
// calls arandu.ui.define and arandu.ui.action.
//
// Three things have to hold together and each fails silently on its own.
//
// The asset has to be registered, which is what the import of resources/js in
// bootstrap does -- without it the layout asks for a name nothing registered.
// The bytes served have to be the file on disk, because the script is served
// verbatim: no bundler, no minifier, nothing between the two, so anything else
// arriving means a stale copy is embedded somewhere. And the tag has to come
// after ui.js: both are deferred, deferred scripts run in document order, and
// arandu.ui does not exist until ui.js has run -- a custom.js loaded first
// throws once on a property of undefined and every behaviour it defines is
// simply absent from the page.
func TestThisProjectsScriptIsServedAfterTheFrameworks(t *testing.T) {
	onDisk := []byte(tests.File(t, filepath.Join("resources", "js", "custom.js")))

	// Looked up rather than asked for by name: view.URL refuses an unregistered
	// name by panicking, and a panic in a test reports the stack of the view
	// package instead of what a reader of this file needs to be told.
	var script view.Asset
	for _, a := range view.Assets() {
		if a.Name == "custom.js" {
			script = a
		}
	}
	if script.URL == "" {
		t.Fatalf("custom.js is not a registered asset, so nothing serves resources/js/custom.js.\nbootstrap imports the package that embeds it; without that import the layout's reference to it is refused and no page renders.")
	}

	k := tests.Kernel(t, config.EnvDev)

	served := httptest.NewRecorder()
	k.Handler().ServeHTTP(served, httptest.NewRequest(http.MethodGet, script.URL, nil))
	if served.Code != http.StatusOK {
		t.Fatalf("%s answered %d", script.URL, served.Code)
	}
	if got := served.Body.Bytes(); sum(got) != sum(onDisk) {
		t.Errorf("the browser is served a different script than resources/js/custom.js.\n  served   %s (%d bytes)\n  on disk  %s (%d bytes)",
			sum(got), len(got), sum(onDisk), len(onDisk))
	}
	if ct := served.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/javascript") {
		t.Errorf("custom.js is served as %q, and a browser refuses to execute a script that is not typed as one", ct)
	}

	page := httptest.NewRecorder()
	k.Handler().ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
	body := page.Body.String()

	at := strings.Index(body, `src="`+script.URL+`"`)
	if at < 0 {
		t.Fatalf("no page loads custom.js: the layout carries no <script> pointing at %s.\nThe file is served and nothing fetches it, so every behaviour it registers is dead code.", script.URL)
	}
	if ui := strings.Index(body, view.URL("ui.js")); ui < 0 || ui > at {
		t.Errorf("custom.js is loaded before ui.js.\nBoth are deferred and run in document order, so arandu.ui is undefined when custom.js runs and every behaviour it defines is lost.")
	}
}

// attribute pulls one double-quoted attribute out of a tag body.
func attribute(tag, name string) (string, bool) {
	i := strings.Index(tag, name+`="`)
	if i < 0 {
		return "", false
	}
	rest := tag[i+len(name)+2:]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return "", false
	}
	return rest[:end], true
}
