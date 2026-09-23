// Package public serves the files a browser or a crawler asks for by a fixed
// path.
//
// Everything else this application ships is content-addressed: the stylesheet
// and the scripts live in the framework, are embedded in the binary, and are
// served from /_arandu/assets/<hash>/ with a URL the page writes for itself.
// /favicon.ico and /robots.txt are addresses the client chooses. The official
// SVG brand assets retain stable compatibility URLs here, while pages use
// their content-addressed registrations in the native asset registry.
//
// That is the whole reason this package exists, and the reason it is not a
// second asset pipeline: one path for anything a page references, this one for
// the short list of names the outside world already knows.
//
// There is no document root here either. A public/ directory is what a web
// server points at; ours is compiled into the binary like everything else, so
// the deploy stays one artifact and `git clone && aru dev` answers these URLs
// with nothing mounted and nothing published.
//
// Adding a file means adding it to the go:embed line below. Its extension has
// to be in contentTypes, or the build fails -- a file served with the wrong
// type is a file the browser quietly ignores.
package public

import (
	"bytes"
	"embed"
	"net/http"
	"path"
	"sort"

	fhttp "github.com/arandu-io/framework/http"
	"github.com/arandu-io/hesape/view"
)

// The files, compiled in. The list is explicit rather than a directory glob:
// a glob would also embed this source file, and a public/ that silently
// publishes whatever landed in it is how a stray dump file becomes a URL.
//
//go:embed favicon.ico favicon.png favicon.svg aru-icon.svg arandu.svg logo-hyzis.svg robots.txt
//go:embed site.webmanifest android-chrome-192x192.png android-chrome-512x512.png apple-touch-icon.png favicon-16x16.png favicon-32x32.png
//go:embed social-cover.png social-cover-pt.png social-cover-es.png
var files embed.FS

// contentTypes is the whole table, and it is deliberately short. A public/ that
// grows a third extension has stopped being the handful of names a client asks
// for and become a document root, which is the thing this framework does not
// have.
var contentTypes = map[string]string{
	".webmanifest": "application/manifest+json",
	".ico":         "image/x-icon",
	// Raster compatibility files support favicon fallbacks, home-screen icons
	// and social previews alongside the preferred vector mark.
	".png": "image/png",
	// The mark, drawn rather than rastered. It is an SVG because it is the same
	// file at every size a page asks for, and because a change to it reads as
	// text in a diff.
	".svg": "image/svg+xml",
	// robots.txt is served as plain text. The two files a model asks for are
	// generated controllers and therefore do not belong in this embedded set.
	".txt": "text/plain; charset=utf-8",
}

// cacheControl is an hour, not a year.
//
// The immutable caching the embedded assets get depends on the hash in their
// path: a new build is a new URL, so nothing can go stale. These names never
// change, so a long cache would mean a replaced favicon that browsers keep
// showing until they feel like asking again.
const cacheControl = "public, max-age=3600"

// file is one embedded file, read once at start.
type file struct {
	body        []byte
	contentType string
}

var served = map[string]file{}

func init() {
	entries, err := files.ReadDir(".")
	if err != nil {
		// The files are embedded at build time. If the directory cannot be read,
		// the binary is broken and there is nothing to recover from at runtime.
		panic("public: reading the embedded directory: " + err.Error())
	}
	for _, entry := range entries {
		name := entry.Name()
		contentType, known := contentTypes[path.Ext(name)]
		if !known {
			panic("public: " + name + " has no content type; add its extension to contentTypes")
		}
		body, err := files.ReadFile(name)
		if err != nil {
			panic("public: missing embedded file " + name + ": " + err.Error())
		}
		served[name] = file{body: body, contentType: contentType}
		// Pages use the native content-addressed route for immutable caching.
		// Keep the fixed names as compatibility URLs, with their shorter TTL.
		if path.Ext(name) == ".svg" {
			view.RegisterAsset(name, contentType, body)
			if name == "arandu.svg" {
				// The canonical artwork is the dark-theme brand: warm gold neutrals
				// with the original red and mint accents. Light mode derives only
				// the two gold outline paths as black, leaving every other path intact.
				light := bytes.ReplaceAll(body, []byte(`fill="#FEDD86"`), []byte(`fill="black"`))
				view.RegisterAsset("arandu-light.svg", contentType, light)
			}
		}
	}
}

// Names lists what is served, sorted. `aru routes` prints one line per name.
func Names() []string {
	out := make([]string, 0, len(served))
	for name := range served {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Routes registers one route per file, at the path its name spells.
//
// Registering the names rather than mounting a file server under a prefix is
// what keeps `aru routes` honest: every URL this application answers is a route
// in the table, including these.
func Routes(r *fhttp.Router, sitemapURL string) {
	for _, name := range Names() {
		r.Get("/"+name, handler(name, sitemapURL))
	}
}

// handler serves one file.
func handler(name, sitemapURL string) http.HandlerFunc {
	f := served[name]
	if name == "robots.txt" {
		f.body = append(append([]byte(nil), f.body...), []byte("\nSitemap: "+sitemapURL+"\n")...)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", f.contentType)
		w.Header().Set("Cache-Control", cacheControl)
		_, _ = w.Write(f.body)
	}
}
