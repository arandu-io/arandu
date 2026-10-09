// Package bootstrap composes the application.
//
// It is the single place where everything is wired. The wiring is explicit and
// visible -- no dependency appears by magic. If you want to know where the
// user service comes from, it is written here.
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
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/arandu-io/framework/events"
	"github.com/arandu-io/framework/foundation"
	fwbootstrap "github.com/arandu-io/framework/foundation/bootstrap"
	fwgeo "github.com/arandu-io/framework/geo"
	"github.com/arandu-io/framework/http/middleware"
	"github.com/arandu-io/framework/jobs"
	"github.com/arandu-io/framework/mail"
	"github.com/arandu-io/framework/scheduler"
	"github.com/arandu-io/framework/security"
	fwview "github.com/arandu-io/framework/view"
	"github.com/arandu-io/hesape/auth"
	cache2 "github.com/arandu-io/hesape/cache"
	"github.com/arandu-io/hesape/database"
	"github.com/arandu-io/hesape/exception"
	"github.com/arandu-io/hesape/geo"
	hhttp "github.com/arandu-io/hesape/http"
	"github.com/arandu-io/hesape/http/client"
	httpmiddleware "github.com/arandu-io/hesape/http/middleware"
	hnotifications "github.com/arandu-io/hesape/notifications"
	"github.com/arandu-io/hesape/notifications/channels"
	"github.com/arandu-io/hesape/onetime"
	"github.com/arandu-io/hesape/queue"
	hmiddleware "github.com/arandu-io/hesape/routing/middleware"
	"github.com/arandu-io/hesape/session"
	"github.com/arandu-io/hesape/view"

	clients "github.com/arandu-io/arandu/app/Clients"
	controllers "github.com/arandu-io/arandu/app/Http/Controllers"
	listeners "github.com/arandu-io/arandu/app/Listeners"
	providers "github.com/arandu-io/arandu/app/Providers"
	services "github.com/arandu-io/arandu/app/Services"
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
	Kernel *foundation.Application
	// DB is the application database every service was built over. It is
	// returned for the seeders, which create rows through factories over it.
	DB *database.DB
	// Users is the application-owned account service. Seeders receive this same
	// value instead of reaching through a framework module.
	Users *services.UserService
	// TwoFactor is the application-owned enrolment and challenge service.
	TwoFactor *services.TwoFactorService
	// Notes is the example resource's service. The worker hands it to the
	// nightly digest's handler. Remove it with the list under "The example
	// resource" in README.md.
	Notes *services.NoteService
	// EmailCodes stores purpose-bound verification and reset codes.
	EmailCodes onetime.CodeStore
	// Sessions is the only store allowed to create authenticated identity, after
	// every required factor has succeeded.
	Sessions *security.SessionStore
	// Scheduler runs what the modules declared. `aru schedule:list` reads it.
	Scheduler *scheduler.Module
	// Relay is what empties the outbox. It is returned as well as registered for
	// the reason Users is, sharpened by what it guards: a test that built a relay
	// of its own would pass over an application that wires none, and an
	// application that wires none writes rows nothing ever reads.
	Relay *events.Relay
	// Queue is the job store `aru queue:work` drains: the table in the
	// application's own database, or the RESP queue when QUEUE_CONNECTION
	// names it.
	Queue queue.Queue
	// Mail is what sends. It is returned as well as used, because a job that
	// sends is built outside this function and reaching back in for the mailer
	// later is the hidden coupling the explicit wiring exists to avoid.
	Mail *mail.Mailer
	// Notifier is what tells somebody something: a notification, sent over the
	// channels built for it below. It is returned for the reason Mail is.
	Notifier *hnotifications.Notifier
	// Cache is the store every replica sees, and nil when no setting resolved
	// one.
	//
	// It is returned as well as used because the isolated commands take their
	// lock in it, and the migration commands are built outside this function.
	// Nil is what those commands refuse on outside development: a lock inside
	// one process isolates nothing from the replica beside it.
	Cache cache2.SharedStore
}

