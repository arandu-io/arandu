// Package providers holds this application's own modules.
//
// A provider here is a kernel.Module: it has a name, it registers routes, and it
// may declare migrations, scheduled tasks and a boot step. What it is not is a
// service provider in the container sense -- there is nothing to bind into and
// no deferred resolution, because every dependency is constructed in
// bootstrap/app.go and passed in by hand.
//
// The name survives anyway, and on purpose: this is the file somebody opens
// looking for "where the application registers itself" (RULE 10).
package providers

import (
	"github.com/arandu-io/framework/http"
	"github.com/arandu-io/framework/kernel"

	"github.com/arandu-io/arandu/database/migrations"
	"github.com/arandu-io/arandu/routes"
)

// AppServiceProvider is this application, seen by the kernel as one module.
//
// Everything the project itself adds -- its routes and its own migrations --
// arrives through here, so `aru routes` groups them under one name instead of
// scattering them among the framework's.
type AppServiceProvider struct {
	deps routes.Deps
}

// NewAppServiceProvider returns the provider. bootstrap builds the controllers
// and hands them over; nothing is resolved later.
func NewAppServiceProvider(deps routes.Deps) *AppServiceProvider {
	return &AppServiceProvider{deps: deps}
}

// The optional interfaces this provider implements, asserted at compile time so
// a typo in a method name fails the build instead of silently doing nothing.
var (
	_ kernel.Module      = (*AppServiceProvider)(nil)
	_ kernel.Migratable  = (*AppServiceProvider)(nil)
	_ kernel.Schedulable = (*AppServiceProvider)(nil)
)

// Name is the module identifier, and the group `aru routes` prints.
func (*AppServiceProvider) Name() string { return "app" }

// Routes registers what routes/web.go declares.
func (p *AppServiceProvider) Routes(r *http.Router) { routes.Web(r, p.deps) }

// Migrations are this application's own, from database/migrations.
//
// The framework modules bring theirs -- the users table comes from the auth
// module, the outbox from events, the jobs table from queue -- so this list
// starts empty and holds only what the project adds.
func (*AppServiceProvider) Migrations() []kernel.Migration { return migrations.All() }

// Schedule declares the recurring work of this application.
//
// A module never starts a goroutine of its own: it declares tasks here, and the
// scheduler module runs them with a lock and a Grant.
func (*AppServiceProvider) Schedule() []kernel.Task {
	return nil
}
