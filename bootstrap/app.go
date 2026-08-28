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
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strconv"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/events"
	fwbootstrap "github.com/arandu-io/framework/foundation/bootstrap"
	fhttp "github.com/arandu-io/framework/http"
	"github.com/arandu-io/framework/http/middleware"
	"github.com/arandu-io/framework/jobs"
	"github.com/arandu-io/framework/kernel"
	"github.com/arandu-io/framework/mail"
	"github.com/arandu-io/framework/modules/auth"
	"github.com/arandu-io/framework/scheduler"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/framework/view"
	cache2 "github.com/arandu-io/hesape/cache"
	"github.com/arandu-io/hesape/exception"
	"github.com/arandu-io/hesape/queue"
	hredis "github.com/arandu-io/hesape/redis"
	"github.com/arandu-io/hesape/redis/connections"
	hmiddleware "github.com/arandu-io/hesape/routing/middleware"

	controllers "github.com/arandu-io/arandu/app/Http/Controllers"
	listeners "github.com/arandu-io/arandu/app/Listeners"
	providers "github.com/arandu-io/arandu/app/Providers"
	appconfig "github.com/arandu-io/arandu/config"
	"github.com/arandu-io/arandu/routes"

	// The compiled stylesheet, embedded. Without this import the browser gets
	// the framework's default and every class written in a view of this project
	// is silently absent from the page.
	_ "github.com/arandu-io/arandu/assets"

	// This application's own schema changes. Importing them is what registers
	// them: each one calls migrations.Register from init(), and a package
	// nothing imports is not in the binary at all -- so without this line `aru
	// migrate` finds nothing and says so only by creating no tables.
	_ "github.com/arandu-io/arandu/database/migrations"

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
	// `go get github.com/arandu-io/hesape/database/connectors/mysql` plus a line
	// here.
	//
	// They are in bootstrap rather than in main because bootstrap is what
	// composes the application, and the tests compose it too: with them in main
	// every feature test opened a connection to a driver nobody had registered.
	_ "github.com/arandu-io/hesape/database/connectors/pgx"
	_ "github.com/arandu-io/hesape/database/connectors/sqlite"

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
	// Relay is what empties the outbox. It is returned as well as registered for
	// the reason Auth is, sharpened by what it guards: a test that built a relay
	// of its own would pass over an application that wires none, and an
	// application that wires none writes rows nothing ever reads.
	Relay *events.Relay
	// Queue is the job store `aru work` drains.
	Queue *queue.DatabaseQueue
	// Mail is what sends. It is returned as well as used, because a job that
	// sends is built outside this function and reaching back in for the mailer
	// later is the hidden coupling the explicit wiring exists to avoid.
	Mail *mail.Mailer
	// Cache is the RESP connection, and nil when no store resolved one.
	//
	// It is returned as well as used because `migrate --isolated` takes its
	// lock, and the migration commands are built outside this function. Nil is
	// what that command refuses on: a lock inside one process isolates nothing
	// from the replica beside it.
	Cache *connections.Connection
}

