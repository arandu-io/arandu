//go:build kyse

// Example resource. Remove with the list under "The example resource" in README.md.

package comments

import (
	"fmt"

	"github.com/arandu-io/hesape/pagination"
	"github.com/arandu-io/hesape/view"
	"github.com/arandu-io/kyse/components"
)

@go
// CommentsIndexData is what CommentController.Index hands this page.
type CommentsIndexData struct {
	// view.Page is the chrome the layout draws: the title, the description, the
	// CSRF token and the navigation. Embedded rather than repeated, and what
	// makes this struct fit the layout.
	view.Page
	// Comments is the page of records.
	Comments []CommentRow
	// NewURL is where the "new" button goes: the create screen, addressed by
	// route name in the controller rather than written here as a path. A view
	// has no route table, so a link written here could only be a literal -- and
	// a literal keeps rendering after the route moves.
	NewURL string
	// URL is the listing's own address, without a query string: where every
	// control of the table goes, addressed by route name in the controller.
	URL string
	// Result is what the listing returned: the state that was read, the page
	// with its total and the page sizes. The table is drawn from it as it came
	// back.
	Result *pagination.ListingResult
}

// Compile-time proof that this page fits the layout it extends.
var _ view.Layout = CommentsIndexData{}

// CommentsTableID is the id of the table's root element: the region every
// control of the table swaps, and what CommentController.Index reads from
// HX-Target to tell a request for the table from a request for the page.
const CommentsTableID = "comments-index"

// CommentRow is one record, formatted by the controller: what the listing
// and the record show, and what the edit form starts at.
type CommentRow struct {
	// ID is what the row is addressed by.
	ID string
	// URL is where the row links to, built from the route name by the
	// controller.
	URL string
	// Body is the Body column.
	Body string
	// Created is the creation timestamp, already formatted.
	Created string
	// EditURL is where the row's edit entry goes, and empty when the person may
	// not change the comment, so the menu offers only what would be answered.
	EditURL string
}

// CommentIndexTable adapts the server page to the native Kyse DataTable.
// The Service searches, orders and pages the list and returns the listing
// Result; the table is drawn from it, so every control is an address the state
// builds and the component owns only the rows and column presentation of the
// page it was handed.
func CommentIndexTable(data CommentsIndexData) components.DataTableProps {
	rows := make([]components.TableRow, 0, len(data.Comments))
	for _, comment := range data.Comments {
		actions := []components.MenuItem{{Label: "View", URL: comment.URL}}
		if comment.EditURL != "" {
			actions = append(actions, components.MenuItem{Label: "Edit", URL: comment.EditURL})
		}
		rows = append(rows, components.TableRow{
			Key:   comment.ID,
			Label: "Select comment " + comment.Body,
			Cells: []components.TableCell{
				{HTML: components.Link(components.LinkProps{
					Label: fmt.Sprint(comment.Body),
					URL: comment.URL,
					Variant: "hover",
				})},
				{Text: comment.Created},
			},
			Actions: actions,
		})
	}
	return components.DataTableProps{
		ID: CommentsTableID,
		Label: data.Title,
		URL: data.URL,
		Result: data.Result,
		PageData: data.Page,
		Searchable: true,
		FullPager: true,
		HeaderMenu: true,
		SelectName: "ids",
		ColumnsLabel: "Columns",
		Columns: []components.TableColumn{
			{Label: "Body", Key: "body", Sortable: true},
			{Label: "Created", Key: "created_at", Sortable: true, Hideable: true},
		},
		Rows: rows,
		Empty: components.EmptyProps{
			Title: "No comment yet.",
			Message: "Add the first comment to get started.",
			ActionLabel: "New comment",
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
		<a class="rounded-md border border-slate-300 px-3 py-2 text-sm font-medium hover:bg-slate-50 dark:border-slate-700 dark:hover:bg-slate-900" href="{{ .NewURL }}">New comment</a>
	</div>

	{{-- The table is a file of its own, because it is the one part of this page
	     the server also answers alone: a sort, a search or a page step replaces
	     it and nothing else. @include hands this page's data over unchanged. --}}
	<div class="mt-8">
		@include('partials.comments_table')
	</div>
@endsection
