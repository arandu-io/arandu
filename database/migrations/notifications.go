package migrations

import (
	"github.com/arandu-io/hesape/database/migrations"
	"github.com/arandu-io/hesape/notifications"
)

// The notifications table belongs to the package that reads it: the database
// channel of hesape/notifications, which stores what the bell menu draws. Its
// migration is registered here, under the name the package gives it, because
// this application builds that channel in bootstrap/app.go -- and a channel
// whose table nobody created fails on the first notification it sends.
func init() { migrations.Register(notifications.CreateNotificationsTable{}) }
