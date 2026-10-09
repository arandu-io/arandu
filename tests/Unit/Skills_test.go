package unit_test

import (
	"bufio"
	"bytes"
	"context"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/arandu-io/arandu/tests"
)

// The skills under .agents/skills are what an assistant reads before it writes
// code here, and nothing runs them. A snippet that stopped compiling, a flag a
// command never had and a gate list that differs from AGENTS.md are all taught
// with the same confidence as the rest, so these tests are what runs them.

// familySkills are the procedures this project hands every application, one
// per family of code. arandu-feature is the entry, and names the others.
var familySkills = []string{
	"arandu-feature",
	"arandu-module",
	"arandu-http",
	"arandu-api",
	"arandu-view",
	"arandu-async",
	"arandu-integrations",
	"arandu-policy",
	"arandu-doctor",
	"arandu-ecosystem",
}

// skillSections are the headings every family skill carries, in this order, so
// an assistant that has read one knows where to look in the next.
var skillSections = []string{
	"## When to use",
	"## Before you start",
	"## Contracts and imports",
	"## Procedure",
	"## Commands",
	"## Example",
	"## Do not",
	"## Extending it",
	"## Wiring",
	"## Acceptance test",
	"## Limits",
	"## Gates",
}

// skillFile is one SKILL.md, by the name of its directory.
type skillFile struct {
	name string
	path string
	body string
}

