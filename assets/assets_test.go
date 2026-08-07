package assets_test

import (
	"crypto/md5"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/arandu-io/framework/httpx"
	"github.com/arandu-io/framework/view"

	_ "github.com/arandu-io/arandu/assets"
)

// TestTheBrowserGetsThisProjectsStylesheet is the regression guard for a whole
// pipeline whose output nobody consumed.
//
// `aru view:build` ran Tailwind over resources/css/app.css and wrote
// assets/app.css, and nothing embedded it. The browser kept receiving the
// framework's default -- byte for byte, same md5 -- so every class written in a
// view of this project was absent from the page, with no error anywhere.
//
// The check is the md5 of what comes over HTTP against the md5 of the file on
// disk. Anything weaker passes in the broken state: the framework's default is
// also valid CSS, also served with a 200, and also has Tailwind's banner.
func TestTheBrowserGetsThisProjectsStylesheet(t *testing.T) {
	onDisk, err := os.ReadFile("app.css")
	if err != nil {
		t.Fatalf("assets/app.css is missing: it is committed so that `go build` works on a fresh clone: %v", err)
	}

	r := httpx.NewRouter()
	view.NewModule().Routes(r)
	server := httptest.NewServer(r)
	defer server.Close()

	resp, err := http.Get(server.URL + view.URL(view.Stylesheet))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	served, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the stylesheet answered %d", resp.StatusCode)
	}

	if sum(served) != sum(onDisk) {
		t.Errorf("the browser is served a different stylesheet than assets/app.css.\n  served   %s (%d bytes)\n  on disk  %s (%d bytes)\nThis is the framework's default: nothing registered this project's.",
			sum(served), len(served), sum(onDisk), len(onDisk))
	}
}

func sum(b []byte) string {
	h := md5.Sum(b)
	return hex.EncodeToString(h[:])
}
