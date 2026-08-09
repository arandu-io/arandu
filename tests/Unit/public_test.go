package unit_test

import (
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arandu-io/arandu/tests"

	"github.com/arandu-io/framework/config"
)

// The two URLs nobody writes a link to and every client asks for anyway.
//
// A browser requests /favicon.ico on its own, and the layout links it by name;
// a crawler requests /robots.txt before anything else. Both used to answer 404,
// because the skeleton had the files on disk and no way to serve them: there is
// no document root here, so a file that is not embedded and routed does not
// exist.

func TestTheFixedPublicPathsAreServed(t *testing.T) {
	k := tests.Kernel(t, config.EnvDev)

	for _, tc := range []struct {
		path        string
		contentType string
	}{
		{"/favicon.ico", "image/x-icon"},
		{"/robots.txt", "text/plain"},
	} {
		rec := httptest.NewRecorder()
		k.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))

		if rec.Code != http.StatusOK {
			t.Errorf("GET %s answered %d, want 200: the file is not embedded or not routed", tc.path, rec.Code)
			continue
		}
		if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, tc.contentType) {
			t.Errorf("GET %s served %q, want %s", tc.path, got, tc.contentType)
		}
		if rec.Body.Len() == 0 {
			t.Errorf("GET %s served an empty body", tc.path)
		}
	}
}

// TestTheLayoutLinksAFaviconThatExists closes the loop the layout opens. It
// emits <link rel="icon" href="/favicon.ico">, and a page that asks for a file
// the application does not answer for is a 404 on every single request.
func TestTheLayoutLinksAFaviconThatExists(t *testing.T) {
	k := tests.Kernel(t, config.EnvDev)

	page := httptest.NewRecorder()
	k.Handler().ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/", nil))
	href := `href="/favicon.ico"`
	if !strings.Contains(page.Body.String(), href) {
		t.Skipf("the layout no longer links %s; nothing to close the loop on", href)
	}

	icon := httptest.NewRecorder()
	k.Handler().ServeHTTP(icon, httptest.NewRequest(http.MethodGet, "/favicon.ico", nil))
	if icon.Code != http.StatusOK {
		t.Fatalf("the layout links /favicon.ico and the application answers %d", icon.Code)
	}
}

// TestTheFaviconIsARealIcon: an empty file is served with 200 and shows nothing,
// which is indistinguishable from a working icon in every test that only checks
// the status. The skeleton shipped exactly that -- favicon.ico, zero bytes.
func TestTheFaviconIsARealIcon(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(tests.Root(t), "public", "favicon.ico"))
	if err != nil {
		t.Fatalf("reading public/favicon.ico: %v", err)
	}
	if len(body) < 22 {
		t.Fatalf("public/favicon.ico is %d bytes: too short to be an icon", len(body))
	}
	// ICONDIR: two reserved bytes, type 1 (icon), then the number of images.
	if binary.LittleEndian.Uint16(body[0:2]) != 0 || binary.LittleEndian.Uint16(body[2:4]) != 1 {
		t.Fatal("public/favicon.ico does not start with an ICONDIR header")
	}
	count := binary.LittleEndian.Uint16(body[4:6])
	if count == 0 {
		t.Fatal("public/favicon.ico declares no images")
	}
	// Every declared image has to be inside the file, or a renderer reads past
	// the end and draws nothing.
	for i := 0; i < int(count); i++ {
		entry := 6 + i*16
		if entry+16 > len(body) {
			t.Fatalf("image %d has no directory entry", i)
		}
		length := binary.LittleEndian.Uint32(body[entry+8 : entry+12])
		offset := binary.LittleEndian.Uint32(body[entry+12 : entry+16])
		if length == 0 {
			t.Errorf("image %d is empty", i)
		}
		if int(offset)+int(length) > len(body) {
			t.Errorf("image %d claims %d bytes at %d, past the end of a %d byte file", i, length, offset, len(body))
		}
	}
}
