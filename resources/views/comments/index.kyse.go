//go:build kyse

// Example resource. Remove with the list under "The example resource" in README.md.

package comments

import (
	"fmt"

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
	// Paginator is the page of the listing the query returned, its addresses
	// set to the listing's route by the controller. components.Pagination
	// draws the trail from it, and nothing on a listing of one page.
	Paginator components.Paginator
}

// Compile-time proof that this page fits the layout it extends.
var _ view.Layout = CommentsIndexData{}

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
}

// CommentIndexTable adapts the server page to the native Kyse DataTable.
// The Service pages the listing; the component owns only the rows and column
// presentation of the page it was handed.
func CommentIndexTable(data CommentsIndexData) components.DataTableProps {
	rows := make([]components.TableRow, 0, len(data.Comments))
	for _, comment := range data.Comments {
		rows = append(rows, components.TableRow{
			Key: comment.ID,
			Cells: []components.TableCell{
				{HTML: components.Link(components.LinkProps{
					Label: fmt.Sprint(comment.Body),
					URL: comment.URL,
					Variant: "hover",
				})},
				{Text: comment.Created},
			},
		})
	}
	return components.DataTableProps{
		ID: "comments-index",
		Label: data.Title,
		ColumnsLabel: "Columns",
		Columns: []components.TableColumn{
			{Label: "Body", Key: "body"},
			{Label: "Created", Key: "created_at", Hideable: true},
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

	<div class="mt-8">
		{!! components.DataTable(CommentIndexTable(.)) !!}
	</div>

	{!! components.Pagination(components.PaginationProps{
		Paginator:      .Paginator,
		PageData:       .,
		ComponentProps: components.ComponentProps{Class: "mt-6"},
	}) !!}
@endsection
