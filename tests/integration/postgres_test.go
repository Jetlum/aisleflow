//go:build integration

package integration

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"example.com/aisleflow/backend/analytics/core"
	"example.com/aisleflow/backend/analytics/store"
	"example.com/aisleflow/backend/common/contracts"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestPostgresAtomicOutboxAndRLS(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	require.NotEmpty(t, dsn, "set TEST_DATABASE_URL to the non-superuser runtime connection")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	p, err := store.Open(ctx, dsn)
	require.NoError(t, err)
	defer p.DB.Close()
	tenant := "test-" + uuid.NewString()
	other := "test-" + uuid.NewString()
	o := core.Observation{Tenant: tenant, Site: "hall", Aisle: "A01", WorkflowID: contracts.WorkflowID(tenant, "wave"), RunID: uuid.NewString(), TaskID: "pick_01", EventID: uuid.NewString(), Seconds: 30, At: time.Now()}
	a := &core.Alert{Reroute: contracts.Reroute{AlertID: uuid.NewString(), TenantID: tenant, SiteID: o.Site, Aisle: o.Aisle, WorkflowID: o.WorkflowID, RunID: o.RunID}, Mean: 30, Threshold: 15}
	inserted, err := p.Commit(ctx, o, a)
	require.NoError(t, err)
	require.True(t, inserted)
	inserted, err = p.Commit(ctx, o, a)
	require.NoError(t, err)
	require.False(t, inserted)
	movement := contracts.Movement{Input: contracts.Input{TenantID: tenant, SiteID: "hall", WaveID: "wave"}, TaskID: "pick_01", Aisle: "A01"}
	require.NoError(t, p.Receipt(ctx, movement, o.WorkflowID, o.RunID))
	require.NoError(t, p.Receipt(ctx, movement, o.WorkflowID, o.RunID), "activity retries must reuse the generated task receipt")
	items, err := p.Pending(ctx, tenant, 10)
	require.NoError(t, err)
	require.Len(t, items, 1)
	items, err = p.Pending(ctx, other, 10)
	require.NoError(t, err)
	require.Empty(t, items)
	// Deliberately omit the tenant WHERE clause; RLS must still filter the data.
	tx, err := p.DB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, "SELECT set_config('app.tenant_id',$1,true)", other)
	require.NoError(t, err)
	var count int
	require.NoError(t, tx.QueryRowContext(ctx, "SELECT count(*) FROM observations WHERE event_id=$1", o.EventID).Scan(&count))
	require.Zero(t, count)
	_, err = tx.ExecContext(ctx, "INSERT INTO observations (id,tenant_id,created_at,event_id,site_id,aisle,workflow_id,run_id,task_id,seconds,observed_at) VALUES ($1,$2,now(),$3,'hall','A01','w','r','t',10,now())", uuid.NewString(), tenant, uuid.NewString())
	require.Error(t, err, "WITH CHECK must reject another tenant's insert")
	require.NoError(t, tx.Rollback())
	// Force an alert failure after observation insertion; neither may commit.
	bad := *a
	bad.TenantID = other
	failed := o
	failed.EventID = uuid.NewString()
	_, err = p.Commit(ctx, failed, &bad)
	require.Error(t, err)
	inserted, err = p.Commit(ctx, failed, nil)
	require.NoError(t, err)
	require.True(t, inserted, "failed transaction must not leave an observation behind")
	// Reopening the repository demonstrates durable pending delivery after process restart.
	p2, err := store.Open(ctx, dsn)
	require.NoError(t, err)
	defer p2.DB.Close()
	items, err = p2.Pending(ctx, tenant, 10)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.NoError(t, p2.Delivered(ctx, tenant, a.AlertID))
	items, err = p.Pending(ctx, tenant, 10)
	require.NoError(t, err)
	require.Empty(t, items)
	// No tenant setting on a pooled connection must expose no rows.
	var id string
	err = p.DB.QueryRowContext(ctx, "SELECT event_id FROM observations WHERE event_id=$1", o.EventID).Scan(&id)
	require.ErrorIs(t, err, sql.ErrNoRows)
}
