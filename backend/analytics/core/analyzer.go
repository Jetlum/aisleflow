// Package core provides transport-independent congestion detection and outbox delivery.
package core

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"example.com/aisleflow/backend/common/contracts"
)

var ErrCapacity = errors.New("detector key capacity exhausted")
var ErrInvalid = errors.New("invalid movement")

type Key struct{ Tenant, Site, Aisle string }
type Baseline struct{ Mean, StdDev float64 }
type Config struct {
	Baselines map[Key]Baseline
	Window    int
	Sigma     float64
	Cooldown  time.Duration
	WindowTTL time.Duration
	MaxKeys   int
}

type Observation struct {
	Tenant, Site, Aisle, WorkflowID, RunID, TaskID, EventID string
	Seconds                                                 float64
	At                                                      time.Time
}
type Alert struct {
	contracts.Reroute
	Mean, Threshold float64
	Attempts        int
}

type Store interface {
	// Commit atomically deduplicates an observation and inserts its optional outbox alert.
	// False means this event was already committed; the detector must not change state.
	Commit(context.Context, Observation, *Alert) (bool, error)
	Pending(context.Context, string, int) ([]Alert, error)
	Delivered(context.Context, string, string) error
	Failed(context.Context, string, string, string, int, bool) error
}

type windowKey struct {
	Key
	WorkflowID, RunID string
}
type window struct {
	values              []float64
	lastAlert, lastSeen time.Time
}
type Analyzer struct {
	mu      sync.Mutex
	cfg     Config
	store   Store
	windows map[windowKey]window
	now     func() time.Time
}

func New(cfg Config, store Store) (*Analyzer, error) {
	if cfg.Window < 2 || cfg.Window > 100 || cfg.Sigma <= 0 || math.IsNaN(cfg.Sigma) || math.IsInf(cfg.Sigma, 0) || cfg.Cooldown <= 0 || cfg.WindowTTL <= 0 || cfg.MaxKeys <= 0 || store == nil {
		return nil, fmt.Errorf("invalid analyzer configuration")
	}
	baselines := make(map[Key]Baseline, len(cfg.Baselines))
	for k, b := range cfg.Baselines {
		if !contracts.ValidID(k.Tenant) || !contracts.ValidID(k.Site) || !contracts.ValidID(k.Aisle) || b.Mean <= 0 || b.StdDev <= 0 || math.IsNaN(b.Mean) || math.IsNaN(b.StdDev) || math.IsInf(b.Mean+cfg.Sigma*b.StdDev, 0) {
			return nil, fmt.Errorf("invalid baseline for %+v", k)
		}
		baselines[k] = b
	}
	cfg.Baselines = baselines
	return &Analyzer{cfg: cfg, store: store, windows: map[windowKey]window{}, now: time.Now}, nil
}

// Observe is serialized per process so commit and rolling-window state advance together.
// Deploy one analyzer replica; multi-replica operation requires partition ownership.
func (a *Analyzer) Observe(ctx context.Context, o Observation) (bool, error) {
	if !contracts.ValidID(o.Tenant) || !contracts.ValidID(o.Site) || !contracts.ValidID(o.Aisle) || !contracts.ValidID(o.TaskID) || o.EventID == "" || o.RunID == "" || o.Seconds <= 0 || o.Seconds > 3600 || math.IsNaN(o.Seconds) || math.IsInf(o.Seconds, 0) {
		return false, ErrInvalid
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.now()
	if o.At.IsZero() || o.At.After(now.Add(time.Minute)) || o.At.Before(now.Add(-5*time.Minute)) {
		return false, ErrInvalid
	}
	k := windowKey{Key: Key{o.Tenant, o.Site, o.Aisle}, WorkflowID: o.WorkflowID, RunID: o.RunID}
	b, ok := a.cfg.Baselines[k.Key]
	if !ok {
		return false, fmt.Errorf("%w: no baseline for tenant/site/aisle", ErrInvalid)
	}
	for key, w := range a.windows {
		if now.Sub(w.lastSeen) > a.cfg.WindowTTL {
			delete(a.windows, key)
		}
	}
	w, exists := a.windows[k]
	if !exists && len(a.windows) >= a.cfg.MaxKeys {
		return false, ErrCapacity
	}
	// Clone before persistence: a failed commit must leave the live window untouched.
	values := append(append([]float64(nil), w.values...), o.Seconds)
	if len(values) > a.cfg.Window {
		values = values[len(values)-a.cfg.Window:]
	}
	threshold := b.Mean + a.cfg.Sigma*b.StdDev
	var alert *Alert
	if len(values) == a.cfg.Window && (w.lastAlert.IsZero() || now.Sub(w.lastAlert) >= a.cfg.Cooldown) {
		mean := 0.0
		for _, v := range values {
			mean += v
		}
		mean /= float64(len(values))
		if mean >= threshold {
			alert = &Alert{Reroute: contracts.Reroute{AlertID: contracts.StableID(o.Tenant, o.EventID, "congestion-v1"), TenantID: o.Tenant, SiteID: o.Site, WorkflowID: o.WorkflowID, RunID: o.RunID, Aisle: o.Aisle}, Mean: mean, Threshold: threshold}
		}
	}
	inserted, err := a.store.Commit(ctx, o, alert)
	if err != nil || !inserted {
		return false, err
	}
	w.values = values
	w.lastSeen = now
	if alert != nil {
		w.lastAlert = now
	}
	a.windows[k] = w
	return alert != nil, nil
}

type Signaler interface {
	Signal(context.Context, contracts.Reroute) error
}
type Dispatcher struct {
	Store       Store
	Signaler    Signaler
	Tenants     []string
	MaxAttempts int
}

func (d Dispatcher) Dispatch(ctx context.Context) error {
	var failures []error
	for _, tenant := range d.Tenants {
		items, err := d.Store.Pending(ctx, tenant, 100)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		for _, a := range items {
			if err := d.Signaler.Signal(ctx, a.Reroute); err != nil {
				dead := a.Attempts+1 >= d.MaxAttempts
				if saveErr := d.Store.Failed(ctx, tenant, a.AlertID, err.Error(), a.Attempts+1, dead); saveErr != nil {
					failures = append(failures, saveErr)
				}
				failures = append(failures, fmt.Errorf("alert %s: %w", a.AlertID, err))
				continue
			}
			if err := d.Store.Delivered(ctx, tenant, a.AlertID); err != nil {
				failures = append(failures, err)
			}
		}
	}
	return errors.Join(failures...)
}
