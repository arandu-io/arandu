package main

import (
	"os"
	"strings"
	"testing"
)

// The Dockerfile is a deploy contract, and the properties below are the ones
// that fail expensively and late: an image that runs as root, one that carries a
// shell, one built with cgo that will not start on distroless, and one that
// migrates at boot.
//
// The CI builds the image for real. This checks the shape, which is what catches
// an edit that looks harmless.

func dockerfile(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile("../../Dockerfile")
	if err != nil {
		t.Fatalf("the skeleton has no Dockerfile, and doc 17 says it does: %v", err)
	}
	return string(body)
}

func TestTheImageIsStaticAndDistroless(t *testing.T) {
	body := dockerfile(t)

	// CGO off is what makes the binary static, and what lets distroless run it
	// at all. With cgo on, the image builds and the container exits immediately
	// with "no such file or directory" -- naming the binary that is right there.
	if !strings.Contains(body, "CGO_ENABLED=0") {
		t.Error("the build does not disable cgo, and a dynamically linked binary will not start on distroless")
	}
	if !strings.Contains(body, "distroless") {
		t.Error("the runtime stage is not distroless")
	}
	// Multi-stage, or the toolchain ships to production.
	if strings.Count(body, "FROM ") < 2 {
		t.Error("the Dockerfile is not multi-stage: the Go toolchain would ship in the image")
	}
	// -trimpath keeps the build machine's paths out of the binary, so a stack
	// trace from production does not name somebody's home directory.
	if !strings.Contains(body, "-trimpath") {
		t.Error("the build does not trim paths")
	}
}

// TestTheContainerDoesNotRunAsRoot: a container running as root is one escape
// away from being root on the node.
func TestTheContainerDoesNotRunAsRoot(t *testing.T) {
	body := dockerfile(t)

	if !strings.Contains(body, "USER nonroot") {
		t.Error("the image does not drop to a non-root user")
	}
}

// TestTheImageDoesNotMigrateAtBoot is RULE 16, checked where it would be
// violated. With N replicas starting together, N migrations race -- and the
// tempting place to put `aru migrate` is exactly the entrypoint.
func TestTheImageDoesNotMigrateAtBoot(t *testing.T) {
	body := dockerfile(t)

	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !strings.HasPrefix(trimmed, "ENTRYPOINT") && !strings.HasPrefix(trimmed, "CMD") {
			continue
		}
		if strings.Contains(trimmed, "migrate") {
			t.Errorf("the image migrates at boot, and N replicas would race: %s", trimmed)
		}
	}
}

// TestTheBuildContextExcludesSecrets: .env in an image is a credential shipped
// to a registry, and registries are copied around.
func TestTheBuildContextExcludesSecrets(t *testing.T) {
	body, err := os.ReadFile("../../.dockerignore")
	if err != nil {
		t.Fatalf("there is no .dockerignore: %v", err)
	}

	for _, want := range []string{".env", ".git"} {
		if !strings.Contains(string(body), want) {
			t.Errorf(".dockerignore does not exclude %s", want)
		}
	}
}
