package bootstrap

import (
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/arandu-io/framework/foundation"
	"github.com/arandu-io/hesape/publish"
)

// The publication command: what the registered modules offer this project, and
// writing it into the tree.
//
// It answers here rather than in the CLI for the reason migrate and routes do:
// the modules are wired by hand in bootstrap/app.go, and nothing compiled
// outside this binary can read that list.

// publishLock is where the record of what was published lives, relative to the
// project root.
//
// It is committed, and that is the whole of why it is at the root beside
// go.sum. The record is the only thing separating a file this command wrote
// from one somebody else did: without it every published file comes back as a
// conflict, so a clone that did not carry it is a clone where publishing stops.
// It also has to be reviewed, because a line appearing in it is a file the
// project did not write appearing in the tree.
const publishLock = "vendor-publish.lock"

// Publish reports what the registered modules would write into the project at
// root, and writes it when --apply is given.
//
// Nothing is written without --apply. Publishing twice writes nothing the
// second time. A file changed outside its custom markers is reported and left
// alone, and --force publishes over that one -- carrying what is between the
// markers forward even then.
//
// Exported for the reason Open is: a test drives the command a person runs,
// under a root of its own, rather than a second copy of it.
func Publish(root string, publications []foundation.Publication, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("vendor:publish", flag.ContinueOnError)
	tag := flags.String("tag", "", "publish only one kind of file")
	apply := flags.Bool("apply", false, "write the files; without it nothing is written")
	force := flags.Bool("force", false, "publish over a file changed outside its custom markers")
	if err := flags.Parse(args); err != nil {
		return err
	}

	sources, err := publishSources(publications, foundation.PublishTag(*tag))
	if err != nil {
		return err
	}
	if len(sources) == 0 {
		return nothingToPublish(out, *tag)
	}

	lockPath := filepath.Join(root, publishLock)
	lock, err := publish.ReadLock(lockPath)
	if err != nil {
		return err
	}

	changes, err := publish.Plan(root, lock, publish.Options{Force: *force}, sources...)
	if err != nil {
		return err
	}
	if err := report(out, changes); err != nil {
		return err
	}

	if !*apply {
		_, err := fmt.Fprintln(out, "nothing was written. Run it again with --apply.")
		return err
	}

	// The lock is written whatever Apply answered, and before the answer is
	// returned. A file written and not recorded is a file the next run calls a
	// conflict, so a publication that stopped halfway has to leave behind what
	// it got through.
	_, applyErr := publish.Apply(root, lock, changes)
	if len(lock.Files) > 0 {
		if err := lock.Write(lockPath); err != nil {
			return err
		}
	}
	if applyErr != nil {
		return applyErr
	}

	if _, err := fmt.Fprintln(out, "applied."); err != nil {
		return err
	}
	return leftAlone(changes)
}

// publishSources turns what the modules declared into what the engine writes,
// keeping only the kind of file that was asked for.
//
// The origin recorded against every file is the tag, because a tag is all a
// publication still carries by the time it arrives here: collecting them over
// the modules flattens the list, and the name of the module that offered one
// does not survive it.
func publishSources(publications []foundation.Publication, tag foundation.PublishTag) ([]publish.Source, error) {
	if tag != "" && !tag.Valid() {
		return nil, fmt.Errorf("%q is not a kind of file a module publishes (expected one of %s)",
			tag, strings.Join(publishTagNames(), ", "))
	}

	var sources []publish.Source
	for _, publication := range publications {
		if tag != "" && publication.Tag != tag {
			continue
		}
		sources = append(sources, publish.Source{
			Files:  publication.Files,
			From:   publication.From,
			To:     publication.To,
			Origin: string(publication.Tag),
		})
	}
	return sources, nil
}

// publishTagNames is the closed set, spelled the way a person types it.
//
// It is derived rather than written out, so the six a refusal names are the six
// a module may declare and the two cannot disagree.
func publishTagNames() []string {
	tags := foundation.PublishTags()
	names := make([]string, 0, len(tags))
	for _, tag := range tags {
		names = append(names, string(tag))
	}
	return names
}

// nothingToPublish answers the two ways the plan can be empty, and they are
// different questions: a project whose modules publish nothing at all, and a
// tag that no module offers.
func nothingToPublish(out io.Writer, tag string) error {
	if tag != "" {
		_, err := fmt.Fprintf(out, "no registered module publishes a %s.\n", tag)
		return err
	}
	if _, err := fmt.Fprintln(out, "no registered module publishes anything."); err != nil {
		return err
	}
	_, err := fmt.Fprintln(out, "A module offers files with Publishes() []foundation.Publication.")
	return err
}

// report prints one line per file, then the tally.
//
// The words are the actions themselves -- create, update, unchanged, conflict
// -- so the summary needs no plural of its own and reads the same for one file
// as for forty.
func report(out io.Writer, changes []publish.Change) error {
	if len(changes) == 0 {
		// A module that offers a tree with nothing in it. The tally below would
		// be a blank line, which reads as output that went missing.
		_, err := fmt.Fprintln(out, "the modules that publish offer no files.")
		return err
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	for _, change := range changes {
		fmt.Fprintf(w, "%s\t%s\n", change.Action, change.Path)
	}
	if err := w.Flush(); err != nil {
		return err
	}

	counted := map[publish.Action]int{}
	for _, change := range changes {
		counted[change.Action]++
	}
	var parts []string
	for _, action := range []publish.Action{publish.Create, publish.Update, publish.Unchanged, publish.Conflict} {
		if counted[action] > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", action, counted[action]))
		}
	}
	_, err := fmt.Fprintf(out, "\n%s\n", strings.Join(parts, ", "))
	return err
}

// leftAlone is the error a publication ends with when it skipped a file.
//
// An error and not one more line, because a run that skipped files and exited
// zero is a run that says it published and did not: whatever comes next in a
// pipeline reads the status, never the listing.
func leftAlone(changes []publish.Change) error {
	var skipped []string
	for _, change := range changes {
		if change.Action == publish.Conflict {
			skipped = append(skipped, change.Path)
		}
	}
	if len(skipped) == 0 {
		return nil
	}

	return fmt.Errorf("left alone, having been changed outside the arandu:begin custom markers "+
		"or never published here:\n  %s\n"+
		"--force publishes over them, and carries what is between the markers forward even then",
		strings.Join(skipped, "\n  "))
}
