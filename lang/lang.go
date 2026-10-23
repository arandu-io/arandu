// Package lang carries this application's catalogue into the binary.
//
// The catalogue is the JSON files beside this one: lang/<locale>.json holds
// the lines a page says through .Trans, keyed by name, and a group is
// lang/<locale>/<group>.json. A locale is added by adding its file -- a
// lang/pt-BR.json with the same keys -- and the translator reads it at boot,
// where a file that does not parse stops the application instead of losing a
// sentence on the one request that needed it.
//
// It sits beside the files it embeds because //go:embed cannot reference a
// parent directory. Embedded rather than read from disk, so the image carries
// the catalogue the binary was built with and nothing has to be copied beside
// it.
package lang

import "embed"

// Files is the catalogue, rooted at this directory: en.json is at "en.json".
// A locale directory added here joins it once the pattern below names it.
//
//go:embed *.json
var Files embed.FS
