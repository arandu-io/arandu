// Package migrations holds this application's own schema changes.
//
// The shape is the conventional one: one file per change, applied
// in order, recorded in a table. Two things are deliberately different.
//
// There is no schema builder. A migration is SQL, written once in the portable
// subset every supported database shares, and what you read is what runs.
// A schema builder exists where an ORM hides the database; here the point
// is that nothing hides it.
//
// And a migration never runs at boot. `aru migrate` is a pipeline step: with N
// replicas starting together, N migrations race. Every migration is also
// compatible with the previous version of the binary during a rollout -- a new
// column is nullable or has a default, and dropping one takes two releases.
package migrations

import "github.com/arandu-io/framework/kernel"

// All returns this application's migrations, in the order they are applied.
//
// It starts empty, and that is correct: the users table comes from the auth
// module, the outbox from events and the jobs table from queue. The kernel
// collects those from the modules themselves, so repeating them here would apply
// each one twice.
//
// `aru make:module` appends to the list. Adding one by hand means adding the
// file next to this one and its value below.
func All() []kernel.Migration {
	return []kernel.Migration{
		// arandu:begin custom
		// createInvoicesTable,
		// arandu:end custom
	}
}
