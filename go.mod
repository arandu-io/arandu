module github.com/arandu-io/arandu

go 1.26

require github.com/arandu-io/framework v0.41.0

require (
	github.com/arandu-io/hesape v0.19.1
	github.com/arandu-io/hesape/database/connectors/pgx v0.7.0
	github.com/arandu-io/hesape/database/connectors/sqlite v0.7.0
	github.com/arandu-io/hesape/redis v0.7.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/redis/go-redis/v9 v9.22.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
)

require (
	github.com/arandu-io/kyse v0.14.0
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.10.0 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/crypto v0.53.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	modernc.org/libc v1.74.1 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
	modernc.org/sqlite v1.55.0 // indirect
)

// This is the project skeleton, not a library: nobody imports it, you clone it
// once. That is what allows it to depend on a driver at all -- the core keeps
// its two dependencies. See 10-adr/ADR-0004-dependency-free-core.md and 10-adr/ADR-0006-cli-in-separate-module.md.
//
// It requires two compartments because .env offers two engines out of the box.
// A project that settles on one deletes the other import and runs `go mod
// tidy`: the driver leaves the build, go.sum and the vulnerability surface.
// That is what ADR 0014 bought.
