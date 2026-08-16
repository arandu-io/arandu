// Package bootstrap composes the application.
//
// It is the single place where everything is wired. The wiring is explicit and
// visible -- no dependency appears by magic. If you want to know where the
// user repository comes from, it is written here.
//
// `aru make:module` does NOT edit this file. It writes the code and prints the
// three lines to paste, because a generator that edited it behind your back
// would be a generator whose output nobody can account for -- and this file
// saying what the application is, exactly, is the point.
//
// Everything below is ordinary Go: read it top to bottom and you know the
// whole application.
package bootstrap

import (
	"strconv"
	"time"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/events"
	"github.com/arandu-io/framework/http/middleware"
	"github.com/arandu-io/framework/kernel"
	"github.com/arandu-io/framework/mail"
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

	// The compiled stylesheet, embedded. Without this import the browser gets
	// the framework's default and every class written in a view of this project
	// is silently absent from the page.
	_ "github.com/arandu-io/arandu/assets"

	// Importing the views is what registers them: every generated view calls
	// view.Register from init(), the same shape a database/sql driver has. Drop
	// one and ctx.View("home") answers "no view named home" -- and drop the
	// layouts one and every page fails instead, because a page renders its
	// layout.
	//
	// One line per directory of views. `aru view:build` writes the generated
	// package under storage/framework/views/, mirroring the source tree, and
	// each directory is its own package. Adding a directory means adding a line
	// here, and a view nobody can reach says so at the first request rather
	// than never.

	// The engines this binary can speak. Each is its own module, so removing an
	// import removes the driver from the build, from go.sum and from the
	// vulnerability surface -- which is the whole reason they are separate.
	//
	// SQLite is the development default and needs no cgo. Adding MySQL is
	// `go get github.com/arandu-io/database/mysql` plus a line here.
	//
	// They are in bootstrap rather than in main because bootstrap is what
	// composes the application, and the tests compose it too: with them in main
	// every feature test opened a connection to a driver nobody had registered.
	_ "github.com/arandu-io/database/pgx"
	_ "github.com/arandu-io/database/sqlite"

	_ "github.com/arandu-io/arandu/storage/framework/views"
	_ "github.com/arandu-io/arandu/storage/framework/views/layouts"
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
	// Mail is what sends. It is returned as well as used, because a job that
	// sends is built outside this function and reaching back in for the mailer
	// later is the hidden coupling the explicit wiring exists to avoid.
	Mail *mail.Mailer
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

	// The mailer, and which transport is configuration rather than a decision
	// the calling code makes. Development is the log transport, so `aru dev`
	// works with nothing installed -- an application that needs a mail server to
	// start is one nobody runs.
	mailer := mail.New(mailTransport(cfg.Mail), view.NewRenderer(), mail.Address{
		Email: cfg.Mail.FromAddress,
		Name:  cfg.Mail.FromName,
	})

	authService := auth.NewService(auth.NewUserRepo(db), sessions, csrf)

	// The controllers, built here and handed to the routes. A controller that
	// constructed its own collaborators would be a controller no test can pin.
	deps := routes.Deps{
		Home: controllers.NewHomeController(cfg.App.Name, sessions, csrf, authService, cfg.Auth.Tenant),
		// What the route guards read. The same store the pipeline and the
		// controllers were given, and it has to be: two stores over one key
		// would agree about the signature and disagree about which sessions
		// exist.
		Sessions: sessions,
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

	return App{Kernel: k, Auth: authService, Scheduler: sched, Queue: queueStore, Mail: mailer}
}

// mailTransport picks the transport the configuration asked for.
//
// A switch here rather than a registry: there are four, they are all in this
// file, and a name that matches nothing is refused at boot rather than at the
// first message. An application that starts and cannot send is one that finds
// out from a customer.
func mailTransport(cfg appconfig.Mail) mail.Transport {
	switch cfg.Mailer {
	case appconfig.MailerSMTP:
		return mail.SMTP{
			Host:     cfg.Host,
			Port:     strconv.Itoa(cfg.Port),
			Username: cfg.Username,
			Password: cfg.Password,
		}
	case appconfig.MailerArray:
		return &mail.Array{}
	case appconfig.MailerResend:
		// Both transports are in the core: each one is an HTTPS call to a
		// documented endpoint, so there is no client library to make optional.
		// Set MAIL_MAILER and MAIL_KEY and it sends.
		return mail.Resend{Key: cfg.Key}
	case appconfig.MailerSendGrid:
		return mail.SendGrid{Key: cfg.Key}
	default:
		return mail.Log{}
	}
}
