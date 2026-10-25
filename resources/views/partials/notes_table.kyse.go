//go:build kyse

// Example resource. Remove with the list under "The example resource" in README.md.

// Package partials holds the parts of a screen the server answers on their own.
package partials

import (
	"github.com/arandu-io/kyse/components"

	notes "github.com/arandu-io/arandu/storage/framework/views/notes"
)

@go
// NotesTableData is what the notes listing's table draws.
//
// It is the listing page's own data. The page draws this file with @include,
// which hands its data over unchanged, and NoteController.Index fills the same
// struct when it answers the table alone, so both draw from one template.
type NotesTableData = notes.NotesIndexData
@endgo

{{-- The table alone: its root element is the region every control swaps. A
     sort, a facet, a page step, a new size and a column switch ask the listing's
     address with hx-get, name this element as the target, and replace it whole;
     the controller answers that request with this file and nothing around it,
     and every other request to the same address -- a direct visit, a boosted
     link, a history restore -- with the whole page. The table pushes the
     canonical address of its state into the history itself, so a reload asks for
     what the person is looking at.

     Every control is a real address beside its hx-get: with scripts off, it is
     an ordinary navigation to the same page. The table draws its own pager, and
     there is no other. --}}
{!! components.DataTable(notes.NoteIndexTable(.)) !!}
