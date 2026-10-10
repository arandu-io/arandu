#!/usr/bin/env bash
# Copies Basecoat into this directory, as files, from a checkout of upstream.
#
# Basecoat is MIT and it is the only third-party front-end code in Arandu. It
# arrives as source in the tree, never from npm and never from a CDN: there is
# no package.json to install it with, and the Content-Security-Policy is
# `script-src 'self'`, so a script served from another host would not run even
# if one were referenced.
#
# What is copied is deliberately a subset:
#
#   LICENSE.md             the notice
#   base/base.css          the design tokens, as custom properties. Changing a
#                          colour is overriding one of these, which is what the
#                          theme picker does
#   components/*.css       the component layer
#   styles/vega.css        ONE style pack. Upstream ships eight; shipping all of
#                          them would be eight ways to style a button, and
#                          docs/09-uma-forma-so.md refuses that
#
# Stylesheets and the licence, and nothing that executes. Upstream also ships
# JavaScript for the components with behaviour, and it stays upstream: resources/
# is the input of `aru view:build`, which knows .kyse.go and .css and nothing
# else, so a .js copied in here is never built, never embedded and never served.
#
# components.css is not copied. It is this project's list of the components it
# imports, and upstream's list of every component would replace that choice
# with all of them.
#
# This directory also holds CSS of this project: components upstream does not
# ship, rules appended to upstream files, a patched line, a second notice in
# LICENSE.md, the animations. So the script deletes nothing, and it writes a
# file only when the file is absent or is still exactly what it last wrote.
# vendor.sum beside it records the SHA-256 of every file it wrote. A file whose
# content no longer matches its line there, or a file of this project that
# upstream now ships under the same name, is kept as it is and named at the
# end: merging upstream's version into it is a person's job, because only a
# person knows which half of the difference is theirs.
#
# Usage: vendor.sh <path-to-basecoat-checkout>
set -euo pipefail

src="${1:?usage: vendor.sh <path-to-basecoat-checkout>}"
dst="$(cd "$(dirname "$0")" && pwd)"
sum="$dst/vendor.sum"

for required in LICENSE.md src/css/base/base.css src/css/styles/vega.css src/css/components; do
	[ -e "$src/$required" ] || { echo "not a basecoat checkout: $src has no $required" >&2; exit 1; }
done

# sha256 prints the hex digest of a file. sha256sum is GNU coreutils and shasum
# is what macOS ships; either answers the same digest.
sha256() {
	if command -v sha256sum > /dev/null 2>&1; then
		sha256sum "$1" | cut -d' ' -f1
	else
		shasum -a 256 "$1" | cut -d' ' -f1
	fi
}

# recorded prints the digest vendor.sum holds for a path, or nothing.
recorded() {
	[ -f "$sum" ] || return 0
	awk -v p="$1" '$2 == p { print $1; exit }' "$sum"
}

# The pairs to copy, upstream path and the path here, one per line.
pairs=$(
	printf '%s %s\n' "LICENSE.md" "LICENSE.md"
	printf '%s %s\n' "src/css/base/base.css" "base/base.css"
	printf '%s %s\n' "src/css/styles/vega.css" "styles/vega.css"
	for f in "$src"/src/css/components/*.css; do
		name=$(basename "$f")
		printf '%s %s\n' "src/css/components/$name" "components/$name"
	done
)

next="$(mktemp "${TMPDIR:-/tmp}/vendor.sum.XXXXXX")"
trap 'rm -f "$next" "$next.tmp"' EXIT
[ -f "$sum" ] && cp "$sum" "$next"

written=0
kept=""
while read -r from to; do
	target="$dst/$to"
	if [ -e "$target" ]; then
		was=$(recorded "$to")
		if [ -z "$was" ]; then
			kept="$kept
  $to (a file of this project; upstream now ships one with the same name)"
			continue
		fi
		if [ "$(sha256 "$target")" != "$was" ]; then
			kept="$kept
  $to (changed here since it was vendored)"
			continue
		fi
	fi
	mkdir -p "$(dirname "$target")"
	cp "$src/$from" "$target"
	digest=$(sha256 "$target")
	awk -v p="$to" '$2 != p' "$next" > "$next.tmp" && mv "$next.tmp" "$next"
	printf '%s  %s\n' "$digest" "$to" >> "$next"
	written=$((written + 1))
done <<EOF
$pairs
EOF

LC_ALL=C sort -k2 "$next" > "$sum"

echo "vendored $written files from $src"
if [ -n "$kept" ]; then
	echo "kept, with upstream's version not copied over them:$kept"
fi
