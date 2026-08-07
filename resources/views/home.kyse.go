//go:build kyse

package views

@go
// HomeData is what HomeController.Index hands this page.
//
// A struct, never a map. That is what turns a typo in a field name into a
// compile error instead of a blank space on a page that answered 200.
//
// The embedded Page is the state the layout draws -- the title, the brand, the
// token and the navigation. This struct declares only what this page shows, and
// satisfies the layout's contract by embedding.
type HomeData struct {
	Page

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

// Compile-time proof that this page fits the layout it extends.
var _ Layout = HomeData{}
@endgo

@extends('layouts.app')

@section('content')
	<h1 class="text-3xl font-semibold tracking-tight sm:text-4xl">
		Hello {{ .Name }}
	</h1>
	<p class="mt-4 text-base text-slate-600 dark:text-slate-300">
		It is not the developer who guarantees the architecture. It is the compiler.
	</p>

	@if(len(d.Features) > 0)
		<ul class="mt-10 grid gap-4 sm:grid-cols-2">
			@foreach(.Features as feature)
				<li class="rounded-lg border border-slate-200 p-5 dark:border-slate-800">
					<h2 class="text-sm font-semibold">{{ feature.Title }}</h2>
					<p class="mt-1 text-sm text-slate-600 dark:text-slate-300">{{ feature.Body }}</p>
				</li>
			@endforeach
		</ul>
	@endif
@endsection
