package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/arandu-io/framework/data"

	appconfig "github.com/arandu-io/arandu/config"
)

// The migration commands mirror Laravel's, because the steps of a deploy should
// be the ones a developer already knows: migrate, migrate:rollback,
// migrate:status, migrate:fresh.
//
// What is deliberately different: there is no schema builder. A migration is
// SQL, written once in the portable subset every supported database shares, and
// what you read is what runs. Laravel needs a Blueprint because Eloquent hides
// the database; here the point is that nothing hides it.

func migrate(ctx context.Context, db *data.DB, migrations []data.Migration) error {
	applied, err := data.Migrate(ctx, db, migrations)
	if err != nil {
		return err
	}
	if len(applied) == 0 {
		fmt.Println("nothing to migrate")
		return nil
	}
	for _, id := range applied {
		fmt.Println("applied", id)
	}
	return nil
}

func rollback(ctx context.Context, db *data.DB, migrations []data.Migration) error {
	reverted, err := data.Rollback(ctx, db, migrations)
	if err != nil {
		return err
	}
	if len(reverted) == 0 {
		fmt.Println("nothing to roll back")
		return nil
	}
	for _, id := range reverted {
		fmt.Println("rolled back", id)
	}
	return nil
}

func migrateStatus(ctx context.Context, db *data.DB, migrations []data.Migration) error {
	status, err := data.Status(ctx, db, migrations)
	if err != nil {
		return err
	}
	if len(status) == 0 {
		fmt.Println("no migrations declared")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "MIGRATION\tBATCH\tAPPLIED AT")
	for _, s := range status {
		if s.Batch == 0 {
			fmt.Fprintf(w, "%s\t-\tpending\n", s.ID)
			continue
		}
		fmt.Fprintf(w, "%s\t%d\t%s\n", s.ID, s.Batch, s.AppliedAt.Format("2006-01-02 15:04:05"))
	}
	return w.Flush()
}

// fresh rolls everything back and applies it again.
//
// It refuses to run outside development. Laravel guards migrate:fresh with a
// confirmation prompt in production; a framework whose thesis is that the
// compiler enforces the rules should not rely on someone reading a prompt at
// 3am, so this one simply does not run there.
func fresh(ctx context.Context, cfg appconfig.Config, db *data.DB, migrations []data.Migration) error {
	if !cfg.App.IsDev() {
		return fmt.Errorf("migrate:fresh drops every table and only runs with APP_ENV=dev (this is %s)", cfg.App.Env)
	}

	for {
		reverted, err := data.Rollback(ctx, db, migrations)
		if err != nil {
			return err
		}
		if len(reverted) == 0 {
			break
		}
		for _, id := range reverted {
			fmt.Println("rolled back", id)
		}
	}
	return migrate(ctx, db, migrations)
}
