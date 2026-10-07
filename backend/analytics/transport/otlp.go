// Package transport accepts a constrained, authenticated subset of OTLP traces.
package transport

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"strings"
	"time"

	"example.com/aisleflow/backend/analytics/core"
	"example.com/aisleflow/backend/common/contracts"
	collector "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	common "go.opentelemetry.io/proto/otlp/common/v1"
	trace "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type Observer interface {
	Observe(context.Context, core.Observation) (bool, error)
}
type Receiver struct {
	collector.UnimplementedTraceServiceServer
	Analyzer Observer
	// Tokens bind credentials to tenants. Telemetry attributes are never authentication.
	Tokens map[string]string
}

func (r *Receiver) Export(ctx context.Context, req *collector.ExportTraceServiceRequest) (*collector.ExportTraceServiceResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	md, _ := metadata.FromIncomingContext(ctx)
	values := md.Get("x-api-key")
	tenant := ""
	if len(values) == 1 {
		for token, t := range r.Tokens {
			if subtle.ConstantTimeCompare([]byte(token), []byte(values[0])) == 1 {
				tenant = t
			}
		}
	}
	if tenant == "" {
		return nil, status.Error(codes.Unauthenticated, "invalid telemetry credential")
	}
	count := 0
	for _, rs := range req.GetResourceSpans() {
		for _, ss := range rs.GetScopeSpans() {
			count += len(ss.GetSpans())
		}
	}
	if count > 1000 {
		return nil, status.Error(codes.ResourceExhausted, "maximum 1000 spans per export")
	}
	rejected := int64(0)
	for _, rs := range req.GetResourceSpans() {
		for _, ss := range rs.GetScopeSpans() {
			for _, s := range ss.GetSpans() {
				if s.GetName() != contracts.MovementSpan {
					continue
				} // ordinary traces are intentionally ignored
				o, err := decode(s, tenant)
				if err != nil {
					rejected++
					continue
				}
				if _, err = r.Analyzer.Observe(ctx, o); err != nil {
					if errors.Is(err, core.ErrInvalid) {
						rejected++
						continue
					}
					// Store failures must be retryable. Valid previously committed spans are deduplicated.
					return nil, status.Error(codes.Unavailable, "observation could not be committed")
				}
			}
		}
	}
	resp := &collector.ExportTraceServiceResponse{}
	if rejected > 0 {
		resp.PartialSuccess = &collector.ExportTracePartialSuccess{RejectedSpans: rejected, ErrorMessage: "invalid movement identity, tenant, timestamps, or attributes"}
	}
	return resp, nil
}

func decode(s *trace.Span, tenant string) (core.Observation, error) {
	a, err := attrs(s.GetAttributes())
	if err != nil {
		return core.Observation{}, err
	}
	if a["tenant.id"] != tenant {
		return core.Observation{}, fmt.Errorf("tenant mismatch")
	}
	for _, k := range []string{"tenant.id", "warehouse.site", "warehouse.aisle", "warehouse.task", "warehouse.wave"} {
		if !contracts.ValidID(a[k]) {
			return core.Observation{}, fmt.Errorf("missing/invalid %s", k)
		}
	}
	workflowID := contracts.WorkflowID(tenant, a["warehouse.wave"])
	if a["temporal.workflow_id"] != workflowID {
		return core.Observation{}, fmt.Errorf("workflow mismatch")
	}
	if !contracts.ValidID(a["temporal.run_id"]) {
		return core.Observation{}, fmt.Errorf("invalid run ID")
	}
	if s.Status != nil && s.Status.Code == trace.Status_STATUS_CODE_ERROR {
		return core.Observation{}, fmt.Errorf("failed movement")
	}
	start, end := s.GetStartTimeUnixNano(), s.GetEndTimeUnixNano()
	if start == 0 || end <= start || end > uint64(^uint64(0)>>1) || end-start > uint64(time.Hour) {
		return core.Observation{}, fmt.Errorf("invalid duration")
	}
	at := time.Unix(0, int64(end))
	now := time.Now()
	if at.After(now.Add(time.Minute)) || at.Before(now.Add(-5*time.Minute)) {
		return core.Observation{}, fmt.Errorf("stale/future movement")
	}
	if len(s.TraceId) != 16 || len(s.SpanId) != 8 || allZero(s.TraceId) || allZero(s.SpanId) {
		return core.Observation{}, fmt.Errorf("invalid trace/span IDs")
	}
	// Logical identity survives duplicate exports AND activity retries with new trace IDs.
	eventID := contracts.StableID(tenant, workflowID, a["temporal.run_id"], a["warehouse.task"])
	return core.Observation{Tenant: tenant, Site: a["warehouse.site"], Aisle: a["warehouse.aisle"], WorkflowID: workflowID, RunID: a["temporal.run_id"], TaskID: a["warehouse.task"], EventID: eventID, Seconds: float64(end-start) / 1e9, At: at}, nil
}
func attrs(kvs []*common.KeyValue) (map[string]string, error) {
	a := map[string]string{}
	for _, kv := range kvs {
		if kv == nil {
			continue
		}
		if !(strings.HasPrefix(kv.Key, "warehouse.") || strings.HasPrefix(kv.Key, "tenant.") || strings.HasPrefix(kv.Key, "temporal.")) {
			continue
		}
		if _, ok := a[kv.Key]; ok {
			return nil, fmt.Errorf("duplicate attribute")
		}
		v, ok := kv.Value.GetValue().(*common.AnyValue_StringValue)
		if !ok {
			return nil, fmt.Errorf("string attribute required")
		}
		a[kv.Key] = v.StringValue
	}
	return a, nil
}
func allZero(b []byte) bool {
	for _, v := range b {
		if v != 0 {
			return false
		}
	}
	return true
}
