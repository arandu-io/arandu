// Rendered by aru model:build for models.User. Everything outside the custom block is rewritten on the next build.

package factories

import (
	"context"
	"strings"
	"time"

	"github.com/arandu-io/hesape/auth"
	"github.com/arandu-io/hesape/database/model"
	factory "github.com/arandu-io/hesape/database/model/factories"
	"github.com/arandu-io/hesape/faker"

	models "github.com/arandu-io/arandu/app/Models"
)

// UserFactory builds rows of users, for tests and for seeding.
//
// Make builds rows and stores nothing. Create stores them, and takes the Grant
// every write takes: the tenant comes off it, and a factory is no way around the
// policy that guards the table.
//
// Every method returns a new factory, so a factory kept in a variable is never
// changed by a caller that adds a state to it.
type UserFactory struct{ f *factory.Factory }

// Users returns the factory of users over db, with defineUser as
// its default state.
//
//	rows, err := factories.Users(db).Count(10).Create(ctx, g)
//	one, err := factories.Users(db).State(func(u *models.User) { ... }).MakeOne()
//
// The values come from a seeded faker -- the same rows on every run, so a
// failure reproduces -- and Seed asks for others. The key is left empty: the
// model draws a fresh one for every row it stores, so two batches never share
// one, and Make builds rows that have none yet.
func Users(db model.DB) *UserFactory {
	return &UserFactory{f: factory.New(models.Users(db).Base(), func(f faker.Faker, row model.Entity) {
		*row.(*models.User) = defineUser(f)
	})}
}

// Count returns a factory that makes n rows.
func (x *UserFactory) Count(n int) *UserFactory { return &UserFactory{f: x.f.Count(n)} }

// Seed returns a factory whose values start from seed.
func (x *UserFactory) Seed(seed int64) *UserFactory { return &UserFactory{f: x.f.Seed(seed)} }

// State returns a factory that applies fn to every row after the default state.
func (x *UserFactory) State(fn func(*models.User)) *UserFactory {
	return &UserFactory{f: x.f.State(func(row model.Entity) { fn(row.(*models.User)) })}
}

// Sequence returns a factory that cycles through states, one per row.
func (x *UserFactory) Sequence(states ...func(*models.User)) *UserFactory {
	adapted := make([]func(model.Entity), len(states))
	for i, state := range states {
		adapted[i] = func(row model.Entity) { state(row.(*models.User)) }
	}
	return &UserFactory{f: x.f.Sequence(adapted...)}
}

// AfterMaking returns a factory that runs fn on each row once it is built.
func (x *UserFactory) AfterMaking(fn func(*models.User)) *UserFactory {
	return &UserFactory{f: x.f.AfterMaking(func(row model.Entity) { fn(row.(*models.User)) })}
}

// AfterCreating returns a factory that runs fn on each row once it is stored.
func (x *UserFactory) AfterCreating(fn func(context.Context, auth.Grant, *models.User) error) *UserFactory {
	return &UserFactory{f: x.f.AfterCreating(func(ctx context.Context, g auth.Grant, row model.Entity) error {
		return fn(ctx, g, row.(*models.User))
	})}
}

// Make returns the rows without storing any of them.
func (x *UserFactory) Make() (models.UserCollection, error) {
	rows, err := x.f.Make()
	return x.collection(rows), err
}

// MakeOne returns one row without storing it, whatever Count says.
func (x *UserFactory) MakeOne() (*models.User, error) {
	e, err := x.f.MakeOne()
	row, _ := e.(*models.User)
	return row, err
}

// Create stores the rows and returns them.
func (x *UserFactory) Create(ctx context.Context, g auth.Grant) (models.UserCollection, error) {
	rows, err := x.f.Create(ctx, g)
	return x.collection(rows), err
}

// CreateOne stores one row and returns it, whatever Count says.
func (x *UserFactory) CreateOne(ctx context.Context, g auth.Grant) (*models.User, error) {
	e, err := x.f.CreateOne(ctx, g)
	row, _ := e.(*models.User)
	return row, err
}

// collection converts the rows the core factory returns. It is a method of
// its own so that Create, which spends a Grant, calls nothing but the core.
func (x *UserFactory) collection(rows model.Rows) models.UserCollection {
	return models.UserCollectionOf(rows)
}

// arandu:begin custom
// defineUser is the default state: every row the factory makes starts here,
// and a State changes the part a test cares about. Named states go here too.
//
// Every account is an ordinary member with a verified address at a domain
// reserved for documentation, so a seeded database cannot mail a stranger.
func defineUser(f faker.Faker) models.User {
	// The faker's handles repeat across rows, and an address is unique in
	// its tenant: a fragment of a drawn id keeps a batch apart.
	handle := f.UserName() + "." + strings.SplitN(f.UUID(), "-", 2)[0]
	verified := f.Time(verifiedFrom, verifiedUntil)
	return models.User{
		Name:       f.Name(),
		Email:      handle + "@example.test",
		Password:   UnusablePassword,
		Roles:      models.Roles{models.RoleMember},
		VerifiedAt: &verified,
	}
}

// UnusablePassword is the password column of an account made by the factory.
// It is no hash of anything, so no password signs in as the account: a seeded
// database holds people to look at, and nobody to sign in as by accident.
// A test that needs to sign in sets a real hash with a State.
const UnusablePassword = "!"

// verifiedFrom and verifiedUntil bound the verification time a made account
// carries. Fixed rather than relative to now, so the same seed draws the same
// rows on every run.
var (
	verifiedFrom  = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	verifiedUntil = time.Date(2026, time.June, 30, 0, 0, 0, 0, time.UTC)
)

// arandu:end custom
