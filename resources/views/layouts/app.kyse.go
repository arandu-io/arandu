//go:build kyse

package layouts

import "github.com/arandu-io/kyse/components"

<!doctype html>
{{-- No x-data on <html>. theme.js applies the theme to that element before the
     body is parsed, and Alpine only reads it back afterwards. Binding it here
     instead threw on every page: x-data="theme" names a component and theme.js
     registers a store, so the name was never going to resolve.

     This is a kyse comment, not an HTML one: it does not reach the page. --}}
<html lang="en" class="h-full">
<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">

	{{-- What a rejected form means.
	     htmx swaps a response only when its table of response handling says to,
	     and the default in the copy this framework embeds ends with
	     {"code":"[45]..","swap":false} -- so a 422 is fetched, is correct, and is
	     thrown away. The person sees the form they submitted, unchanged, with no
	     message on it, and concludes the button does nothing. That is exactly
	     what happened: a password one character short answered 422 with the
	     reason in the body, and the screen said nothing at all.
	     422 comes before the catch-all because htmx takes the first entry that
	     matches. It lives here, once: the layout is what decides what a fragment
	     answer means, and a per-page opt-in would be a second way to answer a
	     rejected form (RULE 9). A meta tag is not a script, so it costs nothing
	     against script-src 'self'. See framework/httpx/context.go. --}}
	<meta name="htmx-config" content='{"responseHandling":[{"code":"204","swap":false},{"code":"422","swap":true},{"code":"[23]..","swap":true},{"code":"[45]..","swap":false,"error":true}]}'>
	<title>{{ .PageTitle() }}</title>
	<link rel="icon" href="/favicon.ico" sizes="any">
	<link rel="icon" href="/favicon.png" type="image/png">

	<!-- What a page says about itself. Each one is written only when the page
	     filled it in: an empty description is worse than none, because a search
	     engine that finds one stops looking for a better sentence in the body. -->
	@if(.PageDescription() != "")
		<meta name="description" content="{{ .PageDescription() }}">
		<meta property="og:description" content="{{ .PageDescription() }}">
	@endif
	@if(.CanonicalURL() != "")
		<link rel="canonical" href="{{ .CanonicalURL() }}">
		<meta property="og:url" content="{{ .CanonicalURL() }}">
	@endif
	<meta property="og:title" content="{{ .PageTitle() }}">
	<meta property="og:site_name" content="{{ .BrandName() }}">
	<meta property="og:type" content="website">
	<meta name="twitter:card" content="summary_large_image">

	<!-- Every asset is embedded in the binary and addressed by content hash. No
	     CDN, because the CSP is script-src 'self'; no build directory, because
	     there is no bundler to write one (RULE 13). -->
	<link rel="stylesheet" href="{{ view.URL("app.css") }}">
	<script src="{{ view.URL("htmx.min.js") }}" defer></script>
	<script src="{{ view.URL("alpine.min.js") }}" defer></script>
	<script src="{{ view.URL("basecoat.bundle.js") }}" defer></script>

	<!-- The theme is read before the first paint, so a person who chose dark does
	     not get a white flash on every navigation. It is the one piece of script
	     that cannot wait for Alpine, and it is four lines. -->
	<script src="{{ view.URL("theme.js") }}"></script>
</head>
<!-- hx-headers is load-bearing: without it every hx-post fails the CSRF check,
     and the failure reads like a broken session rather than a missing attribute. -->
<body hx-boost="true" hx-headers='{"X-CSRF-Token": "{{ .CSRFToken() }}"}' class="bg-background text-foreground min-h-full antialiased">
	<div class="mx-auto flex min-h-full w-full max-w-3xl flex-col px-6">
		<header class="flex items-center justify-between border-b py-6">
			<a class="text-sm font-semibold tracking-tight" href="{{ .HomeLink() }}">{{ .BrandName() }}</a>
			<nav class="flex items-center gap-3 text-sm">
				{!! components.ThemeToggle() !!}
				@if(!.SignedIn())
					<a class="btn" data-variant="ghost" data-size="sm" href="{{ .LoginLink() }}">Sign in</a>
					@if(.RegisterLink() != "")
						<a class="btn" data-size="sm" href="{{ .RegisterLink() }}">Register</a>
					@endif
				@endif
				@if(.SignedIn())
					<span class="text-muted-foreground">{{ .SignedInName() }}</span>
					<form method="post" action="{{ .LogoutLink() }}">
						@csrf
						<button class="btn" data-variant="ghost" data-size="sm" type="submit">Sign out</button>
					</form>
				@endif
			</nav>
		</header>

		<main class="flex-1 py-12">
			@yield('content')
		</main>

		<!-- The tray flash messages land in. An endpoint that saves something answers
		     with a toast fragment and hx-swap="beforeend" on this element; the vendored
		     script arms whatever appears inside it. Empty until then, and it costs one
		     element to never write the "do I have a message" branch again. -->
		<div id="toaster" class="toaster" aria-live="polite"></div>

		<!-- One @yield, and it is 'content'. A section only one layout yields is a
		     section that disappears without a word when the layout is replaced. -->
		<footer class="text-muted-foreground border-t py-6 text-sm">
			<p>Built with Arandu.</p>
		</footer>
	</div>
</body>
</html>
