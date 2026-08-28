package unit_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/arandu-io/arandu/tests"
)

// This file is the guard for a defect that shipped, and the shape of it is worth
// stating before the code: a test that lives here runs in two places.
//
// It runs in this repository, where the views are the ones this repository
// ships. And it is copied into every project generated from this skeleton, where
// the views are the first thing the owner replaces. A test that reads what a
// view draws therefore passes here and fails there -- in somebody else's
// project, on their first run, before they have written a line.
//
// That is not hypothetical. The proof that two instances share a session read
// the front page and looked for the sign-in link and the sign-out button. Both
// are drawn because the layout in resources/views draws them. In a generated
// project the front page carries neither, and the proof failed on its first
// guard while the same test passed here.
//
// The division the guard enforces is one of ownership: a page is the
// application's, and the application replaces it. What the framework guarantees
// -- a status code, a header, a redirect from a guard, a refusal at boot -- is
// the same in every project that registers the module, whatever the views say.
// So a test in this skeleton asserts on the second and never on the first.

// visibleText is the text a view draws between two tags: >Sign out<.
//
// It is deliberately narrower than every string a view file holds. An attribute
// value, a class name or a route is not what this guard is about -- a test may
// legitimately name a URL the framework routes. What it is about is the words a
// reader sees, because those are the words an owner rewrites.
var visibleText = regexp.MustCompile(`>([A-Z][A-Za-z][A-Za-z ]{2,28})<`)

// testLiteral is a double-quoted string in Go source.
var testLiteral = regexp.MustCompile(`"([^"\\]{3,40})"`)

// TestNoSkeletonTestAssertsOnWhatAViewDraws.
//
// It reads the phrases the views draw, then reads every test in this tree, and
// refuses a test that carries one of those phrases as a string literal.
//
// # What it does not catch, said plainly
//
// A single word. "Register" is a button here and an action name in a module's
// tests, and a guard that refused both would be deleted the first week. Only a
// phrase of two words or more is taken, because that is where a view label
// stops being ordinary vocabulary.
//
// A phrase built at run time, or one that reaches the assertion through a
// variable. This reads literals in source, not values.
//
// And it says nothing about tests that read a page without naming its words --
// counting elements, or asserting a page renders at all. Those couple to the
// views too, and the second reading catches them: plans/prova-ponta-a-ponta.sh
// runs this suite inside a freshly generated project, where the views are not
// these.
func TestNoSkeletonTestAssertsOnWhatAViewDraws(t *testing.T) {
	root := tests.Root(t)

	drawn := phrasesTheViewsDraw(t, filepath.Join(root, "resources", "views"))
	if len(drawn) == 0 {
		t.Fatal("no visible text was found in resources/views, so this guard would pass over anything: " +
			"either the views moved or the pattern stopped matching them")
	}

	var offences []string
	walkGoFiles(t, filepath.Join(root, "tests"), func(path string, source []byte) {
		// This file names the phrases on purpose, to say what it refuses.
		if filepath.Base(path) == "SkeletonTestsDoNotReadTheViews_test.go" {
			return
		}
		for _, m := range testLiteral.FindAllSubmatch(source, -1) {
			literal := string(m[1])
			if !drawn[literal] {
				continue
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				rel = path
			}
			offences = append(offences, rel+": "+literal)
		}
	})

	if len(offences) > 0 {
		sort.Strings(offences)
		t.Fatalf("these tests assert on words a view draws, and a generated project draws other words:\n\t%s\n\n"+
			"Assert on what the framework guarantees instead -- a status code, a header, a redirect from a guard.\n"+
			"A page belongs to the application, and the application replaces it.",
			strings.Join(offences, "\n\t"))
	}
}

// phrasesTheViewsDraw collects the visible text of every view under dir.
func phrasesTheViewsDraw(t *testing.T, dir string) map[string]bool {
	t.Helper()

	drawn := map[string]bool{}
	if err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".kyse.go") {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range visibleText.FindAllSubmatch(source, -1) {
			phrase := strings.TrimSpace(string(m[1]))
			// Only a phrase, never a single word, and the reason was measured:
			// a first draft took any label of six characters or more, and a
			// generated project failed on "Delete" -- a word its views draw as
			// a button and its tests name as an action. The two have nothing to
			// do with each other.
			//
			// A single word that a view draws is almost always also ordinary
			// vocabulary somewhere else. A phrase of two or more is not, which
			// is what makes it safe to refuse.
			if strings.Contains(phrase, " ") {
				drawn[phrase] = true
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("reading the views: %v", err)
	}
	return drawn
}

// walkGoFiles hands every .go file under dir to visit.
func walkGoFiles(t *testing.T, dir string, visit func(path string, source []byte)) {
	t.Helper()

	if err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		visit(path, source)
		return nil
	}); err != nil {
		t.Fatalf("reading the tests: %v", err)
	}
}
