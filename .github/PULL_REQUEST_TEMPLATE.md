<!--
Two short sections and a verification block is a complete pull request. Delete
any heading that does not apply rather than filling it with nothing.
-->

## What changed

## Why

The part that is not in the diff, and the part somebody will need in two years.
If the change contradicts a decision recorded at arandu.io/docs, say which one
and argue for changing it -- that is a normal thing to do, and it is better than
a patch that quietly works around it.

## Verification

Run this before opening. CI runs the same thing, and a formatting difference is
a failure rather than a comment.

```
gofmt -l .        # no output
go vet ./...
go test -race ./...
```

- [ ] Every commit carries a `Signed-off-by` line (`git commit -s`)
- [ ] No new dependency, or this pull request argues for the one it adds
- [ ] No AI attribution in any commit message: no `Co-Authored-By` for an
      assistant, no "generated with" footer
