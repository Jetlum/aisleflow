// Package closedloop exercises the actual OTLP gRPC boundary and generated Temporal
// workflow together. Temporal's SDK test environment substitutes for its server.
package closedloop

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"example.com/aisleflow/backend/analytics/core"
	"example.com/aisleflow/backend/analytics/store"
	"example.com/aisleflow/backend/analytics/transport"
	"example.com/aisleflow/backend/common/contracts"
	"example.com/aisleflow/backend/temporal/activities"
	"example.com/aisleflow/backend/temporal/generated"
	flowruntime "example.com/aisleflow/backend/temporal/runtime"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	collector "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/testsuite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type signals struct {
	env *testsuite.TestWorkflowEnvironment
}

func (s signals) Signal(_ context.Context, r contracts.Reroute) error {
	s.env.SignalWorkflow(contracts.SignalName, r)
	return nil
}

func TestGeneratedWorkflowOTLPCongestionReroute(t *testing.T) {
	ctx := context.Background()
	repo := store.NewMemory()
	baselines := map[core.Key]core.Baseline{}
	for _, aisle := range []string{"A01", "A02", "PACK", "EXPRESS"} {
		baselines[core.Key{Tenant: "demo", Site: "hall-1", Aisle: aisle}] = core.Baseline{Mean: 10, StdDev: 2}
	}
	a, err := core.New(core.Config{Baselines: baselines, Window: 3, Sigma: 2.5, Cooldown: time.Minute, WindowTTL: time.Minute, MaxKeys: 100}, repo)
	require.NoError(t, err)
	lis := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	collector.RegisterTraceServiceServer(server, &transport.Receiver{Analyzer: a, Tokens: map[string]string{"test-token-for-demo": "demo"}})
	go func() { _ = server.Serve(lis) }()
	defer server.Stop()
	conn, err := grpc.NewClient("passthrough:///bufnet", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }))
	require.NoError(t, err)
	defer conn.Close()
	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn), otlptracegrpc.WithHeaders(map[string]string{"x-api-key": "test-token-for-demo"}))
	require.NoError(t, err)
	duplicates := &duplicateExporter{SpanExporter: exporter}
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(duplicates))
	defer func() { require.NoError(t, provider.Shutdown(context.Background())) }()
	mover := &activities.Movement{Tracer: provider.Tracer("closedloop")}
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	input := contracts.Input{TenantID: "demo", SiteID: "hall-1", WaveID: "interview", Variables: map[string]string{"service": "express"}}
	env.SetStartWorkflowOptions(client.StartWorkflowOptions{ID: contracts.WorkflowID(input.TenantID, input.WaveID)})
	dispatcher := core.Dispatcher{Store: repo, Signaler: signals{env}, Tenants: []string{"demo"}, MaxAttempts: 3}
	env.RegisterActivityWithOptions(func(activityCtx context.Context, m contracts.Movement) error {
		if err := mover.Move(activityCtx, m); err != nil {
			return err
		}
		return dispatcher.Dispatch(ctx)
	}, activity.RegisterOptions{Name: flowruntime.MoveActivity})
	env.ExecuteWorkflow(generated.PickingWorkflow, input)
	require.NoError(t, env.GetWorkflowError())
	var result contracts.Result
	require.NoError(t, env.GetWorkflowResult(&result))
	require.Len(t, result.Completed, 9)
	require.Len(t, result.AcceptedAlerts, 1)
	duplicates.mu.Lock()
	require.NoError(t, duplicates.err)
	require.Equal(t, 18, duplicates.exports)
	duplicates.mu.Unlock()
	require.Equal(t, []string{"A01", "A01", "A01", "A01", "A02", "A02", "A02", "A02", "EXPRESS"}, result.Aisles)
	t.Logf("BPMN -> generated workflow -> OTLP gRPC -> rolling mean -> outbox -> signal -> approved alternative: %+v", result)
}

// duplicateExporter injects transport redelivery without changing production activity code.
type duplicateExporter struct {
	sdktrace.SpanExporter
	mu      sync.Mutex
	exports int
	err     error
}

func (e *duplicateExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := 0; i < 2; i++ {
		e.exports++
		if err := e.SpanExporter.ExportSpans(ctx, spans); err != nil {
			e.err = err
			return err
		}
	}
	return nil
}
