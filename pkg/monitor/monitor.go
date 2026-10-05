package monitor

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/exelban/EndPoll/pkg/connectivity"
	"github.com/exelban/EndPoll/pkg/dialer"
	"github.com/exelban/EndPoll/pkg/notify"
	"github.com/exelban/EndPoll/store"
	"github.com/exelban/EndPoll/types"
)

// Monitor - main service which track the hosts liveness
type Monitor struct {
	Store store.Interface

	// CacheTTL - how long the computed stats are cached (0 disables the cache).
	// The cache is dropped whenever a check produces a new result.
	CacheTTL time.Duration
	cache    cache

	dialer       *dialer.Dialer
	notify       *notify.Notify
	connectivity *connectivity.Checker

	watchers map[string]*watcher

	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
}

// Run - applies the configuration: creates a watcher for every new host, updates the
// existing ones (restarting only those whose configuration changed) and removes the
// watchers for hosts that are not in the configuration anymore. The ctx bounds the
// lifetime of the monitor and is captured on the first call only.
func (m *Monitor) Run(ctx context.Context, cfg *types.Cfg) error {
	n, err := notify.New(cfg)
	if err != nil {
		return err
	}

	m.mu.Lock()
	if m.watchers == nil {
		m.watchers = make(map[string]*watcher)
	}
	first := m.ctx == nil
	if first {
		m.ctx, m.cancel = context.WithCancel(ctx)
	}
	m.cache.reset()
	oldDialer := m.dialer
	m.dialer = dialer.New(cfg.MaxConn)
	m.connectivity = connectivity.New(cfg.Connectivity)
	m.notify = n
	d, c := m.dialer, m.connectivity
	m.mu.Unlock()

	if oldDialer != nil {
		oldDialer.Close()
	}
	if first {
		go n.Startup()
	}

	// add hosts which do not have watchers, update the existing ones
	for _, host := range cfg.Hosts {
		m.mu.RLock()
		w, ok := m.watchers[host.ID]
		m.mu.RUnlock()
		if !ok {
			m.add(host, d, n, c)
			continue
		}
		if changed := w.update(host, d, n, c); changed {
			w.stop()
			w.start(m.ctx, false)
		}
	}

	// remove watchers that are not present in the config
	removed := make([]*watcher, 0)
	m.mu.Lock()
	for id, w := range m.watchers {
		ok := false
		for _, host := range cfg.Hosts {
			if host.ID == id {
				ok = true
				break
			}
		}
		if !ok {
			removed = append(removed, w)
			delete(m.watchers, id)
		}
	}
	m.mu.Unlock()
	for _, w := range removed {
		w.stop()
	}

	return nil
}

// Stop - stops every watcher, waits for them to finish and sends the shutdown notification.
func (m *Monitor) Stop() {
	m.mu.Lock()
	cancel := m.cancel
	n := m.notify
	d := m.dialer
	watchers := make([]*watcher, 0, len(m.watchers))
	for _, w := range m.watchers {
		watchers = append(watchers, w)
	}
	m.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	for _, w := range watchers {
		w.stop()
	}
	if n != nil {
		n.Shutdown()
	}
	if d != nil {
		d.Close()
	}
}

// add - creates and starts a watcher for the host
func (m *Monitor) add(host *types.Host, d *dialer.Dialer, n *notify.Notify, c *connectivity.Checker) {
	w := &watcher{
		dialer:       d,
		notify:       n,
		connectivity: c,
		store:        m.Store,
		host:         host,
		onChange:     m.cache.reset,
	}

	m.mu.Lock()
	m.watchers[host.ID] = w
	ctx := m.ctx
	m.mu.Unlock()

	w.start(ctx, true)
}

// Snapshot - the current state of a monitored host
type Snapshot struct {
	Host         *types.Host
	Status       types.StatusType
	LastCheck    time.Time
	LastResponse *types.HttpResponse
}

// Snapshots - returns the current state of every host, ordered as in the configuration
func (m *Monitor) Snapshots() []Snapshot {
	m.mu.RLock()
	watchers := make([]*watcher, 0, len(m.watchers))
	for _, w := range m.watchers {
		watchers = append(watchers, w)
	}
	m.mu.RUnlock()

	res := make([]Snapshot, 0, len(watchers))
	for _, w := range watchers {
		res = append(res, w.snap())
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i].Host.Index < res[j].Host.Index
	})
	return res
}

// Snapshot - returns the current state of the host
func (m *Monitor) Snapshot(id string) (Snapshot, error) {
	m.mu.RLock()
	w, ok := m.watchers[id]
	m.mu.RUnlock()
	if !ok {
		return Snapshot{}, types.ErrHostNotFound
	}
	return w.snap(), nil
}
