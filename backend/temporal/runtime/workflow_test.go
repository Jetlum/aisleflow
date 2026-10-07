package runtime_test

import (
	"context"
	"testing"
	"time"

	"example.com/aisleflow/backend/common/contracts"
	"example.com/aisleflow/backend/temporal/generated"
	flowruntime "example.com/aisleflow/backend/temporal/runtime"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
)

func TestSignalValidationAndDuplicateSuppression(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	i := contracts.Input{TenantID: "demo", SiteID: "hall", WaveID: "w", Variables: map[string]string{"service": "standard"}}
	id := contracts.WorkflowID(i.TenantID, i.WaveID)
	env.SetStartWorkflowOptions(client.StartWorkflowOptions{ID: id})
	env.RegisterActivityWithOptions(func(ctx context.Context, m contracts.Movement) error {
		if m.TaskID == "pick_01" {
			r := contracts.Reroute{AlertID: "good", TenantID: i.TenantID, SiteID: i.SiteID, WorkflowID: id, RunID: activity.GetInfo(ctx).WorkflowExecution.RunID, Aisle: "A01"}
			bad := r
			bad.TenantID = "other"
			env.SignalWorkflow(contracts.SignalName, bad)
			bad = r
			bad.RunID = "old-run"
			env.SignalWorkflow(contracts.SignalName, bad)
			bad = r
			bad.SiteID = "other-site"
			env.SignalWorkflow(contracts.SignalName, bad)
			bad = r
			bad.WorkflowID = "wrong-workflow"
			env.SignalWorkflow(contracts.SignalName, bad)
			env.SignalWorkflow(contracts.SignalName, r)
			env.SignalWorkflow(contracts.SignalName, r)
		}
		return nil
	}, activity.RegisterOptions{Name: flowruntime.MoveActivity})
	env.ExecuteWorkflow(generated.PickingWorkflow, i)
	require.NoError(t, env.GetWorkflowError())
	var result contracts.Result
	require.NoError(t, env.GetWorkflowResult(&result))
	require.Equal(t, []string{"good"}, result.AcceptedAlerts)
	require.Equal(t, "A01", result.Aisles[0])
	require.Equal(t, "A02", result.Aisles[1])
	require.Equal(t, "PACK", result.Aisles[8])
}
func TestNoApprovedAlternativeStopsWorkflow(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	i := contracts.Input{TenantID: "demo", SiteID: "hall", WaveID: "w"}
	id := contracts.WorkflowID("demo", "w")
	env.SetStartWorkflowOptions(client.StartWorkflowOptions{ID: id})
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(contracts.SignalName, contracts.Reroute{AlertID: "stop", TenantID: "demo", SiteID: "hall", WorkflowID: id, RunID: "default-test-run-id", Aisle: "A01"})
	}, 0)
	env.ExecuteWorkflow(func(ctx workflow.Context, i contracts.Input) (contracts.Result, error) {
		if err := workflow.Sleep(ctx, time.Millisecond); err != nil {
			return contracts.Result{}, err
		}
		return flowruntime.Run(ctx, i, []contracts.Step{{ID: "one", Aisle: "A01"}})
	}, i)
	require.ErrorContains(t, env.GetWorkflowError(), "no approved unblocked route")
}
func TestResolveRejectsMissingBranch(t *testing.T) {
	_, err := generated.Resolve(contracts.Input{TenantID: "demo", SiteID: "hall", WaveID: "w"})
	require.ErrorContains(t, err, "no branch")
}
