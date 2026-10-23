// Package providers holds this application's service providers: each one a
// foundation.Module listed in bootstrap/app.go, built over the drivers the
// framework made from the configuration.
package providers

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/arandu-io/framework/events"
	"github.com/arandu-io/framework/foundation"
	"github.com/arandu-io/hesape/auth"
	"github.com/arandu-io/hesape/auth/account"
	"github.com/arandu-io/hesape/database/model"
	hevents "github.com/arandu-io/hesape/events"
	"github.com/arandu-io/hesape/geo"
	hhttp "github.com/arandu-io/hesape/http"
	hnotifications "github.com/arandu-io/hesape/notifications"
	"github.com/arandu-io/hesape/notifications/channels"
	"github.com/arandu-io/hesape/onetime"
	"github.com/arandu-io/hesape/queue"
	hcontrollers "github.com/arandu-io/hesape/routing/controllers"
	"github.com/arandu-io/hesape/translation"

	clients "github.com/arandu-io/arandu/app/Clients"
	controllers "github.com/arandu-io/arandu/app/Http/Controllers"
	appmiddleware "github.com/arandu-io/arandu/app/Http/Middleware"
	appjobs "github.com/arandu-io/arandu/app/Jobs"
	listeners "github.com/arandu-io/arandu/app/Listeners"
	models "github.com/arandu-io/arandu/app/Models"
	policies "github.com/arandu-io/arandu/app/Policies"
	services "github.com/arandu-io/arandu/app/Services"
	appconfig "github.com/arandu-io/arandu/config"
	"github.com/arandu-io/arandu/database/seeders"
	"github.com/arandu-io/arandu/lang"
	"github.com/arandu-io/arandu/routes"
)

// AppServiceProvider builds this application's services once, over the
// drivers, and hands them to the routes, the console, the worker and the
// seeders. It is a module so it boots and shuts down with the application; it
// registers no routes of its own -- those are routes/web.go, handed to
// WithRouting.
type AppServiceProvider struct {
	cfg     appconfig.Config
	drivers *foundation.Drivers

	// Users is the account rules over this application's users table.
	Users *account.Users
	// TwoFactor is the authenticator second factor of those accounts.
	TwoFactor *account.Factors
	// Tokens issues, revokes and resolves personal access tokens.
	Tokens *services.PersonalAccessTokenService
	// Notes is the example resource's service. Remove it with the list under
	// "The example resource" in README.md.
	Notes *services.NoteService
	// EmailCodes stores purpose-bound verification and reset codes.
	EmailCodes onetime.CodeStore
	// Notifier tells somebody something, over the channels built for it.
	Notifier *hnotifications.Notifier
	// Relay empties the outbox into the listeners.
	Relay *events.Relay
	// Translator is the catalogue in lang/, which bootstrap hands every
	// request: a page says its lines through .Trans in the request's language,
	// and a component its own words.
	Translator *translation.Translator

	web routes.Deps
}

var _ foundation.Module = (*AppServiceProvider)(nil)

