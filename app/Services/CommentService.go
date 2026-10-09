// Example resource. Remove with the list under "The example resource" in README.md.

package services

import (
	"context"

	"github.com/arandu-io/hesape/auth"
	"github.com/arandu-io/hesape/database"
	"github.com/arandu-io/hesape/log"
	"github.com/arandu-io/hesape/pagination"

	requests "github.com/arandu-io/arandu/app/Http/Requests"
	models "github.com/arandu-io/arandu/app/Models"
	policies "github.com/arandu-io/arandu/app/Policies"
)

// commentPerPage is how many comments one page of the listing holds. A
// listing is always a page, never everything: an unbounded query is how one
// page load takes a database down.
const commentPerPage = 25

// CommentService holds the business rules. It receives its dependencies through
// the constructor -- explicit wiring, no container.
//
// Every method authorizes before it reaches the Model, and the errors it returns
// are the ones the router answers: validation.Errors back to the form, a missing
// row as 404, a refusal as 403.
type CommentService struct {
	db     *database.DB
	policy policies.CommentPolicy
	// parents loads the note a comment belongs to, under the
	// note's own policy: the parent in a path is where the person
	// navigated, never whose data it is.
	parents *NoteService
}

// NewCommentService wires the service.
func NewCommentService(db *database.DB, parents *NoteService) *CommentService {
	return &CommentService{db: db, parents: parents}
}

// Create walks the mandatory path: validate, Authorize, Grant, Model.
// There is no other order that compiles.
func (s *CommentService) Create(ctx context.Context, actor auth.Subject, noteID string, in requests.CommentRequest) (*models.Comment, error) {
	if errs := in.Validate(); errs.Any() {
		return nil, errs
	}

	// The note is loaded through its own service, so one this
	// subject may not see is a refusal here and never a row under it.
	parent, err := s.parents.Get(ctx, actor, noteID)
	if err != nil {
		return nil, err
	}

	var proposed models.Comment
	s.fill(&proposed, in)
	proposed.NoteID = parent.ID
	proposed.UserID = actor.ID
	g, err := auth.Authorize(ctx, s.policy, actor, policies.CommentCreate, proposed)
	if err != nil {
		return nil, err
	}

	record, err := models.Comments(s.db).New()
	if err != nil {
		return nil, err
	}
	s.fill(record, in)
	record.NoteID = parent.ID
	// The tenant comes from the Grant, never from the request or the subject
	// directly. The model writes the same value over the insert attributes.
	record.TenantID = auth.Tenant(g)
	// The author is who is asking, never a field of the form.
	record.UserID = actor.ID
	if _, err := record.Save(ctx, g); err != nil {
		return nil, err
	}
	// Guarded: the entity is a struct value, and boxing it into `any` allocates
	// at the call site even though RecordEvent is a no-op on a nil Collector.
	if col := log.FromContext(ctx); col != nil {
		col.RecordEvent("comment.created", record)
	}
	return record, nil
}

// Get returns one comment.
//
// It authorizes twice: once to look at all, and once with the row that was
// read, which is the decision about this row rather than about the action.
func (s *CommentService) Get(ctx context.Context, actor auth.Subject, id string) (*models.Comment, error) {
	g, err := auth.Authorize(ctx, s.policy, actor, policies.CommentView, models.Comment{})
	if err != nil {
		return nil, err
	}
	found, err := models.Comments(s.db).FindOrFail(ctx, g, id)
	if err != nil {
		return nil, err
	}
	if _, err := auth.Authorize(ctx, s.policy, actor, policies.CommentView, *found); err != nil {
		return nil, err
	}
	return found, nil
}

// List returns one page of the comments of one note, newest
// first, and where the previous and the next page are. The note is
// loaded under its own policy first, and the page is filtered by the one that
// was loaded -- not by the id the path carried.
func (s *CommentService) List(ctx context.Context, actor auth.Subject, noteID string, page int) (models.CommentCollection, *pagination.Page, error) {
	parent, err := s.parents.Get(ctx, actor, noteID)
	if err != nil {
		return nil, nil, err
	}
	g, err := auth.Authorize(ctx, s.policy, actor, policies.CommentList, models.Comment{NoteID: parent.ID})
	if err != nil {
		return nil, nil, err
	}
	return models.Comments(s.db).Where("note_id", parent.ID).Latest().OrderBy("id").
		SimplePaginate(ctx, g, commentPerPage, page, pagination.Options{})
}

// Update changes the mutable fields.
//
// It reads before writing, so the policy decides against the stored row rather
// than against what the client claims the row is. Skipping this is how a check
// passes on attacker-supplied data.
func (s *CommentService) Update(ctx context.Context, actor auth.Subject, id string, in requests.CommentRequest) (*models.Comment, error) {
	if errs := in.Validate(); errs.Any() {
		return nil, errs
	}

	stored, err := s.Get(ctx, actor, id)
	if err != nil {
		return nil, err
	}

	g, err := auth.Authorize(ctx, s.policy, actor, policies.CommentUpdate, *stored)
	if err != nil {
		return nil, err
	}
	s.fill(stored, in)
	if _, err := stored.Save(ctx, g); err != nil {
		return nil, err
	}
	return stored, nil
}

// Delete removes a comment.
func (s *CommentService) Delete(ctx context.Context, actor auth.Subject, id string) error {
	stored, err := s.Get(ctx, actor, id)
	if err != nil {
		return err
	}

	g, err := auth.Authorize(ctx, s.policy, actor, policies.CommentDelete, *stored)
	if err != nil {
		return err
	}
	if _, err := stored.Delete(ctx, g); err != nil {
		return err
	}
	if col := log.FromContext(ctx); col != nil {
		col.RecordEvent("comment.deleted", stored)
	}
	return nil
}

// fill writes the request onto the record. It is the one place a field of the
// form becomes a column, for Create and Update alike.
func (s *CommentService) fill(co *models.Comment, in requests.CommentRequest) {
	co.Body = in.Body
}

// arandu:begin custom
// Business rules beyond CRUD go here, and survive regeneration.
// arandu:end custom
