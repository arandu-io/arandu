// Package bootstrap configures the application, and nothing else.
//
// It is the one place the application is composed: the drivers the framework
// builds from config/, the providers in app/Providers, the routes in routes/,
// the middleware, the exceptions, the commands and the schedule. Read it top to
// bottom and you know what this application is. Nothing is resolved from a
// container and nothing is discovered: every line is a typed call.
//
// `aru make:module` does NOT edit this file. It prints the line to paste.
package bootstrap

import (
	"github.com/arandu-io/framework/events"
	"github.com/arandu-io/framework/foundation"
	fwview "github.com/arandu-io/framework/view"
	hgeo "github.com/arandu-io/hesape/geo"
	"github.com/arandu-io/hesape/routing/controllers"

	providers "github.com/arandu-io/arandu/app/Providers"
	appconfig "github.com/arandu-io/arandu/config"
	"github.com/arandu-io/arandu/routes"

	// The compiled stylesheet, embedded. Without this import the browser gets
	// the framework's default and every class written in a view of this project
	// is silently absent from the page.
	_ "github.com/arandu-io/arandu/assets"

	// This project's own client script, embedded. It registers itself as an
	// asset, and the layout asks for that asset by name -- so without this
	// import the page refuses to render rather than quietly losing the
	// behaviours the script defines.
	_ "github.com/arandu-io/arandu/resources/js"

	// This application's own schema changes. Importing them is what registers
	// them: each one calls migrations.Register from init(), and a package
	// nothing imports is not in the binary at all -- so without this line `aru
	// migrate` finds nothing and says so only by creating no tables.
	_ "github.com/arandu-io/arandu/database/migrations"

	// Importing the views is what registers them: every generated view calls
	// view.Register from init(), the same shape a database/sql driver has. A
	// view whose package nothing imports answers ctx.View with "no view named
	// ..." -- and without the layouts every page fails instead, because a page
	// renders its layout.
	//
	// `aru view:build` writes the generated package under
	// storage/framework/views/, mirroring the source tree, and each directory
	// is its own package. A directory whose data types a controller names is
	// linked by that controller's import -- NoteController imports
	// views/notes, so it has no line here. Any other directory needs one, and
	// a view nobody links says so at the first request rather than never.

	// The engines this binary can speak, and it speaks no other. Each connector
	// is its own module and registers itself from init(), so the import is what
	// links the driver: one that is not imported is not in the build, in go.sum
	// or in the vulnerability surface -- which is the whole reason they are
	// separate.
	//
	// SQLite is the one a new project links, because it is the one that needs
	// nothing installed: a file, no server and no cgo. Every other engine is two
	// lines, a `go get` and a blank import beside this one:
	//
	//	Postgres, for DATABASE_URL=postgres://...
	//	    go get github.com/arandu-io/hesape/database/connectors/pgx
	//	    _ "github.com/arandu-io/hesape/database/connectors/pgx"
	//
	//	Redis, Valkey, Dragonfly or KeyDB, for CACHE_STORE=redis and SESSION_DRIVER=redis
	//	    go get github.com/arandu-io/hesape/redis
	//	    _ "github.com/arandu-io/hesape/redis"
	//
	//	the same server, for QUEUE_CONNECTION=redis
	//	    go get github.com/arandu-io/hesape/queue/connectors/redis
	//	    _ "github.com/arandu-io/hesape/queue/connectors/redis"
	//
	// MySQL is github.com/arandu-io/hesape/database/connectors/mysql, the same
	// way. A setting that names an engine this binary does not link stops the
	// boot and prints both lines, so the environment cannot ask for a driver the
	// build left out and get something else instead.
	//
	// They are in bootstrap rather than in main because bootstrap is what
	// composes the application, and the tests compose it too: with them in main
	// every feature test opened a connection to a driver nobody had registered.
	_ "github.com/arandu-io/hesape/database/connectors/sqlite"

	_ "github.com/arandu-io/arandu/storage/framework/views"
	_ "github.com/arandu-io/arandu/storage/framework/views/layouts"

	// The notes table, answered alone as well as inside its page, is the one
	// partial this project ships. Remove the line with the list under "The
	// example resource" in README.md when no other partial is left.
	_ "github.com/arandu-io/arandu/storage/framework/views/partials"
)

// App loads the configuration, builds the drivers and this application's
// services over them, and configures the application. version and commit are
// what the build stamped, printed by the version command.
//
// It fails when what the configuration named cannot be assembled: a setting
// naming a driver this binary does not link, a certificate that is not there,
// a command name declared twice.
func App(version, commit string) (*foundation.Application, error) {
	cfg, err := appconfig.Load()
	if err != nil {
		return nil, err
	}

	// Every driver the configuration names: the database, the cache stores,
	// the sessions, the queue, the mailer and the disks. A driver changes by
	// changing the environment, never here.
	drivers, err := foundation.NewDrivers(cfg.Settings)
	if err != nil {
		return nil, err
	}

	// This application's services, built once over the drivers.
	app, err := providers.NewAppServiceProvider(cfg, drivers)
	if err != nil {
		_ = drivers.Close()
		return nil, err
	}

	return Configure(cfg, drivers, app).WithVersion(version, commit).Create()
}

// Configure is the application: the providers in the order they boot, the
// routes, the middleware, the commands and the schedule, the job handlers and
// the seeders. App calls it with what it loaded; a test calls it with a
// configuration and drivers of its own, and adds what it needs before Create.
func Configure(cfg appconfig.Config, drivers *foundation.Drivers, app *providers.AppServiceProvider) *foundation.ApplicationBuilder {
	return foundation.Configure(drivers).
		WithProviders(
			// The view layer: the renderer ctx.View needs and the embedded
			// assets. Without it every page answers with an error naming this
			// missing line, and every stylesheet 404s.
			fwview.NewModule(),
			// Search and GEO surfaces. GEO_INDEXING_ENABLED decides whether
			// this deployment publishes the catalog; disabled, it fails closed.
			foundation.Component(hgeo.NewModule(cfg.Geo, app.Catalog())),
			// The outbox table, and the relay that empties it in the process
			// that serves. WithRelay rather than NewModule: a relay built beside
			// the module and not handed to it publishes nothing and reports
			// itself healthy.
			events.WithRelay(app.Relay),
			// This application's services.
			app,
			// A module this project installs is listed here, by hand.
		).
		WithRouting(func(r *controllers.Router) {
			routes.Web(r, app.Web())
		}).
		// The catalogue in lang/, carried by every request, so a page says
		// .Trans("notes.new") in the request's language. Without it a page
		// shows the key instead of the sentence.
		WithTranslator(app.Translator).
		WithMiddleware(func(m *foundation.Middleware) {
			// A provider posts with no session and no token, and every route
			// under /webhooks/ verifies the provider's signature first.
			m.ValidateCSRFTokensExcept("/webhooks/")
		}).
		WithCommands(routes.Commands(app.Console())...).
		WithSchedule(routes.Schedule(app.Console())...).
		WithTenants(app.Tenants).
		WithJobs(app.RegisterJobs).
		WithSeeders(app.Seed)
}
