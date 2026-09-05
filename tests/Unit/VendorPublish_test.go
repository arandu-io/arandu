package unit_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/arandu-io/framework/foundation"

	"github.com/arandu-io/arandu/bootstrap"
)

// What `aru vendor:publish` reaches when it forwards to this binary.
//
// The three promises are what these tests are for, and none of them is
// checkable by reading the command: nothing is written without --apply, a
// second run writes nothing at all, and a file somebody edited is reported
// rather than replaced. Each of them is a file on disk before and after.

// mailConfig is a published file with a custom block in it, which is the shape
// every one of these tests needs: the region between the markers is the only
// part of a published file a project may edit and keep.
//
// The markers sit at column zero, and that is not a preference. The merge that
// carries a block forward re-emits the indentation of the closing marker line
// on top of the body it already captured, so a block indented inside a function
// gains a tab on every republication and never settles -- the file is rewritten
// forever and the guarantee that publishing twice writes nothing is lost. It is
// a defect in the merge, and a project cannot work around it here: a second
// merge beside the one that exists is the thing one engine was built to
// prevent.
const mailConfig = `package config

// Transports is what this project can send with.
var Transports = []string{"smtp"}

// arandu:begin custom
// Anything this project adds goes here.
// arandu:end custom
`

// publication is one tree a module offers, built the way a module builds it.
func publication(tag foundation.PublishTag, to string, files map[string]string) foundation.Publication {
	tree := fstest.MapFS{}
	for name, body := range files {
		tree[name] = &fstest.MapFile{Data: []byte(body)}
	}
	return foundation.Publication{Tag: tag, Files: tree, To: to}
}

// configPublication is the one most of these tests publish: a single file, into
// a directory every project has.
func configPublication() []foundation.Publication {
	return []foundation.Publication{
		publication(foundation.PublishConfig, "config", map[string]string{"mail.go": mailConfig}),
	}
}

// publishInto runs the command under a root of its own and returns what it
// printed, so a test reads the same listing a person does.
func publishInto(t *testing.T, root string, publications []foundation.Publication, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := bootstrap.Publish(root, publications, args, &out)
	return out.String(), err
}

// read is the file as it is on disk, and fails the test when it is not there.
func read(t *testing.T, root, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	return string(body)
}

// write puts a file under root, creating the directories above it.
func write(t *testing.T, root, name, body string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("creating the directory for %s: %v", name, err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
}

// missing reports whether nothing is at that path.
func missing(t *testing.T, root, name string) bool {
	t.Helper()
	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(name)))
	return os.IsNotExist(err)
}

// TestAPreviewWritesNothing is the first of the three promises, and the one a
// person leans on every time they run the command to look before deciding.
func TestAPreviewWritesNothing(t *testing.T) {
	root := t.TempDir()

	out, err := publishInto(t, root, configPublication())

	if err != nil {
		t.Fatalf("the preview failed: %v", err)
	}
	if !strings.Contains(out, "create") || !strings.Contains(out, "config/mail.go") {
		t.Errorf("the preview does not say what it would do:\n%s", out)
	}
	if !missing(t, root, "config/mail.go") {
		t.Error("a preview wrote the file")
	}
	if !missing(t, root, "vendor-publish.lock") {
		t.Error("a preview wrote the lock")
	}
}

// TestApplyWritesTheFileAndRecordsIt: the lock is half of what is written,
// because it is what the next run reads to tell this file from an edited one.
func TestApplyWritesTheFileAndRecordsIt(t *testing.T) {
	root := t.TempDir()

	if _, err := publishInto(t, root, configPublication(), "--apply"); err != nil {
		t.Fatalf("--apply failed: %v", err)
	}

	if got := read(t, root, "config/mail.go"); got != mailConfig {
		t.Errorf("the published file is not what the module offered:\n%s", got)
	}
	lock := read(t, root, "vendor-publish.lock")
	if !strings.Contains(lock, "config/mail.go") {
		t.Errorf("the lock does not record the file that was written:\n%s", lock)
	}
}