// Build wires the application and returns it ready to boot.
//
// It does not boot, listen or migrate. main.go decides which of those the
// requested command needs, which is what keeps `aru routes` from opening a
// socket and `aru queue:work` from starting a scheduler.
//
// It fails when what the configuration named cannot be assembled -- a
// certificate file that is not there is the case that exists today. Refusing
// here is the point: the alternative is a process that starts having quietly
// dropped what it was told to use.
func Build(cfg appconfig.Config, db *database.DB) (App, error) {
	fw := cfg.Framework

	// The CSRF token is bound to the session, and a visitor without one is bound
	// to a random id in a signed cookie of its own. That cookie carries Secure
	// exactly when the session cookie and the flash cookie do, because all
	// three take fw.Session.Secure: SESSION_SECURE_COOKIE when it is written,
	// and otherwise Secure in every environment but dev. Over plain HTTP in
	// development the browser would never send a Secure one back, and every
	// form a guest submits would answer 419.
	csrf := session.NewCSRF(fw.App.Key, cfg.Session.CSRFTTL).Secure(fw.Session.Secure)

	// A setting that names the RESP store is a setting that needs its connector
	// in this binary, and asking here is what puts a missing import in the boot
	// with the two lines that add it -- rather than in the first request, as a
	// store that was never built.
	if err := requireSharedStoreConnector(cfg); err != nil {
		return App{}, err
	}

	// Every cache store this application has, by name. CACHE_STORE names the
	// one the lock below counts in; the session names one of its own, which is
	// why what is passed around is the set and not a single store.
	stores := newCacheStores(cfg.Cache)

	// The lock two things below take: the relay and the scheduler. One value
	// wires into both, which is why it is built here rather than beside either
	// of them -- two lockers over one store would agree about nothing.
	locker, err := cacheLocker(stores, cfg.Cache)
	if err != nil {
		return App{}, err
	}

	// The session, over the store SESSION_DRIVER named.
	backend, err := sessionBackend(cfg.Session, stores)
	if err != nil {
		return App{}, err
	}
	sessions := security.NewSessionStore(fw.App.Key, cfg.Session.TTL, fw.Session.Secure, backend)

	// The rate limit counts in a store rather than in this process, which is the
	// difference between one budget and one budget per replica -- on the
	// endpoints a limit is put there for.
	//
	// It counts in the store CACHE_STORE named, and the in-process one is the
	// honest answer for a single instance: what it must not be is a shared store
	// that everybody assumed was there. Which store it is says which, and the
	// health check says whether it answers.
	limitStore, err := stores.Default()
	if err != nil {
		return App{}, err
	}
	limiter := cache2.NewRateLimiter(limitStore.GetStore())
	emailCodes, err := onetime.New(limitStore.GetStore(), fw.App.Key, onetime.Config{
		TTL: cfg.Auth.PasswordResetTTL,
	})
	if err != nil {
		return App{}, fmt.Errorf("bootstrap: build email code store: %w", err)
	}

	// The queue QUEUE_CONNECTION named. See openQueue.
	queueStore, err := openQueue(cfg, db)
	if err != nil {
		return App{}, err
	}

	// The notifier, and the channels this application tells people through.
	//
	// One channel: the database, which stores the row a bell menu draws from,
	// in the notifications table database/migrations registers. There is no
	// mail channel here: a notification that names mail needs a
	// channels.Mailer, and none is built over the mailer below yet. A
	// notification sent through a channel that is not in this list is refused
	// with notifications.ErrNoChannel rather than dropped.
	notifier := hnotifications.New([]hnotifications.Channel{
		channels.NewDatabase(hnotifications.NewTableStore(db)),
	})

	// The relay that empties the outbox, and the listeners it hands events to.
	//
	// events.NewModule() brings the table and publishes nothing, which is a
	// coherent state -- storing is what cannot be recovered later, publishing can
	// start the day there is somewhere to publish to. It stops being coherent the
	// moment something stores: the application user service writes a row for every
	// registration, every confirmed address and every reset password, and without
	// this line they accumulate in a table no process reads.
	//
	// # It runs in `aru serve`, and in no other command
	//
	// That is not decided here. The module's loop is a foundation.Background one, and
	// Start is called by Kernel.Run, never by Kernel.Boot. The `aru queue:work`
	// command, `aru routes` and every migration command build this same
	// application and start no relay.
	//
	// It is also the right place rather than the convenient one. `aru queue:work`
	// scales with the depth of the job queue, so a relay there is one publisher
	// per worker replica and the count is whatever the queue happened to need. A
	// relay in a command of its own would be a second deployable to build,
	// monitor, page on and forget to restart, for one loop that already has a
	// process to live in. The scheduler is here for that same reason and is the
	// precedent.
	//
	// The Locker is the one built above: one pass reads every unpublished row and
	// marks what it delivered, so N replicas are N publishers of the same row
	// unless something stops them. It is nil for the in-process cache, and a
	// publisher has to tolerate the repeat regardless -- delivery is at-least-once
	// by design, so a mark that fails after a successful publish sends the event
	// again.
	//
	// The relay takes one Publisher, and listeners.Each is the list of them,
	// in order: every committed event goes to each.
	relay := events.NewRelay(events.NewOutbox(db), listeners.Each{
		listeners.NewEventLog(),
		// The example resource's listener: the author of a published note is
		// told in the bell menu. Remove it with the list under "The example
		// resource" in README.md.
		listeners.NewNotifyNoteAuthor(notifier),
	}, events.RelayOptions{Locker: locker})

	// A module that calls another service takes log.Client, not one of
	// its own:
	//
	//	billing.New(svc, log.Client(10*time.Second))
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

	userService := services.NewUserService(db)
	// The sign-in challenge counts the codes each account offers in the store
	// the rate limit counts in, for the reason the limit does: a count per
	// replica is a budget multiplied by the number of replicas.
	twoFactorService, err := services.NewTwoFactorService(db, fw.App.Key, limitStore.GetStore())
	if err != nil {
		return App{}, fmt.Errorf("bootstrap: build two-factor service: %w", err)
	}

	// GEO is native and opt-in at the deployment boundary. The application owns
	// only the catalog: adding a public route to this project means deciding
	// explicitly whether it belongs in this list. Indexing remains disabled by
	// default even though the fail-closed machine endpoints are registered.
	geoCatalog := geo.CatalogFunc(func(context.Context) ([]geo.Document, error) {
		return []geo.Document{{Path: "/", Title: cfg.App.Name}}, nil
	})
	geoModule := fwgeo.NewModule(cfg.Geo, geoCatalog)

	// The example resource's service, built once: the comments under a note
	// read the note through it, under the note's own policy. Remove it with the
	// list under "The example resource" in README.md.
	notes := services.NewNoteService(db).WithNewsletter(newsletter(cfg.Services.Newsletter))

	// The controllers, built here and handed to the routes. A controller that
	// constructed its own collaborators would be a controller no test can pin.
	deps := routes.Deps{
		Home: controllers.NewHomeController(cfg.App.Name, userService, cfg.Auth.Tenant),
		// What the route guards read. The same store the pipeline was given,
		// and it has to be: two stores over one key would agree about the
		// signature and disagree about which sessions exist.
		Sessions: sessions,
		// The example resource, and the one nested under it. Remove them with
		// the list under "The example resource" in README.md.
		Note:    controllers.NewNoteController(notes),
		Comment: controllers.NewCommentController(services.NewCommentService(db, notes)),
	}

	k := foundation.New(fw)

	// The one handler that answers a failed request, built by the bootstrapper
	// from the configuration the kernel was given: whether the debug page may
	// exist, which editor a stack frame links to, and which frames are this
	// application's -- the module path is passed in because a constant could only
	// be right for the project it was written in.
	//
	// Diagnose is what the registered modules know about the state of the system
	// right now -- the outbox falling behind, and whatever the next module
	// reports. It shows up next to the failure somebody is already looking at.
	//
	// Keeping the value is the point of the bootstrapper returning it. It is
	// where this application registers its own answers, and every one of them is
	// read on each request afterwards:
	//
	//	exceptions.DontReport(ErrExpiredLink)      never written to the log
	//	exceptions.Renderable(func(...) { ... })   drawn this application's way
	//	exceptions.ShouldRenderJSONWhen(...)       answered as a document
	exceptions := fwbootstrap.HandleExceptions(fw, AppModule, k.Diagnose)

	k.
		// The pipeline order is the order of execution. Recover comes FIRST, or
		// a panic in any middleware below it escapes without a page.
		Use(
			exception.Recover(exceptions),
			// The body limit, before anything below does work on the request:
			// a body nobody bounded is read whole by the first form parse, and
			// one POST with no credentials is enough to exhaust the process.
			//
			// Two layers, because a length is a header the client wrote.
			// ValidatePostSize answers 413 to a body that declares itself too
			// large, before the session is read or a CSRF token is checked.
			// LimitBodySize covers the body that declares nothing -- a chunked
			// one -- by refusing to read past the limit, whoever reads it.
			//
			// HTTP_MAX_BODY_BYTES is a ceiling for every route; config/http.go
			// says what to do for a route that takes uploads.
			httpmiddleware.ValidatePostSize(cfg.HTTP.MaxBodyBytes),
			httpmiddleware.LimitBodySize(cfg.HTTP.MaxBodyBytes),
			// The visitor's address, put back before anything keys on it: the
			// request log, the throttle below and the sign-in throttle all read
			// RemoteAddr. Behind a load balancer that is the balancer on every
			// request, so the whole internet shares one budget and the first
			// person locked out locks out everybody.
			//
			// X-Forwarded-For is believed only from a peer TRUSTED_PROXIES
			// lists. The list is empty unless written, and then this does
			// nothing: a header is a string the client chose, and believing it
			// from anyone hands every visitor somebody else's counter.
			httpmiddleware.TrustProxies(cfg.HTTP.TrustedProxies),
			// k.Recorder() is the buffer behind /_arandu/debug. It is nil
			// outside development, and passing nil records nothing -- which is
			// what production does.
			middleware.Observe(cfg.App.IsDev(), fw.Observability.TracingSecret, k.Recorder()),
			httpmiddleware.SecurityHeaders(cfg.App.IsDev()),
			// The budget and the window are one value, which is what a named
			// limiter resolves to. The refusal is passed rather than assumed:
			// how a 4xx is written belongs to the request layer, and this one
			// adds HX-Refresh, without which somebody over the limit presses the
			// button and the screen does not change.
			//
			// The key is the session only while the store still holds it, and
			// the address otherwise: a signed cookie proves only that its id was
			// issued here once, and every expired id a client kept would be a
			// fresh budget.
			hmiddleware.Throttle(limiter, cache2.PerMinute(300),
				middleware.KeyBySession(sessions), hhttp.Refuse),
			// CSRFProtect checks every write and issues the token every page
			// carries: view.New reads it off the request, so no controller
			// issues one by hand.
			middleware.CSRFProtect(csrf, sessions.IDFromRequest),
			httpmiddleware.OverrideMethod(),
		).
		Register(
			// The view layer. It brings the renderer ctx.View needs, through the
			// optional foundation.RendererProvider interface, and serves the
			// embedded assets. Without it every page answers with an error that
			// names this missing line, and every stylesheet 404s.
			fwview.NewModule(),
			// Search/GEO surfaces. The module exists in every generated project,
			// while GEO_INDEXING_ENABLED decides whether this deployment actually
			// publishes its catalog. Disabled indexing fails closed.
			geoModule,
			// The outbox table, and the relay built above that empties it. A
			// module that records domain events stores them in the same
			// transaction as the write, and this is what brings the table those
			// rows land in -- see doc 27.
			//
			// WithRelay rather than NewModule, and the difference is visible from
			// outside the process: the module is what runs the loop, what reports
			// the backlog on /_arandu/health and what puts a stuck outbox on the
			// error page. A relay built beside it and not handed to it publishes
			// nothing and reports itself healthy.
			events.WithRelay(relay),
			// The jobs table. Work that happens after the response is drained by
			// `aru queue:work`, which delegates to this image's internal `work`
			// subcommand and keeps the deploy at one artifact.
			//
			// The module is the framework's and the driver is the one built
			// above: a module registers its routes on the framework's router,
			// and the driver goes through untouched, so the schema this brings
			// is the one the driver carries. A queue that keeps its jobs
			// elsewhere brings no table, and says so by carrying none.
			jobs.NewModule(queueStore),
			// This application: its routes, from routes/web.go. Its migrations
			// arrive by the blank import above, not through here.
			providers.NewAppServiceProvider(deps).WithDatabase(db).WithQueue(queueStore),
			// A module this project installs is registered here, by hand, once
			// it is constructed above. `aru make:module` edits nothing in this
			// file: what it generates is a controller, and it prints the line
			// for the routes.Deps literal above for you to paste.
		)

	// The shared store reports itself on the health check and gives its
	// connection back at shutdown. Without the module, "the store is down"
	// arrives as a class of request failures somebody has to correlate by hand.
	//
	// It is registered whenever a setting resolved the store, which is what
	// makes the probe follow the deployment rather than one setting: a process
	// whose cache is in-process and whose sessions live over RESP depends on
	// that server for every request, and a probe that stayed green because
	// CACHE_STORE said memory would be reporting half of it.
	if shared := stores.SharedStore(); shared != nil {
		k.Register(foundation.NewCacheModule("cache", shared))
	}

	// The scheduler goes last, because it collects the tasks the modules above
	// declared. A module never starts its own goroutine; it declares work, and
	// this is what runs it.
	//
	// The Locker is what keeps a Singleton task on one replica, and it is the
	// one built above, beside the cache it comes from.
	//
	// Tenants is the list a PerTenant task expands to, each with its own
	// Grant, and only the application knows where it lives. This one serves the
	// one tenant every login belongs to, cfg.Auth.Tenant; an application with
	// organizations answers them from its own table here.
	// Recorder for the same reason as the worker: a scheduled task is
	// investigated on the same page as a request, and costs nothing when
	// nothing is recording.
	sched := scheduler.NewModule(k.Tasks(), scheduler.Options{
		Recorder: k.Recorder(),
		Locker:   locker,
		Tenants: func(context.Context) ([]string, error) {
			return []string{cfg.Auth.Tenant}, nil
		},
	})
	k.Register(sched)

	return App{
		// Notes is the example resource's. Remove it with the list under "The
		// example resource" in README.md.
		Notes:  notes,
		Kernel: k, DB: db, Users: userService, TwoFactor: twoFactorService,
		EmailCodes: emailCodes, Sessions: sessions, Scheduler: sched,
		Relay: relay, Queue: queueStore, Mail: mailer, Notifier: notifier, Cache: stores.SharedStore(),
	}, nil
}

