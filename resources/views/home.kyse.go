//go:build kyse

package views

@go
// HomeData is what HomeController.Index hands this page.
//
// A struct, never a map. That is what turns a typo in a field name into a
// compile error instead of a blank space on a page that answered 200.
//
// The embedded view.Page is the chrome the layout draws -- the title, the
// description, the token and the navigation -- and embedding it is all this
// page has to do to fit the frame.
type HomeData struct {
	view.Page

	// Name is who the page greets.
	Name string
	// Features is what the landing page lists.
	Features []Feature
}

// Feature is one entry of the list below.
type Feature struct {
	Title string
	Body  string
}
@endgo

@extends('layouts.app')

@section('content')
	<h1 class="text-3xl font-semibold tracking-tight sm:text-4xl">
		Hello {{ .Name }}
	</h1>
	<p class="text-muted-foreground mt-4 text-base">
		It is not the developer who guarantees the architecture. It is the compiler.
	</p>

	@if(len(.Features) > 0)
		<ul class="mt-10 grid gap-4 sm:grid-cols-2">
			@foreach(.Features as feature)
				<li class="card p-5">
					<h2 class="text-sm font-semibold">{{ feature.Title }}</h2>
					<p class="text-muted-foreground mt-1 text-sm">{{ feature.Body }}</p>
				</li>
			@endforeach
		</ul>
	@endif

@endsection
