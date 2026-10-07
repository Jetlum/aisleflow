// Package runtime contains deterministic workflow orchestration only.
package runtime

import (
	"fmt"
	"time"

	"example.com/aisleflow/backend/common/contracts"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const MoveActivity = "aisleflow.move.v1"

func Run(ctx workflow.Context, input contracts.Input, steps []contracts.Step) (contracts.Result, error) {
	var result contracts.Result
	info := workflow.GetInfo(ctx)
	if info.WorkflowExecution.ID != contracts.WorkflowID(input.TenantID, input.WaveID) {
		return result, fmt.Errorf("workflow ID does not match tenant and wave")
	}
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{StartToCloseTimeout: 30 * time.Second, ScheduleToCloseTimeout: 2 * time.Minute, RetryPolicy: &temporal.RetryPolicy{InitialInterval: time.Second, MaximumAttempts: 3}})
	ch := workflow.GetSignalChannel(ctx, contracts.SignalName)
	seen := map[string]bool{}
	blocked := map[string]bool{}
	allowed := map[string]bool{}
	for _, step := range steps {
		allowed[step.Aisle] = true
		if step.Alternative != "" {
			allowed[step.Alternative] = true
		}
	}
	for _, step := range steps {
		// Signals may arrive while an activity runs. Apply only at the next safe boundary.
		for {
			var signal contracts.Reroute
			if !ch.ReceiveAsync(&signal) {
				break
			}
			if signal.TenantID != input.TenantID || signal.SiteID != input.SiteID || signal.WorkflowID != info.WorkflowExecution.ID || signal.RunID != info.WorkflowExecution.RunID || signal.AlertID == "" || seen[signal.AlertID] || !allowed[signal.Aisle] {
				continue
			}
			seen[signal.AlertID] = true
			blocked[signal.Aisle] = true
			result.AcceptedAlerts = append(result.AcceptedAlerts, signal.AlertID)
		}
		aisle := step.Aisle
		if blocked[aisle] {
			if step.Alternative == "" || blocked[step.Alternative] {
				return result, temporal.NewNonRetryableApplicationError("no approved unblocked route", "RouteUnavailable", nil)
			}
			aisle = step.Alternative
		}
		if err := workflow.ExecuteActivity(ctx, MoveActivity, contracts.Movement{Input: input, TaskID: step.ID, Aisle: aisle}).Get(ctx, nil); err != nil {
			return result, err
		}
		result.Completed = append(result.Completed, step.ID)
		result.Aisles = append(result.Aisles, aisle)
	}
	return result, nil
}
