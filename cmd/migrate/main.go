// migrate is a local-development bootstrap. Production needs reviewed versioned migrations.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"example.com/aisleflow/backend/analytics/ent/gen"
	"example.com/aisleflow/backend/common/config"
	_ "github.com/lib/pq"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	dsn, err := config.Required("MIGRATION_DATABASE_URL")
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	c := gen.NewClient(gen.Driver(entsql.OpenDB(dialect.Postgres, db)))
	if err = c.Schema.Create(ctx); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, table := range []string{"observations", "alerts", "process_tasks"} {
		// Identifiers below are compile-time constants, never user input.
		for _, query := range []string{
			"ALTER TABLE " + table + " ENABLE ROW LEVEL SECURITY",
			"ALTER TABLE " + table + " FORCE ROW LEVEL SECURITY",
			"DROP POLICY IF EXISTS tenant_isolation ON " + table,
			"CREATE POLICY tenant_isolation ON " + table + " USING (tenant_id = current_setting('app.tenant_id', true)) WITH CHECK (tenant_id = current_setting('app.tenant_id', true))",
			"GRANT SELECT, INSERT ON " + table + " TO aisleflow_app",
		} {
			if _, err = tx.ExecContext(ctx, query); err != nil {
				return err
			}
		}
	}
	if _, err = tx.ExecContext(ctx, "GRANT UPDATE (delivered_at, last_error, attempts, next_attempt_at, dead) ON alerts TO aisleflow_app"); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	fmt.Println("Ent schema and forced tenant RLS ready")
	return nil
}
