package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"example.com/aisleflow/backend/analytics/core"
	"example.com/aisleflow/backend/analytics/ent/gen"
	alertdb "example.com/aisleflow/backend/analytics/ent/gen/alert"
	"example.com/aisleflow/backend/analytics/ent/gen/observation"
	"example.com/aisleflow/backend/analytics/ent/gen/processtask"
	"example.com/aisleflow/backend/common/contracts"
	_ "github.com/lib/pq"
)

type Postgres struct{ DB *sql.DB }

func Open(ctx context.Context, dsn string) (*Postgres, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	if err = db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	// RLS must not be bypassed by runtime credentials.
	var bypass bool
	if err = db.QueryRowContext(ctx, "SELECT rolsuper OR rolbypassrls FROM pg_roles WHERE rolname = current_user").Scan(&bypass); err != nil {
		db.Close()
		return nil, err
	}
	if bypass {
		db.Close()
		return nil, fmt.Errorf("runtime database role must not be superuser or BYPASSRLS")
	}
	return &Postgres{DB: db}, nil
}

// boundDriver keeps all Ent statements on the transaction that owns SET LOCAL.
// Nested Ent transaction requests are no-ops: only withTenant owns commit/rollback.
type boundDriver struct{ entsql.Conn }

func (d boundDriver) Dialect() string                        { return dialect.Postgres }
func (d boundDriver) Close() error                           { return nil }
func (d boundDriver) Tx(context.Context) (dialect.Tx, error) { return boundTx{d.Conn}, nil }

type boundTx struct{ entsql.Conn }

func (boundTx) Commit() error   { return nil }
func (boundTx) Rollback() error { return nil }

func (p *Postgres) withTenant(ctx context.Context, tenant, lock string, fn func(*gen.Client) error) error {
	if !contracts.ValidID(tenant) {
		return fmt.Errorf("invalid tenant")
	}
	tx, err := p.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenant); err != nil {
		return err
	}
	if lock != "" {
		if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", tenant+"/"+lock); err != nil {
			return err
		}
	}
	c := gen.NewClient(gen.Driver(boundDriver{entsql.Conn{ExecQuerier: tx}}))
	if err = fn(c); err != nil {
		return err
	}
	return tx.Commit()
}

func (p *Postgres) Commit(ctx context.Context, o core.Observation, a *core.Alert) (bool, error) {
	inserted := false
	err := p.withTenant(ctx, o.Tenant, o.EventID, func(c *gen.Client) error {
		exists, err := c.Observation.Query().Where(observation.TenantID(o.Tenant), observation.EventID(o.EventID)).Exist(ctx)
		if err != nil || exists {
			return err
		}
		if _, err = c.Observation.Create().SetTenantID(o.Tenant).SetEventID(o.EventID).SetSiteID(o.Site).SetAisle(o.Aisle).SetWorkflowID(o.WorkflowID).SetRunID(o.RunID).SetTaskID(o.TaskID).SetSeconds(o.Seconds).SetObservedAt(o.At).Save(ctx); err != nil {
			return err
		}
		if a != nil {
			if a.TenantID != o.Tenant {
				return fmt.Errorf("alert tenant mismatch")
			}
			if _, err = c.Alert.Create().SetTenantID(a.TenantID).SetAlertID(a.AlertID).SetSiteID(a.SiteID).SetAisle(a.Aisle).SetWorkflowID(a.WorkflowID).SetRunID(a.RunID).SetMeanSeconds(a.Mean).SetThresholdSeconds(a.Threshold).Save(ctx); err != nil {
				return err
			}
		}
		inserted = true
		return nil
	})
	return inserted && err == nil, err
}

func (p *Postgres) Pending(ctx context.Context, tenant string, limit int) ([]core.Alert, error) {
	var out []core.Alert
	err := p.withTenant(ctx, tenant, "", func(c *gen.Client) error {
		rows, err := c.Alert.Query().Where(alertdb.TenantID(tenant), alertdb.DeliveredAtIsNil(), alertdb.Dead(false), alertdb.Or(alertdb.NextAttemptAtIsNil(), alertdb.NextAttemptAtLTE(time.Now()))).Order(gen.Asc(alertdb.FieldCreatedAt)).Limit(limit).All(ctx)
		if err != nil {
			return err
		}
		for _, r := range rows {
			out = append(out, core.Alert{Reroute: contracts.Reroute{AlertID: r.AlertID, TenantID: r.TenantID, SiteID: r.SiteID, Aisle: r.Aisle, WorkflowID: r.WorkflowID, RunID: r.RunID}, Mean: r.MeanSeconds, Threshold: r.ThresholdSeconds, Attempts: r.Attempts})
		}
		return nil
	})
	return out, err
}
func (p *Postgres) Delivered(ctx context.Context, tenant, id string) error {
	return p.withTenant(ctx, tenant, "", func(c *gen.Client) error {
		n, err := c.Alert.Update().Where(alertdb.TenantID(tenant), alertdb.AlertID(id)).SetDeliveredAt(time.Now()).SetLastError("").Save(ctx)
		if err == nil && n != 1 {
			return fmt.Errorf("alert not found")
		}
		return err
	})
}
func (p *Postgres) Failed(ctx context.Context, tenant, id, message string, attempt int, dead bool) error {
	return p.withTenant(ctx, tenant, "", func(c *gen.Client) error {
		if len(message) > 2000 {
			message = message[:2000]
		}
		n, err := c.Alert.Update().Where(alertdb.TenantID(tenant), alertdb.AlertID(id)).SetAttempts(attempt).SetLastError(message).SetDead(dead).SetNextAttemptAt(time.Now().Add(RetryDelay(attempt))).Save(ctx)
		if err == nil && n != 1 {
			return fmt.Errorf("alert not found")
		}
		return err
	})
}

// Receipt records a completed simulated movement. It does not make physical commands exactly-once.
func (p *Postgres) Receipt(ctx context.Context, m contracts.Movement, workflowID, runID string) error {
	return p.withTenant(ctx, m.TenantID, contracts.StableID(workflowID, runID, m.TaskID), func(c *gen.Client) error {
		exists, err := c.ProcessTask.Query().Where(processtask.TenantID(m.TenantID), processtask.WorkflowID(workflowID), processtask.RunID(runID), processtask.TaskIDEQ(processtask.TaskID(m.TaskID))).Exist(ctx)
		if err != nil || exists {
			return err
		}
		_, err = c.ProcessTask.Create().SetTenantID(m.TenantID).SetWorkflowID(workflowID).SetRunID(runID).SetTaskID(processtask.TaskID(m.TaskID)).SetAisle(m.Aisle).Save(ctx)
		return err
	})
}