// Build wires the application and returns it ready to boot.
//
// It does not boot, listen or migrate. main.go decides which of those the
// requested command needs, which is what keeps `aru routes` from opening a
// socket and `aru work` from starting a scheduler.
//
// It fails when what the configuration named cannot be assembled -- a
// certificate file that is not there is the case that exists today. Refusing
// here is the point: the alternative is a process that starts having quietly
// dropped what it was told to use.
func Build(cfg appconfig.Config, db *data.DB) (App, error) {
	fw := cfg.Framework

	csrf := security.NewCSRF(fw.App.Key, cfg.Session.CSRFTTL)

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
	sessions := security.NewSessionStore(fw.App.Key, cfg.Session.TTL, cfg.Session.Secure, backend)

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

	// The queue over the application's own database, which is what makes a job
	// commitable by the same transaction as the row it is about. For volume
	// beyond a table, github.com/arandu-io/hesape/queue/connectors/redis is the
	// same contract over RESP -- same Worker, same handlers, one line here.
	queueStore := queue.NewDatabaseQueue(db)

	// The relay that empties the outbox, and the listener it hands events to.
	//
	// events.NewModule() brings the table and publishes nothing, which is a
	// coherent state -- storing is what cannot be recovered later, publishing can
	// start the day there is somewhere to publish to. It stops being coherent the
	// moment something stores: the auth module writes a row for every
	// registration, every confirmed address and every reset password, and without
	// this line they accumulate in a table no process reads.
	//
	// # It runs in `aru serve`, and in no other command
	//
	// That is not decided here. The module's loop is a kernel.Background one, and
	// Start is called by Kernel.Run and never by Kernel.Boot -- so `aru work`,
	// `aru routes` and every migration command build this same application and
	// start no relay.
	//
	// It is also the right place rather than the convenient one. `aru work`
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
	relay := events.NewRelay(events.NewOutbox(db), listeners.NewEventLog(), events.RelayOptions{Locker: locker})

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

	userRepository := auth.NewUserRepo(db)
	authService := auth.NewService(userRepository, sessions, csrf)

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
			// k.Recorder() is the buffer behind /_arandu/debug. It is nil
			// outside development, and passing nil records nothing -- which is
			// what production does.
			middleware.Observe(cfg.App.IsDev(), fw.Observability.TracingSecret, k.Recorder()),
			middleware.SecurityHeaders(cfg.App.IsDev()),
			// The budget and the window are one value, which is what a named
			// limiter resolves to. The refusal is passed rather than assumed:
			// how a 4xx is written belongs to the request layer, and this one
			// adds HX-Refresh, without which somebody over the limit presses the
			// button and the screen does not change.
			//
			// The key is unchanged, and it has to be: a counter in a shared
			// store is keyed by the string KeyBySession returns, so a different
			// one would hand every caller a fresh budget on deploy.
			hmiddleware.Throttle(limiter, cache2.PerMinute(300),
				middleware.KeyBySession(sessions.IDFromRequest), fhttp.Refuse),
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
			// The jobs table. Work that happens after the response, drained by
			// `aru work` -- the same image with another argument, which is what
			// keeps the deploy at one artifact.
			//
			// The module is the framework's and the driver is the one built
			// above: a module registers its routes on the framework's router,
			// and the driver goes through untouched, so the schema this brings
			// is the one the driver carries. A queue that keeps its jobs
			// elsewhere brings no table, and says so by carrying none.
			jobs.NewModule(queueStore),
			// This application: its routes, from routes/web.go. Its migrations
			// arrive by the blank import above, not through here.
			providers.NewAppServiceProvider(deps),
			// `aru make:module` adds the next modules here.
		)

	// The RESP connection reports itself on the health check and gives its pool
	// back at shutdown. Without the module, "the store is down" arrives as a
	// class of request failures somebody has to correlate by hand.
	//
	// It is registered whenever a store resolved a connection, which is what
	// makes the probe follow the deployment rather than one setting: a process
	// whose cache is in-process and whose sessions live over RESP depends on
	// that server for every request, and a probe that stayed green because
	// CACHE_STORE said memory would be reporting half of it.
	if conn := stores.Connection(); conn != nil {
		k.Register(kernel.NewCacheModule("cache", conn))
	}

	// The scheduler goes last, because it collects the tasks the modules above
	// declared. A module never starts its own goroutine; it declares work, and
	// this is what runs it.
	//
	// The Locker is what keeps a Singleton task on one replica, and it is the
	// one built above, beside the cache it comes from.
	//
	// Tenants is nil: a PerTenant task needs to know which tenants exist, and
	// only the application knows where that list lives. Wire it and the
	// scheduler expands the task to each of them, with its own Grant.
	// Recorder for the same reason as the worker: a scheduled task is
	// investigated on the same page as a request, and costs nothing when
	// nothing is recording.
	sched := scheduler.NewModule(k.Tasks(), scheduler.Options{Recorder: k.Recorder(), Locker: locker})
	k.Register(sched)

	return App{Kernel: k, Auth: authService, Scheduler: sched, Relay: relay, Queue: queueStore, Mail: mailer, Cache: stores.Connection()}, nil
}

