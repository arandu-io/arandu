// Example resource. Remove with the list under "The example resource" in README.md.

package requests

import "github.com/arandu-io/hesape/validation"

// NoteRequest is the input contract of creation and update, which take the
// same fields. ctx.Bind fills it through the form tags, and only those: there is
// no mass assignment, so a key the client sends and this struct does not
// declare goes nowhere.
type NoteRequest struct {
	Title  string `form:"title"`
	Body   string `form:"body"`
	Pinned bool   `form:"pinned"`
}

// Validate reports the errors per field.
func (r NoteRequest) Validate() validation.Errors {
	e := validation.Errors{}
	validation.Required(e, "title", r.Title)
	validation.MaxLen(e, "title", r.Title, 255)
	validation.MaxLen(e, "body", r.Body, 5000)

	// arandu:begin custom
	// Domain rules go here: ranges, formats, cross-field checks.
	// arandu:end custom

	return e
}

// Compile-time proof that the request honors the validation contract.
var _ validation.Validatable = NoteRequest{}
