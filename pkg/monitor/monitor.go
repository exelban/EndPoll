package monitor

import (
	"context"
	"sync"

	"github.com/exelban/EndPoll/pkg/dialer"
	"github.com/exelban/EndPoll/pkg/notify"
	"github.com/exelban/EndPoll/store"
	"github.com/exelban/EndPoll/types"
)

// Monitor - main service which track the hosts liveness
type Monitor struct {
	Store store.Interface

	dialer *dialer.Dialer
	notify *notify.Notify

	watchers map[string]*watcher

	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
	once   sync.Once
}

// Run - run the monitor. Creates jobs for each host in the separate threads
func (m *Monitor) Run(cfg *types.Cfg) error {
	m.once.Do(func() {
		m.watchers = make(map[string]*watcher)
	})

	m.mu.Lock()
	if m.cancel != nil {
		m.cancel()
	}
	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.dialer = dialer.New(128)
	n, err := notify.New(m.ctx, cfg)
	if err != nil {
		m.mu.Unlock()
		return err
	}
	m.notify = n
	m.mu.Unlock()

	// add hosts which are does not have watchers, update if some of them changed
	for _, host := range cfg.Hosts {
		m.mu.RLock()
		w, ok := m.watchers[host.ID]
		m.mu.RUnlock()
		if !ok || w == nil {
			if err := m.add(host); err != nil {
				return err
			}
		} else {
			w.cancel()
			go w.run(m.ctx)
		}
	}

	// remove watchers that do not present in the config
	m.mu.Lock()
	for id, w := range m.watchers {
		ok := false
		for _, host := range cfg.Hosts {
			if host.ID == w.host.ID {
				ok = true
			}
		}
		if !ok {
			w.cancel()
			delete(m.watchers, id)
		}
	}
	m.mu.Unlock()

	return nil
}

// add - create a watcher for host
func (m *Monitor) add(host *types.Host) error {
	w := &watcher{
		dialer: m.dialer,
		notify: m.notify,
		store:  m.Store,
		host:   host,
	}
	go w.run(m.ctx)

	m.mu.Lock()
	m.watchers[host.ID] = w
	m.mu.Unlock()

	return nil
}
