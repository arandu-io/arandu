package unit_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/arandu-io/arandu/tests"
)

// The arandu-doctor skill carries a table of every rule the doctor checks. An
// assistant reads that table to learn which finding fails the run and which
// only warns, and a table one release behind the doctor teaches it a rule set
// that is not the one its code is checked against. Nothing fails when they
// drift, so these tests are what fails.

// doctorRule is one row of `aru doctor --list`: the name a finding carries, the
// severity it reports at, and the one profile it is limited to, if any.
type doctorRule struct {
	name     string
	severity string
	profile  string
}

// doctorRules is what `aru doctor --list` prints for the aru the Dockerfile
// pins, in the order it prints it. When the pin moves, this list moves with it
// and the skill's table follows.
var doctorRules = []doctorRule{
	{"file-does-not-parse", "error", ""},
	{"repository-without-policy", "error", ""},
	{"grant-not-received", "error", ""},
	{"grant-not-checked", "error", ""},
	{"grant-check-discarded", "error", ""},
	{"policy-never-opened", "warning", ""},
	{"action-not-a-constant", "error", ""},
	{"enum-rule-not-derived", "error or warning", ""},
	{"handler-reaches-data", "error", ""},
	{"handler-reaches-the-model", "error", ""},
	{"controller-reaches-repository", "error", ""},
	{"tenant-from-request", "error", ""},
	{"tenant-from-header", "error", ""},
	{"system-grant-without-tenant", "error", ""},
	{"system-grant-outside-scope", "warning", ""},
	{"sql-built-with-sprintf", "error", ""},
	{"sql-built-by-concatenation", "error", ""},
	{"sensitive-field-not-redacted", "warning", ""},
	{"session-not-rotated", "error", ""},
	{"csrf-exempt-without-signature", "warning", ""},
	{"view-data-is-a-map", "error", ""},
	{"view-does-not-exist", "error", ""},
	{"permission-not-declared", "error", ""},
	{"permission-not-used", "warning", ""},
	{"view-keeps-state-in-the-browser", "error", ""},
	{"sql-without-tenant-scope", "error", ""},
	{"outbox-not-registered", "error", ""},
	{"resource-not-reauthorized", "warning", ""},
	{"raw-output-is-not-a-component", "warning", ""},
	{"retired-module", "warning", ""},
	{"import-not-canonical", "warning", ""},
	{"test-is-not-run", "warning", ""},
	{"test-outside-the-tests-tree", "warning", ""},
	{"package-clause-is-capitalised", "warning", ""},
	{"scaffolding-ships", "warning", ""},
	{"skills-out-of-date", "warning", ""},
	{"skills-missing", "warning", ""},
	{"generated-skill-retired", "warning", ""},
	{"migrations-not-linked", "warning", ""},
	{"added-column-not-nullable", "warning", ""},
	{"rollback-does-nothing", "warning", ""},
	{"driver-not-linked", "warning", ""},
	{"profile-not-declared", "warning", "performance"},
	{"join-across-aggregates", "error", "performance"},
	{"transaction-across-aggregates", "error", "performance"},
	{"model-query-stale", "error", ""},
	{"model-core-outside-models", "error", ""},
	{"input-read-by-hand", "warning", ""},
	{"validate-called-by-controller", "warning", ""},
	{"json-written-by-hand", "warning", ""},
	{"invalid-form-answered-by-hand", "warning", ""},
	{"session-loaded-in-controller", "warning", ""},
	{"redirect-to-literal-path", "warning", ""},
	{"html-template-in-app", "warning", ""},
	{"service-takes-http", "warning", ""},
	{"service-subpackage", "warning", ""},
	{"service-file-too-large", "warning", ""},
	{"controller-too-many-actions", "warning", ""},
	{"operation-chosen-by-form-field", "warning", ""},
	{"client-outside-clients", "warning", ""},
	{"model-rule-touches-io", "warning", ""},
	{"fragment-without-partial", "warning", ""},
	{"helper-reimplemented", "warning", ""},
	{"raw-sql-outside-repository", "warning", ""},
	{"generated-not-wired", "warning", ""},
	{"subject-built-by-hand", "warning", ""},
}

// performanceOnly opens the description of a row the doctor checks only under
// --profile=performance.
const performanceOnly = "`--profile=performance`:"

