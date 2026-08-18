// Package migrations holds this application's own schema changes.
//
// One file per change and one type per file. The type embeds
// migrations.BaseMigration, says its own name, writes Up, and registers itself
// from init():
//
//	type CreateInvoicesTable struct{ migrations.BaseMigration }
//
//	func (CreateInvoicesTable) GetName() string {
//		return "2026_08_17_000000_create_invoices_table"
//	}
//
//	func (CreateInvoicesTable) Up(ctx context.Context, conn migrations.Connection) error {
//		_, err := conn.Statement(ctx, `CREATE TABLE invoices (...)`, nil)
//		return err
//	}
//
//	func (CreateInvoicesTable) Down(ctx context.Context, conn migrations.Connection) error {
//		_, err := conn.Statement(ctx, `DROP TABLE invoices`, nil)
//		return err
//	}
//
//	func init() { migrations.Register(CreateInvoicesTable{}) }
//
// The name carries the order: the registry sorts by that string and nothing
// else. It is fixed the moment the migration has run against any database,
// because a migration that changes its name is a migration that runs twice.
//
// Registering is what makes a migration reachable, and the blank import of this
// package in bootstrap/app.go is what makes the registering happen. A package
// nothing imports is not in the binary at all.
//
// The connection is handed to Up rather than being a string the migration
// returns, so a migration that has to read before it writes -- a backfill, a
// check that a column is empty before it is dropped -- calls Select and then
// Statement, with no second kind of migration for the case.
//
// This package starts with no migrations of its own, and that is correct: the
// users table comes from the auth module, the outbox from events and the jobs
// table from queue. Each of those registers its own, and repeating them here
// would apply each one twice.
//
// A migration never runs at boot. `aru migrate` is a pipeline step: with N
// replicas starting together, N migrations race. Every migration is also
// compatible with the previous version of the binary during a rollout -- a new
// column is nullable or has a default, and dropping one takes two releases.
package migrations
