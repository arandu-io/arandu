//go:build authui

package conformance_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/arandu-io/framework/test/conformance"
	"github.com/arandu-io/hesape/auth/account"
	"github.com/arandu-io/hesape/routing/controllers"
	"github.com/arandu-io/hesape/session"

	authui "github.com/arandu-io/arandu/app/Http/Controllers/Auth"
	models "github.com/arandu-io/arandu/app/Models"
)

func TestSessionMatrix(t *testing.T) { conformance.SessionMatrix(t, sessionFixture()) }

// sessionFixture is this application's sign-in screens, built over the parts
// the kit controls the way app/Providers builds them over the real ones, and
// the user model that signs in.
func sessionFixture() conformance.SessionFixture {
	return conformance.SessionFixture{
		Account: func(passwordHash string) account.Account {
			return &models.User{
				ID: "user-a", TenantID: "tenant-a", Name: "Ana", Email: "ana@example.test",
				Password: passwordHash, Roles: models.Roles{models.RoleMember},
			}
		},
		Handler: func(p conformance.SessionParts) http.Handler {
			module := authui.New(
				p.Users, p.Factors, nil, p.Sessions, session.NewCSRF(p.AppKey, time.Hour), nil, nil,
				p.AppKey, authui.FixedTenant(p.Tenant), false,
			)
			router := controllers.NewRouter()
			module.Routes(router)
			return router
		},
	}
}
