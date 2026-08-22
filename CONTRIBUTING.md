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
optional and it is what CI runs: `gofmt -l .` skips nothing, and two things here
are not valid Go on purpose -- the doctor fixtures under `testdata/`, one of
which does not parse because that is the test, and the `*.kyse.go` sources,
which the compiler excludes through their build tag. `gofmt` is the only tool in
the chain that ignores build tags.

CI runs these three and a good deal besides; `.github/workflows/ci.yml` is the
list, and it is the one that decides. It will grow, and this file will not
follow it -- what is written here are the checks worth running by hand, because
they are the ones that catch a mistake before it costs a CI run.

The core depends on the standard library and `golang.org/x/crypto`, and nothing
else. A pull request that adds a dependency there needs to argue for it first,
in an issue.

## Where a test goes

Beside the code it tests, named `*_test.go`, in the same directory. There is no
`tests/` directory, and that is not style: `go test` attributes coverage per
directory, so a test filed elsewhere leaves the package under test reporting
0% -- and it can only reach what the package exports.

Which package the test declares is a real choice, and it answers one question:

| declare | when |
|---|---|
| `package X_test` | this is the **contract**. The test sees what a caller sees, which is the point |
| `package X` | this is the **implementation**, and the test genuinely needs something the package does not export |

Prefer the first. Take the second only when you use it -- `plans/testpackages.go`
in the arandu-io working tree checks exactly that, by intersecting the
identifiers a test names with what its package declares unexported, and the
checklist runs it across every repository.

A `package main` has no external form: it cannot be imported, so its tests are
internal and that is the end of it.

## What the commit message says

What changed and why. The why is the part that is not in the diff, and it is the
part someone will need in two years.

No AI attribution of any kind: no `Co-Authored-By` for an assistant, no
"generated with" footer. Commits are authored by the people who submit them.

## Architecture decisions

The decisions this project has already made live in `arandu-io/docs`, and every
one that closed a door has an ADR. If your change contradicts one, say so in the
pull request and argue for the change of decision — that is a normal thing to
do, and it is better than a patch that quietly works around it.
