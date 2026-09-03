#!/usr/bin/env bash
# The mechanical guard for the test layout, in order of importance.
#
# The first check is the one that matters most, and it is not a style rule: go
# test only runs a file whose name ends in _test.go. A file named CacheTest.go
# -- or cache_Test.go, which is the same mistake with a different hand --
# compiles into the package as ordinary code and none of its tests ever run. No
# error, no warning, a green build with the suite switched off.
#
# Every check that asks the toolchain a question treats a question that could
# not be asked as a failure. A guard that goes green because `go list` broke has
# checked nothing and said everything was fine.
#
# Two things about THIS repository shape what follows, and both are written out
# where they bite:
#
#   the compiled views  storage/framework/views/ is build output, gitignored,
#                       and written by `aru view:build`. Check 4 asks the
#                       toolchain, and the toolchain cannot answer before that
#                       command has run -- so this script runs AFTER the view
#                       build, in CI and on a fresh clone alike.
#   the .kyse.go files  resources/views/*.kyse.go are tracked *.go files that
#                       are not valid Go; the compiler drops them through the
#                       `kyse` build tag. Check 3 reads file CONTENT, and it
#                       reads it only under tests/, where none of them lives.
set -uo pipefail

cd "$(dirname "$0")/.." || exit 1

fail=0

# The modules of this repository, by where their go.mod sits. There is one
# today -- the project skeleton is a single module -- and the loop is what makes
# the answer survive a second one, because every rule below is relative to a
# module and not to this directory.
#
# Held as lines rather than an array: the bash macOS ships is 3.2, which has no
# mapfile, and a guard that only runs on the build machine is a guard nobody
# runs before pushing.
modules=$(git ls-files '*go.mod' | xargs -n1 dirname | sort -u)
if [ -z "$modules" ]; then
	echo "[FAILED] no go.mod is tracked, so there is no module to check"
	exit 1
fi

# nearest_module answers the module a path belongs to: the closest go.mod at or
# above its directory.
#
# This is the whole of check 2. Anchoring the tests/ directory at the top of the
# repository is wrong the moment a second module exists -- its tests/ tree is
# not at the top -- and matching tests/ anywhere in the path is wrong in the
# other direction, because it accepts app/Services/tests/ as a test tree.
nearest_module() {
	local dir=$1
	while :; do
		if [ -f "$dir/go.mod" ]; then
			printf '%s\n' "$dir"
			return 0
		fi
		[ "$dir" = "." ] && return 1
		dir=$(dirname "$dir")
	done
}

# module_relative answers a path as its module sees it, or nothing if no module
# claims it.
module_relative() {
	local file=$1 root
	root=$(nearest_module "$(dirname "$file")") || return 1
	if [ "$root" = "." ]; then
		printf '%s\n' "$file"
	else
		printf '%s\n' "${file#"$root"/}"
	fi
}

# Nothing below may pass by having nothing to look at. Every one of these checks
# is a statement about a set of files, and every one of them is true of the
# empty set: pointed at an empty tree the guard reports success and has read
# nothing. The counts are what turn that into a failure.
sources=$(git ls-files '*.go' | grep -c '')
suite=$(git ls-files '*_test.go' | grep -c '')

if [ "$sources" -eq 0 ] || [ "$suite" -eq 0 ]; then
	echo "[FAILED] $sources Go files and $suite test files are tracked here."
	echo "         Every check below is true of nothing, so none of them ran."
	exit 1
fi

# 1. A test file the toolchain does not recognise as one.
#
# The pattern is Tests?\.go with a capital T, which is every shape of the
# mistake -- CacheTest.go, cache_Test.go, CacheTests.go, Test.go -- and no false
# positive: latest.go ends in lowercase test.go and a real test file does too.
#
# It anchors on the END of the name, and here that is load-bearing rather than
# incidental: tests/TestCase.go is the base every suite imports, it is a legal
# non-test file, and it is meant to be there. Widening this to bare `Test` to
# catch "a file with Test in the name" reports it -- and the fix somebody then
# reaches for is renaming the one file the layout is built around.
if offenders=$(git ls-files '*.go' | grep -E 'Tests?\.go$'); then
	echo "[FAILED] go test does not run these, and will not say so:"
	printf '%s\n' "$offenders" | sed 's/^/    /'
	fail=1
fi

