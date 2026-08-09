<p align="center">
  <img src=".github/logo.png" alt="Arandu" width="140" height="140">
</p>

<h1 align="center">arandu-io/arandu</h1>

<p align="center">The Arandu application skeleton.</p>

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

The tree is the conventional one — `app/Http/Controllers`, `app/Models`,
`app/Policies`, `bootstrap/`, `config/`, `database/`, `resources/views`,
`routes/`, `storage/`, `public/` — so nothing about where a file lives has to be
learned.

Three directories carry the difference: `app/Services/`, `app/Repositories/` and
a mandatory `app/Policies/`. Elsewhere those are a habit an organised team keeps;
here they are skeleton, and `aru doctor` asks for them.

It runs with `git clone && aru dev`. No `node_modules`, no `package.json`, no JS
lockfile, and no Node installed — assets are embedded in the binary.

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
commands at the top of that file have to pass, and CI runs exactly them.

## Security Vulnerabilities

Please review [our security policy](SECURITY.md) on how to report a
vulnerability. Never open a public issue for one.

## License

Open-sourced software licensed under the [MIT license](LICENSE.md).