// TestPublishingTwiceChangesNothing is the promise that makes the command safe
// to run when you are not sure whether you already ran it.
//
// The lock is compared byte for byte along with the file, because it is written
// on every apply: a lock that reordered itself would turn every second run into
// a line in somebody's review.
func TestPublishingTwiceChangesNothing(t *testing.T) {
	root := t.TempDir()

	if _, err := publishInto(t, root, configPublication(), "--apply"); err != nil {
		t.Fatalf("first --apply: %v", err)
	}
	first, firstLock := read(t, root, "config/mail.go"), read(t, root, "vendor-publish.lock")

	out, err := publishInto(t, root, configPublication(), "--apply")

	if err != nil {
		t.Fatalf("second --apply: %v", err)
	}
	if !strings.Contains(out, "unchanged") {
		t.Errorf("the second run does not report the file as unchanged:\n%s", out)
	}
	if got := read(t, root, "config/mail.go"); got != first {
		t.Errorf("the second run rewrote the file:\n%s", got)
	}
	if got := read(t, root, "vendor-publish.lock"); got != firstLock {
		t.Errorf("the second run rewrote the lock:\n%s", got)
	}
}

// TestAnEditInsideTheMarkersSurvives.
//
// This is the escape hatch the whole mechanism rests on. Without it nobody runs
// the command a second time, because the first time it ate their work.
func TestAnEditInsideTheMarkersSurvives(t *testing.T) {
	root := t.TempDir()
	if _, err := publishInto(t, root, configPublication(), "--apply"); err != nil {
		t.Fatalf("first --apply: %v", err)
	}

	edited := strings.Replace(mailConfig, "// Anything this project adds goes here.", "const Retries = 3", 1)
	write(t, root, "config/mail.go", edited)

	out, err := publishInto(t, root, configPublication(), "--apply")

	if err != nil {
		t.Fatalf("republishing over a custom block failed: %v", err)
	}
	if strings.Contains(out, "conflict") {
		t.Errorf("an edit inside the markers was called a conflict:\n%s", out)
	}
	if got := read(t, root, "config/mail.go"); got != edited {
		t.Errorf("the edit inside the markers was lost:\n%s", got)
	}
}

// TestAnEditOutsideTheMarkersIsReportedAndNotReplaced is the third promise, and
// the exit status is half of it: a run that skipped a file and exited zero is a
// run that says it published and did not.
func TestAnEditOutsideTheMarkersIsReportedAndNotReplaced(t *testing.T) {
	root := t.TempDir()
	if _, err := publishInto(t, root, configPublication(), "--apply"); err != nil {
		t.Fatalf("first --apply: %v", err)
	}

	edited := mailConfig + "\n// The project wrote this line, outside the markers.\n"
	write(t, root, "config/mail.go", edited)

	out, err := publishInto(t, root, configPublication(), "--apply")

	if err == nil {
		t.Fatal("a file that was left alone was reported as a successful publication")
	}
	if !strings.Contains(err.Error(), "config/mail.go") {
		t.Errorf("the refusal does not name the file: %v", err)
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("the refusal does not say what publishes over it: %v", err)
	}
	if !strings.Contains(out, "conflict") {
		t.Errorf("the listing does not report the conflict:\n%s", out)
	}
	if got := read(t, root, "config/mail.go"); got != edited {
		t.Errorf("the file was replaced anyway:\n%s", got)
	}
}

// TestForcePublishesOverAConflictAndKeepsTheCustomBlock.
//
// --force gives up the edit made outside the markers, and only that one. An
// implementation that skipped the merge on this path would be the command
// eating the block it promised to keep.
func TestForcePublishesOverAConflictAndKeepsTheCustomBlock(t *testing.T) {
	root := t.TempDir()
	if _, err := publishInto(t, root, configPublication(), "--apply"); err != nil {
		t.Fatalf("first --apply: %v", err)
	}

	edited := strings.Replace(mailConfig, "// Anything this project adds goes here.", "const Retries = 3", 1)
	write(t, root, "config/mail.go", edited+"\n// Outside, and this one goes.\n")

	if _, err := publishInto(t, root, configPublication(), "--apply", "--force"); err != nil {
		t.Fatalf("--force failed: %v", err)
	}

	got := read(t, root, "config/mail.go")
	if !strings.Contains(got, "const Retries = 3") {
		t.Errorf("--force ate the custom block:\n%s", got)
	}
	if strings.Contains(got, "Outside, and this one goes") {
		t.Errorf("--force kept an edit made outside the markers:\n%s", got)
	}
}

