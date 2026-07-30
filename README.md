# arandu

The Arandu project skeleton. This is what `aru new` clones — the equivalent of
`laravel/laravel`, not of `laravel/framework`.

Nobody imports this repository. You clone it once and it is yours from then on,
which is what lets the framework evolve without fighting the directory layout of
older projects.

## Layout

```
cmd/app/main.go   the whole composition of the application, explicit
modules/          your modules, one directory per feature
compose.yml       Postgres and Redis for local development
.env.example      copy to .env
```

`cmd/app/main.go` also dispatches `serve`, `migrate` and `routes`: those need the
registered modules, and this binary is what knows them.

## Getting started

```
docker compose up -d
cp .env.example .env
aru key:generate          # paste the line into .env
aru migrate
aru serve
```

## Status

Phase 1. The pgx driver is declared here rather than in the framework: this is a
project, not a library, and the core keeps its two dependencies. See
`adr/0004` and `adr/0006` in `arandu-io/docs`.
