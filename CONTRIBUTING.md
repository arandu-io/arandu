# Contributing

## Sign your commits

Every commit needs a `Signed-off-by` line:

```
git commit -s -m "what changed and why"
```

That line is the [Developer Certificate of Origin](https://developercertificate.org/):
you are stating that you wrote the change, or that you have the right to submit
it under this project's license. It is not a copyright assignment — you keep
your copyright, and this project can never be relicensed behind your back.

We use DCO rather than a CLA on purpose. A CLA would let the project relicense
later, and the price is that every contributor has to sign a legal document
before their first patch.

## Before you open a pull request

```
gofmt -l $(find . -name '*.go' -not -path '*/testdata/*' -not -name '*.kyse.go')
go vet ./...
go test -race ./...
```

The first line prints nothing when the tree is formatted. The filter is not
optional and it is what CI runs: `gofmt -l .` skips nothing, and the views in
`resources/views/` are not valid Go on purpose -- a `*.kyse.go` file opens with
a build tag that keeps the compiler out, and holds template syntax below it.
`gofmt` is the only tool in the chain that ignores build tags, so it is the only
one that has to be told.

The filter also excludes `testdata/`, and this repository has none. It is
carried anyway because the command is the same in every repository of the
project, and one that differs per repository is one nobody can paste.

CI runs these three and a good deal besides; `.github/workflows/ci.yml` is the
list, and it is the one that decides. It will grow, and this file will not
follow it -- what is written here are the checks worth running by hand, because
they are the ones that catch a mistake before it costs a CI run.

The core depends on the standard library and `golang.org/x/crypto`, and nothing
else. A pull request that adds a dependency there needs to argue for it first,
in an issue.

## Where a test goes

Under `tests/`, in the suite that says what the test does -- and which suite
that is gets decided by what the test reaches for, not by what it is called:

| suite | what belongs in it |
|---|---|
| `tests/Feature/` | boots the application, or drives more than one piece of it at once |
| `tests/Unit/` | one thing, with nothing running |

The split earns its keep the day the suite gets slow: `go test ./tests/Unit/` is
the one somebody runs on every save, and it stops being that the first time a
test in it opens a database -- which is why opening one counts as booting, even
where nothing is served. `tests/Unit/structure_test.go` checks the placement and
the split by command, reading what each file reaches for rather than what it is
named, and it is an ordinary test, so the `go test ./...` above is already
running it.

The directories are capitalised and the `package` clause is not: a directory
name is a label, an identifier is code. What the suites share sits in
`tests/TestCase.go`, which is `package tests` for that reason -- and ordinary Go
rather than a `_test.go`, which is what makes `tests.App` and `tests.Root`
callable from both.

That `go test` attributes coverage per directory is true, and what follows from
it here is a flag rather than an argument: a suite in a directory of its own is
run with `-coverpkg=./...`, or the run reports the coverage of the test packages
themselves, which is near zero and reads as though the suite broke.

The same fact decides the one test that does not move. A test that reads an
identifier the package does not export cannot live in another package -- Go
decides that, not this project -- and beside the code is also where its coverage
lands on the package under test with no flag at all. It is named
`<file>_internal_test.go`, so the name carries the reason it is there. The
guard's standing exception is `assets/`, which proves the bytes it embeds are
the ones the build produced; it does not yet know the suffix, so an internal
test teaches it the name in the same change or arrives red.

And having taken that package, use it. `plans/testpackages.go` in the arandu-io
working tree intersects the identifiers a test names with what its package
declares unexported -- an empty intersection is a test that took an access it
never needed -- and the checklist runs it across every Go repository in the project. `package
main` is outside that question: it cannot be imported, which is why what the
tests drive lives in `bootstrap` and not at the root.

## What the commit message says

What changed and why. The why is the part that is not in the diff, and it is the
part someone will need in two years.

No AI attribution of any kind: no `Co-Authored-By` for an assistant, no
"generated with" footer. Commits are authored by the people who submit them.

## Architecture decisions

The decisions this project has already made live at arandu.io/docs, and every
one that closed a door has an ADR. If your change contradicts one, say so in the
pull request and argue for the change of decision — that is a normal thing to
do, and it is better than a patch that quietly works around it.