// skillFiles reads every skill this project carries, sorted by name.
func skillFiles(t *testing.T) []skillFile {
	t.Helper()
	root := tests.Root(t)
	paths, err := filepath.Glob(filepath.Join(root, ".agents", "skills", "*", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	var out []skillFile
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(root, path)
		out = append(out, skillFile{name: filepath.Base(filepath.Dir(path)), path: filepath.ToSlash(rel), body: string(body)})
	}
	if len(out) == 0 {
		t.Fatal("no skill under .agents/skills")
	}
	return out
}

func TestEveryFamilySkillHasTheSameSections(t *testing.T) {
	byName := map[string]skillFile{}
	for _, s := range skillFiles(t) {
		byName[s.name] = s
	}
	for _, name := range familySkills {
		s, ok := byName[name]
		if !ok {
			t.Errorf(".agents/skills/%s/SKILL.md is missing", name)
			continue
		}
		if !strings.Contains(s.body, "\nname: "+name+"\n") {
			t.Errorf("%s: the frontmatter does not name the skill %s", s.path, name)
		}
		at := 0
		for _, heading := range skillSections {
			found := strings.Index(s.body[at:], "\n"+heading+"\n")
			if found < 0 {
				t.Errorf("%s: no %q after the sections before it", s.path, heading)
				continue
			}
			at += found + len(heading)
		}
	}
}

// gateBlock answers the fenced block that opens with export GOWORK=off, which
// is the list of gates, or "" when the text has none.
func gateBlock(body string) string {
	start := strings.Index(body, "```sh\nexport GOWORK=off\n")
	if start < 0 {
		return ""
	}
	rest := body[start+len("```sh\n"):]
	end := strings.Index(rest, "```")
	if end < 0 {
		return ""
	}
	return rest[:end]
}

func TestTheGateBlockIsTheSameEverywhere(t *testing.T) {
	want := gateBlock(tests.File(t, "AGENTS.md"))
	if want == "" {
		t.Fatal("AGENTS.md has no gate block opening with export GOWORK=off")
	}
	family := map[string]bool{}
	for _, name := range familySkills {
		family[name] = true
	}
	for _, s := range skillFiles(t) {
		got := gateBlock(s.body)
		switch {
		case got == "" && family[s.name]:
			t.Errorf("%s has no gate block", s.path)
		case got != "" && got != want:
			t.Errorf("%s lists other gates than AGENTS.md:\n%s\nwant:\n%s", s.path, got, want)
		}
	}
}

// snippet is one fenced block of Go a skill marks as compiling, with the line
// of the skill its first line of code is on.
type snippet struct {
	skill string
	line  int
	code  string
}

// compilingSnippets reads the blocks fenced as ```go compile. Each is a whole
// file: a package clause, its imports, and code that compiles against this
// module, with <module> standing for the path go.mod declares.
func compilingSnippets(t *testing.T) []snippet {
	t.Helper()
	var out []snippet
	for _, s := range skillFiles(t) {
		var current *snippet
		var code strings.Builder
		for i, line := range strings.Split(s.body, "\n") {
			switch {
			case current == nil && strings.TrimSpace(line) == "```go compile":
				current = &snippet{skill: s.path, line: i + 2}
				code.Reset()
			case current != nil && strings.TrimSpace(line) == "```":
				current.code = code.String()
				out = append(out, *current)
				current = nil
			case current != nil:
				code.WriteString(line)
				code.WriteString("\n")
			}
		}
		if current != nil {
			t.Errorf("%s:%d: a go compile block is never closed", s.path, current.line-1)
		}
	}
	return out
}

// modulePath is what go.mod declares.
func modulePath(t *testing.T) string {
	t.Helper()
	for _, line := range strings.Split(tests.File(t, "go.mod"), "\n") {
		if rest, ok := strings.CutPrefix(line, "module "); ok {
			return strings.TrimSpace(rest)
		}
	}
	t.Fatal("go.mod declares no module")
	return ""
}

func TestTheGoSnippetsInTheSkillsCompile(t *testing.T) {
	root := tests.Root(t)
	// The snippets are written against the example resource, the one module
	// every step of the anatomy has. A project that removed it keeps the skills
	// and loses what they compile against; the skeleton compiles them.
	if _, err := os.Stat(filepath.Join(root, "app", "Models", "Note.go")); err != nil {
		t.Skip("the example resource was removed; the skill snippets are compiled where it exists, in the skeleton")
	}
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go is not on PATH")
	}

	snippets := compilingSnippets(t)
	if len(snippets) < len(familySkills) {
		t.Fatalf("%d compiling snippets, want at least one per family skill (%d)", len(snippets), len(familySkills))
	}
	module := modulePath(t)

	// A directory inside the module, so the snippets import it as any package
	// of it does. The leading underscore keeps it out of ./... for everything
	// else that runs while this does.
	dir, err := os.MkdirTemp(root, "_skill-snippets-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	where := map[string]snippet{}
	var packages []string
	for i, s := range snippets {
		code := strings.ReplaceAll(s.code, "<module>", module)
		if formatted, err := format.Source([]byte(code)); err != nil {
			t.Errorf("%s:%d: the snippet does not parse: %v", s.skill, s.line, err)
			continue
		} else if string(formatted) != code {
			t.Errorf("%s:%d: the snippet is not gofmt-formatted", s.skill, s.line)
		}
		pkg := filepath.Join(dir, "s"+strconv.Itoa(i))
		if err := os.MkdirAll(pkg, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(pkg, "snippet.go"), []byte(code), 0o644); err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(root, pkg)
		packages = append(packages, "./"+filepath.ToSlash(rel))
		where["s"+strconv.Itoa(i)] = s
	}
	if t.Failed() {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, goTool, append([]string{"build"}, packages...)...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.CombinedOutput()
	if err == nil {
		return
	}

	// Each line names the snippet's file; it is rewritten to the skill and
	// the line of the skill the error is on.
	located := regexp.MustCompile(regexp.QuoteMeta(filepath.Base(dir)) + `/(s\d+)/snippet\.go:(\d+)`)
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := located.ReplaceAllStringFunc(scanner.Text(), func(m string) string {
			parts := located.FindStringSubmatch(m)
			s := where[parts[1]]
			n, _ := strconv.Atoi(parts[2])
			return s.skill + ":" + strconv.Itoa(s.line+n-1)
		})
		t.Error(line)
	}
}

// aruNamed is one command a skill names, with the flags written after it.
type aruNamed struct {
	skill   string
	command string
	flags   []string
}

var (
	// aruCall is aru followed by a command name, where a command starts.
	aruCall = regexp.MustCompile(`(?:^|[\s(\x60])aru ([a-z][a-z0-9:-]*)`)
	// aruFlag is a long flag.
	aruFlag = regexp.MustCompile(`--[a-z][a-z-]*`)
	// inlineCode is a code span of prose.
	inlineCode = regexp.MustCompile("\x60[^\x60\n]+\x60")
)

// codeOf answers the parts of a skill a command is written in: the lines of
// every fenced block that is not Go, and every code span of the prose.
func codeOf(body string) []string {
	var out []string
	fence := ""
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			if fence == "" {
				fence = strings.TrimPrefix(trimmed, "```")
				if fence == "" {
					fence = "text"
				}
			} else {
				fence = ""
			}
			continue
		}
		switch {
		case fence == "":
			out = append(out, inlineCode.FindAllString(line, -1)...)
		case !strings.HasPrefix(fence, "go"):
			out = append(out, line)
		}
	}
	return out
}

