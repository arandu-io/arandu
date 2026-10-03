// Rendered by aru model:build for models.Note. Everything outside the custom block is rewritten on the next build.

package factories

import (
	"context"

	"github.com/arandu-io/hesape/auth"
	"github.com/arandu-io/hesape/database/model"
	factory "github.com/arandu-io/hesape/database/model/factories"
	"github.com/arandu-io/hesape/faker"

	models "github.com/arandu-io/arandu/app/Models"
)

// NoteFactory builds rows of notes, for tests and for seeding.
//
// Make builds rows and stores nothing. Create stores them, and takes the Grant
// every write takes: the tenant comes off it, and a factory is no way around the
// policy that guards the table.
//
// Every method returns a new factory, so a factory kept in a variable is never
// changed by a caller that adds a state to it.
type NoteFactory struct{ f *factory.Factory }

// Notes returns the factory of notes over db, with defineNote as
// its default state.
//
//	rows, err := factories.Notes(db).Count(10).Create(ctx, g)
//	one, err := factories.Notes(db).State(func(n *models.Note) { ... }).MakeOne()
//
// The values come from a seeded faker -- the same rows on every run, so a
// failure reproduces -- and Seed asks for others. The key is left empty: the
// model draws a fresh one for every row it stores, so two batches never share
// one, and Make builds rows that have none yet.
func Notes(db model.DB) *NoteFactory {
	return &NoteFactory{f: factory.New(models.Notes(db).Base(), func(f faker.Faker, row model.Entity) {
		*row.(*models.Note) = defineNote(f)
	})}
}

// Count returns a factory that makes n rows.
func (x *NoteFactory) Count(n int) *NoteFactory { return &NoteFactory{f: x.f.Count(n)} }

// Seed returns a factory whose values start from seed.
func (x *NoteFactory) Seed(seed int64) *NoteFactory { return &NoteFactory{f: x.f.Seed(seed)} }

// State returns a factory that applies fn to every row after the default state.
func (x *NoteFactory) State(fn func(*models.Note)) *NoteFactory {
	return &NoteFactory{f: x.f.State(func(row model.Entity) { fn(row.(*models.Note)) })}
}

// Sequence returns a factory that cycles through states, one per row.
func (x *NoteFactory) Sequence(states ...func(*models.Note)) *NoteFactory {
	adapted := make([]func(model.Entity), len(states))
	for i, state := range states {
		adapted[i] = func(row model.Entity) { state(row.(*models.Note)) }
	}
	return &NoteFactory{f: x.f.Sequence(adapted...)}
}

// AfterMaking returns a factory that runs fn on each row once it is built.
func (x *NoteFactory) AfterMaking(fn func(*models.Note)) *NoteFactory {
	return &NoteFactory{f: x.f.AfterMaking(func(row model.Entity) { fn(row.(*models.Note)) })}
}

// AfterCreating returns a factory that runs fn on each row once it is stored.
func (x *NoteFactory) AfterCreating(fn func(context.Context, auth.Grant, *models.Note) error) *NoteFactory {
	return &NoteFactory{f: x.f.AfterCreating(func(ctx context.Context, g auth.Grant, row model.Entity) error {
		return fn(ctx, g, row.(*models.Note))
	})}
}

// Make returns the rows without storing any of them.
func (x *NoteFactory) Make() (models.NoteCollection, error) {
	rows, err := x.f.Make()
	return x.collection(rows), err
}

// MakeOne returns one row without storing it, whatever Count says.
func (x *NoteFactory) MakeOne() (*models.Note, error) {
	e, err := x.f.MakeOne()
	row, _ := e.(*models.Note)
	return row, err
}

// Create stores the rows and returns them.
func (x *NoteFactory) Create(ctx context.Context, g auth.Grant) (models.NoteCollection, error) {
	rows, err := x.f.Create(ctx, g)
	return x.collection(rows), err
}

// CreateOne stores one row and returns it, whatever Count says.
func (x *NoteFactory) CreateOne(ctx context.Context, g auth.Grant) (*models.Note, error) {
	e, err := x.f.CreateOne(ctx, g)
	row, _ := e.(*models.Note)
	return row, err
}

// collection converts the rows the core factory returns. It is a method of
// its own so that Create, which spends a Grant, calls nothing but the core.
func (x *NoteFactory) collection(rows model.Rows) models.NoteCollection {
	return models.NoteCollectionOf(rows)
}

// arandu:begin custom
// Example resource. Remove with the list under "The example resource" in README.md.

// defineNote is the default state: every row the factory makes starts here,
// and a State changes the part a test cares about. Named states go here too.
func defineNote(f faker.Faker) models.Note {
	return models.Note{
		Title:  f.Sentence(4),
		Body:   f.Paragraph(2),
		Pinned: f.Bool(),
	}
}

// WrittenBy is the state of a note whose author is the account with this id.
// The definition leaves the author empty: an owner is a row that has to exist
// first, and a factory that drew a random id would make notes nobody wrote.
func WrittenBy(userID string) func(*models.Note) {
	return func(n *models.Note) { n.UserID = userID }
}

// arandu:end custom
