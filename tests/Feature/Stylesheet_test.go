package feature_test

import (
	"crypto/md5"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	fhttp "github.com/arandu-io/framework/http"
	"github.com/arandu-io/framework/view"

	_ "github.com/arandu-io/arandu/assets"
	"github.com/arandu-io/arandu/tests"
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
	onDisk := []byte(tests.File(t, filepath.Join("assets", "app.css")))

	r := fhttp.NewRouter()
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
// names. resources/css/app.css can name directories of this project, but the
// components are an imported module whose source is in the module cache -- so a
// stylesheet can be 183 KB and still hold zero occurrences of
// `.text-destructive`, which components.Field writes on the error line of every
// form, and zero of `size-3 rounded-full`, which are the theme picker's colour
// swatches: spans with no rule for size, which is a span of no size.
//
// Nothing fails when that happens. The build reports success and the page
// renders. This file is committed and is what `aru new` hands to every project,
// so a stylesheet quietly missing a class is a defect that ships by being
// copied.
//
// The two halves are checked differently, on purpose. A class that can only
// come from the imported library is named here, because naming it is the only
// way to say which source stopped being read. A class from this project's own
// views is READ OUT OF THE LAYOUT instead of named: the starter kit replaces
// that file, so a hardcoded ".max-w-3xl" passes here and fails in a project that
// installed the kit -- the width the kit draws with is not the one this layout
// draws with, and Tailwind emits only what it read. A test that has to be edited
// whenever a view changes is a test that gets edited into passing.
func TestTheStylesheetCarriesTheClassesTheMarkupRenders(t *testing.T) {
	stylesheet := tests.File(t, filepath.Join("assets", "app.css"))

	// From the imported component library, which is the half that goes missing
	// silently: its source is in the module cache and nothing in
	// resources/css/app.css names it.
	for _, want := range []struct {
		class  string
		drawn  string
		broken string
	}{
		{".text-destructive", "the validation message under a components.Field",
			"a rejected form explains itself in the body colour, so nothing on the screen says which field was refused"},
		{".w-40", "the menu of components.ThemeToggle",
			"the theme menu has no width and collapses onto its trigger"},
		{".size-4", "the icon beside each option of components.ThemeToggle",
			"the icons have no size, so a menu of three modes shows three words and no marks"},
		{".tabular-nums", "the figures column of a components.StatCard",
			"the figures are set in proportional digits, so a column of them does not line up and shifts width as the values change"},
	} {
		if !strings.Contains(stylesheet, want.class) {
			t.Errorf("%s is not in the compiled stylesheet, and it is what draws %s: %s.\nThe imported component library is not being read as a source -- see ADR 0037.",
				want.class, want.drawn, want.broken)
		}
	}

	// And from this project's own layout, whatever it currently is.
	layout := filepath.Join("resources", "views", "layouts", "app.kyse.go")
	classes := utilities(tests.File(t, layout))
	if len(classes) < 5 {
		t.Fatalf("only %d plain utility classes were found in %s; this test cannot say anything", len(classes), layout)
	}
	for _, class := range classes {
		if !strings.Contains(stylesheet, "."+class) {
			t.Errorf("%s renders class %q and the compiled stylesheet has no rule for it.\nRun `aru view:build`; if that does not put it there, this project's own views are not declared as a source in resources/css/app.css.",
				layout, class)
		}
	}
}

// utilities pulls the plain class names out of markup.
//
// Plain on purpose: a variant (sm:flex), an arbitrary value (w-[3px]) and an
// interpolated one are all things this test cannot reason about without
// reimplementing Tailwind's scanner, and a test that guesses is a test that
// fails for the wrong reason.
func utilities(markup string) []string {
	seen := map[string]bool{}
	var out []string
	for _, attr := range classAttr.FindAllStringSubmatch(markup, -1) {
		for _, name := range strings.Fields(attr[1]) {
			if plainClass.MatchString(name) && !seen[name] {
				seen[name] = true
				out = append(out, name)
			}
		}
	}
	return out
}

var (
	classAttr  = regexp.MustCompile(`class="([^"{}]*)"`)
	plainClass = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)
)

func sum(b []byte) string {
	h := md5.Sum(b)
	return hex.EncodeToString(h[:])
}
