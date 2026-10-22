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

// pinnedAction is a step's action pinned to a full commit, with the release it
// is in the comment: owner/repo[/path]@<40 hex> # vMAJOR.MINOR.PATCH.
var pinnedAction = regexp.MustCompile(`^[\w.-]+/[\w.-]+(/[\w./-]+)?@[0-9a-f]{40} # v\d+\.\d+\.\d+$`)

// TestEveryActionIsPinnedToACommit: a tag is a pointer its owner can move, and
// a moved tag runs somebody else's code with the repository's token. Every
// third-party action in every workflow `aru new` copies is pinned to a commit,
// and the comment names the release so a person and Dependabot can read it.
func TestEveryActionIsPinnedToACommit(t *testing.T) {
	dir := filepath.Join(tests.Root(t), ".github", "workflows")
	files, err := filepath.Glob(filepath.Join(dir, "*.yml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no workflow under %s: %v", dir, err)
	}
	checked := 0
	for _, file := range files {
		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("reading %s: %v", file, err)
		}
		for i, raw := range strings.Split(string(body), "\n") {
			line := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "- "))
			if !strings.HasPrefix(line, "uses:") {
				continue
			}
			action := strings.TrimSpace(strings.TrimPrefix(line, "uses:"))
			if strings.HasPrefix(action, "./") {
				continue
			}
			checked++
			if !pinnedAction.MatchString(action) {
				t.Errorf("%s:%d uses %q: pin it to the release's commit, as owner/repo@<sha> # vX.Y.Z",
					filepath.Base(file), i+1, action)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no action found in the workflows, so nothing was checked")
	}
}

// TestTheCIInstallsTheAruTheDockerfilePins: an aru installed @latest makes the
// same commit pass or fail depending on the day, and lets the CI check the
// architecture with a release other than the one the image compiles the views
// with. The workflow reads ARU_VERSION from the Dockerfile and installs that.
func TestTheCIInstallsTheAruTheDockerfilePins(t *testing.T) {
	body := workflow(t)
	readsThePin, installsThePin := false, false
	for i, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Contains(line, "arandu-io/aru@") && !strings.Contains(line, "arandu-io/aru@${aru_version}") {
			t.Errorf("%s:%d installs aru at %q, not at the Dockerfile's ARU_VERSION", workflowPath, i+1, line)
		}
		if strings.Contains(line, `sed -n 's/^ARG ARU_VERSION=//p' Dockerfile`) {
			readsThePin = true
		}
		if strings.Contains(line, `go install "github.com/arandu-io/aru@${aru_version}"`) {
			installsThePin = true
		}
	}
	if !readsThePin || !installsThePin {
		t.Errorf("the workflow does not install the aru the Dockerfile pins (reads ARU_VERSION: %v, installs it: %v)",
			readsThePin, installsThePin)
	}
}
