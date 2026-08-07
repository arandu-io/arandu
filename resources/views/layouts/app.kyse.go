//go:build kyse

package views

@go
// Layout is what every page hands the application layout.
//
// An interface rather than a struct, and that is the whole design: a page keeps
// its own typed data -- one struct per page, so a typo in a field name is a
// compile error -- and still fits the frame, because it embeds Page and Page
// implements this. Pages carrying different data therefore share one layout,
// which is what RULE 9 asks for: one layout, not one per shape of data.
//
// It asks for what it draws and nothing else. It asks for exactly what the
// layout `aru make:auth` publishes draws, too: that command replaces this file
// and leaves every page alone, so the two have to want the same things.
type Layout interface {
	// PageTitle is what the browser tab shows.
	PageTitle() string
	// BrandName is the application name in the navigation bar.
	BrandName() string
	// CSRFToken is the token every write of this session carries.
	CSRFToken() string
	// SignedIn decides which half of the navigation bar is drawn.
	SignedIn() bool
	// SignedInName is who that half greets.
	SignedInName() string
	// HomeLink is where the brand points.
	HomeLink() string
	// LoginLink is the sign-in screen.
	LoginLink() string
	// LogoutLink is what the sign-out form posts to.
	LogoutLink() string
	// RegisterLink is the sign-up screen, or empty when registration is not
	// open -- and the link is not drawn then.
	RegisterLink() string
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
<!-- hx-headers is load-bearing: without it every hx-post fails the CSRF check,
     and the failure reads like a broken session rather than a missing attribute. -->
<body hx-headers='{"X-CSRF-Token": "{{ .CSRFToken() }}"}' class="h-full bg-white text-slate-900 antialiased dark:bg-slate-950 dark:text-slate-100">
	<div class="mx-auto flex min-h-full w-full max-w-3xl flex-col px-6">
		<header class="flex items-center justify-between border-b border-slate-200 py-6 dark:border-slate-800">
			<a class="text-sm font-semibold tracking-tight hover:text-slate-600 dark:hover:text-slate-300" href="{{ .HomeLink() }}">{{ .BrandName() }}</a>
			<nav class="flex items-center gap-3 text-sm text-slate-500 dark:text-slate-400">
				@if(!d.SignedIn())
					<a class="hover:text-slate-900 dark:hover:text-slate-100" href="{{ .LoginLink() }}">Sign in</a>
					@if(d.RegisterLink() != "")
						<a class="hover:text-slate-900 dark:hover:text-slate-100" href="{{ .RegisterLink() }}">Register</a>
					@endif
				@endif
				@if(d.SignedIn())
					<span>{{ .SignedInName() }}</span>
					<form method="post" action="{{ .LogoutLink() }}">
						@csrf
						<button class="hover:text-slate-900 dark:hover:text-slate-100" type="submit">Sign out</button>
					</form>
				@endif
			</nav>
		</header>

		<main class="flex-1 py-12">
			@yield('content')
		</main>

		<!-- One @yield, and it is 'content'. A section only one layout yields is a
		     section that disappears without a word when the layout is replaced --
		     and `aru make:auth` replaces this file with one that yields exactly
		     this and nothing else. -->
		<footer class="border-t border-slate-200 py-6 text-sm text-slate-500 dark:border-slate-800 dark:text-slate-400">
			<p>Built with Arandu.</p>
		</footer>
	</div>
</body>
</html>