// requireSharedStoreConnector refuses the boot when CACHE_STORE or
// SESSION_DRIVER names the RESP store and no connector for it is linked into
// this binary.
//
// The error is the connector registry's, unchanged: it names the setting that
// asked, the `go get` and the blank import, and a sentence of this file's own
// around it would be a second wording of the one fix there is.
//
// The two settings are asked separately because either can name the store
// alone -- sessions shared across replicas over a cache kept in each process
// is a deployment, not a mistake -- and the error has to name the line that was
// actually written.
func requireSharedStoreConnector(cfg appconfig.Config) error {
	if cfg.Cache.Store == appconfig.CacheRedis {
		if err := cache2.Linked("CACHE_STORE", respStore); err != nil {
			return err
		}
	}
	if cfg.Session.Driver == appconfig.SessionRedis {
		if err := cache2.Linked("SESSION_DRIVER", respStore); err != nil {
			return err
		}
	}
	return nil
}

// openQueue builds the queue QUEUE_CONNECTION named.
//
// The database one keeps its jobs in a table of the application's own
// database, which is what makes a job commitable by the same transaction as the
// row it is about, and it needs nothing installed. The RESP one is for volume
// beyond a table: the same queue.Queue contract, so the worker, the handlers and
// every queue command are the ones the database queue has.
//
// The RESP queue is a connector of its own, linked by a blank import, and the
// setting naming it without the import stops the boot with the two lines that
// add it. It is opened over the endpoint the shared cache store is, because it
// is the same server described by the same settings -- and opening dials
// nothing, so a binary that only migrates never reaches it.
//
// The setting was accepted and then ignored before this function existed:
// every binary queued over the table whatever QUEUE_CONNECTION said, and a
// deployment that asked for RESP got a table nobody had planned for.
func openQueue(cfg appconfig.Config, db *database.DB) (queue.Queue, error) {
	switch cfg.Queue.Connection {
	case appconfig.QueueDatabase:
		return queue.NewDatabaseQueue(db), nil

	case appconfig.QueueRedis:
		driver := string(appconfig.QueueRedis)
		if err := queue.Linked("QUEUE_CONNECTION", driver); err != nil {
			return nil, err
		}
		endpoint, err := respEndpoint(cfg.Cache)
		if err != nil {
			return nil, err
		}
		return queue.Open(driver, endpoint)

	default:
		// Unreachable through Load, which refuses the value first. It is here
		// for the reason sessionBackend has its default: a configuration built
		// in Go skips that check, and a connection nobody recognises must not
		// fall through to a queue nobody asked for.
		return nil, fmt.Errorf("QUEUE_CONNECTION has unsupported value %q; expected %s or %s",
			cfg.Queue.Connection, appconfig.QueueDatabase, appconfig.QueueRedis)
	}
}

