package listeners

import (
	"context"

	"github.com/arandu-io/hesape/events"
)

// Each is the listeners the relay hands every committed event to, in the
// order they are listed.
//
// The relay takes one Publisher, and an application has a listener per event:
// this is the list, written in bootstrap/app.go where it can be read. There is
// no registry and nothing subscribes itself -- a listener not named in the list
// does not run, and that is visible in the wiring.
//
// The first listener that fails stops the event, and the relay hands it over
// again later, to every listener: delivery is at-least-once, so each of them
// is already written to be safe to run twice.
type Each []events.Publisher

// Compile-time proof that the relay takes the list as its one Publisher.
var _ events.Publisher = Each(nil)

// Publish hands the event to each listener in turn.
func (l Each) Publish(ctx context.Context, e events.Stored) error {
	for _, listener := range l {
		if err := listener.Publish(ctx, e); err != nil {
			return err
		}
	}
	return nil
}