// NewAppServiceProvider builds the services. It fails when what the
// configuration named cannot be assembled.
func NewAppServiceProvider(cfg appconfig.Config, d *foundation.Drivers) (*AppServiceProvider, error) {
	p := &AppServiceProvider{cfg: cfg, drivers: d}
	db := d.DB

	// The catalogue, in APP_LOCALE and falling back to English. A file in lang/
	// that does not parse stops the boot here.
	translator, err := translation.NewFromDirectory(lang.Files, ".", cfg.App.Locale, "en")
	if err != nil {
		return nil, fmt.Errorf("providers: read the catalogue in lang: %w", err)
	}
	p.Translator = translator

	emailCodes, err := onetime.New(d.Store.GetStore(), cfg.Framework.App.Key, onetime.Config{TTL: cfg.Auth.PasswordResetTTL})
	if err != nil {
		return nil, fmt.Errorf("providers: build the email code store: %w", err)
	}
	p.EmailCodes = emailCodes

	// One channel: the database, which stores the row a bell menu draws from.
	p.Notifier = hnotifications.New([]hnotifications.Channel{
		channels.NewDatabase(hnotifications.NewTableStore(db)),
	})

	// The outbox the account rules store into and the relay that empties it,
	// claiming each pass where every replica sees it.
	outbox := hevents.NewOutbox(db)
	p.Relay = events.NewRelay(outbox, listeners.Each{
		listeners.NewEventLog(),
		listeners.NewNotifyNoteAuthor(p.Notifier),
	}, events.RelayOptions{Locker: d.Locker})

	// Closing an account, or resetting its password, revokes its tokens in that
	// write's own transaction: a token minted by whoever held the old password
	// would otherwise outlive it.
	p.Users, err = account.NewUsers(account.Config{
		DB:     db,
		Outbox: outbox,
		Query:  func() *model.Builder { return models.Users(db).Base() },
		Policy: account.PolicyOf(policies.UserPolicy{}),
		Hooks: account.Hooks{
			AfterClose: func(ctx context.Context, g auth.Grant, user account.Account) error {
				return p.Tokens.RevokeAll(ctx, g, user.AccountID())
			},
			AfterResetPassword: func(ctx context.Context, g auth.Grant, user account.Account) error {
				return p.Tokens.RevokeAll(ctx, g, user.AccountID())
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("providers: build the account rules: %w", err)
	}
	p.TwoFactor, err = account.NewFactors(db, cfg.Framework.App.Key, d.Store.GetStore(), outbox, p.Users)
	if err != nil {
		return nil, fmt.Errorf("providers: build the second factor: %w", err)
	}
	p.Tokens = services.NewPersonalAccessTokenService(db, p.Users, cfg.Auth.Tenant)

	// A session keeps who signed in, not what they may do: every request reads
	// the account again through currentSubject, so a role taken away, or an
	// account removed or closed, stops working on the next request.
	d.Sessions.ResolveSubjects(p.currentSubject)

	p.Notes = services.NewNoteService(db).WithNewsletter(clients.NewNewsletter(clients.NewsletterConfig{
		BaseURL: cfg.Services.Newsletter.URL,
		Token:   cfg.Services.Newsletter.Secret,
		Timeout: 10 * time.Second,
	}))

	p.web = routes.Deps{
		Home:        controllers.NewHomeController(p.Users, cfg.Auth.Tenant),
		Sessions:    d.Sessions,
		Tokens:      appmiddleware.NewPersonalAccessTokens(p.Tokens),
		Idempotency: d.Idempotency,
		Note:        controllers.NewNoteController(p.Notes, cfg.Framework.App.Timezone),
		Comment:     controllers.NewCommentController(services.NewCommentService(db, p.Notes), cfg.Framework.App.Timezone),
		NewsletterWebhook: controllers.NewNewsletterWebhookController(cfg.Services.NewsletterWebhook.Secret,
			services.NewNewsletterEventService(db, cfg.Auth.Tenant)),
	}
	return p, nil
}

// currentSubject is the session store's resolver: the subject of the account a
// session was opened for, as the account is stored now. It is built by
// services.CurrentSubject, through User.Subject, the one place a subject is
// made from a user, so a session and a sign-in cannot disagree about what an
// account may do.
//
// An account that is gone, closed or refused answers hhttp.ErrSubjectGone,
// which ends the session. Any other error is the read failing, and the store
// fails the request with it instead of using the roles the session began with.
func (p *AppServiceProvider) currentSubject(ctx context.Context, who auth.Subject) (auth.Subject, error) {
	subject, err := services.CurrentSubject(ctx, p.Users, who.Tenant, who.ID)
	if errors.Is(err, services.ErrAccountGone) {
		return auth.Subject{}, hhttp.ErrSubjectGone
	}
	return subject, err
}

// Name is the module identifier.
func (*AppServiceProvider) Name() string { return "app" }

// Routes registers nothing: the routes are routes/web.go.
func (*AppServiceProvider) Routes(*hcontrollers.Router) {}

// Web answers what routes/web.go dispatches to.
func (p *AppServiceProvider) Web() routes.Deps { return p.web }

// Console answers what routes/console.go's commands and schedule reach.
func (p *AppServiceProvider) Console() routes.ConsoleDeps {
	return routes.ConsoleDeps{Notes: p.Notes, Queue: p.drivers.Queue, Tenant: p.cfg.Auth.Tenant}
}

// Catalog is the public pages the GEO module publishes.
func (p *AppServiceProvider) Catalog() geo.Catalog {
	return geo.CatalogFunc(func(context.Context) ([]geo.Document, error) {
		return []geo.Document{{Path: "/", Title: p.cfg.App.Name}}, nil
	})
}

// RegisterJobs is every kind of work this application does in the background.
func (p *AppServiceProvider) RegisterJobs(w *queue.Worker) error {
	w.Handle(appjobs.SendNotesDigestName, appjobs.NewSendNotesDigestHandler(p.Notes))
	return nil
}

// Tenants is the list a per-tenant task expands to: the one tenant every login
// belongs to.
func (p *AppServiceProvider) Tenants(context.Context) ([]string, error) {
	return []string{p.cfg.Auth.Tenant}, nil
}

// Seed runs the seeders over what a seeder is allowed to touch.
func (p *AppServiceProvider) Seed(ctx context.Context, args []string) (string, error) {
	return seeders.Run(ctx, seeders.Deps{
		Users: p.Users, DB: p.drivers.DB, Tenant: p.cfg.Auth.Tenant, Development: p.cfg.App.IsDev(),
	}, args)
}
