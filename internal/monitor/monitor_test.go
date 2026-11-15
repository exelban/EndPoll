package monitor

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/exelban/EndPoll/store"
	"github.com/exelban/EndPoll/types"
	"github.com/stretchr/testify/require"
)

func TestMonitor_Run(t *testing.T) {
	ts, status, shutdown := srv(0)
	defer shutdown()

	m := Monitor{
		Store: store.NewMemory(context.Background()),
	}

	name := "host-0"
	groupName := "group-0"
	idelay := time.Millisecond * 10
	rd := time.Millisecond * 30
	td := time.Millisecond * 100

	hosts := &types.Cfg{
		Hosts: []*types.Host{
			{
				Name:             &name,
				URL:              ts.URL,
				SuccessThreshold: 2,
				FailureThreshold: 3,
				InitialDelay:     &idelay,
				Interval:         &rd,
				TimeoutInterval:  &td,
				Conditions: &types.Success{
					Code: []int{200},
				},
			},
			{
				Name:             &name,
				URL:              ts.URL,
				Group:            &groupName,
				SuccessThreshold: 2,
				FailureThreshold: 3,
				InitialDelay:     &idelay,
				Interval:         &rd,
				TimeoutInterval:  &td,
				Conditions: &types.Success{
					Code: []int{200},
				},
			},
		},
	}
	for _, h := range hosts.Hosts {
		h.ID = h.GenerateID()
	}

	require.NoError(t, m.Run(context.Background(), hosts))
	time.Sleep(time.Millisecond * 10)
	require.Len(t, m.watchers, 2)

	t.Run("status", func(t *testing.T) {
		t.Run("must be up", func(t *testing.T) {
			time.Sleep(time.Millisecond * 100)
			m.mu.RLock()
			var watch *watcher
			for _, w := range m.watchers {
				if w.host.Name == &name {
					watch = w
					break
				}
			}
			require.NotNil(t, watch)
			watch.mu.RLock()
			require.Equal(t, types.UP, watch.status)
			watch.mu.RUnlock()
			m.mu.RUnlock()
		})
		t.Run("must does down", func(t *testing.T) {
			status.Store(false)
			time.Sleep(time.Millisecond * 100)
			m.mu.RLock()
			var watch *watcher
			for _, w := range m.watchers {
				if w.host.Name == &name {
					watch = w
					break
				}
			}
			require.NotNil(t, watch)
			watch.mu.RLock()
			require.Equal(t, types.DOWN, watch.status)
			watch.mu.RUnlock()
			m.mu.RUnlock()
		})
		t.Run("must does down", func(t *testing.T) {
			status.Store(true)
			time.Sleep(time.Millisecond * 30)
			m.mu.RLock()
			var watch *watcher
			for _, w := range m.watchers {
				if w.host.Name == &name {
					watch = w
					break
				}
			}
			require.NotNil(t, watch)
			watch.mu.RLock()
			require.Equal(t, types.DOWN, watch.status)
			watch.mu.RUnlock()
			m.mu.RUnlock()
			//require.Equal(t, types.DOWN, m.Status()["host-0"])
		})
	})

	t.Run("hosts combinations", func(t *testing.T) {
		t.Run("omit", func(t *testing.T) {
			t.Run("same url and name", func(t *testing.T) {
				initLen := len(m.watchers)
				newHost := &types.Host{
					Name:             &name,
					URL:              ts.URL,
					SuccessThreshold: 2,
					FailureThreshold: 3,
					InitialDelay:     &idelay,
					Interval:         &rd,
					TimeoutInterval:  &td,
				}
				newHost.ID = newHost.GenerateID()
				hosts.Hosts = append(hosts.Hosts, newHost)
				require.NoError(t, m.Run(context.Background(), hosts))
				time.Sleep(time.Millisecond * 10)
				require.Len(t, m.watchers, initLen)
			})
			t.Run("same url and different name", func(t *testing.T) {
				initLen := len(m.watchers)
				newName := "host-1"
				newHost := &types.Host{
					Name:             &newName,
					URL:              ts.URL,
					SuccessThreshold: 2,
					FailureThreshold: 3,
					InitialDelay:     &idelay,
					Interval:         &rd,
					TimeoutInterval:  &td,
				}
				newHost.ID = newHost.GenerateID()
				hosts.Hosts = append(hosts.Hosts, newHost)
				require.NoError(t, m.Run(context.Background(), hosts))
				time.Sleep(time.Millisecond * 10)
				require.Len(t, m.watchers, initLen)
			})
			t.Run("same url, same group and same name", func(t *testing.T) {
				initLen := len(m.watchers)
				newHost := &types.Host{
					Name:             &name,
					URL:              ts.URL,
					Group:            &groupName,
					SuccessThreshold: 2,
					FailureThreshold: 3,
					InitialDelay:     &idelay,
					Interval:         &rd,
					TimeoutInterval:  &td,
				}
				newHost.ID = newHost.GenerateID()
				hosts.Hosts = append(hosts.Hosts, newHost)
				require.NoError(t, m.Run(context.Background(), hosts))
				time.Sleep(time.Millisecond * 10)
				require.Len(t, m.watchers, initLen)
			})
		})
		t.Run("add", func(t *testing.T) {
			t.Run("different url and same name", func(t *testing.T) {
				initLen := len(m.watchers)
				newHost := &types.Host{
					Name:             &name,
					URL:              fmt.Sprintf("%s/1", ts.URL),
					SuccessThreshold: 2,
					FailureThreshold: 3,
					InitialDelay:     &idelay,
					Interval:         &rd,
					TimeoutInterval:  &td,
				}
				newHost.ID = newHost.GenerateID()
				hosts.Hosts = append(hosts.Hosts, newHost)
				require.NoError(t, m.Run(context.Background(), hosts))
				time.Sleep(time.Millisecond * 10)
				require.Len(t, m.watchers, initLen+1)
			})
			t.Run("different url and different name", func(t *testing.T) {
				initLen := len(m.watchers)
				newName := "host-1"
				newHost := &types.Host{
					Name:             &newName,
					URL:              fmt.Sprintf("%s/2", ts.URL),
					SuccessThreshold: 2,
					FailureThreshold: 3,
					InitialDelay:     &idelay,
					Interval:         &rd,
					TimeoutInterval:  &td,
				}
				newHost.ID = newHost.GenerateID()
				hosts.Hosts = append(hosts.Hosts, newHost)
				require.NoError(t, m.Run(context.Background(), hosts))
				time.Sleep(time.Millisecond * 10)
				require.Len(t, m.watchers, initLen+1)
			})
			t.Run("same url and different group", func(t *testing.T) {
				initLen := len(m.watchers)
				newGroup := "group-1"
				newHost := &types.Host{
					Name:             &name,
					URL:              ts.URL,
					Group:            &newGroup,
					SuccessThreshold: 2,
					FailureThreshold: 3,
					InitialDelay:     &idelay,
					Interval:         &rd,
					TimeoutInterval:  &td,
				}
				newHost.ID = newHost.GenerateID()
				hosts.Hosts = append(hosts.Hosts, newHost)
				require.NoError(t, m.Run(context.Background(), hosts))
				time.Sleep(time.Millisecond * 10)
				require.Len(t, m.watchers, initLen+1)
			})
			t.Run("same url, same name and different name", func(t *testing.T) {
				initLen := len(m.watchers)
				newGroup := "group-3"
				newHost := &types.Host{
					Name:             &name,
					URL:              ts.URL,
					Group:            &newGroup,
					SuccessThreshold: 2,
					FailureThreshold: 3,
					InitialDelay:     &idelay,
					Interval:         &rd,
					TimeoutInterval:  &td,
				}
				newHost.ID = newHost.GenerateID()
				hosts.Hosts = append(hosts.Hosts, newHost)
				require.NoError(t, m.Run(context.Background(), hosts))
				time.Sleep(time.Millisecond * 10)
				require.Len(t, m.watchers, initLen+1)
			})
		})
		t.Run("update", func(t *testing.T) {
			hosts.Hosts[0].Group = &groupName
			require.NoError(t, m.Run(context.Background(), hosts))
			time.Sleep(time.Millisecond * 10)
		})
	})

	t.Run("remove host", func(t *testing.T) {
		hosts.Hosts = hosts.Hosts[:1]
		require.NoError(t, m.Run(context.Background(), hosts))
		require.Len(t, m.watchers, 1)
	})
}

