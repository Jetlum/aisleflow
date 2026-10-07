// Package store supplies in-memory and Ent/PostgreSQL adapters.
package store

import (
	"context"
	"sort"
	"sync"
	"time"

	"example.com/aisleflow/backend/analytics/core"
)

type Memory struct {
	mu              sync.Mutex
	events          map[string]bool
	alerts          map[string]core.Alert
	delivered, dead map[string]bool
	retry           map[string]time.Time
}

func NewMemory() *Memory {
	return &Memory{events: map[string]bool{}, alerts: map[string]core.Alert{}, delivered: map[string]bool{}, dead: map[string]bool{}, retry: map[string]time.Time{}}
}
func key(tenant, id string) string { return tenant + "/" + id }
func (m *Memory) Commit(ctx context.Context, o core.Observation, a *core.Alert) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	k := key(o.Tenant, o.EventID)
	if m.events[k] {
		return false, nil
	}
	m.events[k] = true
	if a != nil {
		m.alerts[key(o.Tenant, a.AlertID)] = *a
	}
	return true, nil
}
func (m *Memory) Pending(ctx context.Context, tenant string, limit int) ([]core.Alert, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []core.Alert
	for k, a := range m.alerts {
		if a.TenantID == tenant && !m.delivered[k] && !m.dead[k] && !time.Now().Before(m.retry[k]) {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AlertID < out[j].AlertID })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, ctx.Err()
}
func (m *Memory) Delivered(_ context.Context, tenant, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.delivered[key(tenant, id)] = true
	return nil
}
func (m *Memory) Failed(_ context.Context, tenant, id, message string, attempt int, dead bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := key(tenant, id)
	a := m.alerts[k]
	a.Attempts = attempt
	m.alerts[k] = a
	m.dead[k] = dead
	m.retry[k] = time.Now().Add(RetryDelay(attempt))
	return nil
}
func RetryDelay(attempt int) time.Duration {
	if attempt > 6 {
		attempt = 6
	}
	return time.Second * time.Duration(1<<uint(attempt))
}
