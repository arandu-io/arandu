package unit_test

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arandu-io/arandu/tests"
)

// resources/css/basecoat holds two kinds of CSS in one tree: the files
// vendor.sh copies from upstream, and the files of this project -- components
// upstream does not ship, rules appended to upstream files, a patched line, a
// second licence notice. The updater used to remove the directories it wrote
// into and copy upstream's component list over components.css, so running it
// once deleted every component of this project and the edits in the rest.
//
// These tests run the real script against a checkout made up here, in which
// upstream ships a file under every name this project uses, and require that
// it writes nothing but files it wrote before and that nobody has changed
// since.

// TestVendoringBasecoatKeepsEveryFileOfThisProject: vendoring again updates
// the files vendor.sh wrote and nobody changed since, adds what upstream added,
// copies no script, and leaves every other file -- components.css, the
// components of this project, the vendored files edited here -- byte for byte,
// deleting none.
func TestVendoringBasecoatKeepsEveryFileOfThisProject(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Fatalf("vendor.sh is a bash script and bash is not on PATH: %v", err)
	}

	dst := filepath.Join(t.TempDir(), "basecoat")
	copyTree(t, filepath.Join(tests.Root(t), "resources", "css", "basecoat"), dst)
	before := readTree(t, dst)
	sum := readSum(t, filepath.Join(dst, "vendor.sum"))

	// Upstream, as vendor.sh reads it, with a file under every name this
	// project has in components/ -- its own included, which is the case of
	// upstream adding a component this project already wrote -- and one name
	// nobody has yet.
	src := t.TempDir()
	upstream := []string{
		"LICENSE.md",
		"src/css/base/base.css",
		"src/css/styles/vega.css",
		"src/css/basecoat-components.css",
		"src/js/basecoat.js",
		"src/css/components/brand-new.css",
	}
	for rel := range before {
		if strings.HasPrefix(rel, "components/") && strings.HasSuffix(rel, ".css") {
			upstream = append(upstream, "src/css/"+rel)
		}
	}
	for _, from := range upstream {
		writeFile(t, filepath.Join(src, from), sentinel(from))
	}

	out, err := exec.Command(bash, filepath.Join(dst, "vendor.sh"), src).CombinedOutput()
	if err != nil {
		t.Fatalf("vendor.sh: %v\n%s", err, out)
	}
	after := readTree(t, dst)

	written, kept := 0, 0
	for rel, was := range before {
		if rel == "vendor.sum" {
			continue
		}
		now, ok := after[rel]
		if !ok {
			t.Errorf("%s was deleted", rel)
			continue
		}
		if digest, vendored := sum[rel]; vendored && digest == sha(was) {
			// Exactly what the script wrote last time: upstream's new version
			// replaces it.
			if want := sentinel(upstreamOf(rel)); !bytes.Equal(now, want) {
				t.Errorf("%s is still what it last vendored and was not updated from upstream", rel)
			}
			written++
			continue
		}
		if !bytes.Equal(now, was) {
			t.Errorf("%s is a file of this project, or a vendored file changed here, and vendor.sh overwrote it", rel)
		}
		// A kept file is named, with why: the person merging upstream by hand
		// needs to know which files to look at and which side of the
		// difference is theirs.
		why := "a file of this project; upstream now ships one with the same name"
		if _, vendored := sum[rel]; vendored {
			why = "changed here since it was vendored"
		}
		if upstreamShips(upstream, rel) && !bytes.Contains(out, []byte(rel+" ("+why)) {
			t.Errorf("vendor.sh kept %s without naming it as %q:\n%s", rel, why, out)
		}
		kept++
	}
	if written == 0 || kept == 0 {
		t.Fatalf("updated %d and kept %d files: the tree no longer has both kinds, so this test checked half of what it is for", written, kept)
	}

	if got, want := after["components/brand-new.css"], sentinel("src/css/components/brand-new.css"); !bytes.Equal(got, want) {
		t.Error("a component upstream added was not vendored")
	}
	for rel := range after {
		if filepath.Ext(rel) == ".js" {
			t.Errorf("vendor.sh copied %s, and nothing under resources/ builds, embeds or serves a script", rel)
		}
	}

	// The new record names what was written with the digest of what is on disk
	// now, and keeps the old line of every file it kept.
	next := readSum(t, filepath.Join(dst, "vendor.sum"))
	for rel, digest := range next {
		content, ok := after[rel]
		if !ok {
			t.Errorf("vendor.sum names %s, which is not there", rel)
			continue
		}
		if was, existed := before[rel]; existed && bytes.Equal(content, was) && sum[rel] != sha(was) {
			if digest != sum[rel] {
				t.Errorf("vendor.sum changed its line for %s, a file it kept", rel)
			}
			continue
		}
		if digest != sha(content) {
			t.Errorf("vendor.sum records %s for %s, which now hashes to %s", digest, rel, sha(content))
		}
	}
	if _, ok := next["components/brand-new.css"]; !ok {
		t.Error("vendor.sum does not record the component it added")
	}
}

// TestVendorSumNamesOnlyFilesThatExist: the record is of files in this
// directory, and a line for a file that is gone is a line nothing checks.
func TestVendorSumNamesOnlyFilesThatExist(t *testing.T) {
	dir := filepath.Join(tests.Root(t), "resources", "css", "basecoat")
	sum := readSum(t, filepath.Join(dir, "vendor.sum"))
	if len(sum) == 0 {
		t.Fatal("vendor.sum records nothing")
	}
	for rel := range sum {
		if rel == "components.css" {
			t.Error("vendor.sum records components.css, which is this project's list and never upstream's")
		}
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
			t.Errorf("vendor.sum names %s: %v", rel, err)
		}
	}
}

// upstreamOf is the upstream path vendor.sh copies to rel.
func upstreamOf(rel string) string {
	if rel == "LICENSE.md" {
		return rel
	}
	return "src/css/" + rel
}

// sentinel is the made-up upstream content of a path.
func sentinel(path string) []byte {
	return []byte("/* upstream " + path + " */\n")
}

func sha(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// readSum parses vendor.sum: a digest and a path per line.
func readSum(t *testing.T, path string) map[string]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	defer f.Close()
	out := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			t.Fatalf("%s: malformed line %q", path, scanner.Text())
		}
		out[fields[1]] = fields[0]
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return out
}

// readTree answers every file under root by its slash-separated path.
func readTree(t *testing.T, root string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = content
		return nil
	})
	if err != nil {
		t.Fatalf("reading %s: %v", root, err)
	}
	return out
}

// copyTree copies every file under from to to, keeping the modes.
func copyTree(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.WalkDir(from, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		target := filepath.Join(to, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, content, info.Mode().Perm())
	})
	if err != nil {
		t.Fatalf("copying %s: %v", from, err)
	}
}

func writeFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
}

// upstreamShips reports whether the made-up checkout has a file vendor.sh
// copies to rel, which is when keeping rel is a decision the script reports.
func upstreamShips(upstream []string, rel string) bool {
	for _, from := range upstream {
		if from == upstreamOf(rel) {
			return true
		}
	}
	return false
}
