package unit_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/arandu-io/arandu/tests"
)

// TestEveryTestLivesInTheTestsDirectory.
//
// tests/Feature boots the application and makes a request; tests/Unit checks one
// thing without booting anything. A `_test.go` anywhere else puts a source file
// and its test side by side, doubling every listing with files nobody opens on
// purpose.
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
// The two answers are different on purpose: storage keeps a .gitignore that
// ignores everything but itself, because the contents are produced and must
// never be committed; a source directory that starts empty keeps a .gitkeep,
// because its contents are written by hand and belong in git.
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
		"app/Enums", "app/Events", "app/Jobs", "app/Listeners", "app/Mail",
		"app/Http/Middleware", "app/Http/Requests", "app/Services",
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

// TestTheApplicationTreeHasOneOwnerPerResponsibility freezes the part of the
// skeleton a fresh clone is allowed to teach. Required paths are the places a
// developer must know; optional paths may appear when their first source file
// does; refused paths would introduce a second owner or a Node build.
func TestTheApplicationTreeHasOneOwnerPerResponsibility(t *testing.T) {
	root := tests.Root(t)

	requiredDirectories := []string{
		"app",
		"app/Http/Controllers",
		"app/Http/Middleware",
		"app/Http/Requests",
		"app/Models",
		"app/Policies",
		"app/Providers",
		"app/Services",
		"assets",
		"bootstrap",
		"config",
		"database/factories",
		"database/migrations",
		"database/seeders",
		"public",
		"resources/css",
		// The customisation layer, and it is required rather than optional for
		// the reason a place to put something is only useful when it is already
		// there: a directory a project has to create first is a directory a
		// project invents a name for instead.
		//
		// It was a refused path while nothing embedded, served or referenced
		// what went in it -- a .js under resources/ was then a file that looked
		// like behaviour and could not be one. The Go file beside custom.js is
		// what changed that, and it has to be beside it: //go:embed cannot
		// reference a parent directory.
		"resources/js",
		"resources/views",
		"routes",
		"storage/app/private",
		"storage/app/public",
		"storage/framework/cache",
		"storage/framework/sessions",
		"tests/Feature",
		"tests/Unit",
	}
	for _, path := range requiredDirectories {
		info, err := os.Stat(filepath.Join(root, path))
		switch {
		case err != nil:
			t.Errorf("required directory %s is missing: %v", path, err)
		case !info.IsDir():
			t.Errorf("required directory %s is not a directory", path)
		}
	}

	requiredFiles := []string{
		"go.mod",
		"main.go",
		"arandu.toml",
		"app/Providers/AppServiceProvider.go",
		"assets/assets.go",
		"bootstrap/app.go",
		"bootstrap/console.go",
		"config/app.go",
		"database/migrations/migrations.go",
		"public/public.go",
		// The two files a project customises, and the one that carries the
		// second of them into the binary.
		//
		// They are required because their absence is silent. Deleting
		// custom.css breaks the stylesheet build outright, which is loud, but
		// re-creating it somewhere else is not: a project that customises in a
		// file of its own invented name has an override nothing imports, a page
		// that draws the skeleton's design, and nothing anywhere saying so.
		"resources/css/custom.css",
		"resources/js/custom.js",
		"resources/js/js.go",
		"routes/console.go",
		"routes/web.go",
		"tests/TestCase.go",
	}
	for _, path := range requiredFiles {
		info, err := os.Stat(filepath.Join(root, path))
		switch {
		case err != nil:
			t.Errorf("required file %s is missing: %v", path, err)
		case info.IsDir():
			t.Errorf("required file %s is a directory", path)
		}
	}

	// Repositories is an escape for specialized queries, and Commands appears
	// with the first structured command. Either may be absent without changing
	// the canonical tree; if present, it must still be a directory.
	for _, path := range []string{"app/Repositories", "app/Console/Commands"} {
		info, err := os.Stat(filepath.Join(root, path))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Errorf("reading optional directory %s: %v", path, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("optional path %s is not a directory", path)
		}
	}

	forbiddenPaths := []string{
		"app/Actions",
		"app/Data",
		"app/Rules",
		"app/Support",
		"bootstrap/providers.go",
		"bootstrap/cache",
		"routes/api.go",
		"routes/channels.go",
		"public/assets/manifest.json",
		"storage/logs",
	}
	for _, path := range forbiddenPaths {
		if _, err := os.Lstat(filepath.Join(root, path)); err == nil {
			t.Errorf("refused path %s exists in the application skeleton", path)
		} else if !os.IsNotExist(err) {
			t.Errorf("checking refused path %s: %v", path, err)
		}
	}

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		switch {
		case d.Name() == "node_modules":
			t.Errorf("refused Node directory %s exists in the application skeleton", rel)
			if d.IsDir() {
				return filepath.SkipDir
			}
		case !d.IsDir() && d.Name() == "package.json":
			t.Errorf("refused Node manifest %s exists in the application skeleton", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestEachSuiteHoldsWhatItsNameSays.
//
// The two suites are not two folders to spread files across. Each means
// something:
//
//	Feature  boots the application, or drives more than one piece of it
//	Unit     checks one thing, with nothing running
//
// The distinction earns its keep when a suite is slow: `go test ./tests/Unit/`
// is the one somebody runs on every save, and it stops being that the first time
// a test in it opens a database.
//
// It is checked by what a file reaches for, not by what it is called. A Unit
// test that boots the kernel is in the wrong suite whatever its name is -- and
// two files were, until this ran: the fixed public paths were served in Unit,
// and the icon's bytes were read in Feature.
func TestEachSuiteHoldsWhatItsNameSays(t *testing.T) {
	// What only a Feature test may reach for.
	//
	// Booting and serving, and also opening a database: the relay tests make no
	// HTTP request and are Feature all the same, because they drive the outbox,
	// the store and the publisher together against a real schema. "Feature"
	// means more than one piece interacting, and HTTP is the common case rather
	// than the definition.
	// httptest.NewServer is on the list beside NewRequest, and it is the
	// stronger of the two: NewRequest builds a request for a handler to be
	// called with, and NewServer puts a listener on a port and answers over it.
	// A test that does the second and not the first was reading as booting
	// nothing.
	boots := regexp.MustCompile(`tests\.App\(|tests\.Kernel\(|bootstrap\.Dispatch\(|bootstrap\.Open\(|httptest\.NewRequest\(|httptest\.NewServer\(|migratedDB\(`)

	for _, suite := range []string{"Feature", "Unit"} {
		dir := filepath.Join(tests.Root(t), "tests", suite)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("reading tests/%s: %v", suite, err)
		}

		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}
			body, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				t.Fatal(err)
			}

			switch found := boots.Match(body); {
			case suite == "Unit" && found:
				t.Errorf("tests/Unit/%s boots the application: move it to tests/Feature, or the suite "+
					"nobody waits for becomes one they do", e.Name())
			case suite == "Feature" && !found:
				t.Errorf("tests/Feature/%s boots nothing: move it to tests/Unit, or Feature stops "+
					"meaning what it says", e.Name())
			}
		}
	}
}
