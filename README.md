<p align="center">
  <img src=".github/logo.png" alt="Arandu" width="140" height="140">
</p>

<h1 align="center">arandu-io/arandu</h1>

<p align="center">The project skeleton `aru new` clones — a running application from the first commit.</p>

<p align="center">
<a href="https://github.com/arandu-io/arandu/actions/workflows/ci.yml"><img src="https://github.com/arandu-io/arandu/actions/workflows/ci.yml/badge.svg" alt="Build Status"></a>
<a href="https://pkg.go.dev/github.com/arandu-io/arandu"><img src="https://pkg.go.dev/badge/github.com/arandu-io/arandu.svg" alt="Go Reference"></a>
<a href="https://github.com/arandu-io/arandu/tags"><img src="https://img.shields.io/github/v/tag/arandu-io/arandu?label=version" alt="Latest Version"></a>
<a href="LICENSE.md"><img src="https://img.shields.io/github/license/arandu-io/arandu" alt="License"></a>
</p>


## About the skeleton

> **Note:** this is what a new project starts from. The framework it runs on is
> [arandu-io/framework](https://github.com/arandu-io/framework).

You do not clone it by hand:

```sh
aru new my-app
cd my-app && aru dev
```

That gives you a Go framework for web applications, services and APIs, built
around development speed, a single compiled binary instead of a JavaScript
bundle, and authorization the compiler charges for: a repository call with no
`Grant` does not compile.

## What it delivers

- **A conventional tree** — `app/Http/Controllers`, `app/Models`,
  `app/Policies`, `app/Repositories`, `app/Services`, `app/Jobs`,
  `app/Events`, `app/Listeners`, `app/Mail`, `bootstrap/`, `config/`,
  `database/`, `resources/views/`, `routes/`, `storage/`, `public/` — so
  nothing about where a file lives has to be learned.
- **A mandatory `app/Policies/`** — elsewhere a policy directory is a habit an
  organised team keeps; here `aru doctor` fails a repository whose entity has no
  policy, and the policy denies by default with no allow-all branch.
- **A binary, not a toolchain** — it runs with `git clone && aru dev`. No
  `node_modules`, no `package.json`, no JavaScript lockfile, and no Node
  installed: the view compiler and every script are embedded in the binary.
- **`bootstrap/app.go`** — the one place the application is wired. `aru
  make:module` prints the lines a generated module needs and never edits this
  file itself: a generator that rewrites your wiring behind your back is a
  generator whose output nobody can account for.

`aru doctor` checks this tree against the architecture rules — from a
repository missing its policy to a tenant read off the request instead of the
`Grant` — and CI runs it on every push, without `--strict`: an error fails the
build, a warning stays the to-do it is, and a new project is never red for code
the generator wrote.

3,337 lines of production code and 3,085 of test, across 20 test files —
small on purpose: it is what a project starts from, not what it grows into.

## The rest of Arandu

`aru` is the command line that clones and drives this skeleton;
[`arandu-io/framework`](https://github.com/arandu-io/framework) is what it
runs on; `hesape` is the component collection the framework is built from;
`examples` is a complete application, read-worthy end to end, built the same
way `aru new` starts one.

## Learning Arandu

The API reference is generated from the doc comments and lives on
[pkg.go.dev](https://pkg.go.dev/github.com/arandu-io/framework). Every exported
symbol carries one, and that is deliberate: it is the documentation that cannot
drift from the code, because it sits in the same file.

The CLI documents itself. `aru help` lists every command, and each one explains
what it writes and what to do with it. `aru doctor` explains what it found and
what breaks, not which rule was violated.

A guide and a website do not exist yet, and that is a decision rather than a
gap: a guide written against an API that still moves is work done twice, and the
second time is worse — there is wrong documentation published. The site is the
next phase, and it will be an Arandu application.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Before opening a pull request, the three
commands under "Before you open a pull request" have to pass — CI runs them, and
then the binary, `aru doctor`, the image and `govulncheck` on top.

## Security Vulnerabilities

Please review [our security policy](SECURITY.md) on how to report a
vulnerability. Never open a public issue for one.

## License

Open-sourced software licensed under the [MIT license](LICENSE.md).
