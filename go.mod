module github.com/arandu-io/arandu

go 1.25

require (
	github.com/arandu-io/framework v0.1.0
	github.com/jackc/pgx/v5 v5.7.2
)

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	golang.org/x/crypto v0.31.0 // indirect
	golang.org/x/sync v0.10.0 // indirect
	golang.org/x/sys v0.28.0 // indirect
	golang.org/x/text v0.21.0 // indirect
)

// This is the project skeleton, not a library: nobody imports it, you clone it
// once. That is what allows it to depend on a driver -- the pgx dependency lives
// here, and the framework core keeps its two. See docs/adr/0004 and 0006.
