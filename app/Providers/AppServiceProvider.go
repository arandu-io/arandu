// Package providers holds this application's own modules.
//
// A provider here is a foundation.Module: it has a name, it registers routes, and it
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
	"time"

	"github.com/arandu-io/framework/foundation"
	"github.com/arandu-io/framework/http"
	"github.com/arandu-io/hesape/auth"
	"github.com/arandu-io/hesape/database"
	hfoundation "github.com/arandu-io/hesape/foundation"
	"github.com/arandu-io/hesape/queue"

	appjobs "github.com/arandu-io/arandu/app/Jobs"
	policies "github.com/arandu-io/arandu/app/Policies"
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
	deps  routes.Deps
	db    *database.DB
	queue queue.Queue
}

// NewAppServiceProvider returns the provider. bootstrap builds the controllers
// and hands them over; nothing is resolved later.
func NewAppServiceProvider(deps routes.Deps) *AppServiceProvider {
	return &AppServiceProvider{deps: deps}
}

// WithDatabase adds the application database to the module's health surface.
// It remains explicit in bootstrap so the application provider cannot report
// healthy without probing the connection its routes and services use.
func (p *AppServiceProvider) WithDatabase(db *database.DB) *AppServiceProvider {
	p.db = db
	return p
}

// WithQueue adds the job queue the scheduled tasks enqueue their work on. A
// task decides what work exists and enqueues it; the work itself runs in
// `aru queue:work`, where it gets the retries a scheduled task does not.
func (p *AppServiceProvider) WithQueue(q queue.Queue) *AppServiceProvider {
	p.queue = q
	return p
}

// The optional interfaces this provider implements, asserted at compile time so
// a typo in a method name fails the build instead of silently doing nothing.
var (
	_ foundation.Module       = (*AppServiceProvider)(nil)
	_ hfoundation.Schedulable = (*AppServiceProvider)(nil)
	_ hfoundation.Health      = (*AppServiceProvider)(nil)
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
// scheduler module runs them with a lock and a Grant. `aru schedule:list` shows
// them, and `aru schedule:run <id> --tenant=<id>` runs one now, on the same
// path. A provider built without a queue has nowhere to enqueue the work, and
// schedules nothing.
func (p *AppServiceProvider) Schedule() []hfoundation.Task {
	if p.queue == nil {
		return nil
	}
	return []hfoundation.Task{
		// The example resource's nightly digest: at 02:00, once per tenant and
		// on one replica, enqueue the job that sends the notes published in the
		// last day. The task runs under a Grant for note.list in that tenant,
		// and the job carries it to the worker. Remove it with the list under
		// "The example resource" in README.md.
		{
			ID:        appjobs.SendNotesDigestName,
			Spec:      "0 2 * * *",
			Scope:     hfoundation.PerTenant,
			Singleton: true,
			Timeout:   time.Minute,
			Action:    policies.NoteList,
			Run: func(ctx context.Context, g auth.Grant) error {
				return appjobs.DispatchSendNotesDigest(ctx, p.queue, g, appjobs.SendNotesDigest{
					Since: time.Now().Add(-24 * time.Hour),
				})
			},
		},
	}
}
