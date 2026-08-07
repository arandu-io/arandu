# arandu

The Arandu project skeleton. This is what `aru new` clones — the equivalent of
`laravel/laravel`, not of `laravel/framework`.

Nobody imports this repository. You clone it once and it is yours from then on,
which is what lets the framework evolve without fighting the directory layout of
older projects.

## Layout

The eight directories of Laravel, in the same places and with the same names.

```
main.go                the entry point — public/index.php and artisan, merged
bootstrap/app.go       the whole composition of the application, explicit
config/                ten files, one per domain, each a typed struct
routes/web.go          the routes; routes/console.go the commands
app/Http/Controllers/  controllers, CamelCase files as in PSR-4
app/Models/            structs, no Active Record
app/Services/          business rules — does not exist in Laravel
app/Repositories/      data access, every method takes a Grant
app/Policies/          authorization — required, not recommended
database/              migrations, seeders, factories
resources/views/       kyse sources (.kyse.go) and the Go they compile to
resources/css/app.css  the Tailwind entry; resources/js/app.js is hand-written
public/                barely exists: assets are embedded in the binary
storage/               app/ and framework/ — there is no storage/logs
compose.yml            Postgres and Redis, for when you outgrow SQLite
.env.example           copy to .env
```

What is missing on purpose: `vendor/`, `bootstrap/cache/`, `storage/logs/`,
`package.json` and `node_modules/`.

`main.go` dispatches the commands that need the registered modules: `serve`,
`migrate`, `migrate:rollback`, `migrate:status`, `migrate:fresh`, `routes`,
`db:seed`, `schedule:list`, `schedule:run` and `work`. `aru` delegates to this
binary, because this binary is the one that knows which modules exist.

## Getting started

Nothing to install: the default connection is SQLite, a file under `database/`.

```
cp .env.example .env
aru key:generate          # paste the line into .env
aru view:build            # compiles resources/views and the stylesheet
aru migrate
aru db:seed               # creates the first administrator
aru serve
```

No Node anywhere: `aru view:build` runs the kyse compiler, which is part of the
CLI, and the standalone Tailwind binary, which the CLI downloads and pins in
`arandu.toml`.

Moving to Postgres is `.env` and nothing else:

```
DB_CONNECTION=pgsql
DB_DATABASE=arandu
DB_HOST=127.0.0.1
DB_USERNAME=arandu
DB_PASSWORD=arandu
```

`docker compose up -d` starts Postgres and Redis when you want them locally.

## Two things that are not like Laravel

**No schema builder.** A migration is SQL, written in the subset every supported
database shares. Laravel needs a Blueprint because Eloquent hides the database;
here the point is that nothing hides it.

**A seeder is a type, not a class found by reflection.** A seeder that drifts
from the interface fails the build, rather than failing the first time someone
runs it against production.

## License

MIT, the same license Laravel uses. See `LICENSE.md`.