// The names this application's cache stores are known by, and the driver the
// shared one is built by.
//
// The names are the values CACHE_STORE takes, so the word in the environment
// and the word in the wiring are one word -- and the RESP one is also the name
// its connector registers under, which is what cache.Linked and cache.Open are
// asked for. The driver is a name of the manager's own: it builds array, file,
// database, null and failover and no RESP store, because that store ships in a
// module of its own so its client stays out of the binaries that do not import
// it.
const (
	memoryStore = string(appconfig.CacheMemory)
	respStore   = string(appconfig.CacheRedis)
	respDriver  = "resp"
)

// cacheStores is every cache store this application has, by name.
//
// It is what stands where a single nullable connection stood, and the
// difference is what a caller has to do in order to be wrong. The connection
// was nil for the in-process store and each consumer branched on it by hand; a
// branch somebody forgets is a lock that locks nothing, because what a shared
// store buys is state every replica sees and a store inside the process has
// none to share.
//
// A name is not a branch, and the danger does not come back through it. The
// manager answers with a store or with an error naming the store, never with
// nil, and a caller that needs state every replica can read asks Shared, which
// refuses by name rather than handing back something that does not share. The
// store named memory is always defined; the RESP one is defined only when
// REDIS_URL names an endpoint, so naming it where there is none is an error at
// the boot rather than a process that starts and shares nothing.
type cacheStores struct {
	manager  *cache2.CacheManager
	settings appconfig.Cache

	// shared is the RESP store, opened the first time a store resolves it and
	// nil until then. It is kept because four consumers are written against it
	// rather than against the manager: the health module reports it, the
	// isolated commands take their lock in it, the queue writes the flag `aru
	// queue:pause` sets through it, and the session handler is the one it
	// keeps.
	//
	// Opened on resolution rather than at wiring, so a deployment whose stores
	// are all in-process opens nothing even when REDIS_URL is still set from a
	// configuration it has moved off.
	//
	// It is written while the application is being composed, by one goroutine,
	// and read afterwards. Nothing resolves a store once Build has returned:
	// the manager is not reachable from App.
	shared cache2.SharedStore
}