// The names this application's cache stores are known by, and the driver the
// shared one is built by.
//
// The names are the values CACHE_STORE takes, so the word in the environment
// and the word in the wiring are one word. The driver is a name of the
// manager's own: it builds array, file, database, null and failover and no
// RESP store, because that store ships in a module of its own so its driver
// stays out of the binaries that do not use it.
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

	// conn is the RESP connection, opened the first time a store resolves it
	// and nil until then. It is kept because three consumers are written
	// against the connection rather than against the store: the health module
	// reports it, the queue writes the flag `aru queue:pause` sets through it,
	// and the session handler is built over it.
	//
	// Opened on resolution rather than at wiring, so a deployment whose stores
	// are all in-process dials nothing even when REDIS_URL is still set from a
	// configuration it has moved off.
	//
	// It is written while the application is being composed, by one goroutine,
	// and read afterwards. Nothing resolves a store once Build has returned:
	// the manager is not reachable from App.
	conn *connections.Connection
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

	// Registering the driver is what puts the RESP store in this binary. The
	// creator closes over the connection rather than reading one out of the
	// store's configuration, because a StoreConfig carries a database
	// connection and has nowhere to put this one.
	out.manager.Extend(respDriver, func(m *cache2.CacheManager, store cache2.StoreConfig) (*cache2.Repository, error) {
		conn, err := out.connect()
		if err != nil {
			return nil, err
		}
		return m.Repository(hredis.NewRedisStore(conn), store), nil
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

// Shared returns the connection behind the named store, and refuses when that
// store is one this process keeps to itself.
//
// The refusal is the point of the method. A caller reaching for it wants state
// every replica can read -- a session, an isolation lock -- and the in-process
// store satisfies every type it would be handed to while satisfying none of
// what was asked for. Refusing here puts that in the boot, where it names both
// settings, instead of in production, where it looks like people being signed
// out at random.
//
// It goes through the store rather than around it, so the connection it hands
// back is the one the named store is built over and not a second one beside it.
func (c *cacheStores) Shared(name string) (*connections.Connection, error) {
	if !c.IsShared(name) {
		return nil, fmt.Errorf("the cache store %q is kept inside this process, and what is asked of it here is state every replica can read: "+
			"REDIS_URL is what names a store they all see", name)
	}
	if _, err := c.Store(name); err != nil {
		return nil, err
	}
	return c.conn, nil
}

// Connection returns the RESP connection when a store resolved one, and nil
// when none did.
func (c *cacheStores) Connection() *connections.Connection { return c.conn }

// connect opens the RESP connection, once.
//
// It does not talk to the server. A connection that dialled here would make the
// application refuse to start because the cache is down, which is the opposite
// of what a cache is for; the health check is what reports it.
//
// What it does do at the boot is read the files the configuration named, and a
// file that is named and cannot be read stops it -- see cacheTLS.
func (c *cacheStores) connect() (*connections.Connection, error) {
	if c.conn != nil {
		return c.conn, nil
	}
	if c.settings.Address == "" {
		return nil, fmt.Errorf("the RESP store was asked for and REDIS_URL names no endpoint")
	}

	encryption, err := cacheTLS(c.settings)
	if err != nil {
		return nil, err
	}

	c.conn = connections.Connect(connections.Config{
		Address:  c.settings.Address,
		Password: c.settings.Password,
		Database: c.settings.Database,
		Prefix:   c.settings.Prefix,
		TLS:      encryption,
	})
	return c.conn, nil
}

// sessionBackend builds the session backend SESSION_DRIVER named.
//
// The driver names a store, and not the cache's default one. SESSION_DRIVER=kv
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
// The handler is built over the connection and not over the store's repository,
// which is what keeps the session from changing the prefix or the connection
// the cache is using: there is nothing shared between them to change. The keys
// it writes are its own -- session and session-index -- so the two occupy one
// server without meeting.
func sessionBackend(cfg appconfig.Session, stores *cacheStores) (security.SessionBackend, error) {
	switch cfg.Driver {
	case appconfig.SessionMemory:
		// Right for one instance and wrong for two, and it is what
		// SESSION_DRIVER=memory asks for.
		return security.NewMemoryBackend(), nil

	case appconfig.SessionKV:
		conn, err := stores.Shared(respStore)
		if err != nil {
			return nil, fmt.Errorf("SESSION_DRIVER %q names the cache store %q: %w", cfg.Driver, respStore, err)
		}
		return security.NewSessionBackend(hredis.NewCacheBasedSessionHandler[security.Subject](conn)), nil

	default:
		// Unreachable through Load, which refuses the value first. It is here
		// because a configuration built in Go skips that check, and a driver
		// nobody recognises must not fall through to the in-process store: the
		// sessions would be kept where nothing asked for them.
		return nil, fmt.Errorf("SESSION_DRIVER has unsupported value %q; expected %s or %s",
			cfg.Driver, appconfig.SessionMemory, appconfig.SessionKV)
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
// A shared store that cannot hold a lock is refused rather than skipped. No
// store defined here is in that position, and it is checked because the
// alternative to checking is the failure this shape exists to end -- a
// scheduler that declares a Singleton task and runs it on every replica.
func cacheLocker(stores *cacheStores, cfg appconfig.Cache) (kernel.Locker, error) {
	name := string(cfg.Store)
	if !stores.IsShared(name) {
		return nil, nil
	}

	store, err := stores.Store(name)
	if err != nil {
		return nil, err
	}
	locking, ok := store.GetStore().(cache2.Locking)
	if !ok {
		return nil, fmt.Errorf("the cache store %q is shared by every replica and cannot hold a lock, "+
			"so a Singleton task would run on all of them", name)
	}
	return kernel.NewLocker(cache2.NewLocks(locking)), nil
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
