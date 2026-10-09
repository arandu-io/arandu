// Example resource. Remove with the list under "The example resource" in README.md.

package policies

import (
	"context"
	"fmt"

	"github.com/arandu-io/hesape/auth"

	models "github.com/arandu-io/arandu/app/Models"
)

// The actions of Comment. Constants rather than strings at the call site: a
// typo in an action name would silently authorize nothing, or worse, everything.
//
// They carry the entity in the name because every policy in the application
// lives in this package now, and five constants called ActionView would not
// compile past the first module.
const (
	// CommentView is reading one record.
	CommentView auth.Action = "comment.view"
	// CommentList is paging through the records.
	CommentList auth.Action = "comment.list"
	// CommentCreate is adding one.
	CommentCreate auth.Action = "comment.create"
	// CommentUpdate is changing one.
	CommentUpdate auth.Action = "comment.update"
	// CommentDelete is removing one.
	CommentDelete auth.Action = "comment.delete"
)

// CommentPolicy is the only authority over who does what with Comment.
//
// IT DENIES EVERYTHING. That is deliberate: a generated policy that allowed
// anything would be a hole shipped by default, in every project that ran the
// generator. Open what this module actually needs, and nothing else.
type CommentPolicy struct{}

// Compile-time proof that the policy answers about this entity and no other.
var _ auth.Policy[models.Comment] = CommentPolicy{}

// Can decides whether the subject may perform the action.
func (CommentPolicy) Can(ctx context.Context, s auth.Subject, a auth.Action, co models.Comment) error {
	// Tenant isolation comes first and applies to every action. Without it every
	// check below would be pointless in a multi-tenant system.
	if co.ID != "" && co.TenantID != s.Tenant {
		return fmt.Errorf("comment belongs to another tenant")
	}

	// arandu:begin custom
	// Every action needs somebody signed in to the tenant: the zero subject is
	// refused before any rule below.
	if s.ID == "" || s.Tenant == "" {
		return fmt.Errorf("a comment is read and written by somebody signed in")
	}

	switch a {
	// Everybody in the tenant reads the comments of a note they can read, and
	// writes their own. Whether they can read the note is the note's policy,
	// which CommentService asks first, through NoteService.
	case CommentList, CommentView, CommentCreate:
		return nil

	// A stored comment is changed only by the account that wrote it, compared
	// against the stored row the service read.
	case CommentUpdate, CommentDelete:
		if co.OwnedBy(s.ID) {
			return nil
		}
		return fmt.Errorf("only the author of a comment may change it")
	}
	// arandu:end custom

	return fmt.Errorf("no rule allows %s on comment", a)
}
