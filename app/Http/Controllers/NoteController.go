// Example resource. Remove with the list under "The example resource" in README.md.

package controllers

import (
	"net/http"

	fhttp "github.com/arandu-io/framework/http"
	hhttp "github.com/arandu-io/hesape/http"
	"github.com/arandu-io/hesape/pagination"
	"github.com/arandu-io/hesape/view"

	requests "github.com/arandu-io/arandu/app/Http/Requests"
	models "github.com/arandu-io/arandu/app/Models"
	services "github.com/arandu-io/arandu/app/Services"
	views "github.com/arandu-io/arandu/storage/framework/views/notes"
)

// NoteController answers the seven routes of the notes resource.
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
// view.New is the whole of a page's chrome: the title, what a rejected attempt
// left in the flash, and the CSRF token the protecting middleware issued for this
// request. No action issues a token, so the service is all this needs.
type NoteController struct {
	Controller

	svc *services.NoteService
}

// NewNoteController returns the controller. bootstrap builds it and hands it to
// the routes.
func NewNoteController(svc *services.NoteService) *NoteController {
	return &NoteController{svc: svc}
}

// Compile-time proof of the seven actions fhttp.Router.Resource looks for. It
// registers the ones the controller implements and nothing else, so a route that
// exists is a route that answers -- and a renamed method fails the build here
// rather than answering 404 in production.
var (
	_ fhttp.Indexer   = (*NoteController)(nil)
	_ fhttp.Creator   = (*NoteController)(nil)
	_ fhttp.Storer    = (*NoteController)(nil)
	_ fhttp.Shower    = (*NoteController)(nil)
	_ fhttp.Editor    = (*NoteController)(nil)
	_ fhttp.Updater   = (*NoteController)(nil)
	_ fhttp.Destroyer = (*NoteController)(nil)
)

// Index renders the listing, one page at a time.
//
// It answers the whole page, except to the request the table's next-page link
// makes: htmx names the element it will replace in HX-Target, and that request
// gets the table alone, drawn from the same partial the page includes. A direct
// visit, a boosted link and a history restore name no such target, so each of
// them gets the page, layout and title included, and a reload shows what was
// on screen.
//
// One address, two representations chosen by request headers, so the answer
// says which headers: a cache that did not know would hand the table to a
// navigation, or the page to the hole the table was in.
func (c *NoteController) Index(ctx *hhttp.Context) error {
	who, _ := ctx.User()
	found, page, err := c.svc.List(ctx.Ctx(), who, pagination.ResolveCurrentPage(ctx.Request.URL, ""))
	if err != nil {
		return err
	}

	rows := make([]views.NoteRow, 0, len(found))
	for _, n := range found {
		rows = append(rows, c.row(ctx, n))
	}
	data := views.NotesIndexData{
		Page:    view.New(ctx, "Notes"),
		Notes:   rows,
		NewURL:  ctx.URL("notes.create"),
		NextURL: page.SetPath(ctx.URL("notes.index")).NextPageURL(),
	}

	ctx.Response.Header().Add("Vary", "HX-Request, HX-Target")
	if ctx.IsHTMX() && ctx.Header("HX-Target") == views.NotesTableID {
		return ctx.Fragment(http.StatusOK, "partials.notes_table", data)
	}
	return ctx.View("notes.index", data)
}

// Show renders one record.
func (c *NoteController) Show(ctx *hhttp.Context) error {
	who, _ := ctx.User()
	found, err := c.svc.Get(ctx.Ctx(), who, ctx.Param("id"))
	if err != nil {
		return err
	}

	return ctx.View("notes.show", views.NotesShowData{
		Page:      view.New(ctx, "Note"),
		Note:      c.row(ctx, found),
		IndexURL:  ctx.URL("notes.index"),
		EditURL:   ctx.URL("notes.edit", found.ID),
		DeleteURL: ctx.URL("notes.destroy", found.ID),
		// The resource nested under this one, by its route name and the note.
		CommentsURL: ctx.URL("notes.comments.index", found.ID),
	})
}

// Create renders the empty form, or the rejected one: the page carries what
// was typed and the messages, from the flash the router left.
func (c *NoteController) Create(ctx *hhttp.Context) error {
	return ctx.View("notes.create", views.NotesCreateData{
		Page:     view.New(ctx, "New note"),
		IndexURL: ctx.URL("notes.index"),
		StoreURL: ctx.URL("notes.store"),
	})
}

// Store takes the submitted form.
func (c *NoteController) Store(ctx *hhttp.Context) error {
	var in requests.NoteRequest
	if err := ctx.Bind(&in); err != nil {
		return err
	}
	who, _ := ctx.User()
	created, err := c.svc.Create(ctx.Ctx(), who, in)
	if err != nil {
		return err
	}
	return ctx.RedirectRoute("notes.show", created.ID)
}

// Edit renders the form filled in with the stored record.
func (c *NoteController) Edit(ctx *hhttp.Context) error {
	who, _ := ctx.User()
	found, err := c.svc.Get(ctx.Ctx(), who, ctx.Param("id"))
	if err != nil {
		return err
	}

	return ctx.View("notes.edit", views.NotesEditData{
		Page:      view.New(ctx, "Edit note"),
		Form:      c.row(ctx, found),
		ShowURL:   ctx.URL("notes.show", found.ID),
		UpdateURL: ctx.URL("notes.update", found.ID),
	})
}

// Update writes the submitted form onto the stored record.
func (c *NoteController) Update(ctx *hhttp.Context) error {
	var in requests.NoteRequest
	if err := ctx.Bind(&in); err != nil {
		return err
	}
	who, _ := ctx.User()
	updated, err := c.svc.Update(ctx.Ctx(), who, ctx.Param("id"), in)
	if err != nil {
		return err
	}
	return ctx.RedirectRoute("notes.show", updated.ID)
}

// Destroy removes the record.
func (c *NoteController) Destroy(ctx *hhttp.Context) error {
	who, _ := ctx.User()
	if err := c.svc.Delete(ctx.Ctx(), who, ctx.Param("id")); err != nil {
		return err
	}
	return ctx.RedirectRoute("notes.index")
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
func (c *NoteController) row(ctx *hhttp.Context, n *models.Note) views.NoteRow {
	return views.NoteRow{
		ID:      n.ID,
		URL:     ctx.URL("notes.show", n.ID),
		Title:   n.Title,
		Body:    n.Body,
		Pinned:  n.Pinned,
		Created: n.CreatedAt.Format("2006-01-02 15:04"),
	}
}

// arandu:begin custom
// Actions beyond the seven go here, and survive regeneration. Register them in
// the custom block of routes/web.go.
// arandu:end custom
