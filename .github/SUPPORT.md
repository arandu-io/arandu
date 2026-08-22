# Support

## Where a question goes

The issue tracker of the repository the question is about. That is the only
public channel this project has: there is no chat server, no forum, no mailing
list and no support address, and none is announced before there is somebody
reading it. A channel that swallows the message is worse than no channel at
all -- the same reasoning that put vulnerability reports behind a GitHub
advisory instead of a mailbox.

Blank issues are enabled for exactly this reason. A question is not a defect,
and it does not have to be filed as one to reach anybody.

## Look here first

- **API reference** --
  [pkg.go.dev](https://pkg.go.dev/github.com/arandu-io/framework), generated
  from the doc comments. Every exported symbol carries one, and it cannot drift
  from the code because it sits in the same file.
- **The CLI** -- `aru help` lists every command, and each one explains what it
  writes and what to do with it. `aru doctor` explains what it found and what
  breaks, not which rule was violated.
- **Decisions** -- [arandu.io/docs](https://arandu.io/docs). Every
  decision that closed a door has an architecture decision record, and the
  record carries the reason. When the answer to "why can I not do this" is a
  decision, that is where it is written.

There is no usage guide, tutorial or narrated example yet. That is a later
phase, not an oversight: a guide written against an unstable API gets written
twice, and the second time there is already a wrong one published.

## Reporting a bug

Use the bug report template. It asks for the module and version, the output of
`go version`, the operating system and the smallest sequence that reproduces the
problem. With those four a maintainer can run what you ran, which is the whole
point of the form.

## Reporting a vulnerability

Never in a public issue. `SECURITY.md` in this repository has the private
channel, the timelines and what counts as a vulnerability here. A public issue
about one is closed and moved to a private advisory, with thanks.

## Proposing a change

Open an issue before the pull request when the change adds a dependency, alters
an exported API, or contradicts a decision recorded at arandu.io/docs. For
anything smaller the pull request is the proposal. `CONTRIBUTING.md` has the
rest: sign-off, where a test goes, and what the commit message has to say.
