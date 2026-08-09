package feature_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/arandu-io/framework/config"

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
