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
#   base/base.css          the design tokens, as custom properties. Changing a
#                          colour is overriding one of these, which is what the
#                          theme picker does
#   basecoat-components.css
#   components/*.css       the component layer
#   styles/vega.css        ONE style pack. Upstream ships eight; shipping all of
#                          them would be eight ways to style a button, and
#                          docs/09-uma-forma-so.md refuses that
#   js/*.js                only the components with behaviour that the delivered
#                          screens use. Each is a plain IIFE: no import, no
#                          export, no bundler
#
# Usage: vendor.sh <path-to-basecoat-checkout>
set -euo pipefail

src="${1:?usage: vendor.sh <path-to-basecoat-checkout>}"
dst="$(cd "$(dirname "$0")" && pwd)"

[ -f "$src/LICENSE.md" ] || { echo "not a basecoat checkout: $src" >&2; exit 1; }

rm -rf "$dst/base" "$dst/components" "$dst/styles" "$dst/js"
mkdir -p "$dst/base" "$dst/components" "$dst/styles" "$dst/js"

cp "$src/LICENSE.md" "$dst/LICENSE.md"
cp "$src/src/css/base/base.css" "$dst/base/base.css"
cp "$src/src/css/basecoat-components.css" "$dst/components.css"
cp "$src/src/css"/components/*.css "$dst/components/"
cp "$src/src/css/styles/vega.css" "$dst/styles/vega.css"

# The behaviour we actually mount. basecoat.js is the registry the others
# register into, so it is not optional.
for f in basecoat dropdown-menu popover select sidebar tabs toast; do
	cp "$src/src/js/$f.js" "$dst/js/$f.js"
done

echo "vendored from $src"
ls "$dst"
