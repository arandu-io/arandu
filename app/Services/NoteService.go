// Example resource. Remove with the list under "The example resource" in README.md.

package services

import (
	"context"
	"time"

	frameevents "github.com/arandu-io/framework/events"
	"github.com/arandu-io/hesape/auth"
	"github.com/arandu-io/hesape/database"
	"github.com/arandu-io/hesape/events"
	"github.com/arandu-io/hesape/log"
	"github.com/arandu-io/hesape/pagination"

	appevents "github.com/arandu-io/arandu/app/Events"
	requests "github.com/arandu-io/arandu/app/Http/Requests"
	models "github.com/arandu-io/arandu/app/Models"
	policies "github.com/arandu-io/arandu/app/Policies"
)

// notePerPage is how many notes one page of the listing holds. A
// listing is always a page, never everything: an unbounded query is how one
// page load takes a database down.
const notePerPage = 25

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
	// its own, so a test can pin the time here.
	now func() time.Time
}

// NewNoteService wires the service.
func NewNoteService(db *database.DB) *NoteService {
	return &NoteService{db: db, outbox: frameevents.NewOutbox(db), now: time.Now}
}

// Create walks the mandatory path: validate, Authorize, Grant, Model.
// There is no other order that compiles.
func (s *NoteService) Create(ctx context.Context, actor auth.Subject, in requests.NoteRequest) (*models.Note, error) {
	if errs := in.Validate(); errs.Any() {
		return nil, errs
	}

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

// List returns one page of notes, newest first, and where the
// previous and the next page are.
func (s *NoteService) List(ctx context.Context, actor auth.Subject, page int) (models.NoteCollection, *pagination.Page, error) {
	g, err := auth.Authorize(ctx, s.policy, actor, policies.NoteList, models.Note{})
	if err != nil {
		return nil, nil, err
	}
	return models.Notes(s.db).Latest().OrderBy("id").
		SimplePaginate(ctx, g, notePerPage, page, pagination.Options{})
}

// Update changes the mutable fields.
//
// It reads before writing, so the policy decides against the stored row rather
// than against what the client claims the row is. Skipping this is how a check
// passes on attacker-supplied data.
func (s *NoteService) Update(ctx context.Context, actor auth.Subject, id string, in requests.NoteRequest) (*models.Note, error) {
	if errs := in.Validate(); errs.Any() {
		return nil, errs
	}

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

// arandu:end custom
