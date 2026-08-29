package unit_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arandu-io/arandu/tests"
)

// arandu.toml pins the binaries `aru view:build` downloads, and every project
// made with `aru new` receives this exact file.
//
// It pins only what is actually downloaded. The view engine is kyse, which is
// code inside `aru` rather than a release fetched from somewhere, so it has no
// entry here. A pin for a tool nobody downloads is worse than no pin -- it is a
// version number people maintain, upgrade and argue about for a build step that
// does not exist.

// manifestSection parses one table from the subset of TOML that `aru` reads:
// a section header and key = "value" lines.
func manifestSection(t *testing.T, wanted string) map[string]string {
	t.Helper()

	body, err := os.ReadFile(filepath.Join(tests.Root(t), "arandu.toml"))
	if err != nil {
		t.Fatalf("reading arandu.toml: %v", err)
	}

	out := map[string]string{}
	section := ""
	for _, raw := range strings.Split(string(body), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.Trim(line, "[]")
			continue
		}
		if section != wanted {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			t.Fatalf("arandu.toml: expected key = \"value\", got %q", line)
		}
		out[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"`)
	}
	return out
}

func TestTheManifestRequiresTheOldestAruThatBuildsNativeAuthenticationViews(t *testing.T) {
	if got := manifestSection(t, "arandu")["aru"]; got != "v0.35.0" {
		t.Errorf("arandu.toml requires aru %q, want v0.35.0 for native authentication views", got)
	}
}

func TestTheToolchainPinsOnlyWhatIsDownloaded(t *testing.T) {
	pinned := manifestSection(t, "tools")

	if version, found := pinned["templ"]; found {
		t.Errorf("arandu.toml still pins templ %q, and nothing downloads it: "+
			"the view engine is kyse, which ships inside `aru` (ADR 0020)", version)
	}
	if pinned["tailwindcss"] == "" {
		t.Error("arandu.toml does not pin tailwindcss, and it is the one binary `aru view:build` fetches")
	}
	for name := range pinned {
		if name != "tailwindcss" {
			t.Errorf("arandu.toml pins %q; `aru view:build` downloads tailwindcss and nothing else", name)
		}
	}
}

// TestTheToolchainCommentDoesNotPromiseATemplBuild: the pin was one half of the
// claim and the comment above it was the other. It said templ is a Go binary
// the CLI downloads, which sends the next person looking for a build step that
// does not exist. Every mention had to go, not only the key.
func TestTheToolchainCommentDoesNotPromiseATemplBuild(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(tests.Root(t), "arandu.toml"))
	if err != nil {
		t.Fatalf("reading arandu.toml: %v", err)
	}

	for i, line := range strings.Split(string(body), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "#") {
			continue
		}
		// "templ" and not "templates": the word is the tool.
		for _, field := range strings.FieldsFunc(trimmed, func(r rune) bool {
			return !('a' <= r && r <= 'z') && !('A' <= r && r <= 'Z')
		}) {
			if strings.EqualFold(field, "templ") {
				t.Errorf("arandu.toml:%d still describes templ, and this project does not use it: %s", i+1, trimmed)
			}
		}
	}
}
