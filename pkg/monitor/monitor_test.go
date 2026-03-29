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

	require.NoError(t, m.Run(hosts))
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
			require.Equal(t, types.UP, watch.status)
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
			require.Equal(t, types.DOWN, watch.status)
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
			require.Equal(t, types.DOWN, watch.status)
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
				require.NoError(t, m.Run(hosts))
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
				require.NoError(t, m.Run(hosts))
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
				require.NoError(t, m.Run(hosts))
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
				require.NoError(t, m.Run(hosts))
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
				require.NoError(t, m.Run(hosts))
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
				require.NoError(t, m.Run(hosts))
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
				require.NoError(t, m.Run(hosts))
				time.Sleep(time.Millisecond * 10)
				require.Len(t, m.watchers, initLen+1)
			})
		})
		t.Run("update", func(t *testing.T) {
			hosts.Hosts[0].Group = &groupName
			require.NoError(t, m.Run(hosts))
			time.Sleep(time.Millisecond * 10)
		})
	})

	t.Run("remove host", func(t *testing.T) {
		hosts.Hosts = hosts.Hosts[:1]
		require.NoError(t, m.Run(hosts))
		require.Len(t, m.watchers, 1)
	})
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
