package unit_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// What the repository is allowed to contain, checked by reading the files rather
// than by trusting a name.
//
// Every rule here exists because something got committed. A pattern written for
// one shape of mistake misses the next one, and the next one is what ships.

// sqliteMagic is the first sixteen bytes of every SQLite file, whatever it is
// called. https://sqlite.org/fileformat.html
var sqliteMagic = []byte("SQLite format 3\x00")

// TestNoDatabaseIsTracked.
//
// Three were, twice: arandu_blog, arandu_blog-shm and arandu_blog-wal. The file
// has no extension -- the path in DATABASE_URL was a bare name, which SQLite reads
// relative to wherever the process started -- so *.sqlite missed it, *.db
// missed it, and database/* missed it because it was in the project root.
//
// This reads the first bytes of every tracked file instead. A database cannot
// hide behind a name nobody predicted, because it is not the name being read.
//
// The schema is database/migrations. A committed database disagrees with it by
// the second day, and the diff of a binary that changes on every run is a diff
// nobody can review.
func TestNoDatabaseIsTracked(t *testing.T) {
	for _, name := range tracked(t) {
		f, err := os.Open(name)
		if err != nil {
			// Tracked and absent is a different problem, and not this test's.
			continue
		}
		head := make([]byte, len(sqliteMagic))
		n, _ := f.Read(head)
		f.Close()

		if n == len(sqliteMagic) && bytes.Equal(head, sqliteMagic) {
			t.Errorf(`%s is a SQLite database and it is committed.

    git rm --cached %s

The path in DATABASE_URL is a PATH. A bare name puts the file wherever the
process started, which is the project root, where no ignore rule expects it --
the config refuses that value now, so this file predates the refusal.`, name, name)
		}
	}
}

// TestNoEnvFileIsTracked: .env carries the application key and every credential,
// and a repository is the one place it must never be.
func TestNoEnvFileIsTracked(t *testing.T) {
	for _, name := range tracked(t) {
		base := filepath.Base(name)
		if base == ".env" || (strings.HasPrefix(base, ".env.") && base != ".env.example") {
			t.Errorf("%s is committed, and it carries the application key", name)
		}
	}
}

// TestTheCompiledViewsAreNotTracked.
//
// storage/framework/views is build output: `aru view:build` writes it, every
// file in it opens with DO NOT EDIT, and a generated file in a repository is one
// people read diffs of, resolve conflicts in, and eventually edit.
//
// assets/app.css and assets/fonts are the deliberate exception and stay: `go
// build` has to work on a fresh clone, before any tool has run.
func TestTheCompiledViewsAreNotTracked(t *testing.T) {
	for _, name := range tracked(t) {
		if strings.HasPrefix(name, "storage/framework/views/") && !strings.HasSuffix(name, ".gitignore") {
			t.Errorf("%s is a compiled view and it is committed -- `aru view:build` writes it", name)
		}
	}
}

// tracked is what git has, which is the only list that matters: an ignored file
// on disk is nobody's problem, and a tracked one is in every clone.
func tracked(t *testing.T) []string {
	t.Helper()

	// ../.. and not ..: a Go test runs with the package directory as its working
	// directory, so from tests/Unit the project root is two levels up. One level
	// reached tests/, where git ls-files answers with paths relative to the root
	// anyway -- so every name was joined to the wrong prefix, every file failed
	// to open, and the loop skipped all of them. The test passed by reading
	// nothing, which is the worst way for a guard to pass.
	const root = "../.."

	out, err := exec.Command("git", "-C", root, "ls-files", "-z").Output()
	if err != nil {
		t.Skipf("git is not available, so what is tracked cannot be read: %v", err)
	}

	var names []string
	for _, name := range strings.Split(strings.TrimRight(string(out), "\x00"), "\x00") {
		if name != "" {
			names = append(names, filepath.Join(root, name))
		}
	}
	if len(names) == 0 {
		t.Skip("this is not a git checkout")
	}
	return names
}
