package unit_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arandu-io/arandu/tests"
)

// There is one way the CSRF token reaches an HTMX request, and it is the
// hx-headers attribute on <body>. The layout the starter kit publishes carries it,
// the framework's own sign-in page carries it, and the error page names it by
// that string when a write is rejected.
//
// A resources/js/app.js attaching the same header from a htmx:configRequest
// listener would be a second answer to a question that already has one, and
// nothing embeds, serves or references such a file, so it would never even run.
// This test is what keeps it out.

// TestResourcesHoldNoJavaScript: resources/ is the input of `aru view:build`,
// and that build knows two kinds of file -- .kyse.go, which it compiles, and
// .css, which Tailwind reads. A .js in there is not built, not embedded and not
// served; it is a file that looks like behaviour and is not.
func TestResourcesHoldNoJavaScript(t *testing.T) {
	for _, path := range javaScriptUnder(t, filepath.Join(tests.Root(t), "resources")) {
		t.Errorf("%s is never built, embedded or served: `aru view:build` compiles .kyse.go and .css, "+
			"and the CSRF token travels in hx-headers on <body>", path)
	}
}

// TestTheJavaScriptGuardSeesAPlantedFile is the guard on the guard.
//
// The test above passes on a clean tree and it would pass just as quietly on a
// walk that had stopped reaching anything -- a renamed directory, a swallowed
// error, an extension compared against the wrong string. Planting one file where
// the walk has to find it is what separates "there is no JavaScript here" from
// "nothing looked".
func TestTheJavaScriptGuardSeesAPlantedFile(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "css", "vendor")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	planted := filepath.Join(nested, "behaviour.js")
	if err := os.WriteFile(planted, []byte("(() => {})();\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A stylesheet beside it, because a guard that reported every file would
	// also "find" the planted one.
	if err := os.WriteFile(filepath.Join(nested, "style.css"), []byte(":root{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	found := javaScriptUnder(t, root)
	if len(found) != 1 || found[0] != planted {
		t.Errorf("the walk reported %v, want exactly %s: the guard on resources/ is not looking where it says", found, planted)
	}
}

// javaScriptUnder returns every .js file below a directory.
//
// It is a function rather than a walk inside the test so the same code that
// clears resources/ can be pointed at a tree with a file planted in it. A guard
// checked only against the tree it guards is a guard nothing has ever seen fire.
func javaScriptUnder(t *testing.T, root string) []string {
	t.Helper()

	var found []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && filepath.Ext(path) == ".js" {
			found = append(found, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	return found
}

// TestTheVendorUpdaterCopiesNothingExecutable.
//
// resources/css/basecoat is refreshed by a script, and the script is what
// decides what a refresh puts there. It used to create a js/ directory and copy
// seven upstream scripts into it -- files nothing builds, embeds or serves, and
// which the guard above refuses -- so the repository was one run of its own
// updater away from red, and the run had never happened.
//
// The script is read as commands, with its comments dropped: the prose explains
// why no JavaScript is copied and would otherwise be the thing that fails this.
func TestTheVendorUpdaterCopiesNothingExecutable(t *testing.T) {
	commands := vendorCommands(t)

	stylesheets := false
	for _, line := range commands {
		if strings.Contains(line, ".css") {
			stylesheets = true
		}
		if strings.Contains(line, ".js") {
			t.Errorf("the updater copies JavaScript into resources/: %s", line)
		}
		if strings.HasPrefix(line, "mkdir") && strings.Contains(line, "js") {
			t.Errorf("the updater creates a directory for JavaScript under resources/: %s", line)
		}
	}
	if !stylesheets {
		t.Fatal("the updater copies no stylesheet, so this test read something that is not the updater")
	}

	// The removal stays even though nothing is put back: a checkout where the
	// old updater has already run keeps that directory, and its own test suite
	// stays red until something clears it.
	cleared := false
	for _, line := range commands {
		if strings.HasPrefix(line, "rm ") && strings.Contains(line, "/js") {
			cleared = true
		}
	}
	if !cleared {
		t.Error("the updater no longer removes a js/ left by an earlier version of itself, so a tree that " +
			"ran that version stays refused by the guard above with nothing to clear it")
	}
}

// vendorCommands returns the updater's lines with comments and blanks dropped.
func vendorCommands(t *testing.T) []string {
	t.Helper()

	var commands []string
	for _, line := range strings.Split(tests.File(t, "resources/css/basecoat/vendor.sh"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		commands = append(commands, line)
	}
	if len(commands) == 0 {
		t.Fatal("the updater has no commands in it")
	}
	return commands
}

// TestTheStylesheetDoesNotReadItsOwnOutput is the guard for a build that was
// not a function of its inputs.
//
// Tailwind v4 detects sources by itself on top of whatever @source declares,
// and the walk reaches assets/app.css -- the output of this very file. It read
// the class names back out of the previous build and fed them in again, so
// `aru view:build` twice in a row wrote two different stylesheets and a class
// removed from the last view that used it stayed in for one more build.
//
// source(none) turns the automatic half off and makes the @source lines the
// whole list. Every project generated by `aru new` clones this file, so the
// defect and the fix reach all of them.
func TestTheStylesheetDoesNotReadItsOwnOutput(t *testing.T) {
	source, err := os.ReadFile(filepath.Join(tests.Root(t), "resources", "css", "app.css"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), `@import "tailwindcss" source(none)`) {
		t.Error("resources/css/app.css lets Tailwind detect sources by itself: it will read assets/app.css, its own output")
	}
	if !strings.Contains(string(source), `@source "../views/`) {
		t.Error("with automatic detection off, the views have to be declared, or the stylesheet compiles to nothing")
	}
}

// TestTheViewsCarryNoClientDirectives keeps expression-evaluating directives out
// of the templates, and it is not a style rule.
//
// A directive library compiles the string in an attribute into a function at
// runtime. The Content-Security-Policy this application serves is
// script-src 'self' with no 'unsafe-eval', so that compilation is refused: the
// directive does not misbehave, it throws where it is evaluated. The page still
// answers 200, the markup is still correct, and the behaviour the attribute
// described never runs once. Nothing fails loudly enough to be noticed from a
// browser tab, which is why this is a test and not a review habit.
//
// Client behaviour here is delegation on data-* attributes, in one embedded
// script. An attribute carries data; it is never evaluated.
//
// The scan reads attribute POSITION rather than the text of a file, because the
// text is full of honest look-alikes: x- inside a value is a Tailwind class
// (space-x-4, overflow-x-auto), and an @ at the start of a line is a kyse
// directive (@if, @section, @csrf). Three shapes are directives and all three
// are looked for -- x-on:click="...", its @click="..." shorthand, and the
// :aria-current="..." binding shorthand.
//
// Kyse comments are stripped first. They do not reach the page, and they are
// where this project explains itself: a comment saying an element deliberately
// carries no x-data is telling the truth, and failing it for saying so only
// teaches the next person to write the explanation somewhere nobody finds it.
func TestTheViewsCarryNoClientDirectives(t *testing.T) {
	views := filepath.Join(tests.Root(t), "resources", "views")

	inspected := 0
	err := filepath.WalkDir(views, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		inspected++

		for _, name := range clientDirectives(string(body)) {
			t.Errorf("%s carries %s: the CSP is script-src 'self' with no 'unsafe-eval', so the expression "+
				"in that attribute is never evaluated and the behaviour never runs. Delegate on a data-* attribute instead.",
				path, name)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the views: %v", err)
	}

	// Every claim above is true of an empty tree, so a walk that read nothing
	// would report a clean project having opened no file.
	if inspected == 0 {
		t.Fatal("no view was read, so this test checked nothing")
	}
}

// clientDirectives reports the attribute names in one template that name a
// client-side directive, in the order they appear.
func clientDirectives(source string) []string {
	var found []string
	for _, tag := range tagBodies(stripKyseComments(source)) {
		for _, attr := range attributes(tag) {
			switch {
			case strings.HasPrefix(attr.name, "x-"):
				// The long form, with or without a value: x-cloak carries none.
				found = append(found, attr.name)
			case attr.assigned && (strings.HasPrefix(attr.name, "@") || strings.HasPrefix(attr.name, ":")):
				// The two shorthands. Both are assignments, and requiring the
				// value is what separates @click="..." from @endsection.
				found = append(found, attr.name)
			}
		}
	}
	return found
}

// attribute is one attribute of a tag: its name, and whether a value followed.
type attribute struct {
	name     string
	assigned bool
}

// attributes reads the attributes out of the region between an element name and
// the bracket that closes its tag.
//
// A value is skipped whole rather than parsed, which is the point: a class list
// is a value, and reading it as a row of names is how a guard like this starts
// reporting Tailwind.
func attributes(body string) []attribute {
	var out []attribute

	for i := 0; i < len(body); {
		switch body[i] {
		case ' ', '\t', '\n', '\r', '/', '"', '\'':
			i++
			continue
		}

		start := i
		for i < len(body) && !space(body[i]) && body[i] != '=' && body[i] != '"' && body[i] != '\'' {
			i++
		}
		name := body[start:i]

		assigned := false
		j := i
		for j < len(body) && space(body[j]) {
			j++
		}
		if j < len(body) && body[j] == '=' {
			assigned = true
			j++
			for j < len(body) && space(body[j]) {
				j++
			}
			if j < len(body) && (body[j] == '"' || body[j] == '\'') {
				quote := body[j]
				j++
				for j < len(body) && body[j] != quote {
					j++
				}
				if j < len(body) {
					j++
				}
			} else {
				for j < len(body) && !space(body[j]) {
					j++
				}
			}
			i = j
		}

		if name != "" {
			out = append(out, attribute{name: name, assigned: assigned})
		}
	}
	return out
}

// tagBodies returns the attribute region of every tag: what sits between the
// element name and the bracket that closes the tag.
//
// Anything opening with a byte that cannot start an element name is skipped --
// <!doctype and <!-- among them -- and so is a bare < written as prose.
func tagBodies(source string) []string {
	var bodies []string

	for i := 0; i < len(source); {
		if source[i] != '<' {
			i++
			continue
		}

		j := i + 1
		if j < len(source) && source[j] == '/' {
			j++
		}
		if j >= len(source) || !letter(source[j]) {
			i++
			continue
		}
		for j < len(source) && (letter(source[j]) || digit(source[j]) || source[j] == '-') {
			j++
		}

		end, ok := closingBracket(source, j)
		if !ok {
			// An unterminated tag. Everything after it would be read as one
			// enormous attribute region, so the scan stops instead.
			break
		}
		bodies = append(bodies, source[j:end])
		i = end + 1
	}
	return bodies
}

// closingBracket answers where a tag ends, ignoring a bracket inside a quoted
// value -- the htmx-config meta carries several.
func closingBracket(source string, from int) (int, bool) {
	var quote byte

	for i := from; i < len(source); i++ {
		c := source[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			quote = c
		case '>':
			return i, true
		}
	}
	return 0, false
}

// stripKyseComments removes every {{-- --}} block. They are compiled away and
// never reach a page.
func stripKyseComments(source string) string {
	const opener, closer = "{{--", "--}}"

	var b strings.Builder
	for {
		i := strings.Index(source, opener)
		if i < 0 {
			b.WriteString(source)
			return b.String()
		}
		b.WriteString(source[:i])

		rest := source[i+len(opener):]
		end := strings.Index(rest, closer)
		if end < 0 {
			// Unterminated: the rest of the file is comment.
			return b.String()
		}
		source = rest[end+len(closer):]
	}
}

func space(c byte) bool  { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }
func letter(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }
func digit(c byte) bool  { return c >= '0' && c <= '9' }
