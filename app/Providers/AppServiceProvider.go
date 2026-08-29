// Package providers holds this application's own modules.
//
// A provider here is a kernel.Module: it has a name, it registers routes, and it
// may declare migrations, scheduled tasks and a boot step. What it is not is a
// service provider in the container sense -- there is nothing to bind into and
// no deferred resolution, because every dependency is constructed in
// bootstrap/app.go and passed in by hand.
//
// The name survives anyway, and on purpose: this is the file somebody opens
// looking for "where the application registers itself".
package providers

import (
	"context"
	"errors"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/http"
	"github.com/arandu-io/framework/kernel"

	"github.com/arandu-io/arandu/routes"
)

// AppServiceProvider is this application, seen by the kernel as one module.
//
// The routes the project adds arrive through here, so `aru routes` groups them
// under one name instead of scattering them among the framework's. Its
// migrations do not: a migration registers itself from init() in
// database/migrations, and a module that also listed them would be a second
// place to look for the same schema.
type AppServiceProvider struct {
	deps routes.Deps
	db   *data.DB
}

// NewAppServiceProvider returns the provider. bootstrap builds the controllers
// and hands them over; nothing is resolved later.
func NewAppServiceProvider(deps routes.Deps) *AppServiceProvider {
	return &AppServiceProvider{deps: deps}
}

// WithDatabase adds the application database to the module's health surface.
// It remains explicit in bootstrap so the application provider cannot report
// healthy without probing the connection its routes and services use.
func (p *AppServiceProvider) WithDatabase(db *data.DB) *AppServiceProvider {
	p.db = db
	return p
}

// The optional interfaces this provider implements, asserted at compile time so
// a typo in a method name fails the build instead of silently doing nothing.
var (
	_ kernel.Module      = (*AppServiceProvider)(nil)
	_ kernel.Schedulable = (*AppServiceProvider)(nil)
	_ kernel.Health      = (*AppServiceProvider)(nil)
)

// Name is the module identifier, and the group `aru routes` prints.
func (*AppServiceProvider) Name() string { return "app" }

// Routes registers what routes/web.go declares.
func (p *AppServiceProvider) Routes(r *http.Router) { routes.Web(r, p.deps) }

// Health proves the application's own database is reachable.
func (p *AppServiceProvider) Health(ctx context.Context) error {
	if p.db == nil {
		return errors.New("app: database is not wired")
	}
	return p.db.PingContext(ctx)
}

// Schedule declares the recurring work of this application.
//
// A module never starts a goroutine of its own: it declares tasks here, and the
// scheduler module runs them with a lock and a Grant.
func (*AppServiceProvider) Schedule() []kernel.Task {
	return nil
}