// newCacheStores defines the stores the configuration describes.
//
// It opens nothing, and neither does opening: see connect.
func newCacheStores(cfg appconfig.Cache) *cacheStores {
	out := &cacheStores{settings: cfg}

	defined := map[string]cache2.StoreConfig{
		// In-process, which is what CACHE_STORE=memory says: a single replica,
		// caching inside itself.
		memoryStore: {Driver: "array"},
	}
	if cfg.Address != "" {
		defined[respStore] = cache2.StoreConfig{Driver: respDriver}
	}

	out.manager = cache2.NewCacheManager(cache2.Config{
		Default: string(cfg.Store),
		Prefix:  cfg.Prefix,
		Stores:  defined,
	})

	// Registering the driver is what lets the manager build the RESP store. The
	// creator closes over the store the connector opened rather than reading
	// one out of the store's configuration, because a StoreConfig carries a
	// database connection and has nowhere to put this one.
	out.manager.Extend(respDriver, func(m *cache2.CacheManager, store cache2.StoreConfig) (*cache2.Repository, error) {
		shared, err := out.connect()
		if err != nil {
			return nil, err
		}
		return m.Repository(shared, store), nil
	})

	return out
}

// Store returns the named store, building it the first time it is asked for.
//
// A name the configuration does not define is an error naming it. That is the
// property the whole type rests on: nothing stands in for the store that was
// asked for.
func (c *cacheStores) Store(name string) (*cache2.Repository, error) {
	return c.manager.Store(name)
}

