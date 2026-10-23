package conformance_test

// The Postgres connector, linked into this test binary and not into the
// application's: the kits run on the engine ARANDU_TEST_DATABASE_URL names,
// and a URL naming an engine whose connector is not linked stops at sql.Open.
import _ "github.com/arandu-io/hesape/database/connectors/pgx"
