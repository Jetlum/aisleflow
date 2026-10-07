// Package activities contains side effects; workflow code itself performs no I/O.
package activities

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"example.com/aisleflow/backend/common/contracts"
	"example.com/aisleflow/backend/temporal/generated"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.temporal.io/sdk/activity"
)

type Receipts interface {
	Receipt(context.Context, contracts.Movement, string, string) error
}
type Movement struct {
	Tracer   trace.Tracer
	Receipts Receipts
	Delay    time.Duration
}

var _ generated.Activities = (*Movement)(nil)

// Move simulates a completed scanner movement. Span duration is synthetic; wall-clock
// delay leaves enough time to show the asynchronous control loop in a short demo.
func (m *Movement) Move(ctx context.Context, input contracts.Movement) error {
	if err := input.Input.Validate(); err != nil {
		return err
	}
	info := activity.GetInfo(ctx)
	if info.WorkflowExecution.ID != contracts.WorkflowID(input.TenantID, input.WaveID) {
		return fmt.Errorf("workflow tenant mismatch")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(m.Delay):
	}
	if m.Receipts != nil {
		if err := m.Receipts.Receipt(ctx, input, info.WorkflowExecution.ID, info.WorkflowExecution.RunID); err != nil {
			return err
		}
	}
	seconds := SyntheticSeconds(input.TaskID, input.Aisle)
	end := time.Now()
	start := end.Add(-time.Duration(seconds * float64(time.Second)))
	_, span := m.Tracer.Start(ctx, contracts.MovementSpan, trace.WithTimestamp(start), trace.WithAttributes(
		attribute.String("tenant.id", input.TenantID), attribute.String("warehouse.site", input.SiteID), attribute.String("warehouse.aisle", input.Aisle), attribute.String("warehouse.task", input.TaskID), attribute.String("warehouse.wave", input.WaveID),
		attribute.String("temporal.workflow_id", info.WorkflowExecution.ID), attribute.String("temporal.run_id", info.WorkflowExecution.RunID), attribute.Bool("demo.synthetic", true),
	))
	span.End(trace.WithTimestamp(end))
	slog.Info("movement completed", "task", input.TaskID, "aisle", input.Aisle, "synthetic_seconds", seconds)
	return nil
}
func SyntheticSeconds(task, aisle string) float64 {
	if aisle != "A01" {
		return 10
	}
	switch task {
	case "pick_01":
		return 9
	case "pick_02":
		return 10
	case "pick_03":
		return 11
	default:
		return 30
	}
}
