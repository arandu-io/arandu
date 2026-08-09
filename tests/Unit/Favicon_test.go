package unit_test

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/arandu-io/arandu/tests"
)

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
