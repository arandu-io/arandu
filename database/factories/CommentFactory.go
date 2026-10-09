// Rendered by aru model:build for models.Comment. Everything outside the custom block is rewritten on the next build.

package factories

import (
	"context"

	"github.com/arandu-io/hesape/auth"
	"github.com/arandu-io/hesape/database/model"
	factory "github.com/arandu-io/hesape/database/model/factories"
	"github.com/arandu-io/hesape/faker"

	models "github.com/arandu-io/arandu/app/Models"
)

// CommentFactory builds rows of comments, for tests and for seeding.
//
// Make builds rows and stores nothing. Create stores them, and takes the Grant
// every write takes: the tenant comes off it, and a factory is no way around the
// policy that guards the table.
//
// Every method returns a new factory, so a factory kept in a variable is never
// changed by a caller that adds a state to it.
type CommentFactory struct{ f *factory.Factory }

// Comments returns the factory of comments over db, with defineComment as
// its default state.
//
//	rows, err := factories.Comments(db).Count(10).Create(ctx, g)
//	one, err := factories.Comments(db).State(func(co *models.Comment) { ... }).MakeOne()
//
// The values come from a seeded faker -- the same rows on every run, so a
// failure reproduces -- and Seed asks for others. The key is left empty: the
// model draws a fresh one for every row it stores, so two batches never share
// one, and Make builds rows that have none yet.
func Comments(db model.DB) *CommentFactory {
	return &CommentFactory{f: factory.New(models.Comments(db).Base(), func(f faker.Faker, row model.Entity) {
		*row.(*models.Comment) = defineComment(f)
	})}
}

// Count returns a factory that makes n rows.
func (x *CommentFactory) Count(n int) *CommentFactory { return &CommentFactory{f: x.f.Count(n)} }

// Seed returns a factory whose values start from seed.
func (x *CommentFactory) Seed(seed int64) *CommentFactory { return &CommentFactory{f: x.f.Seed(seed)} }

// State returns a factory that applies fn to every row after the default state.
func (x *CommentFactory) State(fn func(*models.Comment)) *CommentFactory {
	return &CommentFactory{f: x.f.State(func(row model.Entity) { fn(row.(*models.Comment)) })}
}

// Sequence returns a factory that cycles through states, one per row.
func (x *CommentFactory) Sequence(states ...func(*models.Comment)) *CommentFactory {
	adapted := make([]func(model.Entity), len(states))
	for i, state := range states {
		adapted[i] = func(row model.Entity) { state(row.(*models.Comment)) }
	}
	return &CommentFactory{f: x.f.Sequence(adapted...)}
}

// AfterMaking returns a factory that runs fn on each row once it is built.
func (x *CommentFactory) AfterMaking(fn func(*models.Comment)) *CommentFactory {
	return &CommentFactory{f: x.f.AfterMaking(func(row model.Entity) { fn(row.(*models.Comment)) })}
}

// AfterCreating returns a factory that runs fn on each row once it is stored.
func (x *CommentFactory) AfterCreating(fn func(context.Context, auth.Grant, *models.Comment) error) *CommentFactory {
	return &CommentFactory{f: x.f.AfterCreating(func(ctx context.Context, g auth.Grant, row model.Entity) error {
		return fn(ctx, g, row.(*models.Comment))
	})}
}

// Make returns the rows without storing any of them.
func (x *CommentFactory) Make() (models.CommentCollection, error) {
	rows, err := x.f.Make()
	return x.collection(rows), err
}

// MakeOne returns one row without storing it, whatever Count says.
func (x *CommentFactory) MakeOne() (*models.Comment, error) {
	e, err := x.f.MakeOne()
	row, _ := e.(*models.Comment)
	return row, err
}

// Create stores the rows and returns them.
func (x *CommentFactory) Create(ctx context.Context, g auth.Grant) (models.CommentCollection, error) {
	rows, err := x.f.Create(ctx, g)
	return x.collection(rows), err
}

// CreateOne stores one row and returns it, whatever Count says.
func (x *CommentFactory) CreateOne(ctx context.Context, g auth.Grant) (*models.Comment, error) {
	e, err := x.f.CreateOne(ctx, g)
	row, _ := e.(*models.Comment)
	return row, err
}

// collection converts the rows the core factory returns. It is a method of
// its own so that Create, which spends a Grant, calls nothing but the core.
func (x *CommentFactory) collection(rows model.Rows) models.CommentCollection {
	return models.CommentCollectionOf(rows)
}

// arandu:begin custom
// defineComment is the default state: every row the factory makes starts here,
// and a State changes the part a test cares about. Named states go here too.
func defineComment(f faker.Faker) models.Comment {
	return models.Comment{
		Body: f.UserName(),
	}
}

// arandu:end custom
