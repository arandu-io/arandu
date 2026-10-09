// Example resource. Remove with the list under "The example resource" in README.md.

package policies

import (
	"context"
	"fmt"

	"github.com/arandu-io/hesape/auth"

	models "github.com/arandu-io/arandu/app/Models"
)

// The actions of Note. Constants rather than strings at the call site: a
// typo in an action name would silently authorize nothing, or worse, everything.
//
// They carry the entity in the name because every policy in the application
// lives in this package now, and five constants called ActionView would not
// compile past the first module.
const (
	// NoteView is reading one record.
	NoteView auth.Action = "note.view"
	// NoteList is paging through the records.
	NoteList auth.Action = "note.list"
	// NoteCreate is adding one.
	NoteCreate auth.Action = "note.create"
	// NoteUpdate is changing one.
	NoteUpdate auth.Action = "note.update"
	// NoteDelete is removing one.
	NoteDelete auth.Action = "note.delete"
)

// NotePolicy is the only authority over who does what with Note.
//
// IT DENIES EVERYTHING. That is deliberate: a generated policy that allowed
// anything would be a hole shipped by default, in every project that ran the
// generator. Open what this module actually needs, and nothing else.
type NotePolicy struct{}

// Compile-time proof that the policy answers about this entity and no other.
var _ auth.Policy[models.Note] = NotePolicy{}

// Can decides whether the subject may perform the action.
func (NotePolicy) Can(ctx context.Context, s auth.Subject, a auth.Action, n models.Note) error {
	// Tenant isolation comes first and applies to every action. Without it every
	// check below would be pointless in a multi-tenant system.
	if n.ID != "" && n.TenantID != s.Tenant {
		return fmt.Errorf("note belongs to another tenant")
	}

	// arandu:begin custom
	// Every action needs somebody signed in to the tenant: the zero subject is
	// refused before any rule below.
	if s.ID == "" || s.Tenant == "" {
		return fmt.Errorf("a note is read and written by somebody signed in")
	}

	switch a {
	// Everybody in the tenant reads the notes of the tenant, and writes their
	// own.
	case NoteList, NoteView, NoteCreate:
		return nil

	// A stored note is changed only by the account that wrote it. The service
	// asks about the row it read, never about what the request claims the row
	// is, so the owner here is the stored one -- and a note with no owner, the
	// empty one included, is changed by nobody.
	case NoteUpdate, NoteDelete:
		if n.OwnedBy(s.ID) {
			return nil
		}
		return fmt.Errorf("only the author of a note may change it")
	}
	// arandu:end custom

	return fmt.Errorf("no rule allows %s on note", a)
}
