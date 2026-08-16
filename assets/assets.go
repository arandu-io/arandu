// Package assets carries what `aru view:build` compiled, into the binary.
//
// This is one file with one job, and it exists because the alternative was a
// pipeline whose output nobody consumed: `aru view:build` ran Tailwind over
// resources/css/app.css and wrote assets/app.css, and nothing embedded it, so
// the browser kept receiving the framework's default stylesheet -- byte for
// byte, same md5. Every class written in a view of this project was absent from
// the page, and nothing said so.
package assets

import (
	_ "embed"

	"github.com/arandu-io/framework/view"
)

// stylesheet is the compiled output of resources/css/app.css.
//
// It is committed rather than gitignored, for the same reason a Go project
// commits generated code: `go build` has to work on a fresh clone, before any
// tool has run. `aru view:build` overwrites it.
//
//go:embed app.css
var stylesheet []byte

// init hands the stylesheet to the view layer, which serves it under the one
// name there is.
//
// It replaces the framework's default rather than adding a second file: one
// name, one URL, one set of bytes, no cascade order to get wrong.
//
// Importing this package is what makes it happen -- the same shape as a
// database/sql driver, and the same shape the views themselves use. bootstrap
// carries the import, so deleting it there is what turns this off.
func init() {
	view.RegisterStylesheet(stylesheet)
}
