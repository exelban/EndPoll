package monitor

import (
	"context"
	"testing"
	"time"

	"github.com/exelban/EndPoll/store"
	"github.com/exelban/EndPoll/types"
	"github.com/stretchr/testify/require"
)

func TestMonitor_StatsCache(t *testing.T) {
	ctx := context.Background()
	interval := time.Hour
	m := Monitor{
		Store:    store.NewMemory(ctx),
		CacheTTL: time.Hour,
		watchers: map[string]*watcher{
			"a": {host: &types.Host{ID: "a", URL: "a", Interval: &interval}, status: types.UP},
		},
	}

	first, err := m.Stats(ctx)
	require.NoError(t, err)
	second, err := m.Stats(ctx)
	require.NoError(t, err)
	require.Same(t, first, second, "must be served from the cache")

	byID1, err := m.StatsByID(ctx, "a", false)
	require.NoError(t, err)
	byID2, err := m.StatsByID(ctx, "a", false)
	require.NoError(t, err)
	require.Same(t, byID1, byID2)

	m.cache.reset()
	third, err := m.Stats(ctx)
	require.NoError(t, err)
	require.NotSame(t, first, third, "reset must drop the cache")

	t.Run("disabled with zero ttl", func(t *testing.T) {
		m.CacheTTL = 0
		m.cache.reset()
		a, _ := m.Stats(ctx)
		b, _ := m.Stats(ctx)
		require.NotSame(t, a, b)
	})

	t.Run("expires", func(t *testing.T) {
		c := &cache{}
		c.set("k", 1, 10*time.Millisecond)
		_, ok := c.get("k")
		require.True(t, ok)
		time.Sleep(20 * time.Millisecond)
		_, ok = c.get("k")
		require.False(t, ok)
	})
}

// a recorded check must be visible immediately, not after the cache ttl
func TestMonitor_CacheInvalidatedByCheck(t *testing.T) {
	ts, status, shutdown := srv(0)
	defer shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m := &Monitor{Store: store.NewMemory(ctx), CacheTTL: time.Hour}
	require.NoError(t, m.Run(ctx, cacheTestCfg(ts.URL)))
	time.Sleep(60 * time.Millisecond)

	s, err := m.Stats(ctx)
	require.NoError(t, err)
	require.Equal(t, types.UP, s.Status)

	status.Store(false)
	require.Eventually(t, func() bool {
		s, err := m.Stats(ctx)
		return err == nil && s.Status == types.DOWN
	}, 3*time.Second, 20*time.Millisecond)

	snaps := m.Snapshots()
	require.Len(t, snaps, 1)
	require.Equal(t, types.DOWN, snaps[0].Status)
	require.NotNil(t, snaps[0].LastResponse)
	require.False(t, snaps[0].LastCheck.IsZero())
	m.Stop()
}

func cacheTestCfg(url string) *types.Cfg {
	interval := 20 * time.Millisecond
	timeout := 100 * time.Millisecond
	h := &types.Host{
		URL:              url,
		Interval:         &interval,
		TimeoutInterval:  &timeout,
		SuccessThreshold: 1,
		FailureThreshold: 1,
		Conditions:       &types.Success{Code: []int{200}},
	}
	h.ID = h.GenerateID()
	h.Type = h.GetType()
	return &types.Cfg{MaxConn: 8, Hosts: []*types.Host{h}}
}
