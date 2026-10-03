// Example resource. Remove with the list under "The example resource" in README.md.

package services

import (
	"context"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/observability"
	"github.com/arandu-io/framework/security"
	"github.com/arandu-io/hesape/pagination"

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
	db     *data.DB
	policy policies.NotePolicy
}

// NewNoteService wires the service.
func NewNoteService(db *data.DB) *NoteService {
	return &NoteService{db: db}
}

// Create walks the mandatory path: validate, Authorize, Grant, Model.
// There is no other order that compiles.
func (s *NoteService) Create(ctx context.Context, actor security.Subject, in requests.NoteRequest) (*models.Note, error) {
	if errs := in.Validate(); errs.Any() {
		return nil, errs
	}

	var proposed models.Note
	s.fill(&proposed, in)
	proposed.UserID = actor.ID
	g, err := security.Authorize(ctx, s.policy, actor, policies.NoteCreate, proposed)
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
	record.TenantID = data.Tenant(g)
	// The author is who is asking, from the session the route guard loaded --
	// never a field of the form.
	record.UserID = actor.ID
	if _, err := record.Save(ctx, g); err != nil {
		return nil, err
	}
	// Guarded: the entity is a struct value, and boxing it into `any` allocates
	// at the call site even though RecordEvent is a no-op on a nil Collector.
	if col := observability.FromContext(ctx); col != nil {
		col.RecordEvent("note.created", record)
	}
	return record, nil
}

// Get returns one note.
//
// It authorizes twice: once to look at all, and once with the row that was
// read, which is the decision about this row rather than about the action.
func (s *NoteService) Get(ctx context.Context, actor security.Subject, id string) (*models.Note, error) {
	g, err := security.Authorize(ctx, s.policy, actor, policies.NoteView, models.Note{})
	if err != nil {
		return nil, err
	}
	found, err := models.Notes(s.db).FindOrFail(ctx, g, id)
	if err != nil {
		return nil, err
	}
	if _, err := security.Authorize(ctx, s.policy, actor, policies.NoteView, *found); err != nil {
		return nil, err
	}
	return found, nil
}

// List returns one page of notes, newest first, and where the
// previous and the next page are.
func (s *NoteService) List(ctx context.Context, actor security.Subject, page int) (models.NoteCollection, *pagination.Page, error) {
	g, err := security.Authorize(ctx, s.policy, actor, policies.NoteList, models.Note{})
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
func (s *NoteService) Update(ctx context.Context, actor security.Subject, id string, in requests.NoteRequest) (*models.Note, error) {
	if errs := in.Validate(); errs.Any() {
		return nil, errs
	}

	stored, err := s.Get(ctx, actor, id)
	if err != nil {
		return nil, err
	}

	g, err := security.Authorize(ctx, s.policy, actor, policies.NoteUpdate, *stored)
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
func (s *NoteService) Delete(ctx context.Context, actor security.Subject, id string) error {
	stored, err := s.Get(ctx, actor, id)
	if err != nil {
		return err
	}

	g, err := security.Authorize(ctx, s.policy, actor, policies.NoteDelete, *stored)
	if err != nil {
		return err
	}
	if _, err := stored.Delete(ctx, g); err != nil {
		return err
	}
	if col := observability.FromContext(ctx); col != nil {
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
// arandu:end custom
