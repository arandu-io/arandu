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

// Validate reports the field errors. NewsletterEventService calls it.
func (r NewsletterEventRequest) Validate() validation.Errors {
	err := validation.Errors{}
	validation.Required(err, "event", r.Event)
	validation.MaxLen(err, "event", r.Event, 64)
	validation.Required(err, "digest", r.Digest)
	validation.MaxLen(err, "digest", r.Digest, 255)
	return err
}
