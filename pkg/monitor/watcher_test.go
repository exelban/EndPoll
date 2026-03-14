package monitor

import (
	"context"
	"crypto/rand"
	"fmt"
	"testing"
	"time"

	"github.com/exelban/EndPoll/pkg/dialer"
	"github.com/exelban/EndPoll/pkg/notify"
	"github.com/exelban/EndPoll/store"
	"github.com/exelban/EndPoll/types"
	"github.com/stretchr/testify/require"
)

func TestWatcher_check(t *testing.T) {
	ts, status, shutdown := srv(0)
	defer shutdown()
	ctx := context.Background()

	ri := 100 * time.Millisecond

	w := &watcher{
		dialer: dialer.New(1),
		notify: &notify.Notify{},
		store:  store.NewMemory(ctx),
		host: &types.Host{
			URL: ts.URL,
			Conditions: &types.Success{
				Code: []int{200},
			},
			SuccessThreshold: 2,
			FailureThreshold: 3,
			Interval:         &ri,
		},
		ctx: ctx,
	}

	w.check()
	require.Equal(t, types.Unknown, w.status)

	w.check()
	require.Equal(t, types.UP, w.status)

	status.Store(false)
	w.check()
	require.Equal(t, types.UP, w.status)
	w.check()
	require.Equal(t, types.UP, w.status)
	w.check()
	require.Equal(t, types.DOWN, w.status)

	status.Store(true)
	w.check()
	require.Equal(t, types.DOWN, w.status)
	w.check()
	require.Equal(t, types.UP, w.status)

	// reach the history limit
	for i := 0; i < 30; i++ {
		w.check()
	}

	status.Store(false)
	w.check()
	require.Equal(t, types.UP, w.status)
	w.check()
	require.Equal(t, types.UP, w.status)
	w.check()
	require.Equal(t, types.DOWN, w.status)

	status.Store(true)
	w.check()
	require.Equal(t, types.DOWN, w.status)
	w.check()
	require.Equal(t, types.UP, w.status)
}

func TestWatcher_validate(t *testing.T) {
	ctx := context.Background()

	t.Run("no thresholds", func(t *testing.T) {
		w := &watcher{
			host:   &types.Host{},
			notify: &notify.Notify{},
			store:  store.NewMemory(ctx),
		}
		w.validate(&types.HttpResponse{
			Status: true,
		})
		require.Equal(t, types.UP, w.status)
		w.validate(&types.HttpResponse{
			Status: false,
		})
		require.Equal(t, types.DOWN, w.status)
		w.validate(&types.HttpResponse{
			Status: true,
		})
		require.Equal(t, types.UP, w.status)
	})

	t.Run("min thresholds", func(t *testing.T) {
		w := &watcher{
			notify: &notify.Notify{},
			store:  store.NewMemory(ctx),
			host: &types.Host{
				ID:               id(),
				SuccessThreshold: 2,
				FailureThreshold: 2,
			},
		}

		w.validate(&types.HttpResponse{
			Status: true,
		})
		require.Equal(t, types.Unknown, w.status)

		w.validate(&types.HttpResponse{
			Status: false,
		})
		require.Equal(t, types.Unknown, w.status)
		w.validate(&types.HttpResponse{
			Status: false,
		})
		require.Equal(t, types.DOWN, w.status)
		w.validate(&types.HttpResponse{
			Status: true,
		})
		require.Equal(t, types.DOWN, w.status)
		w.validate(&types.HttpResponse{
			Status: true,
		})
		require.Equal(t, types.UP, w.status)

		w.validate(&types.HttpResponse{
			Status: false,
		})
		require.Equal(t, types.UP, w.status)
	})

	t.Run("success", func(t *testing.T) {
		w := &watcher{
			notify: &notify.Notify{},
			store:  store.NewMemory(ctx),
			host: &types.Host{
				ID:               id(),
				SuccessThreshold: 3,
				FailureThreshold: 2,
			},
		}

		for i := 0; i < 6; i++ {
			w.validate(&types.HttpResponse{
				Status: false,
			})
		}
		w.validate(&types.HttpResponse{
			Status: true,
		})
		w.validate(&types.HttpResponse{
			Status: true,
		})
		require.Equal(t, types.DOWN, w.status)

		w.validate(&types.HttpResponse{
			Status: true,
		})
		require.Equal(t, types.UP, w.status)

		w.validate(&types.HttpResponse{
			Status: false,
		})
		require.Equal(t, types.UP, w.status)
		w.validate(&types.HttpResponse{
			Status: false,
		})
		require.Equal(t, types.DOWN, w.status)
	})

	t.Run("failure", func(t *testing.T) {
		w := &watcher{
			notify: &notify.Notify{},
			store:  store.NewMemory(ctx),
			host: &types.Host{
				ID:               id(),
				SuccessThreshold: 2,
				FailureThreshold: 3,
			},
		}

		for i := 0; i < 6; i++ {
			w.validate(&types.HttpResponse{
				Status: true,
			})
		}
		w.validate(&types.HttpResponse{
			Status: false,
		})
		w.validate(&types.HttpResponse{
			Status: false,
		})
		require.Equal(t, types.UP, w.status)
		w.validate(&types.HttpResponse{
			Status: false,
		})
		require.Equal(t, types.DOWN, w.status)

		w.validate(&types.HttpResponse{
			Status: true,
		})
		require.Equal(t, types.DOWN, w.status)
	})
}

