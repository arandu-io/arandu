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

{{-- One element, and it is the hole the next page is swapped into: the
     pager below asks for the following or the previous page with hx-get,
     names this element as its hx-target, and replaces it whole. The
     controller answers that request with this file and nothing around it,
     and every other request to the same address -- a direct visit, a boosted
     link, a history restore -- with the whole page. hx-push-url on this
     element is inherited by the pager's links and keeps the address in step,
     so a reload asks for the page the person is looking at.

     Every entry of the pager is a real address beside its hx-get: with
     scripts off, it is an ordinary navigation to the same page. --}}
<div id="{{ notes.NotesTableID }}" hx-push-url="true">
	{!! components.DataTable(notes.NoteIndexTable(.)) !!}

	{!! components.Pagination(components.PaginationProps{
		Paginator:      .Paginator,
		PageData:       .,
		HxTarget:       "#" + notes.NotesTableID,
		HxSwap:         "outerHTML",
		ComponentProps: components.ComponentProps{Class: "mt-6"},
	}) !!}
</div>
