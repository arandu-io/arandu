package main

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arandu-io/framework/config"
)

// There is one way the CSRF token reaches an HTMX request, and it is the
// hx-headers attribute on <body>. The layout `aru make:auth` writes carries it,
// the framework's own sign-in page carries it, and the error page names it by
// that string when a write is rejected.
//
// The skeleton also used to ship resources/js/app.js, which attached the same
// header from a htmx:configRequest listener. Nothing embedded it, nothing served
// it and no layout referenced it, so it never ran -- and had it run, it would
// have been a second answer to a question that already has one (RULE 9). It was
// deleted. This test is what keeps it deleted.

// TestResourcesHoldNoJavaScript: resources/ is the input of `aru view:build`,
// and that build knows two kinds of file -- .kyse.go, which it compiles, and
// .css, which Tailwind reads. A .js in there is not built, not embedded and not
// served; it is a file that looks like behaviour and is not.
func TestResourcesHoldNoJavaScript(t *testing.T) {
	err := filepath.WalkDir("resources", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".js" {
			return nil
		}
		t.Errorf("%s is never built, embedded or served: `aru view:build` compiles .kyse.go and .css, "+
			"and the CSRF token travels in hx-headers on <body>", path)
		return nil
	})
	if err != nil {
		t.Fatalf("walking resources: %v", err)
	}
}

// TestTheOnlyScriptsServedAreTheEmbeddedOnes pins the other half of the claim:
// every <script> a page emits points at the content-addressed assets the
// framework embeds. A tag pointing anywhere else is a 404 or a CDN, and the CSP
// is script-src 'self'.
func TestTheOnlyScriptsServedAreTheEmbeddedOnes(t *testing.T) {
	k := testKernel(t, config.EnvDev)

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