// TestAFileTheProjectAlreadyHasIsAConflict.
//
// "The file is there" cannot separate what this command wrote from what
// somebody else did, so a file with no record behind it is left alone -- which
// is what stops a module from quietly replacing a project's own config on the
// first run.
func TestAFileTheProjectAlreadyHasIsAConflict(t *testing.T) {
	root := t.TempDir()
	mine := "package config\n\n// Mine, and nothing published it.\n"
	write(t, root, "config/mail.go", mine)

	out, err := publishInto(t, root, configPublication(), "--apply")

	if err == nil {
		t.Fatal("publishing over a file it never wrote was reported as a success")
	}
	if !strings.Contains(out, "conflict") {
		t.Errorf("the listing does not report the conflict:\n%s", out)
	}
	if got := read(t, root, "config/mail.go"); got != mine {
		t.Errorf("the project's own file was replaced:\n%s", got)
	}
}

// TestTagPublishesOneKindAndRefusesAnyOther.
//
// The set is closed at six. A tag from outside it has to be refused by name and
// the six listed, because the alternative is a command that publishes nothing
// and says nothing about why.
func TestTagPublishesOneKindAndRefusesAnyOther(t *testing.T) {
	publications := []foundation.Publication{
		publication(foundation.PublishConfig, "config", map[string]string{"mail.go": mailConfig}),
		publication(foundation.PublishTranslation, "resources/lang", map[string]string{"en.json": "{}\n"}),
	}

	t.Run("one kind", func(t *testing.T) {
		root := t.TempDir()

		if _, err := publishInto(t, root, publications, "--tag=translation", "--apply"); err != nil {
			t.Fatalf("--tag=translation: %v", err)
		}

		if missing(t, root, "resources/lang/en.json") {
			t.Error("the translation was not published")
		}
		if !missing(t, root, "config/mail.go") {
			t.Error("--tag=translation published the configuration too")
		}
	})

	t.Run("a kind that does not exist", func(t *testing.T) {
		_, err := publishInto(t, t.TempDir(), publications, "--tag=stylesheet")

		if err == nil {
			t.Fatal("a tag from outside the set was accepted")
		}
		for _, tag := range foundation.PublishTags() {
			if !strings.Contains(err.Error(), string(tag)) {
				t.Errorf("the refusal does not name %s: %v", tag, err)
			}
		}
	})

	t.Run("a kind nothing offers", func(t *testing.T) {
		root := t.TempDir()

		out, err := publishInto(t, root, publications, "--tag=migration", "--apply")

		if err != nil {
			t.Fatalf("a tag nothing offers is not a failure: %v", err)
		}
		if !strings.Contains(out, "migration") {
			t.Errorf("the answer does not say which kind nothing offers:\n%s", out)
		}
		if !missing(t, root, "vendor-publish.lock") {
			t.Error("a publication with nothing in it wrote a lock")
		}
	})
}

// TestAnApplicationThatPublishesNothingSaysSo, which is what a project gets
// before it adds its first module -- and every message about an empty answer
// has to say what would fill it.
func TestAnApplicationThatPublishesNothingSaysSo(t *testing.T) {
	root := t.TempDir()

	out, err := publishInto(t, root, nil, "--apply")

	if err != nil {
		t.Fatalf("publishing nothing is not a failure: %v", err)
	}
	if !strings.Contains(out, "Publishes()") {
		t.Errorf("the answer does not say how a module offers files:\n%s", out)
	}
	if !missing(t, root, "vendor-publish.lock") {
		t.Error("an application that publishes nothing wrote a lock")
	}
}
