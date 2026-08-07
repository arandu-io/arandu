// Package bootstrap composes the application.
//
// It is the equivalent of Laravel's bootstrap/app.php: the single place where
// everything is wired. Notice that the wiring is explicit and visible -- no
// dependency appears by magic. If you want to know where the user repository
// comes from, it is written here.
//
// `aru make:module` does NOT edit this file. It writes the code and prints the
// three lines to paste, because a generator that edited it behind your back
// would be a generator whose output nobody can account for -- and this file
// saying what the application is, exactly, is the point of ADR 0001.
//
// Everything below is ordinary Go: read it top to bottom and you know the whole
// application.
package bootstrap

import (
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/events"
	"github.com/arandu-io/framework/httpx/middleware"
	"github.com/arandu-io/framework/kernel"
	"github.com/arandu-io/framework/modules/auth"
	"github.com/arandu-io/framework/observability/errorpage"
	"github.com/arandu-io/framework/scheduler"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/framework/view"
	"github.com/arandu-io/queue"

	controllers "github.com/arandu-io/arandu/app/Http/Controllers"
	providers "github.com/arandu-io/arandu/app/Providers"
	appconfig "github.com/arandu-io/arandu/config"
	"github.com/arandu-io/arandu/routes"

	// Importing the views is what registers them: every generated view calls
	// view.Register from init(), the same shape a database/sql driver has. Drop
	// this import and ctx.View("home") answers "no view named home".
	// The compiled stylesheet, embedded. Without this import the browser gets
	// the framework's default and every class written in a view of this project
	// is silently absent from the page.
	_ "github.com/arandu-io/arandu/assets"

	_ "github.com/arandu-io/arandu/resources/views"
)

// AppModule is this project's module path. The error page uses it to tell your
// frames from the framework's, and shows yours expanded.
const AppModule = "github.com/arandu-io/arandu"

// App is everything the wiring produced.
//
// A struct rather than four return values, because the fifth one is always the
// one that breaks every call site.
type App struct {
	// Kernel is the composed application: configuration, modules, the global
	// middleware pipeline and the router.
	Kernel *kernel.Kernel
	// Auth is returned as well as registered, because the seeders need it and
	// reaching into the module to fetch it later would be exactly the kind of
	// hidden coupling the explicit wiring exists to avoid.
	Auth *auth.Service
	// Scheduler runs what the modules declared. `aru schedule:list` reads it.
	Scheduler *scheduler.Module
	// Queue is the job store `aru work` drains.
	Queue *queue.Store
}

// Build wires the application and returns it ready to boot.
//
// It does not boot, listen or migrate. main.go decides which of those the
// requested command needs, which is what keeps `aru routes` from opening a
// socket and `aru work` from starting a scheduler.
func Build(cfg appconfig.Config, db *data.DB) App {
	fw := cfg.Framework

	csrf := security.NewCSRF(fw.AppKey, cfg.Session.CSRFTTL)

	// The core ships the in-memory session backend, which is right for one
	// instance and wrong for two: behind a load balancer, half the requests land
	// on the replica that never saw the login. Behind more than one pod, swap
	// this for kv.NewSessionBackend(client) -- github.com/arandu-io/kv, same
	// interface, one line. SESSION_DRIVER is what says which one is expected;
	// the same applies to the limiter below.
	sessions := security.NewSessionStore(fw.AppKey, cfg.Session.TTL, cfg.Session.Secure, security.NewMemoryBackend())

	limiter := middleware.NewMemoryLimiter()

	// The queue over the application's own database, which is what makes a job
	// commitable by the same transaction as the row it is about. For volume
	// beyond a table, github.com/arandu-io/queue/kv is the same contract over
	// RESP -- same Worker, same handlers, one line here.
	queueStore := queue.New(db)

	// A module that calls another service takes observability.Client, not one of
	// its own:
	//
	//	billing.New(svc, observability.Client(10*time.Second))
	//
	// Going through it is what puts the call on the request timeline and on the
	// console. A handler that builds its own http.Client is a handler whose
	// 800ms wait shows up as "other", and the timeout is not optional --
	// http.Client has none by default, and a call with no deadline is how one
	// slow dependency turns into every request of the process hanging.

	authService := auth.NewService(auth.NewUserRepo(db), sessions, csrf)

	// The controllers, built here and handed to the routes. A controller that
	// constructed its own collaborators would be a controller no test can pin.
	deps := routes.Deps{
		Home: controllers.NewHomeController(cfg.App.Name, sessions, csrf),
	}

	k := kernel.New(fw)

	k.
		// The pipeline order is the order of execution. Recover comes FIRST, or
		// a panic in any middleware below it escapes without a page.
		Use(
			middleware.Recover(cfg.App.IsDev(), errorpage.Options{
				Editor:    fw.Editor,
				AppModule: AppModule,
				// What the registered modules know about the state of the
				// system right now -- the outbox falling behind, and whatever
				// the next module reports. It shows up next to the failure
				// somebody is already looking at.
				Diagnose: k.Diagnose,
			}),
			// k.Recorder() is the buffer behind /_arandu/debug. It is nil
			// outside development, and passing nil records nothing -- which is
			// what production does.
			middleware.Observe(cfg.App.IsDev(), fw.TracingSecret, k.Recorder()),
			middleware.SecurityHeaders(cfg.App.IsDev()),
			middleware.RateLimit(limiter, 300, time.Minute, middleware.KeyBySession(sessions.IDFromRequest)),
			middleware.CSRFProtect(csrf, sessions.IDFromRequest),
		).
		Register(
			// The view layer. It brings the renderer ctx.View needs, through the
			// optional kernel.RendererProvider interface, and serves the
			// embedded assets. Without it every page answers with an error that
			// names this missing line, and every stylesheet 404s.
			view.NewModule(),
			// Single tenant: every login belongs to one constant. A multi-tenant
			// application swaps this for a resolver that reads the host name --
			// same code path, same queries, one line different.
			auth.New(authService, auth.FixedTenant(cfg.Auth.Tenant)),
			// The outbox table. A module that records domain events stores them
			// in the same transaction as the write, and this is what brings the
			// table those rows land in -- see doc 27.
			events.NewModule(),
			// The jobs table. Work that happens after the response, drained by
			// `aru work` -- the same image with another argument, which is what
			// keeps the deploy at one artifact.
			queue.NewModule(queueStore),
			// This application: its routes, from routes/web.go, and its own
			// migrations, from database/migrations.
			providers.NewAppServiceProvider(deps),
			// `aru make:module` adds the next modules here.
		)

	// The scheduler goes last, because it collects the tasks the modules above
	// declared. A module never starts its own goroutine; it declares work, and
	// this is what runs it.
	//
	// Locker is nil here: one replica. Behind more than one, pass
	// kv.NewLocker(client) or every replica runs every task.
	//
	// Tenants is nil too: a PerTenant task needs to know which tenants exist,
	// and only the application knows where that list lives. Wire it and the
	// scheduler expands the task to each of them, with its own Grant.
	// Recorder for the same reason as the worker: a scheduled task is
	// investigated on the same page as a request, and costs nothing when
	// nothing is recording.
	sched := scheduler.NewModule(k.Tasks(), scheduler.Options{Recorder: k.Recorder()})
	k.Register(sched)

	return App{Kernel: k, Auth: authService, Scheduler: sched, Queue: queueStore}
}
