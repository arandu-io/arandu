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

// Rules declares what each field may be, by the name of its form input. ctx.Bind
// runs them over the raw input before it converts anything, and a request that
// fails never reaches the service.
func (NoteRequest) Rules() validation.Rules {
	return validation.Rules{
		"title":  "required|string|max:255",
		"body":   "string|max:5000",
		"pinned": "boolean",
	}
}

// Compile-time proof that the request declares its rules.
var _ validation.RequestRules = NoteRequest{}

// arandu:begin custom
// What the fields may be together, or against the data, is a Validate method of
// this request, and the service calls it before it authorizes:
//
//	func (r NoteRequest) Validate() validation.Errors { ... }
// arandu:end custom
