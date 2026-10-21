//go:build browser

// Package browser_test is where this application's tests that drive a real
// browser live. They sit behind the browser build tag, so `go test ./...` and
// the ordinary CI step never compile them, and `aru test --browser` runs them:
//
//	aru test --browser    go test -tags browser ./tests/Browser/...
//
// Each test boots the application on a port of its own and points Chrome at it,
// headless. A test that finds no Chrome skips, naming what it looked for, so
// the suite answers on a machine without one instead of failing there.
//
// Screenshots and reports go to tests/Browser/screenshots, which is
// gitignored.
package browser_test

import (
	"context"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arandu-io/arandu/tests"
)

// chromeCandidates are where Chrome or Chromium is found, after CHROME_PATH:
// the names on PATH first, then the macOS application bundles.
var chromeCandidates = []string{
	"google-chrome",
	"google-chrome-stable",
	"chromium",
	"chromium-browser",
	"chrome",
	"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
	"/Applications/Chromium.app/Contents/MacOS/Chromium",
}

// chrome answers the browser binary, or skips the test naming where it looked.
func chrome(t *testing.T) string {
	t.Helper()
	if path := os.Getenv("CHROME_PATH"); path != "" {
		return path
	}
	for _, candidate := range chromeCandidates {
		if path, err := exec.LookPath(candidate); err == nil {
			return path
		}
	}
	t.Skipf("no Chrome or Chromium was found (CHROME_PATH, then %s); a browser test needs one",
		strings.Join(chromeCandidates, ", "))
	return ""
}

// headless runs Chrome once against url with the flags given and answers what
// it printed. The profile is a directory of the test's own, so a browser the
// developer has open is not touched.
func headless(t *testing.T, binary, url string, flags ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	args := append([]string{
		"--headless=new",
		"--disable-gpu",
		"--no-first-run",
		"--no-default-browser-check",
		"--user-data-dir=" + t.TempDir(),
	}, flags...)
	out, err := exec.CommandContext(ctx, binary, append(args, url)...).Output()
	if err != nil {
		t.Fatalf("running %s against %s: %v", filepath.Base(binary), url, err)
	}
	return string(out)
}

// TestTheHomePageRendersInABrowser is the example: the application served on
// a real port, the landing page loaded by Chrome, the DOM it built read back
// and a screenshot kept for whoever looks at the run.
func TestTheHomePageRendersInABrowser(t *testing.T) {
	binary := chrome(t)

	app := tests.Booted(t)
	server := httptest.NewServer(app.Kernel.Handler())
	t.Cleanup(server.Close)

	dom := headless(t, binary, server.URL+"/", "--dump-dom")
	if !strings.Contains(dom, tests.AppName) {
		t.Errorf("the page Chrome built does not draw the application's name %q:\n%s", tests.AppName, dom)
	}

	shots := filepath.Join(tests.Root(t), "tests", "Browser", "screenshots")
	if err := os.MkdirAll(shots, 0o755); err != nil {
		t.Fatalf("creating %s: %v", shots, err)
	}
	shot := filepath.Join(shots, "home.png")
	headless(t, binary, server.URL+"/", "--window-size=1280,800", "--screenshot="+shot)
	if info, err := os.Stat(shot); err != nil || info.Size() == 0 {
		t.Errorf("Chrome wrote no screenshot at %s: %v", shot, err)
	}
}
