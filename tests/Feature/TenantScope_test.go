package feature_test

import (
	"context"
	"sort"
	"testing"

	"github.com/arandu-io/hesape/database"
	"github.com/arandu-io/hesape/database/schema"
)

// Every table that carries a tenant is named as one that filters by it.
//
// Tenant comes from the Grant and from nowhere else, and the way that stops
// being true is not a policy somebody removes -- it is a table somebody adds. A
// new table with a tenant_id nobody filters on reads every tenant's rows for
// whoever asks, and it looks exactly like a table that is fine until somebody
// opens the query.
//
// So the list of tables is READ FROM THE DATABASE, through the schema builder
// rather than written out here. A hand-written list is a list that stops
// mentioning the table added after it, which is precisely the table this is
// about. The map below is the other half: it is the claim, one entry per table
// whose reads take data.Tenant(g), and the catalogue is the fact. The two are
// compared in both directions.
//
// # What it does not reach
//
// A table holding one tenant's rows WITHOUT a tenant column -- a pivot keyed by
// two foreign ids, a child row scoped only through its parent -- is invisible
// here, and no mechanical check finds it: nothing in the schema says which
// tenant a row belongs to. A clean run means no unscoped tenant column was
// found, not that no tenant data is reachable without a Grant. The Grant on the
// repository signature is what covers the rest.

// tenantColumn is the name the framework writes. A project that scopes a table
// under another name changes it here, and gets the same check.
const tenantColumn = "tenant_id"

// scopedByTenant is the claim: reads of these tables take data.Tenant(g).
//
// It is written by hand ON PURPOSE, and it is the only hand-written half. Adding
// a name here is a statement that somebody looked at the queries, so it is meant
// to be the step that makes a person look.
var scopedByTenant = map[string]string{
	"users":  "the auth module's, and every read of it goes through a policy",
	"outbox": "the domain events, sealed with the Grant that produced them",
	"jobs":   "the queue, whose rows carry the tenant they were enqueued for",
}

func TestEveryTableWithATenantColumnIsOneThatFiltersByTenant(t *testing.T) {
	db := migratedDB(t)
	ctx := context.Background()

	// The same connection the migration commands build, so the catalogue read
	// here is the catalogue the application would see. ForSchema is what adapts
	// it to the builder, and the builder is what makes this portable: it asks
	// the engine for its tables rather than reading sqlite_master, so the check
	// still runs on Postgres.
	connection := database.NewConnection(db.Unwrap(), "", "", map[string]any{
		"driver": string(db.Dialect()),
	})
	builder := schema.NewBuilder(database.ForSchema(connection))

	tables, err := builder.GetTables(ctx)
	if err != nil {
		t.Fatalf("reading the catalogue: %v", err)
	}
	// Fast, and before anything is compared. Every assertion below is true of an
	// empty catalogue, so a database that was never migrated would report a clean
	// tree and check nothing -- which is the failure a test like this one dies
	// of, silently, for as long as nobody looks.
	if len(tables) == 0 {
		t.Fatal("the catalogue is empty, so nothing was checked: the database was not migrated")
	}

	seen := make(map[string]bool, len(tables))

	for _, table := range tables {
		seen[table.Name] = true

		carries, err := builder.HasColumn(ctx, table.Name, tenantColumn)
		if err != nil {
			t.Fatalf("reading the columns of %s: %v", table.Name, err)
		}
		_, claimed := scopedByTenant[table.Name]

		switch {
		case carries && !claimed:
			t.Errorf("%s has a %s and nothing says it is filtered by one.\n"+
				"        Every read of it -- List, Find, a read model, a report, an export -- has to take "+
				"data.Tenant(g) before the name goes in scopedByTenant. A tenant column nobody filters on "+
				"is one tenant reading another's rows.\n"+
				"        Once every read does take it, the line to add is:\n"+
				"            %q: \"why every read of it is scoped\",\n"+
				"        A generated module lands here the first time it is generated, and it is meant to: "+
				"the generator writes the table and the person writes the claim, because the claim is the "+
				"step where somebody reads the queries.",
				table.Name, tenantColumn, table.Name)

		case !carries && claimed:
			t.Errorf("scopedByTenant names %s and the table has no %s.\n"+
				"        Either the column was dropped and the reads now scope by nothing, or the claim "+
				"outlived the table it was about. Both are the map saying something the database does not.",
				table.Name, tenantColumn)
		}
	}

	// A claim about a table that is not there at all. It is the same failure as
	// the one above and cannot be found in the loop, because a table nothing
	// returns is a table the loop never visits.
	missing := make([]string, 0)
	for name := range scopedByTenant {
		if !seen[name] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	for _, name := range missing {
		t.Errorf("scopedByTenant names %s and the catalogue has no such table: "+
			"a claim nobody can check is one nobody is keeping true", name)
	}
}