// Default returns the store CACHE_STORE named.
func (c *cacheStores) Default() (*cache2.Repository, error) {
	return c.Store(string(c.settings.Store))
}

// IsShared reports whether every replica of this deployment sees the named
// store.
func (c *cacheStores) IsShared(name string) bool {
	return name == respStore && c.settings.Address != ""
}

// Shared returns the store every replica sees behind the named one, and refuses
// when that store is one this process keeps to itself.
//
// The refusal is the point of the method. A caller reaching for it wants state
// every replica can read -- a session, an isolation lock -- and the in-process
// store satisfies every type it would be handed to while satisfying none of
// what was asked for. Refusing here puts that in the boot, where it names both
// settings, instead of in production, where it looks like people being signed
// out at random.
//
// It goes through the manager rather than around it, so the store it hands back
// is the one the named repository is built over and not a second one beside it.
func (c *cacheStores) Shared(name string) (cache2.SharedStore, error) {
	if !c.IsShared(name) {
		return nil, fmt.Errorf("the cache store %q is kept inside this process, and what is asked of it here is state every replica can read: "+
			"REDIS_URL is what names a store they all see", name)
	}
	if _, err := c.Store(name); err != nil {
		return nil, err
	}
	return c.shared, nil
}

// SharedStore returns the RESP store when a setting resolved it, and nil when
// none did.
//
// The nil is an untyped one: the field is the interface itself, so a caller's
// comparison with nil answers what it asks.
func (c *cacheStores) SharedStore() cache2.SharedStore { return c.shared }

