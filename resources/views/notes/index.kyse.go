//go:build kyse

// Example resource. Remove with the list under "The example resource" in README.md.

package notes

import (
	"fmt"

	"github.com/arandu-io/hesape/view"
	"github.com/arandu-io/kyse/components"
)

@go
// NotesIndexData is what NoteController.Index hands this page.
type NotesIndexData struct {
	// view.Page is the chrome the layout draws: the title, the description, the
	// CSRF token and the navigation. Embedded rather than repeated, and what
	// makes this struct fit the layout.
	view.Page
	// Notes is the page of records.
	Notes []NoteRow
	// NewURL is where the "new" button goes: the create screen, addressed by
	// route name in the controller rather than written here as a path. A view
	// has no route table, so a link written here could only be a literal -- and
	// a literal keeps rendering after the route moves.
	NewURL string
	// NextURL is the following page of the listing. It is empty on the last
	// page, and the link is not rendered then.
	NextURL string
}

// Compile-time proof that this page fits the layout it extends.
var _ view.Layout = NotesIndexData{}

// NotesTableID is the id of the element partials/notes_table draws: the target
// its next-page link names, and what NoteController.Index reads from HX-Target
// to tell a request for the table from a request for the page.
const NotesTableID = "notes-table"

// NoteRow is one record, formatted by the controller: what the listing
// and the record show, and what the edit form starts at.
type NoteRow struct {
	// ID is what the row is addressed by.
	ID string
	// URL is where the row links to, built from the route name by the
	// controller.
	URL string
	// Title is the Title column.
	Title string
	// Body is the Body column.
	Body string
	// Pinned is the Pinned column.
	Pinned bool
	// Created is the creation timestamp, already formatted.
	Created string
	// Published is when the note was published, already formatted, and empty
	// while it is a draft.
	Published string
}

// NoteIndexTable adapts the server page to the native Kyse DataTable.
// The Service pages the listing; the component owns only the rows and column
// presentation of the page it was handed.
func NoteIndexTable(data NotesIndexData) components.DataTableProps {
	rows := make([]components.TableRow, 0, len(data.Notes))
	for _, note := range data.Notes {
		rows = append(rows, components.TableRow{
			Key: note.ID,
			Cells: []components.TableCell{
				{HTML: components.Link(components.LinkProps{
					Label: fmt.Sprint(note.Title),
					URL: note.URL,
					Variant: "hover",
				})},
				{Text: fmt.Sprint(note.Body)},
				{Text: fmt.Sprint(note.Pinned)},
				{Text: note.Created},
			},
		})
	}
	return components.DataTableProps{
		ID: "notes-index",
		Label: data.Title,
		ColumnsLabel: "Columns",
		Columns: []components.TableColumn{
			{Label: "Title", Key: "title"},
			{Label: "Body", Key: "body", Hideable: true},
			{Label: "Pinned", Key: "pinned", Hideable: true},
			{Label: "Created", Key: "created_at", Hideable: true},
		},
		Rows: rows,
		Empty: components.EmptyProps{
			Title: "No note yet.",
			Message: "Add the first note to get started.",
			ActionLabel: "New note",
			ActionURL: data.NewURL,
		},
	}
}

// arandu:begin custom
// Anything else these pages need in Go goes here, and survives regeneration.
// arandu:end custom
@endgo

@extends('layouts.app')

@section('content')
	<div class="flex items-center justify-between gap-4">
		<h1 class="text-3xl font-semibold tracking-tight">{{ .Title }}</h1>
		<a class="rounded-md border border-slate-300 px-3 py-2 text-sm font-medium hover:bg-slate-50 dark:border-slate-700 dark:hover:bg-slate-900" href="{{ .NewURL }}">New note</a>
	</div>

	{{-- The table is a file of its own, because it is the one part of this page
	     the server also answers alone: the next page of the listing replaces
	     it and nothing else. @include hands this page's data over unchanged. --}}
	<div class="mt-8">
		@include('partials.notes_table')
	</div>
@endsection
