package unit_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/arandu-io/arandu/tests"
)

// The workflow is not this repository's private business. `aru new` clones the
// skeleton and deletes its history, so .github/workflows/ci.yml is the CI of
// every project made with this framework, from its first commit. The two
// properties below are what a file in that position has to have.

const workflowPath = ".github/workflows/ci.yml"

func workflow(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(tests.Root(t), workflowPath))
	if err != nil {
		t.Fatalf("reading %s: %v", workflowPath, err)
	}
	return string(body)
}

// TestTheGeneratedCIIsGreenOnADayOldProject.
//
// `aru doctor --strict` promotes every warning to an error, and one of the
// warnings is `policy-never-opened`: `aru make:module` writes a policy that
// denies every action, on purpose, and the doctor says so. Under --strict the
// first push of a project that has done nothing but generate a module fails,
// for a file the generator wrote and a decision the developer has not had the
// chance to make yet.
//
// The policy still denies by default -- that is the thesis and it does not move.
// What this pins is that the shipped CI does not turn an expected to-do into a
// build failure. Errors still fail: `aru doctor` exits non-zero on any of them.
func TestTheGeneratedCIIsGreenOnADayOldProject(t *testing.T) {
	body := workflow(t)

	ranDoctor := false
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") || !strings.Contains(trimmed, "aru doctor") {
			continue
		}
		ranDoctor = true
		if strings.Contains(trimmed, "--strict") {
			t.Errorf("the shipped CI runs %q: a freshly generated module trips policy-never-opened, "+
				"so the first push of every new project would be red", trimmed)
		}
	}
	if !ranDoctor {
		t.Error("the workflow does not run `aru doctor`: the architecture rules would only hold where the type system reaches")
	}
}

// portugueseMarkers are function words that exist in Portuguese and not in
// English. The list is short and boring on purpose: it has to catch a comment
// somebody wrote in the language of the conversation, and never fire on an
// English one.
var portugueseMarkers = []string{
	"nao", "não", "sao", "são", "arquivo", "proposito", "propósito",
	"porque", "pela", "pelo", "tambem", "também", "entao", "então",
	"isso", "aqui", "uma", "quando", "onde", "tudo", "toda", "qualquer",
	"sempre", "nunca", "cada", "entre", "sobre", "fazer", "feito",
	"coisa", "linha", "nome", "erro", "dados", "que", "para",
}

// TestTheWorkflowIsWrittenInEnglish.
//
// RULE 6: Portuguese belongs to the conversation and to the decision documents.
// Everything that ships is English, and this workflow ships -- into the
// repository of a person who has no reason to read Portuguese, to explain why
// their build just failed.
//
// The check is scoped to this one file rather than to the tree. The rest of the
// tree becomes the user's own code the moment `aru new` finishes, and a
// skeleton that failed a Brazilian developer's build for commenting in
// Portuguese would be enforcing a rule that is ours, not theirs.
func TestTheWorkflowIsWrittenInEnglish(t *testing.T) {
	body := workflow(t)

	for _, marker := range portugueseMarkers {
		re := regexp.MustCompile(`(?i)(^|[^\p{L}])` + regexp.QuoteMeta(marker) + `($|[^\p{L}])`)
		for i, line := range strings.Split(body, "\n") {
			if re.MatchString(line) {
				t.Errorf("%s:%d is not English (%q): %s", workflowPath, i+1, marker, strings.TrimSpace(line))
			}
		}
	}
}
