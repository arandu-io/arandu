// Example resource. Remove with the list under "The example resource" in README.md.

package factories

import (
	"github.com/arandu-io/framework/data"
	factory "github.com/arandu-io/hesape/database/model/factories"
	"github.com/arandu-io/hesape/faker"

	models "github.com/arandu-io/arandu/app/Models"
)

// NoteFactory returns the factory of notes over db.
//
//	rows, err := factories.NoteFactory(db).Count(10).Create(ctx, g)
//	one, err := factories.NoteFactory(db).State(func(n *models.Note) { ... }).MakeOne()
//
// Make builds rows and stores nothing. Create stores them, and takes the Grant
// every write takes: the tenant comes off it, and a factory is no way around the
// policy that guards the table.
//
// The values come from a seeded faker -- the same rows on every run, so a
// failure reproduces -- and Seed asks for others. The key is left empty: the
// model draws a fresh one for every row it stores, so two batches never share
// one, and Make builds rows that have none yet.
func NoteFactory(db *data.DB) *factory.Factory[models.Note] {
	return factory.For(models.Notes(db), func(f faker.Faker) models.Note {
		return models.Note{
			Title:  f.Sentence(4),
			Body:   f.Paragraph(2),
			Pinned: f.Bool(),
		}
	})
}

// arandu:begin custom
// Named states go here, and survive regeneration: a factory with one thing
// said about it, built on the one above.

// WrittenBy is the state of a note whose author is the account with this id.
// The definition leaves the author empty: an owner is a row that has to exist
// first, and a factory that drew a random id would make notes nobody wrote.
func WrittenBy(userID string) func(*models.Note) {
	return func(n *models.Note) { n.UserID = userID }
}

// arandu:end custom
