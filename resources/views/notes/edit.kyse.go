//go:build kyse

// Example resource. Remove with the list under "The example resource" in README.md.

package notes

import (
	"github.com/arandu-io/kyse/components"

	"github.com/arandu-io/hesape/view"
)

@go
// NotesEditData is what NoteController.Edit hands this page: the form
// filled in with a stored record. A rejected update comes back with what was
// typed in place of the stored values, from the flash on the page.
type NotesEditData struct {
	// Page is the state the layout draws, and what every input asks for its
	// message and for what was typed. Its Token is what @csrf writes into the
	// hidden field.
	view.Page
	// Form is the stored record, which is what the inputs start at.
	Form NoteRow
	// ShowURL is the record this form edits, and UpdateURL is where it submits.
	// Both are built from the route names by the controller: a view has no route
	// table, so a path written here could only be a literal -- and a literal
	// keeps rendering after the route moves.
	ShowURL   string
	UpdateURL string
}

// Compile-time proof that this page fits the layout it extends.
var _ view.Layout = NotesEditData{}

// arandu:begin custom
// Anything else this page needs in Go goes here, and survives regeneration.
// arandu:end custom
@endgo

@extends('layouts.app')

@section('content')
	<nav class="text-sm text-slate-500 dark:text-slate-400">
		<a class="underline underline-offset-2 hover:text-slate-900 dark:hover:text-slate-100" href="{{ .ShowURL }}">Back</a>
	</nav>

	<h1 class="mt-2 text-3xl font-semibold tracking-tight">{{ .Title }}</h1>

	<!-- hx-put, and no action: a browser form can only send GET and POST, and
	the update route is PUT. HTMX sends the real method, which is why this
	stack does not need a hidden _method field. -->
	<form class="mt-8 space-y-6" hx-put="{{ .UpdateURL }}">
		@csrf
		
		{!! components.Field(components.FieldProps{
			Name:  "title",
			Label: "Title",
			Type:  "text",
			Value: .Form.Title,
			Page: .,
			Required: true,
		}) !!}

		{!! components.Textarea(components.TextareaProps{
			Name:  "body",
			Label: "Body",
			Value: .Form.Body,
			Page: .,
			Rows:  6,
		}) !!}

		{!! components.Checkbox(components.CheckboxProps{
			Name:    "pinned",
			Label:   "Pinned",
			Checked: .Form.Pinned,
			Page: .,
		}) !!}

		<div class="flex items-center gap-3">
			<button class="rounded-md bg-slate-900 px-3 py-2 text-sm font-medium text-white hover:bg-slate-700 dark:bg-slate-100 dark:text-slate-900 dark:hover:bg-slate-300" type="submit">Save</button>
			<a class="text-sm text-slate-500 underline underline-offset-2 dark:text-slate-400" href="{{ .ShowURL }}">Cancel</a>
		</div>
	</form>
@endsection
