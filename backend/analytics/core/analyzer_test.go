package core_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"example.com/aisleflow/backend/analytics/core"
	"example.com/aisleflow/backend/analytics/store"
	"example.com/aisleflow/backend/common/contracts"
	"github.com/stretchr/testify/require"
)

func config() core.Config {
	return core.Config{Baselines: map[core.Key]core.Baseline{{Tenant: "a", Site: "s", Aisle: "A01"}: {Mean: 10, StdDev: 2}, {Tenant: "b", Site: "s", Aisle: "A01"}: {Mean: 100, StdDev: 2}}, Window: 3, Sigma: 2.5, Cooldown: time.Minute, WindowTTL: time.Minute, MaxKeys: 100}
}
func obs(tenant, event string, seconds float64) core.Observation {
	return core.Observation{Tenant: tenant, Site: "s", Aisle: "A01", WorkflowID: contracts.WorkflowID(tenant, "w"), RunID: "run", TaskID: event, EventID: event, Seconds: seconds, At: time.Now()}
}
func TestWindowDedupTenantIsolationAndCooldown(t *testing.T) {
	s := store.NewMemory()
	a, err := core.New(config(), s)
	require.NoError(t, err)
	ctx := context.Background()
	for _, o := range []core.Observation{obs("a", "one", 20), obs("a", "one", 20), obs("b", "one", 20), obs("a", "two", 20)} {
		alert, err := a.Observe(ctx, o)
		require.NoError(t, err)
		require.False(t, alert)
	}
	alert, err := a.Observe(ctx, obs("a", "three", 20))
	require.NoError(t, err)
	require.True(t, alert)
	alert, err = a.Observe(ctx, obs("a", "four", 30))
	require.NoError(t, err)
	require.False(t, alert)
	items, err := s.Pending(ctx, "a", 10)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, 15.0, items[0].Threshold)
	items, err = s.Pending(ctx, "b", 10)
	require.NoError(t, err)
	require.Empty(t, items)
}
func TestExactThresholdAndMissingBaseline(t *testing.T) {
	s := store.NewMemory()
	a, err := core.New(config(), s)
	require.NoError(t, err)
	for i := 0; i < 3; i++ {
		fired, err := a.Observe(context.Background(), obs("a", fmt.Sprint(i), 15))
		require.NoError(t, err)
		require.Equal(t, i == 2, fired)
	}
	_, err = a.Observe(context.Background(), obs("c", "one", 10))
	require.ErrorIs(t, err, core.ErrInvalid)
	for _, v := range []float64{-1, 0, math.NaN(), math.Inf(1), 3601} {
		_, err = a.Observe(context.Background(), obs("a", "bad", v))
		require.ErrorIs(t, err, core.ErrInvalid)
	}
}

type failingStore struct {
	core.Store
	fail bool
}

func (s *failingStore) Commit(ctx context.Context, o core.Observation, a *core.Alert) (bool, error) {
	if s.fail {
		return false, errors.New("db unavailable")
	}
	return s.Store.Commit(ctx, o, a)
}
func TestCommitFailureDoesNotAdvanceWindow(t *testing.T) {
	s := &failingStore{Store: store.NewMemory(), fail: true}
	a, err := core.New(config(), s)
	require.NoError(t, err)
	_, err = a.Observe(context.Background(), obs("a", "lost", 100))
	require.Error(t, err)
	s.fail = false
	for i := 0; i < 2; i++ {
		fired, err := a.Observe(context.Background(), obs("a", fmt.Sprint(i), 20))
		require.NoError(t, err)
		require.False(t, fired)
	}
}
func TestConcurrentDeliveryDeduplicates(t *testing.T) {
	s := store.NewMemory()
	a, err := core.New(config(), s)
	require.NoError(t, err)
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := a.Observe(context.Background(), obs("a", "same", 30))
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	items, err := s.Pending(context.Background(), "a", 10)
	require.NoError(t, err)
	require.Empty(t, items)
}

type signaler struct {
	err   error
	calls int
}

func (s *signaler) Signal(context.Context, contracts.Reroute) error { s.calls++; return s.err }
func TestOutboxSuccessAndDeadLetter(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			s := store.NewMemory()
			ctx := context.Background()
			_, err := s.Commit(ctx, obs("a", "event", 30), &core.Alert{Reroute: contracts.Reroute{TenantID: "a", AlertID: "alert"}})
			require.NoError(t, err)
			sig := &signaler{}
			if fail {
				sig.err = errors.New("closed workflow")
			}
			d := core.Dispatcher{Store: s, Signaler: sig, Tenants: []string{"a"}, MaxAttempts: 1}
			err = d.Dispatch(ctx)
			require.Equal(t, fail, err != nil)
			require.Equal(t, 1, sig.calls)
			require.NoError(t, d.Dispatch(ctx))
			require.Equal(t, 1, sig.calls)
		})
	}
}
