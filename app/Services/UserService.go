// Package services coordinates the application's use cases.
package services

import (
	"context"

	"github.com/arandu-io/framework/security"

	models "github.com/arandu-io/arandu/app/Models"
	policies "github.com/arandu-io/arandu/app/Policies"
)

// UserNames is the specialized user read used by the application chrome.
type UserNames interface {
	NamesByID(ctx context.Context, g security.Grant, ids []string) (map[string]string, error)
}

// UserService coordinates authorized user reads for the application.
type UserService struct {
	repository UserNames
	policy     policies.UserPolicy
}

// NewUserService wires the user read service.
func NewUserService(repository UserNames) *UserService {
	return &UserService{repository: repository}
}

// DisplayName returns the acting user's display name after authorizing the read.
func (s *UserService) DisplayName(ctx context.Context, actor security.Subject) (string, error) {
	target := models.User{ID: actor.ID, TenantID: actor.Tenant}
	grant, err := security.Authorize(ctx, s.policy, actor, policies.ActionUserView, target)
	if err != nil {
		return "", err
	}

	names, err := s.repository.NamesByID(ctx, grant, []string{actor.ID})
	if err != nil {
		return "", err
	}
	return names[actor.ID], nil
}
