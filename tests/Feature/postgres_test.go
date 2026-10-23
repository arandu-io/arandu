package feature_test

// The Postgres connector, linked into this test binary and not into the
// application's. tests.Serve makes every feature test a database on the
// engine ARANDU_TEST_DATABASE_URL names -- a copy of a migrated template on
// Postgres -- and a URL naming an engine whose connector is not linked stops
// at sql.Open. The application links the engines bootstrap/app.go imports;
// the suite links the one CI proves it on as well.
import _ "github.com/arandu-io/hesape/database/connectors/pgx"
