// Example resource. Remove with the list under "The example resource" in README.md.

package services

import (
	"context"
	"fmt"
	"time"

	"github.com/arandu-io/hesape/auth"
	"github.com/arandu-io/hesape/database"
	"github.com/arandu-io/hesape/events"
	"github.com/arandu-io/hesape/log"
	"github.com/arandu-io/hesape/pagination"
	"github.com/arandu-io/hesape/support"

	clients "github.com/arandu-io/arandu/app/Clients"
	appevents "github.com/arandu-io/arandu/app/Events"
	requests "github.com/arandu-io/arandu/app/Http/Requests"
	models "github.com/arandu-io/arandu/app/Models"
	policies "github.com/arandu-io/arandu/app/Policies"
)

// noteListing declares what the notes list may be searched, ordered, filtered,
// paged and hidden by, once. A request names columns by their public key and
// the listing keeps only the keys declared here, so no column, direction or
// size of a request reaches the query unchecked. A listing is always a page,
// never everything: an unbounded query is how one page load takes a database
// down.
var noteListing = func() *pagination.Listing {
	listing, err := pagination.NewListing(pagination.ListingSpec{
		Search:      []string{"title", "body"},
		Sorts:       map[string]string{"title": "title", "body": "body", "pinned": "pinned", "created_at": "created_at"},
		DefaultSort: "created_at",
		DefaultDir:  pagination.SortDesc,
		TieBreak:    "id",
		// pinned is a boolean column, compared as the text a boolean is written as
		// on every engine.
		Facets:          []pagination.FacetSpec{{Key: "pinned", Column: "pinned", Values: []string{"1", "0"}}},
		PageSizes:       []int{10, 25, 50},
		DefaultPageSize: 25,
		Hideable:        []string{"body", "pinned", "created_at"},
	})
	if err != nil {
		// A spec the package refuses is a mistake in this file, and it is found
		// when the program starts and not on the first request.
		panic(fmt.Sprintf("services: the notes listing: %v", err))
	}
	return listing
}()

// NoteService holds the business rules. It receives its dependencies through
// the constructor -- explicit wiring, no container.
//
// Every method authorizes before it reaches the Model, and the errors it returns
// are the ones the router answers: validation.Errors back to the form, a missing
// row as 404, a refusal as 403.
type NoteService struct {
	db     *database.DB
	policy policies.NotePolicy
	// outbox stores the events a write produces, in the write's transaction.
	outbox *events.Outbox
	// now is the clock the transitions are handed. The entity reads none of
	// its own, so a test can pin the time here. It answers UTC, the zone every
	// instant is stored in: a time in the server's zone loses its offset on
	// the way into a column that keeps none, and comes back as UTC hours off.
	now func() time.Time
	// newsletter is where the nightly digest goes, and nil when the project
	// names no newsletter. The service depends on the interface, so a test
	// hands it the fake and nothing leaves the process.
	newsletter clients.Newsletter
}

// NewNoteService wires the service.
func NewNoteService(db *database.DB) *NoteService {
	return &NoteService{db: db, outbox: events.NewOutbox(db), now: utcNow}
}

// utcNow is the framework's clock, which a test moves with support.Travel,
// read in UTC.
func utcNow() time.Time { return support.Now().UTC() }

// Create walks the mandatory path: validate, Authorize, Grant, Model.
// There is no other order that compiles.
//
// The request was validated by its Rules when ctx.Bind filled it. A rule that
// spans fields, or asks the data, is a Validate method of the request, called
// first thing here: if errs := in.Validate(); errs.Any() { return nil, errs }.
func (s *NoteService) Create(ctx context.Context, actor auth.Subject, in requests.NoteRequest) (*models.Note, error) {
	var proposed models.Note
	s.fill(&proposed, in)
	proposed.UserID = actor.ID
	g, err := auth.Authorize(ctx, s.policy, actor, policies.NoteCreate, proposed)
	if err != nil {
		return nil, err
	}

	record, err := models.Notes(s.db).New()
	if err != nil {
		return nil, err
	}
	s.fill(record, in)
	// The tenant comes from the Grant, never from the request or the subject
	// directly. The model writes the same value over the insert attributes.
	record.TenantID = auth.Tenant(g)
	// The author is who is asking, from the session the route guard loaded --
	// never a field of the form.
	record.UserID = actor.ID
	if _, err := record.Save(ctx, g); err != nil {
		return nil, err
	}
	// Guarded: the entity is a struct value, and boxing it into `any` allocates
	// at the call site even though RecordEvent is a no-op on a nil Collector.
	if col := log.FromContext(ctx); col != nil {
		col.RecordEvent("note.created", record)
	}
	return record, nil
}

