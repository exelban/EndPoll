package monitor

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/exelban/EndPoll/internal/dialer"
	"github.com/exelban/EndPoll/internal/notify"
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

type failingIncidentStore struct {
	store.Interface
	failAdd bool
	failEnd bool
}

func (s *failingIncidentStore) AddIncident(ctx context.Context, hostID string, e *types.Incident) error {
	if s.failAdd {
		e.ID = 99
		return errors.New("write failed")
	}
	return s.Interface.AddIncident(ctx, hostID, e)
}

func (s *failingIncidentStore) EndIncident(ctx context.Context, hostID string, id int, ts time.Time) error {
	if s.failEnd {
		return errors.New("write failed")
	}
	return s.Interface.EndIncident(ctx, hostID, id, ts)
}

func TestWatcher_InitialOutageAndIncidentRetries(t *testing.T) {
	ctx := context.Background()
	s := &failingIncidentStore{
		Interface: store.NewMemory(ctx),
		failAdd:   true,
	}

	w := &watcher{
		store: s,
		host: &types.Host{
			ID:               "host",
			SuccessThreshold: 1,
			FailureThreshold: 2,
		},
	}

	failed := &types.HttpResponse{Code: 503, Timestamp: time.Now()}
	require.Empty(t, w.validate(failed))
	require.Equal(t, types.DOWN, w.validate(failed))
	require.Equal(t, types.DOWN, w.status)
	require.NotNil(t, w.incident)
	require.Zero(t, w.incident.ID)

	w.incident.StartTS = time.Now().Add(-time.Minute)
	start := w.incident.StartTS
	s.failAdd = false
	require.Empty(t, w.validate(failed))
	require.Positive(t, w.incident.ID)

	incidents, err := s.FindIncidents(ctx, "host", 0, 0)
	require.NoError(t, err)
	require.Len(t, incidents, 1)
	require.Equal(t, start, incidents[0].StartTS)

	s.failEnd = true
	require.Equal(t, types.UP, w.validate(&types.HttpResponse{Status: true}))
	require.Equal(t, types.UP, w.status)
	require.Nil(t, w.incident)
	require.Len(t, w.pendingIncidents, 1)
	end := *w.pendingIncidents[0].EndTS

	delay := time.Hour
	w.host.InitialDelay = &delay
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	w.loop(cancelled, true)
	require.Nil(t, w.incident)
	require.Equal(t, &end, w.pendingIncidents[0].EndTS)

	s.failEnd = false
	require.Empty(t, w.validate(&types.HttpResponse{Status: true}))
	require.Nil(t, w.incident)
	require.Empty(t, w.pendingIncidents)

	incidents, err = s.FindIncidents(ctx, "host", 0, 0)
	require.NoError(t, err)
	require.Equal(t, end, *incidents[0].EndTS)
}

func TestWatcher_SecondOutageDuringRecoveryWriteFailure(t *testing.T) {
	ctx := context.Background()
	s := &failingIncidentStore{
		Interface: store.NewMemory(ctx),
		failEnd:   true,
	}
	e := &types.Incident{StartTS: time.Now().Add(-time.Minute)}
	require.NoError(t, s.AddIncident(ctx, "host", e))

	w := &watcher{
		store:    s,
		status:   types.DOWN,
		incident: e,
		host: &types.Host{
			ID:               "host",
			SuccessThreshold: 1,
			FailureThreshold: 1,
		},
	}

	require.Equal(t, types.UP, w.validate(&types.HttpResponse{Status: true}))
	end := *w.pendingIncidents[0].EndTS
	require.Equal(t, types.DOWN, w.validate(&types.HttpResponse{
		Code:      503,
		Timestamp: time.Now(),
	}))
	time.Sleep(1100 * time.Millisecond)

	s.failEnd = false
	require.Equal(t, types.UP, w.validate(&types.HttpResponse{Status: true}))

	incidents, err := s.FindIncidents(ctx, "host", 0, 0)
	require.NoError(t, err)
	require.Len(t, incidents, 2)
	require.NotNil(t, incidents[0].EndTS)
	require.Equal(t, end, *incidents[1].EndTS)
	require.Empty(t, w.pendingIncidents)
}

func TestMonitor_ConcurrentIncidentStats(t *testing.T) {
	ctx := context.Background()
	s := store.NewMemory(ctx)
	interval := time.Minute

	m := &Monitor{
		Store: s,
		watchers: map[string]*watcher{
			"host": {
				host: &types.Host{
					ID:       "host",
					Interval: &interval,
				},
				status: types.DOWN,
			},
		},
	}

	e := &types.Incident{
		StartTS: time.Now().Add(-time.Hour),
		Details: types.IncidentDetails{StatusCode: 500},
	}
	require.NoError(t, s.AddIncident(ctx, "host", e))

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				if _, err := m.StatsByID(ctx, "host", false); err != nil {
					t.Error(err)
				}
				if err := s.EndIncident(ctx, "host", e.ID, time.Now()); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
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
	time.Sleep(50 * time.Millisecond) // well before delay
	w.mu.RLock()
	require.Equal(t, types.StatusType(""), w.status) // not checked yet
	w.mu.RUnlock()

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
