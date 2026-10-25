// Example resource. Remove with the list under "The example resource" in README.md.

package requests

import "github.com/arandu-io/hesape/validation"

// NewsletterEventRequest is what the newsletter provider posts to
// /webhooks/newsletter about a digest, once its signature was verified. ctx.Bind
// fills it from the JSON body through the form tags.
type NewsletterEventRequest struct {
	// Event is what happened, such as "digest.delivered".
	Event string `form:"event"`
	// Digest is the key the digest was sent under.
	Digest string `form:"digest"`
}

// Rules declares what each field may be, by the name of its form input.
// ctx.Bind runs them over the raw input before it converts anything.
func (NewsletterEventRequest) Rules() validation.Rules {
	return validation.Rules{
		"event":  "required|string|max:64",
		"digest": "required|string|max:255",
	}
}

// Compile-time proof that the request declares its rules.
var _ validation.RequestRules = NewsletterEventRequest{}
