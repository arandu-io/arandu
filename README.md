# arandu

The Arandu project skeleton. This is what `aru new` clones — the equivalent of
`laravel/laravel`, not of `laravel/framework`.

Nobody imports this repository. You clone it once and it is yours from then on,
which is what lets the framework evolve without fighting the directory layout of
older projects.

## Layout

```
cmd/app/main.go        the whole composition of the application, explicit
database/seeders/      seeders, DatabaseSeeder first — same shape as Laravel
modules/               your modules, one directory per feature
compose.yml            Postgres and Redis, for when you outgrow SQLite
.env.example           copy to .env
```

`cmd/app/main.go` also dispatches the commands that need the registered modules:
`serve`, `migrate`, `migrate:rollback`, `migrate:status`, `migrate:fresh`,
`routes` and `db:seed`. `aru` delegates to this binary, because this binary is
the one that knows which modules exist.

## Getting started

Nothing to install: the default connection is SQLite, a file under `database/`.

```
cp .env.example .env
aru key:generate          # paste the line into .env
aru migrate
aru db:seed               # creates the first administrator
aru serve
```

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
