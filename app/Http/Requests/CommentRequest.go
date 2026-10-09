// Example resource. Remove with the list under "The example resource" in README.md.

package requests

import "github.com/arandu-io/hesape/validation"

// CommentRequest is the input contract of creation and update, which take the
// same fields. ctx.Bind fills it through the form tags, and only those: there is
// no mass assignment, so a key the client sends and this struct does not
// declare goes nowhere.
type CommentRequest struct {
	Body string `form:"body"`
}

// Validate reports the errors per field.
func (r CommentRequest) Validate() validation.Errors {
	e := validation.Errors{}
	validation.Required(e, "body", r.Body)
	validation.MaxLen(e, "body", r.Body, 5000)

	// arandu:begin custom
	// Domain rules go here: ranges, formats, cross-field checks.
	// arandu:end custom

	return e
}

// Compile-time proof that the request honors the validation contract.
var _ validation.Validatable = CommentRequest{}
