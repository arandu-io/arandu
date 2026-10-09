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

{{-- One element, and it is the hole the next page is swapped into: the link
     below asks for the following page with hx-get, names this element as its
     hx-target, and replaces it whole. The controller answers that request with
     this file and nothing around it, and every other request to the same
     address -- a direct visit, a boosted link, a history restore -- with the
     whole page. hx-push-url keeps the address in step, so a reload asks for
     the page the person is looking at.

     href stays beside hx-get: with scripts off, the link is an ordinary
     navigation to the same page. --}}
<div id="{{ notes.NotesTableID }}">
	{!! components.DataTable(notes.NoteIndexTable(.)) !!}

	@if(.NextURL != "")
		<a class="mt-6 inline-block text-sm underline underline-offset-2" href="{{ .NextURL }}" hx-get="{{ .NextURL }}" hx-target="#{{ notes.NotesTableID }}" hx-swap="outerHTML" hx-push-url="true">Next page</a>
	@endif
</div>
