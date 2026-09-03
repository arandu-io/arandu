// Package js carries this project's client script into the binary.
//
// It sits beside the file it embeds because //go:embed cannot reference a
// parent directory: a script under resources/ can be embedded only from a Go
// file under resources/. That is also where it belongs -- resources/ is what a
// person edits, and custom.js is a file a person edits.
//
// One file with one job, and the job is transport. Everything about what the
// script does is in the script.
package js

import (
	_ "embed"

	"github.com/arandu-io/hesape/view"
)

// script is where the application registers its own behaviours.
//
// It is served verbatim: there is no bundler, no transpiler and no minifier in
// this project, so the bytes here are the bytes the browser runs.
//
//go:embed custom.js
var script []byte

// init hands the script to the view layer, which serves it content-addressed
// from this origin, and the layout references it by name.
//
// Importing this package is what makes it happen -- the same shape the
// stylesheet and the views use -- so deleting the import in bootstrap is what
// turns it off. It has to happen: the layout asks for the asset by name, and a
// name nothing registered is refused rather than served as a broken URL.
//
// An empty file is refused here too, when the application starts. A zero-byte
// script is a tag that loads, runs nothing, and reads like a behaviour that was
// never registered.
func init() {
	view.RegisterAsset("custom.js", "application/javascript; charset=utf-8", script)
}
