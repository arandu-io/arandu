package migrations

import (
	"github.com/arandu-io/hesape/auth/account"
	"github.com/arandu-io/hesape/database/migrations"
)

// The second factor's two tables, user_two_factor and user_recovery_codes,
// belong to the package that reads and writes them: hesape/auth/account, whose
// rules bootstrap/app.go builds over this application's users. Their migration
// is registered here, under the name the package gives it, so `aru migrate`
// creates them with the rest; the users table they hang off is this
// application's, and its migrations are the ones beside this file.
func init() {
	for _, migration := range account.Migrations() {
		migrations.Register(migration)
	}
}
