package transport

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/aisleflow/backend/analytics/core"
	"example.com/aisleflow/backend/common/contracts"
	"github.com/stretchr/testify/require"
	collector "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	common "go.opentelemetry.io/proto/otlp/common/v1"
	trace "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type observer struct {
	calls int
	err   error
}

func (o *observer) Observe(context.Context, core.Observation) (bool, error) {
	o.calls++
	return false, o.err
}
func span() *trace.Span {
	now := time.Now()
	s := &trace.Span{Name: contracts.MovementSpan, TraceId: []byte("1234567890123456"), SpanId: []byte("12345678"), StartTimeUnixNano: uint64(now.Add(-10 * time.Second).UnixNano()), EndTimeUnixNano: uint64(now.UnixNano())}
	for _, p := range [][2]string{{"tenant.id", "demo"}, {"warehouse.site", "s"}, {"warehouse.aisle", "A01"}, {"warehouse.task", "pick_01"}, {"warehouse.wave", "w"}, {"temporal.workflow_id", "picking/demo/w"}, {"temporal.run_id", "run-1"}} {
		s.Attributes = append(s.Attributes, &common.KeyValue{Key: p[0], Value: &common.AnyValue{Value: &common.AnyValue_StringValue{StringValue: p[1]}}})
	}
	return s
}
func request(s *trace.Span) *collector.ExportTraceServiceRequest {
	return &collector.ExportTraceServiceRequest{ResourceSpans: []*trace.ResourceSpans{{ScopeSpans: []*trace.ScopeSpans{{Spans: []*trace.Span{s}}}}}}
}
func TestExportRejectsForgedTenantAndMalformedSpans(t *testing.T) {
	for name, mutate := range map[string]func(*trace.Span){
		"tenant": func(s *trace.Span) {
			s.Attributes[0].Value = &common.AnyValue{Value: &common.AnyValue_StringValue{StringValue: "other"}}
		},
		"duration": func(s *trace.Span) { s.EndTimeUnixNano = s.StartTimeUnixNano - 1 },
		"old": func(s *trace.Span) {
			s.EndTimeUnixNano = uint64(time.Now().Add(-time.Hour).UnixNano())
			s.StartTimeUnixNano = s.EndTimeUnixNano - 1
		},
		"duplicate_attribute": func(s *trace.Span) { s.Attributes = append(s.Attributes, s.Attributes[0]) },
		"identity":            func(s *trace.Span) { s.TraceId = nil },
		"error":               func(s *trace.Span) { s.Status = &trace.Status{Code: trace.Status_STATUS_CODE_ERROR} },
	} {
		t.Run(name, func(t *testing.T) {
			o := &observer{}
			r := Receiver{Analyzer: o, Tokens: map[string]string{"secret": "demo"}}
			s := span()
			mutate(s)
			resp, err := r.Export(metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-api-key", "secret")), request(s))
			require.NoError(t, err)
			require.EqualValues(t, 1, resp.PartialSuccess.RejectedSpans)
			require.Zero(t, o.calls)
		})
	}
}
func TestAuthAndRetryableStoreFailure(t *testing.T) {
	o := &observer{err: errors.New("db failed")}
	r := Receiver{Analyzer: o, Tokens: map[string]string{"secret": "demo"}}
	_, err := r.Export(context.Background(), request(span()))
	require.Equal(t, codes.Unauthenticated, status.Code(err))
	require.Zero(t, o.calls)
	_, err = r.Export(metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-api-key", "secret")), request(span()))
	require.Equal(t, codes.Unavailable, status.Code(err))
	require.Equal(t, 1, o.calls)
}
func TestLogicalDedupKeyIgnoresSpanID(t *testing.T) {
	s := span()
	a, err := decode(s, "demo")
	require.NoError(t, err)
	s.SpanId = []byte("87654321")
	b, err := decode(s, "demo")
	require.NoError(t, err)
	require.Equal(t, a.EventID, b.EventID)
}
