package unit_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arandu-io/arandu/tests"
)

// TestComposePublishesTheDataStoresOnLoopbackOnly: compose.yml starts Postgres
// with a password written in the file and Redis with none. A port published as
// "5432:5432" is bound to every interface, so anyone on the same network reads
// and writes both. Each published port has to name 127.0.0.1.
func TestComposePublishesTheDataStoresOnLoopbackOnly(t *testing.T) {
	body, err := os.ReadFile(filepath.Join(tests.Root(t), "compose.yml"))
	if err != nil {
		t.Fatalf("reading compose.yml: %v", err)
	}

	published := 0
	inPorts := false
	for i, raw := range strings.Split(string(body), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if line == "ports:" {
			inPorts = true
			continue
		}
		if !inPorts {
			continue
		}
		if !strings.HasPrefix(line, "- ") {
			inPorts = false
			continue
		}
		published++
		port := strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "- ")), `"'`)
		if !strings.HasPrefix(port, "127.0.0.1:") {
			t.Errorf("compose.yml:%d publishes %q on every interface; write it as \"127.0.0.1:%s\"", i+1, port, port)
		}
	}
	// Every assertion above holds for a file with no ports at all, so a parse
	// that stopped finding them would pass silently.
	if published == 0 {
		t.Fatal("compose.yml publishes no port, so nothing was checked")
	}
}
