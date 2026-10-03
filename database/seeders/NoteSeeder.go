// Example resource. Remove with the list under "The example resource" in README.md.

package seeders

import (
	"context"

	"github.com/arandu-io/framework/security"

	models "github.com/arandu-io/arandu/app/Models"
	policies "github.com/arandu-io/arandu/app/Policies"
	factories "github.com/arandu-io/arandu/database/factories"
)

// NoteSeeder seeds the example notes, written by two accounts the user factory
// makes. Those accounts have no password anybody can type, so seeding creates
// nobody to sign in as: sign in with an account of your own, made with
// UserSeeder, and the notes of the tenant are there to read -- and refused to
// change, because you did not write them.
//
// A seeder writes, and a write needs a security.Grant. This is one of the few
// places security.SystemGrant is legitimate -- there is no request and no
// actor behind it -- and `aru doctor` allows it here
// because of the directory this file is in, not because of what the function
// is called. Anywhere else it is a warning that has to be answered with
// //arandu:system-grant <reason>.
//
// Run must be safe to run twice. A seeder that fails on the second run cannot
// be part of a deploy, and this one runs on every deploy that runs the first.
type NoteSeeder struct{}

// Name is how the seeder is addressed on the command line.
func (NoteSeeder) Name() string { return "NoteSeeder" }

// Run creates the rows through the factory, in d.Tenant -- never a tenant this
// file picked: a seeder that chooses its own seeds rows nobody can reach. A
// table that already has rows is left as it is, which is what makes a second
// run safe.
func (NoteSeeder) Run(ctx context.Context, d Deps) error {
	seeded, err := models.Notes(d.DB).Exists(ctx, security.SystemGrant(policies.NoteList, d.Tenant))
	if err != nil || seeded {
		return err
	}
	// arandu:begin custom
	// Rows the factory's defaults do not describe go here: a fixed record, a
	// state applied to some of them.
	authors, err := factories.Users(d.DB).Count(2).Create(ctx, security.SystemGrant(policies.ActionUserCreate, d.Tenant))
	if err != nil {
		return err
	}
	byAuthor := factories.Notes(d.DB).Count(6).
		Sequence(factories.WrittenBy(authors[0].ID), factories.WrittenBy(authors[1].ID))
	// arandu:end custom

	if _, err := byAuthor.Create(ctx, security.SystemGrant(policies.NoteCreate, d.Tenant)); err != nil {
		return err
	}
	return nil
}

// compile-time proof that the seeder honors the contract. A seeder that drifts
// from the interface fails the build rather than failing when someone runs it.
var _ Seeder = NoteSeeder{}