// Get returns one note.
//
// It authorizes twice: once to look at all, and once with the row that was
// read, which is the decision about this row rather than about the action.
func (s *NoteService) Get(ctx context.Context, actor auth.Subject, id string) (*models.Note, error) {
	g, err := auth.Authorize(ctx, s.policy, actor, policies.NoteView, models.Note{})
	if err != nil {
		return nil, err
	}
	found, err := models.Notes(s.db).FindOrFail(ctx, g, id)
	if err != nil {
		return nil, err
	}
	if _, err := auth.Authorize(ctx, s.policy, actor, policies.NoteView, *found); err != nil {
		return nil, err
	}
	return found, nil
}

// Listing is the declaration of the notes list: the controller reads the
// request's state through it, and nothing else of the query string.
func (s *NoteService) Listing() *pagination.Listing { return noteListing }

// List returns the page of notes the listing state asks for -- searched,
// filtered, ordered and paged -- with the result a table is drawn from: the
// state that was read, the page with its total and the count of every facet
// value. path is the address the listing is served at.
//
// The state is read from the request by Listing, which drops every key, value
// and size it did not declare. The Grant is the only source of the tenant, and
// the page, its total and every count are filtered by it.
func (s *NoteService) List(ctx context.Context, actor auth.Subject, state pagination.ListingState, path string) (models.NoteCollection, *pagination.ListingResult, error) {
	g, err := auth.Authorize(ctx, s.policy, actor, policies.NoteList, models.Note{})
	if err != nil {
		return nil, nil, err
	}
	return models.Notes(s.db).Listing(ctx, g, state, path)
}

// Update changes the mutable fields.
//
// It reads before writing, so the policy decides against the stored row rather
// than against what the client claims the row is. Skipping this is how a check
// passes on attacker-supplied data.
//
// The request was validated by its Rules when ctx.Bind filled it; a Validate
// method of the request, for a rule that spans fields or asks the data, is
// called first thing here, as Create does.
func (s *NoteService) Update(ctx context.Context, actor auth.Subject, id string, in requests.NoteRequest) (*models.Note, error) {
	stored, err := s.Get(ctx, actor, id)
	if err != nil {
		return nil, err
	}

	g, err := auth.Authorize(ctx, s.policy, actor, policies.NoteUpdate, *stored)
	if err != nil {
		return nil, err
	}
	s.fill(stored, in)
	if _, err := stored.Save(ctx, g); err != nil {
		return nil, err
	}
	return stored, nil
}

// Delete removes a note.
func (s *NoteService) Delete(ctx context.Context, actor auth.Subject, id string) error {
	stored, err := s.Get(ctx, actor, id)
	if err != nil {
		return err
	}

	g, err := auth.Authorize(ctx, s.policy, actor, policies.NoteDelete, *stored)
	if err != nil {
		return err
	}
	if _, err := stored.Delete(ctx, g); err != nil {
		return err
	}
	if col := log.FromContext(ctx); col != nil {
		col.RecordEvent("note.deleted", stored)
	}
	return nil
}

// fill writes the request onto the record. It is the one place a field of the
// form becomes a column, for Create and Update alike.
func (s *NoteService) fill(n *models.Note, in requests.NoteRequest) {
	n.Title = in.Title
	n.Body = in.Body
	n.Pinned = in.Pinned
}

// arandu:begin custom
// Business rules beyond CRUD go here, and survive regeneration.

