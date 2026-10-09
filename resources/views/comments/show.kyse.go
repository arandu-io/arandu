//go:build kyse

// Example resource. Remove with the list under "The example resource" in README.md.

package comments

import "github.com/arandu-io/hesape/view"

@go
// CommentsShowData is what CommentController.Show hands this page.
type CommentsShowData struct {
	// Page is the state the layout draws, the CSRF token included. The delete
	// button sends no token of its own: an hx-delete carries no form body, so
	// the header is what reaches the server, and the hx-headers the layout puts
	// on <body> is inherited by every htmx request on the page.
	view.Page
	// Comment is the record.
	Comment CommentRow
	// IndexURL, EditURL and DeleteURL are the three addresses this screen
	// offers, built from the route names by the controller. A view has no route
	// table, so a path written here could only be a literal -- and a literal
	// keeps rendering after the route moves.
	IndexURL  string
	EditURL   string
	DeleteURL string
}

// Compile-time proof that this page fits the layout it extends.
var _ view.Layout = CommentsShowData{}

// arandu:begin custom
// Anything else this page needs in Go goes here, and survives regeneration.
// arandu:end custom
@endgo

@extends('layouts.app')

@section('content')
	<nav class="text-sm text-slate-500 dark:text-slate-400">
		<a class="underline underline-offset-2 hover:text-slate-900 dark:hover:text-slate-100" href="{{ .IndexURL }}">Comments</a>
	</nav>

	<div class="mt-2 flex items-center justify-between gap-4">
		<h1 class="text-3xl font-semibold tracking-tight">{{ .Title }}</h1>
		<div class="flex items-center gap-3">
			<a class="rounded-md border border-slate-300 px-3 py-2 text-sm font-medium hover:bg-slate-50 dark:border-slate-700 dark:hover:bg-slate-900" href="{{ .EditURL }}">Edit</a>
			<button class="rounded-md border border-red-300 px-3 py-2 text-sm font-medium text-red-700 hover:bg-red-50 dark:border-red-900 dark:text-red-400 dark:hover:bg-red-950" type="button" hx-delete="{{ .DeleteURL }}" hx-confirm="Delete this comment?">Delete</button>
		</div>
	</div>

	<dl class="mt-8 divide-y divide-slate-100 border-t border-slate-200 text-sm dark:divide-slate-900 dark:border-slate-800">
				<div class="grid grid-cols-3 gap-4 py-3">
			<dt class="text-slate-500 dark:text-slate-400">Body</dt>
			<dd class="col-span-2">{{ d.Comment.Body }}</dd>
		</div>
		<div class="grid grid-cols-3 gap-4 py-3">
			<dt class="text-slate-500 dark:text-slate-400">Created</dt>
			<dd class="col-span-2">{{ .Comment.Created }}</dd>
		</div>
	</dl>
@endsection
