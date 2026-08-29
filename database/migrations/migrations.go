// Package migrations owns this application's schema changes.
//
// Each migration registers itself under the default application group. Their
// names are historical identities: changing one would make an already-applied
// schema look pending and run the change twice.
package migrations