func TestMonitor_Run_ReconcileHosts(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	delay := time.Hour
	initialization := false
	cfg := &types.Cfg{
		Interval:     time.Hour,
		InitialDelay: &delay,
		Connectivity: &types.Connectivity{Disabled: true},
		Notifications: types.Notifications{
			InitializationMessage: &initialization,
		},
		FileHosts: []*types.Host{
			{URL: ts.URL + "/a"},
			{URL: ts.URL + "/b"},
		},
	}
	require.NoError(t, cfg.Validate())
	m := &Monitor{Store: store.NewMemory(context.Background())}
	defer m.Stop()
	require.NoError(t, m.Run(context.Background(), cfg))

	first, second := cfg.Hosts[0].ID, cfg.Hosts[1].ID
	a, b := m.watchers[first], m.watchers[second]
	a.mu.Lock()
	a.status = types.UP
	a.successCount = 7
	aDone := a.done
	a.mu.Unlock()
	b.mu.RLock()
	bDone := b.done
	b.mu.RUnlock()

	cfg.FileHosts[0], cfg.FileHosts[1] = cfg.FileHosts[1], cfg.FileHosts[0]
	require.NoError(t, cfg.Validate())
	require.NoError(t, m.Run(context.Background(), cfg))
	require.Same(t, a, m.watchers[first])
	require.Same(t, b, m.watchers[second])
	a.mu.RLock()
	status, count, done := a.status, a.successCount, a.done
	a.mu.RUnlock()
	require.Equal(t, types.UP, status)
	require.Equal(t, 7, count)
	require.Equal(t, aDone, done)
	b.mu.RLock()
	done = b.done
	b.mu.RUnlock()
	require.Equal(t, bDone, done)
	snapshots := m.Snapshots()
	require.Len(t, snapshots, 2)
	require.Equal(t, second, snapshots[0].Host.ID)
	require.Equal(t, first, snapshots[1].Host.ID)

	cfg.FileHosts = []*types.Host{
		{URL: ts.URL + "/a", Headers: map[string]string{"X-Check": "updated"}},
		{URL: ts.URL + "/c"},
	}
	require.NoError(t, cfg.Validate())
	require.NoError(t, m.Run(context.Background(), cfg))
	require.Len(t, m.watchers, 2)
	require.Same(t, a, m.watchers[first])
	require.NotContains(t, m.watchers, second)
	require.Contains(t, m.watchers, cfg.Hosts[1].ID)
	a.mu.RLock()
	done = a.done
	a.mu.RUnlock()
	require.NotEqual(t, aDone, done)
	for _, stopped := range []chan struct{}{aDone, bDone} {
		select {
		case <-stopped:
		default:
			t.Fatal("previous watcher loop is still running")
		}
	}
}

func srv(timeout time.Duration) (*httptest.Server, *atomic.Value, func()) {
	router := http.NewServeMux()
	status := atomic.Value{}
	status.Store(true)

	router.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(timeout)
		if status.Load() == true {
			w.WriteHeader(http.StatusOK)
		} else {
			http.Error(w, "error", http.StatusInternalServerError)
		}
	})

	ts := httptest.NewServer(router)
	shutdown := func() {
		ts.Close()
	}

	return ts, &status, shutdown
}