func TestWatcher_validate_IncidentLifecycle(t *testing.T) {
	ctx := context.Background()
	s := store.NewMemory(ctx)

	w := &watcher{
		notify: &notify.Notify{},
		store:  s,
		ctx:    ctx,
		host: &types.Host{
			ID:               id(),
			SuccessThreshold: 1,
			FailureThreshold: 1,
		},
	}

	// First check: unknown → UP (no notification, no incident)
	w.validate(&types.HttpResponse{Status: true})
	require.Equal(t, types.UP, w.status)
	require.Nil(t, w.incident)

	// UP → DOWN: creates incident
	w.validate(&types.HttpResponse{Status: false, Code: 500, Body: "error", Timestamp: time.Now()})
	require.Equal(t, types.DOWN, w.status)
	require.NotNil(t, w.incident)
	require.Equal(t, 500, w.incident.Details.StatusCode)

	incidents, err := s.FindIncidents(ctx, w.host.ID, 0, 0)
	require.NoError(t, err)
	require.Len(t, incidents, 1)
	require.Nil(t, incidents[0].EndTS)

	// DOWN → UP: ends incident
	time.Sleep(2 * time.Second) // sleep > 1s so incident is ended, not deleted
	w.validate(&types.HttpResponse{Status: true})
	require.Equal(t, types.UP, w.status)
	require.Nil(t, w.incident)

	incidents, err = s.FindIncidents(ctx, w.host.ID, 0, 0)
	require.NoError(t, err)
	require.Len(t, incidents, 1)
	require.NotNil(t, incidents[0].EndTS)
}

func TestWatcher_validate_ShortIncidentDeleted(t *testing.T) {
	ctx := context.Background()
	s := store.NewMemory(ctx)

	w := &watcher{
		notify: &notify.Notify{},
		store:  s,
		ctx:    ctx,
		host: &types.Host{
			ID:               id(),
			SuccessThreshold: 1,
			FailureThreshold: 1,
		},
	}

	// Go UP first
	w.validate(&types.HttpResponse{Status: true})
	require.Equal(t, types.UP, w.status)

	// Go DOWN — creates incident
	w.validate(&types.HttpResponse{Status: false, Timestamp: time.Now()})
	require.Equal(t, types.DOWN, w.status)
	require.NotNil(t, w.incident)

	// Immediately go UP — incident < 1s so it should be deleted
	w.validate(&types.HttpResponse{Status: true})
	require.Equal(t, types.UP, w.status)

	incidents, err := s.FindIncidents(ctx, w.host.ID, 0, 0)
	require.NoError(t, err)
	require.Len(t, incidents, 0) // deleted because < 1 second
}

func TestWatcher_validate_CounterReset(t *testing.T) {
	ctx := context.Background()

	w := &watcher{
		notify: &notify.Notify{},
		store:  store.NewMemory(ctx),
		host: &types.Host{
			ID:               id(),
			SuccessThreshold: 3,
			FailureThreshold: 3,
		},
	}

	// 2 successes then 1 failure should reset success count
	w.validate(&types.HttpResponse{Status: true})
	w.validate(&types.HttpResponse{Status: true})
	require.Equal(t, 2, w.successCount)

	w.validate(&types.HttpResponse{Status: false})
	require.Equal(t, 0, w.successCount)
	require.Equal(t, 1, w.failureCount)

	// 2 failures then 1 success should reset failure count
	w.validate(&types.HttpResponse{Status: false})
	require.Equal(t, 2, w.failureCount)

	w.validate(&types.HttpResponse{Status: true})
	require.Equal(t, 0, w.failureCount)
	require.Equal(t, 1, w.successCount)
}

func TestWatcher_run_InitialDelay(t *testing.T) {
	ts, _, shutdown := srv(0)
	defer shutdown()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ri := 100 * time.Millisecond
	delay := 200 * time.Millisecond

	w := &watcher{
		dialer: dialer.New(1),
		notify: &notify.Notify{},
		store:  store.NewMemory(ctx),
		host: &types.Host{
			URL:          ts.URL,
			InitialDelay: &delay,
			Interval:     &ri,
			Conditions:   &types.Success{Code: []int{200}},
		},
	}

	start := time.Now()
	go w.run(ctx)
	time.Sleep(50 * time.Millisecond)                // well before delay
	require.Equal(t, types.StatusType(""), w.status) // not checked yet

	time.Sleep(250 * time.Millisecond) // after delay + first check
	require.True(t, time.Since(start) >= delay)
	cancel()
}

func TestWatcher_run_RestoredIncident(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ts, _, shutdown := srv(0)
	defer shutdown()

	s := store.NewMemory(ctx)
	hostID := id()

	// Pre-store an open incident
	incident := &types.Incident{
		StartTS: time.Now().Add(-10 * time.Minute),
		Details: types.IncidentDetails{StatusCode: 500},
	}
	require.NoError(t, s.AddIncident(ctx, hostID, incident))

	// Pre-store a last response with DOWN status
	require.NoError(t, s.AddResponse(ctx, hostID, &types.HttpResponse{
		Timestamp:  time.Now().Add(-30 * time.Second),
		StatusType: types.DOWN,
	}))

	ri := 50 * time.Millisecond
	w := &watcher{
		dialer: dialer.New(1),
		notify: &notify.Notify{},
		store:  s,
		host: &types.Host{
			ID:               hostID,
			URL:              ts.URL,
			Interval:         &ri,
			SuccessThreshold: 1,
			FailureThreshold: 1,
			Conditions:       &types.Success{Code: []int{200}},
		},
	}

	go w.run(ctx)
	time.Sleep(100 * time.Millisecond) // let it run and recover

	w.mu.RLock()
	require.Equal(t, types.UP, w.status)
	w.mu.RUnlock()

	// Incident should be ended
	incidents, err := s.FindIncidents(ctx, hostID, 0, 1)
	require.NoError(t, err)
	require.Len(t, incidents, 1)
	require.NotNil(t, incidents[0].EndTS)

	cancel()
}

func id() string {
	n := 12
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return fmt.Sprintf("%X", b)
}
