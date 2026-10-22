// Example resource. Remove with the list under "The example resource" in README.md.

package models

import (
	"log/slog"
	"time"

	"github.com/arandu-io/hesape/database/model"
)

// NewsletterDelivery is one row of newsletter_deliveries: a delivery the
// newsletter provider posted to /webhooks/newsletter, kept under the id the
// provider gave it so the same delivery is never stored twice.
//
// The table is unique on the tenant and the delivery id. The signature and the
// timestamp keep a captured delivery out once the tolerance has passed; inside
// it, this row is what turns the same delivery sent again into nothing.
//
// It embeds the model, so a row returned by a query carries the connection and
// can be saved again. Build a new row with NewsletterDeliveries(db).New(): a struct
// literal has no connection and its write methods return model.ErrUnwired.
//
// NewsletterDeliveries, NewsletterDeliveryQuery and NewsletterDeliveryCollection
// are generated beside this file, in NewsletterDeliveryQuery.go, by aru model:build.
type NewsletterDelivery struct {
	model.Model

	ID         string    `db:"id"`
	TenantID   string    `db:"tenant_id"`
	DeliveryID string    `db:"delivery_id"`
	CreatedAt  time.Time `db:"created_at"`
	UpdatedAt  time.Time `db:"updated_at"`
}

// newsletterDeliveryTable is the table NewsletterDelivery is a row of.
//
// UniqueIDs makes the primary key text the model fills on insert.
// The tenant scope is left at its tenant_id default.
var newsletterDeliveryTable = model.NewTable(model.TableSpec{
	Name:      "newsletter_deliveries",
	New:       func() model.Entity { return new(NewsletterDelivery) },
	UniqueIDs: true,
	// arandu:begin custom
	// Hidden, PerPage, Scopes, Events and the rest of model.TableSpec go here.
	// arandu:end custom
})

// LogValue implements slog.LogValuer, so passing the whole entity to a log call
// records the identifiers and nothing else. Add any sensitive field to the
// custom block below and it stays out of logs, dumps and the debug page.
func (n NewsletterDelivery) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("id", n.ID),
		slog.String("tenant", n.TenantID),
	)
}

// arandu:begin custom
// Local scopes are methods on *NewsletterDeliveryQuery, relations are registered
// on newsletterDeliveryTable in an init function, and MarshalJSON, computed fields
// and anything else about this entity go here too.
//
// So do the rules of the entity itself: an invariant, a derived value, a
// transition that changes only this row's fields. They are pure -- no
// database, no network, no Grant, and no clock read here: the time is an
// argument -- and the service calls them between the policy and the save.
//
//	func (n *NewsletterDelivery) Approve(at time.Time) error {
//		if !n.ApprovedAt.IsZero() {
//			return errors.New("this newsletter delivery is already approved")
//		}
//		n.ApprovedAt = at
//		return nil
//	}
// arandu:end custom