// connect opens the RESP store, once.
//
// It does not talk to the server: the connector dials nothing, and a store that
// dialled here would make the application refuse to start because the cache is
// down, which is the opposite of what a cache is for. The health check is what
// reports it.
//
// What it does do at the boot is read the files the configuration named, before
// the connector is asked for anything, and a file that is named and cannot be
// read stops it -- see cacheTLS.
func (c *cacheStores) connect() (cache2.SharedStore, error) {
	if c.shared != nil {
		return c.shared, nil
	}
	if c.settings.Address == "" {
		return nil, fmt.Errorf("the RESP store was asked for and REDIS_URL names no endpoint")
	}

	endpoint, err := respEndpoint(c.settings)
	if err != nil {
		return nil, err
	}
	shared, err := cache2.Open(respStore, endpoint)
	if err != nil {
		return nil, err
	}
	c.shared = shared
	return shared, nil
}

// respEndpoint is where the RESP server is and how to reach it, as the
// connectors take it.
//
// A function of its own rather than a literal in connect, because the endpoint
// is the server and not the cache: whatever else talks to that server is
// described by the same settings, and two translations of REDIS_URL are two
// answers the day one of them learns a field the other has not.
func respEndpoint(cfg appconfig.Cache) (cache2.Endpoint, error) {
	encryption, err := cacheTLS(cfg)
	if err != nil {
		return cache2.Endpoint{}, err
	}
	return cache2.Endpoint{
		Address:  cfg.Address,
		Password: cfg.Password,
		Database: cfg.Database,
		Prefix:   cfg.Prefix,
		TLS:      encryption,
	}, nil
}

// sessionBackend builds the session backend SESSION_DRIVER named.
//
// The driver names a store, and not the cache's default one. SESSION_DRIVER=redis
// with the in-process cache is a deployment that keeps its sessions where every
// replica can read them and caches inside each process, and refusing it would
// be refusing a combination that is right for anybody whose cache is cheap to
// lose and whose sign-ins are not.
//
// A driver that names a store no other replica can see is refused here, at the
// boot, naming what was asked for. The alternative is the failure this wiring
// exists to end: a process that starts, reports itself healthy, and signs half
// its visitors out on every request because the replica beside it never saw the
// login.
//
// The handler is the one the shared store keeps, and not one built over the
// store's repository, which is what keeps the session from changing the prefix
// the cache is using: there is nothing shared between them to change. The keys
// it writes are its own -- session and session-index -- so the two occupy one
// server without meeting. The store hands the payload over still encoded,
// because it was linked without knowing this application's subject type, and
// session.Decode is where that type is named.
func sessionBackend(cfg appconfig.Session, stores *cacheStores) (security.SessionBackend, error) {
	switch cfg.Driver {
	case appconfig.SessionMemory:
		// Right for one instance and wrong for two, and it is what
		// SESSION_DRIVER=memory asks for.
		return security.NewMemoryBackend(), nil

	case appconfig.SessionRedis:
		shared, err := stores.Shared(respStore)
		if err != nil {
			return nil, fmt.Errorf("SESSION_DRIVER %q names the cache store %q: %w", cfg.Driver, respStore, err)
		}
		// Refused rather than skipped: a shared store that keeps no sessions
		// leaves nowhere to put them but this process, which is the failure
		// the setting was written to avoid.
		keeper, ok := shared.(session.Keeper)
		if !ok {
			return nil, fmt.Errorf("SESSION_DRIVER %q names it, and the cache store %q is shared and cannot keep sessions", cfg.Driver, respStore)
		}
		return security.NewSessionBackend(session.Decode[auth.Subject](keeper.Sessions())), nil

	default:
		// Unreachable through Load, which refuses the value first. It is here
		// because a configuration built in Go skips that check, and a driver
		// nobody recognises must not fall through to the in-process store: the
		// sessions would be kept where nothing asked for them.
		return nil, fmt.Errorf("SESSION_DRIVER has unsupported value %q; expected %s or %s",
			cfg.Driver, appconfig.SessionMemory, appconfig.SessionRedis)
	}
}

