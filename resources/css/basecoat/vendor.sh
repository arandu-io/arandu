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
#
# Stylesheets and the licence, and nothing that executes. Upstream also ships
# JavaScript for the components with behaviour, and it stays upstream: resources/
# is the input of `aru view:build`, which knows .kyse.go and .css and nothing
# else, so a .js copied in here is never built, never embedded and never served.
# The client behaviour this application does have is embedded by the framework
# and delivered under the Content-Security-Policy it sets.
#
# Usage: vendor.sh <path-to-basecoat-checkout>
set -euo pipefail

src="${1:?usage: vendor.sh <path-to-basecoat-checkout>}"
dst="$(cd "$(dirname "$0")" && pwd)"

[ -f "$src/LICENSE.md" ] || { echo "not a basecoat checkout: $src" >&2; exit 1; }

# js/ is removed and not recreated. An executable asset does not enter
# resources/: this directory is compiled by `aru view:build`, which reads
# .kyse.go and .css, so a script left here is dead weight that the repository's
# own tests refuse. An earlier version of this updater copied seven of them, and
# the removal below is what cleans up after it.
rm -rf "$dst/base" "$dst/components" "$dst/styles" "$dst/js"
mkdir -p "$dst/base" "$dst/components" "$dst/styles"

cp "$src/LICENSE.md" "$dst/LICENSE.md"
cp "$src/src/css/base/base.css" "$dst/base/base.css"
cp "$src/src/css/basecoat-components.css" "$dst/components.css"
cp "$src/src/css"/components/*.css "$dst/components/"
cp "$src/src/css/styles/vega.css" "$dst/styles/vega.css"

echo "vendored from $src"
ls "$dst"