// GetForUpdate returns the note the subject may change: read and authorized
// as Get reads it, then authorized for note.update against the stored row.
// The edit form asks it, so the form is drawn only for whoever may submit it
// and anybody else is refused with 403 before typing anything.
func (s *NoteService) GetForUpdate(ctx context.Context, actor auth.Subject, id string) (*models.Note, error) {
	stored, err := s.Get(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	if _, err := auth.Authorize(ctx, s.policy, actor, policies.NoteUpdate, *stored); err != nil {
		return nil, err
	}
	return stored, nil
}

// NoteAbilities is what one subject may do with one note, as the policy
// answers it: what a page offers, so it draws no button that would be
// refused.
type NoteAbilities struct {
	// Update is whether the note may be changed.
	Update bool
	// Delete is whether the note may be removed.
	Delete bool
	// Publish is whether the note may be published.
	Publish bool
}

// Abilities asks the policy, through auth.Allows, what actor may do with n,
// a note it read through Get. It authorizes nothing: each action still asks
// for its own Grant when the button is pressed, and that is the decision.
func (s *NoteService) Abilities(ctx context.Context, actor auth.Subject, n *models.Note) NoteAbilities {
	return NoteAbilities{
		Update:  auth.Allows(ctx, s.policy, actor, policies.NoteUpdate, *n),
		Delete:  auth.Allows(ctx, s.policy, actor, policies.NoteDelete, *n),
		Publish: auth.Allows(ctx, s.policy, actor, policies.NotePublish, *n),
	}
}

// Publish makes a note public: the named action of the resource.
//
// The service orders it and the entity decides it. The row is read through
// the Grant and authorized as the row it is; Note.Publish is the transition,
// which refuses a note that is already published; and the row and the
// note.published event are written in one transaction, so the relay never
// hands over a publication that rolled back, and a publication is never stored
// without its event.
func (s *NoteService) Publish(ctx context.Context, actor auth.Subject, id string) (*models.Note, error) {
	stored, err := s.Get(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	g, err := auth.Authorize(ctx, s.policy, actor, policies.NotePublish, *stored)
	if err != nil {
		return nil, err
	}
	if err := stored.Publish(s.now()); err != nil {
		return nil, err
	}

	err = database.Transaction(ctx, s.db, func(ctx context.Context) error {
		if _, err := stored.Save(ctx, g); err != nil {
			return err
		}
		published := appevents.NotePublished{Title: stored.Title, Author: stored.UserID}
		return s.outbox.Store(ctx, g, []events.Event{published.Event(stored.ID)})
	})
	if err != nil {
		return nil, err
	}
	return stored, nil
}

// digestSize bounds one digest: a listing is always a page, and so is what
// goes to the newsletter.
const digestSize = 50

// WithNewsletter returns the service sending its digest to the newsletter.
func (s *NoteService) WithNewsletter(newsletter clients.Newsletter) *NoteService {
	s.newsletter = newsletter
	return s
}

// SendDigest hands the newsletter the notes published since a time, newest
// first, in the tenant of the Grant.
//
// It is the one method here that takes a Grant rather than a subject, because
// its caller has no subject: the nightly job runs under the Grant the worker
// rebuilt from the job's row, issued for note.list in one tenant. The Grant is
// checked for that action before the read, and the read is scoped by its
// tenant like every other. key identifies the digest across retries, so the
// newsletter sends a repeated one once.
//
// A project that names no newsletter sends nothing, and that is not a failure.
func (s *NoteService) SendDigest(ctx context.Context, g auth.Grant, since time.Time, key string) error {
	if s.newsletter == nil {
		return nil
	}
	if err := g.Check(policies.NoteList); err != nil {
		return err
	}
	published, err := models.Notes(s.db).Where("published_at", ">=", since).
		Latest("published_at").Limit(digestSize).Get(ctx, g)
	if err != nil || len(published) == 0 {
		return err
	}

	digest := clients.NewsletterDigest{Key: key, Notes: make([]clients.NewsletterNote, 0, len(published))}
	for _, n := range published {
		digest.Notes = append(digest.Notes, clients.NewsletterNote{Title: n.Title, PublishedAt: *n.PublishedAt})
	}
	return s.newsletter.SendDigest(ctx, digest)
}

// arandu:end custom
