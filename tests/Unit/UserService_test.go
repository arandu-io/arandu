package unit_test

import (
	"context"
	"errors"
	"testing"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"

	"github.com/arandu-io/arandu/app/Policies"
	"github.com/arandu-io/arandu/app/Services"
)

type userNamesRepository struct {
	called bool
	tenant string
	err    error
}

func (r *userNamesRepository) NamesByID(_ context.Context, g security.Grant, ids []string) (map[string]string, error) {
	r.called = true
	if err := g.Check(policies.ActionUserView); err != nil {
		return nil, err
	}
	r.tenant = data.Tenant(g)
	if r.err != nil {
		return nil, r.err
	}
	return map[string]string{ids[0]: "Ada"}, nil
}

func TestUserServiceAuthorizesTheDisplayNameRead(t *testing.T) {
	repo := &userNamesRepository{}
	service := services.NewUserService(repo)
	actor := security.Subject{ID: "user-1", Tenant: "tenant-from-session"}

	name, err := service.DisplayName(context.Background(), actor)
	if err != nil {
		t.Fatalf("DisplayName: %v", err)
	}
	if name != "Ada" {
		t.Errorf("DisplayName = %q, want Ada", name)
	}
	if repo.tenant != actor.Tenant {
		t.Errorf("repository tenant = %q, want the tenant from the authorized subject", repo.tenant)
	}
}

func TestUserServiceRefusesAReadWithoutAnAuthorizedSubject(t *testing.T) {
	repo := &userNamesRepository{}
	service := services.NewUserService(repo)

	if _, err := service.DisplayName(context.Background(), security.Subject{}); err == nil {
		t.Fatal("DisplayName accepted a read with no authorized subject")
	}
	if repo.called {
		t.Fatal("the repository was reached before a Policy issued a Grant")
	}
}

func TestUserServicePropagatesTheRepositoryFailure(t *testing.T) {
	want := errors.New("database unavailable")
	service := services.NewUserService(&userNamesRepository{err: want})
	actor := security.Subject{ID: "user-1", Tenant: "tenant-1"}

	_, err := service.DisplayName(context.Background(), actor)
	if !errors.Is(err, want) {
		t.Fatalf("DisplayName error = %v, want %v", err, want)
	}
}
