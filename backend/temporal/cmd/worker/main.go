package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"example.com/aisleflow/backend/analytics/store"
	"example.com/aisleflow/backend/common/config"
	"example.com/aisleflow/backend/common/contracts"
	"example.com/aisleflow/backend/temporal/activities"
	"example.com/aisleflow/backend/temporal/generated"
	flowruntime "example.com/aisleflow/backend/temporal/runtime"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

func main() {
	if err := run(); err != nil {
		slog.Error("worker stopped", "error", err)
		os.Exit(1)
	}
}
func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	dsn, err := config.Required("DATABASE_URL")
	if err != nil {
		return err
	}
	repo, err := store.Open(ctx, dsn)
	if err != nil {
		return err
	}
	defer repo.DB.Close()
	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint(config.Env("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:14317")), otlptracegrpc.WithInsecure())
	if err != nil {
		return err
	}
	provider := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter, sdktrace.WithBatchTimeout(100*time.Millisecond)), sdktrace.WithResource(resource.NewSchemaless(attribute.String("service.name", "aisleflow-worker"))), sdktrace.WithSampler(sdktrace.AlwaysSample()))
	defer func() {
		c, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if err := provider.Shutdown(c); err != nil {
			slog.Warn("trace flush failed", "error", err)
		}
	}()
	tc, err := client.DialContext(ctx, client.Options{HostPort: config.Env("TEMPORAL_ADDRESS", "localhost:7233")})
	if err != nil {
		return err
	}
	defer tc.Close()
	w := worker.New(tc, contracts.TaskQueue, worker.Options{})
	w.RegisterWorkflow(generated.PickingWorkflow)
	a := &activities.Movement{Tracer: provider.Tracer("aisleflow/movements"), Receipts: repo, Delay: time.Second}
	w.RegisterActivityWithOptions(a.Move, activity.RegisterOptions{Name: flowruntime.MoveActivity})
	slog.Info("worker ready", "queue", contracts.TaskQueue)
	return w.Run(worker.InterruptCh())
}
