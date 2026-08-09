package unit_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arandu-io/arandu/tests"
)

// TestEveryTestLivesInTheTestsDirectory.
//
// The suites are Laravel's, and they mean what they mean there: tests/Feature
// boots the application and makes a request, tests/Unit checks one thing without
// booting anything. A `_test.go` anywhere else is the arrangement this project
// moved away from -- a source file and its test side by side, doubling every
// listing with files nobody opens on purpose.
//
// Two exceptions, and neither is a preference:
//
//   - a test that reads an unexported identifier cannot live in another package.
//     Go decides that, not this project.
//   - a module of its own -- assets/ embeds the compiled stylesheet -- owns its
//     tests, because it is a package that ships on its own.
func TestEveryTestLivesInTheTestsDirectory(t *testing.T) {
	root := tests.Root(t)

	allowed := map[string]bool{
		// The stylesheet package proves the bytes it embeds are the ones the
		// build produced, and that is a question about itself.
		"assets": true,
	}

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)

		if strings.HasPrefix(rel, "tests/") {
			return nil
		}
		if allowed[strings.SplitN(rel, "/", 2)[0]] {
			return nil
		}
		t.Errorf("%s is a test outside tests/: move it to tests/Feature if it boots the "+
			"application, or tests/Unit if it does not", rel)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestEveryDirectoryThatMustExistIsKept.
//
// git does not track a directory, only files, so an empty one is not in a clone
// at all -- and storage/framework/cache missing is a runtime failure on the
// first request, not a build error anybody sees.
//
// The two answers are Laravel's, and they are different on purpose: storage
// keeps a .gitignore that ignores everything but itself, because the contents
// are produced and must never be committed; a source directory that starts empty
// keeps a .gitkeep, because its contents are written by hand and belong in git.
func TestEveryDirectoryThatMustExistIsKept(t *testing.T) {
	root := tests.Root(t)

	for _, d := range []struct {
		path string
		file string
	}{
		{"storage/app/private", ".gitignore"},
		{"storage/app/public", ".gitignore"},
		{"storage/framework/cache", ".gitignore"},
		{"storage/framework/sessions", ".gitignore"},
	} {
		body, err := os.ReadFile(filepath.Join(root, d.path, d.file))
		if err != nil {
			t.Errorf("%s has no %s: the directory will not be in a clone, and the first "+
				"request that writes there fails at run time", d.path, d.file)
			continue
		}
		if !strings.Contains(string(body), "*") || !strings.Contains(string(body), "!.gitignore") {
			t.Errorf("%s/%s does not ignore its contents while keeping itself:\n%s", d.path, d.file, body)
		}
	}

	// A source directory that starts empty keeps a .gitkeep. Nothing is produced
	// in these, so ignoring their contents would ignore the code.
	for _, d := range []string{
		"app/Enums", "app/Events", "app/Jobs", "app/Listeners", "app/Mail", "app/Http/Middleware",
	} {
		full := filepath.Join(root, d)
		entries, err := os.ReadDir(full)
		if err != nil {
			t.Errorf("%s does not exist", d)
			continue
		}
		if len(entries) > 0 {
			continue // it has files of its own, and does not need a keeper
		}
		if _, err := os.Stat(filepath.Join(full, ".gitkeep")); err != nil {
			t.Errorf("%s is empty and has no .gitkeep: it will not be in a clone", d)
		}
	}
}
