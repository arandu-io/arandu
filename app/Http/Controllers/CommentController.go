// Example resource. Remove with the list under "The example resource" in README.md.

package controllers

import (
	fhttp "github.com/arandu-io/framework/http"
	hhttp "github.com/arandu-io/hesape/http"
	"github.com/arandu-io/hesape/pagination"
	"github.com/arandu-io/hesape/view"

	requests "github.com/arandu-io/arandu/app/Http/Requests"
	models "github.com/arandu-io/arandu/app/Models"
	services "github.com/arandu-io/arandu/app/Services"
	views "github.com/arandu-io/arandu/storage/framework/views/comments"
)

// CommentController answers the seven routes of the notes.comments resource.
//
// It nests under notes, shallow: the listing, the form and the store
// read the note from the path, and the four that act on one record
// read the record alone.
//
// It is thin on purpose: read the request, call the service, render. There is no
// repository here and there cannot be one -- hhttp.Context carries no database
// handle, so a controller that reached the data layer would be a controller that
// skipped the service, and therefore skipped the policy.
//
// The routes sit behind middleware.RequireAuth, which puts who is asking on the
// request: ctx.User reads it. Without the guard there is nobody there, and the
// policy refuses the zero subject. An error an action returns is answered by the
// router -- validation.Errors back to the form with the messages and what was
// typed, a missing row as 404, a refusal as 403 -- so no action maps one itself.
//
// view.New is the whole of a page's chrome: the title, the application name the
// framework put on the request, what a rejected attempt left in the flash, and
// the CSRF token the protecting middleware issued for this request. No action
// issues a token or passes the name, so the service is all this needs.
type CommentController struct {
	Controller

	svc *services.CommentService
}

// NewCommentController returns the controller. bootstrap builds it and hands it to
// the routes.
func NewCommentController(svc *services.CommentService) *CommentController {
	return &CommentController{svc: svc}
}

// Compile-time proof of the seven actions fhttp.Router.Resource looks for. It
// registers the ones the controller implements and nothing else, so a route that
// exists is a route that answers -- and a renamed method fails the build here
// rather than answering 404 in production.
var (
	_ fhttp.Indexer   = (*CommentController)(nil)
	_ fhttp.Creator   = (*CommentController)(nil)
	_ fhttp.Storer    = (*CommentController)(nil)
	_ fhttp.Shower    = (*CommentController)(nil)
	_ fhttp.Editor    = (*CommentController)(nil)
	_ fhttp.Updater   = (*CommentController)(nil)
	_ fhttp.Destroyer = (*CommentController)(nil)
)

// Index renders the listing, one page at a time.
func (c *CommentController) Index(ctx *hhttp.Context) error {
	// The note the person navigated to. The service loads it under
	// its own policy and lists by the one it loaded.
	parent := ctx.Param("note")
	who, _ := ctx.User()
	found, page, err := c.svc.List(ctx.Ctx(), who, parent, pagination.ResolveCurrentPage(ctx.Request.URL, ""))
	if err != nil {
		return err
	}

	rows := make([]views.CommentRow, 0, len(found))
	for _, co := range found {
		rows = append(rows, c.row(ctx, co))
	}
	return ctx.View("comments.index", views.CommentsIndexData{
		Page:     view.New(ctx, "Comments"),
		Comments: rows,
		NewURL:   ctx.URL("notes.comments.create", parent),
		NextURL:  page.SetPath(ctx.URL("notes.comments.index", parent)).NextPageURL(),
	})
}

// Show renders one record.
func (c *CommentController) Show(ctx *hhttp.Context) error {
	who, _ := ctx.User()
	found, err := c.svc.Get(ctx.Ctx(), who, ctx.Param("comment"))
	if err != nil {
		return err
	}

	return ctx.View("comments.show", views.CommentsShowData{
		Page:      view.New(ctx, "Comment"),
		Comment:   c.row(ctx, found),
		IndexURL:  ctx.URL("notes.comments.index", found.NoteID),
		EditURL:   ctx.URL("notes.comments.edit", found.ID),
		DeleteURL: ctx.URL("notes.comments.destroy", found.ID),
	})
}

// Create renders the empty form, or the rejected one: the page carries what
// was typed and the messages, from the flash the router left.
func (c *CommentController) Create(ctx *hhttp.Context) error {
	parent := ctx.Param("note")
	return ctx.View("comments.create", views.CommentsCreateData{
		Page:     view.New(ctx, "New comment"),
		IndexURL: ctx.URL("notes.comments.index", parent),
		StoreURL: ctx.URL("notes.comments.store", parent),
	})
}

// Store takes the submitted form.
func (c *CommentController) Store(ctx *hhttp.Context) error {
	var in requests.CommentRequest
	if err := ctx.Bind(&in); err != nil {
		return err
	}
	who, _ := ctx.User()
	created, err := c.svc.Create(ctx.Ctx(), who, ctx.Param("note"), in)
	if err != nil {
		return err
	}
	return ctx.RedirectRoute("notes.comments.show", created.ID)
}

// Edit renders the form filled in with the stored record.
func (c *CommentController) Edit(ctx *hhttp.Context) error {
	who, _ := ctx.User()
	found, err := c.svc.Get(ctx.Ctx(), who, ctx.Param("comment"))
	if err != nil {
		return err
	}

	return ctx.View("comments.edit", views.CommentsEditData{
		Page:      view.New(ctx, "Edit comment"),
		Form:      c.row(ctx, found),
		ShowURL:   ctx.URL("notes.comments.show", found.ID),
		UpdateURL: ctx.URL("notes.comments.update", found.ID),
	})
}

// Update writes the submitted form onto the stored record.
func (c *CommentController) Update(ctx *hhttp.Context) error {
	var in requests.CommentRequest
	if err := ctx.Bind(&in); err != nil {
		return err
	}
	who, _ := ctx.User()
	updated, err := c.svc.Update(ctx.Ctx(), who, ctx.Param("comment"), in)
	if err != nil {
		return err
	}
	return ctx.RedirectRoute("notes.comments.show", updated.ID)
}

// Destroy removes the record.
func (c *CommentController) Destroy(ctx *hhttp.Context) error {
	who, _ := ctx.User()
	// Read first: the listing to go back to is the note's, and the
	// path of a member route does not carry it.
	found, err := c.svc.Get(ctx.Ctx(), who, ctx.Param("comment"))
	if err != nil {
		return err
	}
	if err := c.svc.Delete(ctx.Ctx(), who, found.ID); err != nil {
		return err
	}
	return ctx.RedirectRoute("notes.comments.index", found.NoteID)
}

// row turns the entity into what the markup renders: the text a cell shows and
// an input of the edit form starts at.
//
// Formatting happens here rather than in the view: a view that formats a
// time.Time would need the time package, and what a date looks like on screen is
// a decision about presentation, which is this side of the line.
//
// The address is settled here too, for the same reason and one more: the view
// has no route table, so a link written there could only be a literal. This
// takes the context so it can ask for the route by name.
func (c *CommentController) row(ctx *hhttp.Context, co *models.Comment) views.CommentRow {
	return views.CommentRow{
		ID:      co.ID,
		URL:     ctx.URL("notes.comments.show", co.ID),
		Body:    co.Body,
		Created: co.CreatedAt.Format("2006-01-02 15:04"),
	}
}

// arandu:begin custom
// Actions beyond the seven go here, and survive regeneration. Register them in
// the custom block of routes/web.go.
// arandu:end custom