// cacheLocker builds the lock the relay and the scheduler take, over the store
// CACHE_STORE named, and answers nil when that store is one this process keeps
// to itself.
//
// Nil is a claim rather than an omission: what it costs the relay behind two
// replicas is a duplicate delivery and never a lost event, and what it costs
// the scheduler is every replica running every task. A single replica is a
// supported deployment and this is what it looks like.
//
// The interface is left nil rather than filled with a nil pointer: an interface
// holding a typed nil is not nil, and both would call through it.
//
// A shared store that cannot hold a lock does not exist to be refused: holding
// one is part of what cache.SharedStore is, so a connector that could not would
// not compile. That is the failure this shape exists to end -- a scheduler that
// declares a Singleton task and runs it on every replica.
func cacheLocker(stores *cacheStores, cfg appconfig.Cache) (foundation.Locker, error) {
	name := string(cfg.Store)
	if !stores.IsShared(name) {
		return nil, nil
	}

	shared, err := stores.Shared(name)
	if err != nil {
		return nil, err
	}
	return foundation.NewLocker(cache2.NewLocks(shared)), nil
}

// cacheTLS turns the file paths the configuration carries into the settings the
// connection takes, and answers nil when the URL asked for no encryption.
//
// The translation happens here, once, and that asymmetry is deliberate: the
// client speaks crypto/tls because it can, and configuration speaks paths
// because an environment variable cannot carry a parsed certificate. A second
// vocabulary on either side would say less than the one it replaced -- a
// private authority and a client certificate are exactly what tls.Config
// already names.
//
// A file that is named and cannot be read stops the boot, and the message names
// which one. The alternative is a process that starts with encryption off after
// being told to turn it on, and the difference between that and a plain
// connection is invisible from the outside -- which is the whole failure.
func cacheTLS(cfg appconfig.Cache) (*tls.Config, error) {
	named := cfg.TLSCAFile != "" || cfg.TLSCertFile != "" || cfg.TLSKeyFile != "" || cfg.TLSServerName != ""

	if !cfg.TLS {
		if named {
			// Refused rather than ignored, for the reason a retired MAIL_ variable
			// is: certificates configured for a connection that carries none is
			// somebody who believes the traffic is encrypted and is wrong.
			return nil, fmt.Errorf("REDIS_URL asks for no encryption and the REDIS_*_FILE variables name certificates for it: " +
				"write the endpoint as rediss:// to turn it on, or remove them")
		}
		return nil, nil
	}

	// TLS 1.2 is the floor. The default floor of a client is lower, and a
	// connection that carries the password and every session id is not where to
	// accept a version that is only there for what cannot be upgraded.
	out := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: cfg.TLSServerName}

	if cfg.TLSCAFile != "" {
		authority, err := os.ReadFile(cfg.TLSCAFile)
		if err != nil {
			return nil, fmt.Errorf("REDIS_CA_FILE names %s, and the connection cannot be encrypted without it: %w", cfg.TLSCAFile, err)
		}
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(authority) {
			return nil, fmt.Errorf("REDIS_CA_FILE names %s, and it holds no certificate this can read: it has to be PEM", cfg.TLSCAFile)
		}
		// The private root replaces the system pool rather than joining it: a
		// server whose certificate a public authority signed does not need this
		// variable at all, and keeping both would let a certificate from either
		// side pass a check the operator meant to narrow.
		out.RootCAs = roots
	}

	if (cfg.TLSCertFile == "") != (cfg.TLSKeyFile == "") {
		return nil, fmt.Errorf("REDIS_CERT_FILE and REDIS_KEY_FILE are a pair and only one is set: " +
			"a certificate without its key proves nothing, and a key without its certificate is sent to nobody")
	}
	if cfg.TLSCertFile != "" {
		pair, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
		if err != nil {
			return nil, fmt.Errorf("REDIS_CERT_FILE names %s and REDIS_KEY_FILE names %s, and they are not a usable pair: %w",
				cfg.TLSCertFile, cfg.TLSKeyFile, err)
		}
		out.Certificates = []tls.Certificate{pair}
	}

	return out, nil
}

// mailTransport picks the transport the configuration asked for.
//
// A switch here rather than a registry: there are five, they are all in this
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
		// Set MAIL_URL to resend://<key> or sendgrid://<key> and it sends.
		return mail.Resend{Key: cfg.Key}
	case appconfig.MailerSendGrid:
		return mail.SendGrid{Key: cfg.Key}
	default:
		return mail.Log{}
	}
}

// newsletter builds the example resource's newsletter client from its
// credential, or answers nil when none is configured: NoteService then sends
// no digest, and a project runs with no credential at all. Remove it with the
// list under "The example resource" in README.md.
func newsletter(c appconfig.Credential) clients.Newsletter {
	if c.URL == "" {
		return nil
	}
	return clients.NewNewsletterClient(clients.NewsletterConfig{
		BaseURL: c.URL,
		Token:   c.Secret,
		Timeout: 10 * time.Second,
	}, client.NewFactory(nil))
}