# 2. A test outside tests/ has to need an unexported identifier, and says so in
#    its name. Anything else belongs in a category.
#
#    The tree satisfies this today with room to spare: all 20 test files sit
#    under tests/, in Feature/ and Unit/, and there is no _internal_test.go at
#    all. That is the state this exists to keep -- a skeleton is what `aru new`
#    copies, so a test dropped beside a controller here becomes the layout every
#    project generated from it starts with.
while IFS= read -r file; do
	[ -z "$file" ] && continue

	if ! relative=$(module_relative "$file"); then
		echo "[FAILED] $file is under no module, so its place cannot be judged"
		fail=1
		continue
	fi

	case "$relative" in
	tests/*) continue ;;
	esac
	case "$file" in
	*_internal_test.go) continue ;;
	esac

	echo "[FAILED] $file is outside tests/ and is not _internal_test.go"
	fail=1
done < <(git ls-files '*_test.go')

# 3. The directories are capitalised; the package clause is not.
#
#    tests/Feature holds `package feature_test` and tests/Unit holds
#    `package unit_test`. A capitalised import name is legal Go that nobody
#    writes, and the capitalised DIRECTORY is what makes it tempting.
inspected=0
while IFS= read -r file; do
	[ -z "$file" ] && continue

	relative=$(module_relative "$file") || continue
	case "$relative" in
	tests/*) ;;
	*) continue ;;
	esac
	inspected=$((inspected + 1))

	if clause=$(grep -n '^package [A-Z]' "$file"); then
		echo "[FAILED] capitalised package clause in $file:"
		printf '%s\n' "$clause" | sed 's/^/    /'
		fail=1
	fi
done < <(git ls-files '*.go')

if [ "$inspected" -eq 0 ]; then
	echo "[FAILED] no module has a tests/ tree, so the package clauses of one were not read"
	fail=1
fi

# 4. Nothing outside the tests reaches the tests tree. It imports testing, and a
#    package that reaches it registers the flags of a test binary into whatever
#    imports it.
#
#    Here that package has a name: tests/TestCase.go is `package tests`, an
#    ordinary compilable package rather than a _test.go file. Nothing stops a
#    controller from importing it except this check -- and the binary that
#    shipped would then answer -test.v on the command line.
#
#    The question is asked of the PRODUCTION packages and not of ./..., which
#    also lists the test packages themselves -- and every one of those reaches
#    the tests tree, which is what it is for. Asked that way the check reports a
#    failure on any module that has a tests tree at all, which is every module
#    it is meant to protect.
#
#    The tags are passed the way the framework passes them. No suite in this
#    repository sits behind a build tag today -- tests/ is Feature and Unit, and
#    CI runs `go test -race ./...` with no tags -- so the two words change
#    nothing now, and they are what keeps this check honest on the day a suite
#    moves behind one. The tag that must NEVER be passed here is `kyse`: it
#    would pull resources/views/*.kyse.go into the build, and those do not parse
#    as Go by design.
asked=0
while IFS= read -r module; do
	[ -z "$module" ] && continue
	asked=$((asked + 1))

	# The cache is warmed before anything is measured, and it is not a
	# convenience. `go list` writes "go: downloading <module> <version>" to
	# stderr, which is captured here on purpose so that a module that does not
	# build is reported instead of read as an empty package list -- and those two
	# words then arrive as package names, which is what "no required module
	# provides package v0.12.0" is. CI has a cold cache every run, so this failed
	# there and passed here.
	#
	# The `all` is left off, and its absence is the point rather than a
	# shortening. `go mod download all` walks the whole module graph and records
	# a sum for every module in it, including the test-only dependencies OF
	# dependencies -- kr/pretty, stretchr/objx, golang.org/x/net here, none of
	# which this module builds. A module whose go.sum does not already carry them
	# came out of this guard with a tracked file rewritten, and a guard that
	# reports a clean layout by dirtying the tree it was asked about makes every
	# later reading of `git status` lie. The plain form fetches the modules that
	# provide what the main module imports, which is exactly the set `go list`
	# below is about to ask for, so the cache is still warm.
	(cd "$module" && GOWORK=off go mod download >/dev/null 2>&1) || true

	if ! packages=$(cd "$module" && GOWORK=off go list -tags 'integration e2e' ./... 2>&1); then
		echo "[FAILED] go list failed in $module, so nothing was checked there:"
		printf '%s\n' "$packages" | sed 's/^/    /'
		# The hint is printed only when the failure is the one it explains. A
		# guard that answers every breakage with the same advice teaches people
		# to read past it, and the advice is wrong for most of them: an import
		# cycle is not a missing view build.
		if printf '%s\n' "$packages" | grep -q 'storage/framework/views'; then
			echo "         The compiled views are missing: run \`aru view:build\`."
			echo "         That output is gitignored, so a fresh clone has none of"
			echo "         it and nothing in this module resolves until it is built."
		fi
		fail=1
		continue
	fi

	production=$(printf '%s\n' "$packages" | grep -vE '/tests(/|$)')
	if [ -z "$production" ]; then
		continue
	fi

	# One import path per line and none of them has a space: the split is wanted.
	# shellcheck disable=SC2086
	if ! dependencies=$(cd "$module" && GOWORK=off go list -tags 'integration e2e' -deps $production 2>&1); then
		echo "[FAILED] go list -deps failed in $module, so nothing was checked there:"
		printf '%s\n' "$dependencies" | sed 's/^/    /'
		fail=1
		continue
	fi

	if reached=$(printf '%s\n' "$dependencies" | grep -E '/tests(/|$)'); then
		echo "[FAILED] a production package in $module reaches the tests tree:"
		printf '%s\n' "$reached" | sed 's/^/    /'
		fail=1
	fi
done < <(printf '%s\n' "$modules")

if [ "$asked" -eq 0 ]; then
	echo "[FAILED] the toolchain was asked about no module, so check 4 did not run"
	fail=1
fi

exit $fail
