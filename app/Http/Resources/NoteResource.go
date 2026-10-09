// Example resource. Remove with the list under "The example resource" in README.md.

package resources

import (
	hhttp "github.com/arandu-io/hesape/http"

	models "github.com/arandu-io/arandu/app/Models"
)

// NoteResource is what one Note answers with as JSON: the fields ToArray
// lists, and nothing else.
//
// It exists because an encoder handed the entity answers with whatever fields
// the entity has, including the ones somebody adds to it later without reading
// this file: a password hash, an internal note, the tenant the row belongs to.
// A resource answers with a list somebody wrote. ctx.JSON takes one and no
// other value, and so does mcp.JSON, so a controller and a tool over the same
// service answer the same document.
type NoteResource struct {
	record *models.Note
}

// NewNoteResource wraps one Note.
func NewNoteResource(record *models.Note) NoteResource {
	return NoteResource{record: record}
}

// Compile-time proof that ctx.JSON takes it.
var _ hhttp.JsonResource = NoteResource{}

// ToArray returns the fields that may leave, by name. The tenant is not among
// them, and a field added to the model is not either until somebody adds it
// here: delete a line for a field this answer should not carry.
func (r NoteResource) ToArray() map[string]any {
	return map[string]any{
		"id":           r.record.ID,
		"user_id":      r.record.UserID,
		"title":        r.record.Title,
		"body":         r.record.Body,
		"pinned":       r.record.Pinned,
		"published_at": r.record.PublishedAt,
		"created_at":   r.record.CreatedAt,
		"updated_at":   r.record.UpdatedAt,
		// arandu:begin custom
		// A computed field, or one that leaves only for some readers, goes
		// here; a value that reports itself missing is left out of the answer.
		// arandu:end custom
	}
}

// With returns what goes beside "data" at the top of the answer: metadata about
// the answer rather than about the Note. There is none yet.
func (r NoteResource) With() map[string]any { return nil }

// NoteCollection is what a list of notes answers with: each row through
// NoteResource, under "notes".
type NoteCollection struct {
	rows models.NoteCollection
	// next is the address of the following page, empty on the last one.
	next string
}

// NewNoteCollection wraps a page of rows, as the service's List returns them.
func NewNoteCollection(rows models.NoteCollection) NoteCollection {
	return NoteCollection{rows: rows}
}

// Compile-time proof that ctx.JSON takes it.
var _ hhttp.JsonResource = NoteCollection{}

// ToArray returns every row through NoteResource, so a list answers with the same
// fields one record does.
func (c NoteCollection) ToArray() map[string]any {
	items := make([]map[string]any, 0, len(c.rows))
	for _, row := range c.rows {
		items = append(items, NewNoteResource(row).ToArray())
	}
	return map[string]any{"notes": items}
}

// With returns what goes beside "data": the link to the next page, once the
// controller hands it over with Next, and nothing on the last page.
func (c NoteCollection) With() map[string]any {
	if c.next == "" {
		return nil
	}
	return map[string]any{"links": map[string]any{"next": c.next}}
}

// arandu:begin custom

// Next returns the collection with the address of the following page, which
// the controller builds from the route name: a resource has no route table.
func (c NoteCollection) Next(url string) NoteCollection {
	c.next = url
	return c
}

// arandu:end custom
