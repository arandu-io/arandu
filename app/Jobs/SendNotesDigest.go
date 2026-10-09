// Example resource. Remove with the list under "The example resource" in README.md.

package jobs

import (
	"context"
	"time"

	"github.com/arandu-io/hesape/auth"
	hqueue "github.com/arandu-io/hesape/queue"
	hjobs "github.com/arandu-io/hesape/queue/jobs"

	services "github.com/arandu-io/arandu/app/Services"
)

// SendNotesDigestName routes the job to its handler.
//
// A constant rather than a literal at both ends: a typo in the push enqueues
// work nothing drains, and the worker parks it with "no handler registered"
// instead of failing where the mistake is.
const SendNotesDigestName = "notes.digest"

// SendNotesDigest is what the job carries.
//
// These are the arguments the job was created with, as a struct. Keep it to facts and
// ids: a payload that says "look it up" is a payload that reads a row which has
// already changed by the time the worker gets there.
type SendNotesDigest struct {
	// Since is the start of the window: the digest holds the notes published
	// from then on.
	Since time.Time `json:"since"`
}

// DispatchSendNotesDigest enqueues the job.
//
// The Grant is explicit, and hjobs.New is the only constructor, so every job in
// the system carries the tenant, an id and the Grant that authorized it -- there
// is no shape of Job that skipped any of the three.
//
// The caller holds the queue: a listener, once the write that stored its event
// has committed, or a task in a provider's Schedule(). A service the handler
// calls never dispatches it -- this package imports app/Services as soon as a
// handler takes a service, and a service importing it back is an import cycle.
// The write that leads to the job stores an event in the outbox, in its own
// transaction, and the listener the relay hands it to dispatches this.
//
// The digest is the second case: the notes.digest task in
// AppServiceProvider.Schedule dispatches it every night, and NoteService, which
// the handler calls, does not.
func DispatchSendNotesDigest(ctx context.Context, q hqueue.Queue, g auth.Grant, in SendNotesDigest) error {
	j, err := hjobs.New(g, hjobs.DefaultQueue, SendNotesDigestName, in)
	if err != nil {
		return err
	}
	return q.Push(ctx, g, j)
}

// SendNotesDigestHandler is what `aru queue:work` runs.
//
// Its collaborators arrive through the constructor -- there is no container, and
// a handler that built its own service would be a handler no test can pin.
type SendNotesDigestHandler struct {
	// arandu:begin custom
	// The services this job needs. Add the fields here and the parameters to
	// NewSendNotesDigestHandler below; bootstrap already built them.

	// notes reads the published notes and hands them to the newsletter.
	notes *services.NoteService
	// arandu:end custom
}

// NewSendNotesDigestHandler wires the handler.
func NewSendNotesDigestHandler(notes *services.NoteService) *SendNotesDigestHandler {
	return &SendNotesDigestHandler{notes: notes}
}

// Compile-time proof that the worker can register it.
var _ hqueue.Handler = (*SendNotesDigestHandler)(nil)

// Handle does the work.
//
// The Grant is rebuilt by the worker from the row -- the action and the tenant
// the push was authorized under, and not one permission more -- so this reaches
// repositories on exactly the same authorized path a request does.
//
// The job arrives as a pointer because on this contract a job settles itself:
// releasing it, or parking it, is a call on j rather than on the queue.
//
// Delivery is at-least-once. This body has to tolerate running twice: the
// process can die between doing the work and acknowledging it, and no queue
// anywhere solves that. j.UUID is stable across retries and is the key to
// deduplicate on.
func (h *SendNotesDigestHandler) Handle(ctx context.Context, g auth.Grant, j *hjobs.Job) error {
	var in SendNotesDigest
	if err := j.Decode(&in); err != nil {
		return err
	}

	// arandu:begin custom
	// The service does the work, under the Grant the worker rebuilt, and the
	// job's UUID is the digest's key: a retry sends the same digest, which the
	// newsletter sends once.
	return h.notes.SendDigest(ctx, g, in.Since, j.UUID)
	// arandu:end custom
}

var _ = time.Time{}
