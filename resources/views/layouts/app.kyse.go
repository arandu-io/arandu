//go:build kyse

package views

@go
// Layout is what every page hands the application layout.
//
// An interface rather than a struct, so each page keeps its own typed data and
// still fits the frame: one struct per page, one layout, no map anywhere (RULE
// 9). A page that forgets PageTitle does not compile.
type Layout interface {
	// PageTitle is what the browser tab shows.
	PageTitle() string
}
@endgo

<!doctype html>
<html lang="en" class="h-full">
<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<title>{{ .PageTitle() }}</title>
	<link rel="icon" href="/favicon.ico">

	<!-- Every asset is embedded in the binary and addressed by content hash. No
	     CDN, because the CSP is script-src 'self'; no build directory, because
	     there is no bundler to write one (RULE 13). -->
	<link rel="stylesheet" href="{{ view.URL("app.css") }}">
	<script src="{{ view.URL("htmx.min.js") }}" defer></script>
	<script src="{{ view.URL("alpine.min.js") }}" defer></script>
</head>
<body class="h-full bg-white text-slate-900 antialiased dark:bg-slate-950 dark:text-slate-100">
	<div class="mx-auto flex min-h-full w-full max-w-3xl flex-col px-6">
		<header class="flex items-center justify-between border-b border-slate-200 py-6 dark:border-slate-800">
			@yield('header')
		</header>

		<main class="flex-1 py-12">
			@yield('content')
		</main>

		<footer class="border-t border-slate-200 py-6 text-sm text-slate-500 dark:border-slate-800 dark:text-slate-400">
			@yield('footer')
		</footer>
	</div>
</body>
</html>
