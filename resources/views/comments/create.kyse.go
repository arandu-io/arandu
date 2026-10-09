//go:build kyse

// Example resource. Remove with the list under "The example resource" in README.md.

package comments

import (
	"github.com/arandu-io/kyse/components"

	"github.com/arandu-io/hesape/view"
)

@go
// CommentsCreateData is what CommentController.Create hands this page. A
// rejected submission comes back to it too, and the page carries the messages
// and what was typed from the flash: the controller passes nothing for that.
type CommentsCreateData struct {
	// Page is the state the layout draws, and what every input asks for its
	// message and for what was typed. Its Token is what @csrf writes into the
	// hidden field -- it comes from the page data rather than from a global,
	// because a template that reaches for request state outside the data it was
	// given is how a form ends up carrying another session's token under load.
	view.Page
	// Form is what the inputs start at: empty here, and the stored record on the
	// edit screen, which shares these inputs.
	Form CommentRow
	// IndexURL is the listing this screen came from, and StoreURL is where the
	// form submits. Both are built from the route names by the controller: a
	// view has no route table, so a path written here could only be a literal --
	// and a literal keeps rendering after the route moves.
	IndexURL string
	StoreURL string
}

// Compile-time proof that this page fits the layout it extends.
var _ view.Layout = CommentsCreateData{}

// arandu:begin custom
// Anything else this page needs in Go goes here, and survives regeneration.
// arandu:end custom
@endgo

@extends('layouts.app')

@section('content')
	<nav class="text-sm text-slate-500 dark:text-slate-400">
		<a class="underline underline-offset-2 hover:text-slate-900 dark:hover:text-slate-100" href="{{ .IndexURL }}">Comments</a>
	</nav>

	<h1 class="mt-2 text-3xl font-semibold tracking-tight">{{ .Title }}</h1>

	<form class="mt-8 space-y-6" method="post" action="{{ .StoreURL }}" hx-post="{{ .StoreURL }}">
		@csrf
		
		{!! components.Textarea(components.TextareaProps{
			Name:  "body",
			Label: "Body",
			Value: .Form.Body,
			Page: .,
			Rows:  6,
			Required: true,
		}) !!}

		<div class="flex items-center gap-3">
			<button class="rounded-md bg-slate-900 px-3 py-2 text-sm font-medium text-white hover:bg-slate-700 dark:bg-slate-100 dark:text-slate-900 dark:hover:bg-slate-300" type="submit">Save</button>
			<a class="text-sm text-slate-500 underline underline-offset-2 dark:text-slate-400" href="{{ .IndexURL }}">Cancel</a>
		</div>
	</form>
@endsection