// skillRules reads the rows of the "Every rule" table in the arandu-doctor
// skill, in the order they are written.
func skillRules(t *testing.T) []doctorRule {
	t.Helper()
	body := tests.File(t, filepath.Join(".agents", "skills", "arandu-doctor", "SKILL.md"))

	_, section, found := strings.Cut(body, "\n## Every rule\n")
	if !found {
		t.Fatal("the arandu-doctor skill has no \"## Every rule\" section")
	}
	if next := strings.Index(section, "\n## "); next >= 0 {
		section = section[:next]
	}

	var rows []doctorRule
	for _, line := range strings.Split(section, "\n") {
		if !strings.HasPrefix(line, "| `") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		if len(cells) != 3 {
			t.Fatalf("a row of the rule table does not have three cells: %s", line)
		}
		row := doctorRule{
			name:     strings.Trim(strings.TrimSpace(cells[0]), "`"),
			severity: strings.TrimSpace(cells[1]),
		}
		if strings.HasPrefix(strings.TrimSpace(cells[2]), performanceOnly) {
			row.profile = "performance"
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		t.Fatal("the \"Every rule\" table of the arandu-doctor skill has no rows")
	}
	return rows
}

// compareRules reports every difference between two rule lists, position by
// position, and a name one of them has and the other does not.
func compareRules(t *testing.T, gotName string, got []doctorRule, wantName string, want []doctorRule) {
	t.Helper()
	inGot := map[string]bool{}
	for _, r := range got {
		inGot[r.name] = true
	}
	inWant := map[string]bool{}
	for _, r := range want {
		inWant[r.name] = true
	}
	for _, r := range want {
		if !inGot[r.name] {
			t.Errorf("%s has %s and %s does not", wantName, r.name, gotName)
		}
	}
	for _, r := range got {
		if !inWant[r.name] {
			t.Errorf("%s has %s and %s does not", gotName, r.name, wantName)
		}
	}
	if t.Failed() {
		return
	}
	if len(got) != len(want) {
		t.Errorf("%s has %d rows and %s has %d: a rule is listed twice", gotName, len(got), wantName, len(want))
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d: %s says %+v, %s says %+v", i+1, gotName, got[i], wantName, want[i])
		}
	}
}

func TestTheDoctorSkillListsEveryRuleWithItsSeverity(t *testing.T) {
	compareRules(t, "the arandu-doctor skill", skillRules(t), "the rule list of this test", doctorRules)
}

// listColumns splits a line of `aru doctor --list` on the runs of spaces the
// table is aligned with.
var listColumns = regexp.MustCompile(`\s{2,}`)

func TestTheDoctorRuleListIsTheOneThePinnedAruPrints(t *testing.T) {
	aru, err := exec.LookPath("aru")
	if err != nil {
		t.Skip("aru is not on PATH; the skill's table is still compared with the list in this test")
	}
	pinned := dockerArgument(t, "ARU_VERSION")
	running := aruVersion(t, aru)
	if running != pinned {
		t.Skipf("aru on PATH is %s and the Dockerfile pins %s; the rule list describes the pinned release", running, pinned)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, aru, "doctor", "--list")
	cmd.Dir = tests.Root(t)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("aru doctor --list: %v", err)
	}

	var printed []doctorRule
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		cols := listColumns.Split(line, -1)
		if len(cols) < 2 {
			t.Fatalf("aru doctor --list printed a line with no severity: %q", line)
		}
		row := doctorRule{name: cols[0], severity: cols[1]}
		if len(cols) > 2 {
			row.profile = strings.TrimSuffix(strings.TrimPrefix(cols[2], "--profile="), " only")
		}
		printed = append(printed, row)
	}
	compareRules(t, "the rule list of this test", doctorRules, "aru "+pinned+" doctor --list", printed)
}

// aruVersion answers the release of the aru at path, as vMAJOR.MINOR.PATCH.
//
// A release build says it from `aru version`. One built by `go install` says
// "dev" there, and carries its module version in the build information
// instead, which `go version -m` reads. A binary that says neither is a build
// from source, and nothing it lists describes a release, so the test skips.
func aruVersion(t *testing.T, path string) string {
	t.Helper()
	out, err := exec.Command(path, "version").Output()
	if err != nil {
		t.Fatalf("aru version: %v", err)
	}
	if fields := strings.Fields(string(out)); len(fields) == 2 && fields[1] != "dev" {
		return "v" + strings.TrimPrefix(fields[1], "v")
	}

	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("aru on PATH reports no release and go is not on PATH to read its build information")
	}
	info, err := exec.Command(goTool, "version", "-m", path).Output()
	if err != nil {
		t.Skipf("aru on PATH reports no release, and go version -m cannot read it: %v", err)
	}
	for _, line := range strings.Split(string(info), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == "mod" && fields[1] == "github.com/arandu-io/aru" && strings.HasPrefix(fields[2], "v") {
			return fields[2]
		}
	}
	t.Skip("aru on PATH is a build from source, with no release to compare the rule list with")
	return ""
}
