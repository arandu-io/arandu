package assets_test

import (
	"crypto/md5"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/arandu-io/framework/httpx"
	"github.com/arandu-io/framework/view"

	_ "github.com/arandu-io/arandu/assets"
)

// TestTheBrowserGetsThisProjectsStylesheet is the regression guard for a whole
// pipeline whose output nobody consumed.
//
// `aru view:build` ran Tailwind over resources/css/app.css and wrote
// assets/app.css, and nothing embedded it. The browser kept receiving the
// framework's default -- byte for byte, same md5 -- so every class written in a
// view of this project was absent from the page, with no error anywhere.
//
// The check is the md5 of what comes over HTTP against the md5 of the file on
// disk. Anything weaker passes in the broken state: the framework's default is
// also valid CSS, also served with a 200, and also has Tailwind's banner.
func TestTheBrowserGetsThisProjectsStylesheet(t *testing.T) {
	onDisk, err := os.ReadFile("app.css")
	if err != nil {
		t.Fatalf("assets/app.css is missing: it is committed so that `go build` works on a fresh clone: %v", err)
	}

	r := httpx.NewRouter()
	view.NewModule().Routes(r)
	server := httptest.NewServer(r)
	defer server.Close()

	resp, err := http.Get(server.URL + view.URL(view.Stylesheet))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	served, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the stylesheet answered %d", resp.StatusCode)
	}

	if sum(served) != sum(onDisk) {
		t.Errorf("the browser is served a different stylesheet than assets/app.css.\n  served   %s (%d bytes)\n  on disk  %s (%d bytes)\nThis is the framework's default: nothing registered this project's.",
			sum(served), len(served), sum(onDisk), len(onDisk))
	}
}

// TestTheStylesheetCarriesTheClassesTheMarkupRenders guards the other half of
// the same pipeline: not who receives the file, but what is inside it.
//
// Tailwind emits a rule only for a class it read out of a file some @source
// names. resources/css/app.css can name directories of this project, and the
// components are an imported module (ADR 0027) whose source is in the module
// cache -- so this file, 183 KB of it, had zero occurrences of
// `.text-destructive`, which components.Field writes on the error line of every
// form, and zero of `size-3 rounded-full`, which are the theme picker's colour
// swatches: spans with no rule for size, which is a span of no size.
//
// Nothing failed. `aru view:build` reported success and the page rendered. This
// file is committed and is what `aru new` hands to every project, so a stylesheet
// that is quietly missing a class is a defect that ships by being copied.
//
// The two halves are named on purpose. .text-destructive can only come from the
// imported library and .max-w-3xl only from this project's own layout, so the
// test says which of the two sources stopped being read.
func TestTheStylesheetCarriesTheClassesTheMarkupRenders(t *testing.T) {
	css, err := os.ReadFile("app.css")
	if err != nil {
		t.Fatalf("assets/app.css is missing: it is committed so that `go build` works on a fresh clone: %v", err)
	}
	stylesheet := string(css)

	for _, want := range []struct {
		class  string
		drawn  string
		broken string
	}{
		{".text-destructive", "the validation message under a components.Field",
			"a rejected form explains itself in the body colour, so nothing on the screen says which field was refused"},
		{".w-44", "the menu of components.ThemeToggle",
			"the theme menu has no width and collapses onto its trigger"},
		{".size-3", "each colour swatch of components.ThemeToggle",
			"the swatches have no size, so the menu offers six choices with nothing to look at"},
		{".rounded-full", "each colour swatch of components.ThemeToggle",
			"the swatches are squares"},
		{".max-w-3xl", "the page column in resources/views/layouts/app",
			"every page runs the full width of the window with no measure at all"},
	} {
		if !strings.Contains(stylesheet, want.class) {
			t.Errorf("%s is not in the compiled stylesheet, and it is what draws %s: %s.\nRun `aru view:build`, and if that does not put it there, the file it is written in is not one the stylesheet declares as a source.",
				want.class, want.drawn, want.broken)
		}
	}
}

func sum(b []byte) string {
	h := md5.Sum(b)
	return hex.EncodeToString(h[:])
}