// namedCommands reads every aru command the skills name, and its flags.
func namedCommands(t *testing.T) []aruNamed {
	t.Helper()
	var out []aruNamed
	for _, s := range skillFiles(t) {
		for _, segment := range codeOf(s.body) {
			calls := aruCall.FindAllStringSubmatchIndex(segment, -1)
			for i, call := range calls {
				end := len(segment)
				if i+1 < len(calls) {
					end = calls[i+1][0]
				}
				out = append(out, aruNamed{
					skill:   s.path,
					command: segment[call[2]:call[3]],
					flags:   aruFlag.FindAllString(segment[call[3]:end], -1),
				})
			}
		}
	}
	return out
}

func TestTheAruCommandsTheSkillsNameExist(t *testing.T) {
	aru, err := exec.LookPath("aru")
	if err != nil {
		t.Skip("aru is not on PATH")
	}
	pinned := dockerArgument(t, "ARU_VERSION")
	if running := aruVersion(t, aru); running != pinned {
		t.Skipf("aru on PATH is %s and the Dockerfile pins %s; the skills name the commands of the pinned release", running, pinned)
	}

	help, err := exec.Command(aru, "help").Output()
	if err != nil {
		t.Fatalf("aru help: %v", err)
	}
	listed := map[string]bool{}
	for _, line := range strings.Split(string(help), "\n") {
		if !strings.HasPrefix(line, "  ") {
			continue
		}
		if fields := strings.Fields(line); len(fields) > 0 {
			listed[fields[0]] = true
		}
	}

	// The usage line of each command, asked once. --help is answered before a
	// command runs, so asking touches no project and no file.
	usage := map[string]string{}
	usageOf := func(command string) string {
		if u, ok := usage[command]; ok {
			return u
		}
		out, err := exec.Command(aru, command, "--help").Output()
		if err != nil {
			t.Fatalf("aru %s --help: %v", command, err)
		}
		u, _, _ := strings.Cut(string(out), "\n\n")
		usage[command] = u
		return u
	}

	named := namedCommands(t)
	if len(named) == 0 {
		t.Fatal("the skills name no aru command, which means this test reads them wrong")
	}
	for _, n := range named {
		if !listed[n.command] {
			t.Errorf("%s names aru %s, and aru %s has no such command", n.skill, n.command, pinned)
			continue
		}
		u := usageOf(n.command)
		if strings.Contains(u, "flags for") {
			continue // the command hands its flags to something else
		}
		for _, flag := range n.flags {
			if !regexp.MustCompile(regexp.QuoteMeta(flag) + `\b`).MatchString(u) {
				t.Errorf("%s names aru %s %s, and its usage has no such flag:\n%s", n.skill, n.command, flag, u)
			}
		}
	}
}
