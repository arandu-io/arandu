// Package policies holds this application's authorization decisions.
//
// In Laravel a policy is a convention of an organized team. Here it is skeleton,
// and the compiler and `aru doctor` both enforce it: a repository whose entity
// has no policy fails the check, because an entity that is reachable and that
// nobody decided who may reach is the failure this framework exists to prevent.
//
// A policy answers one question -- may this subject perform this action on this
// resource -- and it denies by default. There is no allow-all branch, and adding
// one is not a shortcut, it is the end of the guarantee.
package policies

import (
	"github.com/arandu-io/framework/modules/auth"
	"github.com/arandu-io/framework/security"
)

// UserPolicy is the only authority over who does what with a user.
//
// It is an alias of the auth module's policy, not a second one. The module owns
// the table, the actions and the tenant check, and a copy here would be the one
// that drifts -- two policies for one entity is two answers to one question
// (RULE 9). The name is here so app/Policies/UserPolicy.go is where a Laravel
// developer finds it.
//
// Replace the alias with a struct of your own the day this application needs a
// rule the module does not have. The contract is security.Policy[models.User],
// and security.Authorize is the only way to turn a decision into a Grant.
type UserPolicy = auth.UserPolicy

// The actions a user can be the subject of, re-exported so a call site names a
// constant instead of a string. A typo in a string would silently authorize
// nothing -- or, in a policy written with a default branch, everything.
const (
	ActionUserView   security.Action = auth.ActionUserView
	ActionUserCreate security.Action = auth.ActionUserCreate
	ActionUserUpdate security.Action = auth.ActionUserUpdate
	ActionUserDelete security.Action = auth.ActionUserDelete
)
