module github.com/arandu-io/arandu

go 1.26.0

retract v0.10.0 // Requires retracted Kyse v0.15.1 and lacks method override in the default HTTP pipeline.

require github.com/arandu-io/framework v0.50.2

require (
	github.com/arandu-io/hesape v0.48.0
	github.com/arandu-io/hesape/database/connectors/sqlite v0.11.0
)

require (
	golang.org/x/mod v0.41.0 // indirect
	golang.org/x/tools v0.49.0 // indirect
)

require (
	github.com/arandu-io/kyse v0.30.0
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/crypto v0.57.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	modernc.org/libc v1.75.7 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
	modernc.org/sqlite v1.59.0 // indirect
)

// This is the project skeleton, not a library: nobody imports it, you clone it
// once. That is what allows it to depend on a driver at all -- the core keeps
// its two dependencies. See 10-adr/ADR-0004-dependency-free-core.md and 10-adr/ADR-0006-cli-in-separate-module.md.
//
// It requires one compartment, SQLite, because a new project runs before
// anybody installs anything: .env.example names SQLite, the in-process cache
// and sessions, and the queue in a table, and none of them needs a server.
// Every other engine is a compartment of its own, required by the project that
// uses it -- a `go get` and a blank import in bootstrap/app.go:
//
//	go get github.com/arandu-io/hesape/database/connectors/pgx     Postgres
//	go get github.com/arandu-io/hesape/redis                       CACHE_STORE and SESSION_DRIVER over RESP
//	go get github.com/arandu-io/hesape/queue/connectors/redis      QUEUE_CONNECTION over RESP
//
// So a driver nobody uses is not in the build, in go.sum or in the
// vulnerability surface, which is what ADR 0014 bought; and a setting that
// names one the binary does not link stops the boot naming both lines.
