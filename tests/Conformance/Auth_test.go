// Package conformance_test runs the framework's authentication kits against
// this application, one call each. What each kit proves is written once, in
// github.com/arandu-io/framework/test/conformance; what is this application's
// -- its root, its users migrations, its user model and policy, its request
// types -- is what it is handed, from fixtures_test.go.
//
// The sign-in screens' matrix is in Sessions_test.go, behind the authui build
// tag: the screens are published into app/Http/Controllers/Auth by `ui auth`,
// and a skeleton that has not run it has no screens to drive.
package conformance_test

import (
	"testing"

	"github.com/arandu-io/framework/test/conformance"
)

func TestBoundaries(t *testing.T) { conformance.Boundaries(t, "../..") }

func TestSchema(t *testing.T) { conformance.Schema(t, schemaFixture()) }

func TestAccount(t *testing.T) { conformance.Account(t, accountFixture) }

func TestModelPersistence(t *testing.T) { conformance.ModelPersistence(t, modelFixture) }

func TestRedaction(t *testing.T) { conformance.Redaction(t, redacted()...) }
