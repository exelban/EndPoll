package store

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/exelban/EndPoll/types"
)

type Memory struct {
	history   map[string]map[time.Time]*types.HttpResponse
	incidents map[string][]*types.Incident
	sync.RWMutex
}

func NewMemory(ctx context.Context) *Memory {
	return &Memory{
		history:   make(map[string]map[time.Time]*types.HttpResponse),
		incidents: make(map[string][]*types.Incident),
	}
}
func (m *Memory) Close() error {
	return nil
}

func (m *Memory) AddResponse(ctx context.Context, hostID string, r *types.HttpResponse) error {
	m.Lock()
	defer m.Unlock()

	if _, ok := m.history[hostID]; !ok {
		m.history[hostID] = make(map[time.Time]*types.HttpResponse)
	}

	m.history[hostID][r.Timestamp.UTC()] = r
	return nil
}
func (m *Memory) DeleteResponse(ctx context.Context, hostID string, keys []time.Time) error {
	m.Lock()
	defer m.Unlock()

	if _, ok := m.history[hostID]; !ok {
		return nil
	}

	for _, key := range keys {
		for ts := range m.history[hostID] {
			if ts == key.UTC() {
				delete(m.history[hostID], ts)
				break
			}
		}
	}

	return nil
}
func (m *Memory) FindResponses(ctx context.Context, hostID string) ([]*types.HttpResponse, error) {
	m.RLock()
	defer m.RUnlock()

	if _, ok := m.history[hostID]; !ok {
		return []*types.HttpResponse{}, nil
	}

	res := make([]*types.HttpResponse, 0, len(m.history[hostID]))
	for _, r := range m.history[hostID] {
		res = append(res, r)
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i].Timestamp.Before(res[j].Timestamp)
	})

	return res, nil
}
func (m *Memory) LastResponse(ctx context.Context, hostID string) (*types.HttpResponse, error) {
	m.RLock()
	defer m.RUnlock()

	if _, ok := m.history[hostID]; !ok {
		return nil, nil
	}

	var last *types.HttpResponse
	for _, r := range m.history[hostID] {
		if last == nil || r.Timestamp.After(last.Timestamp) {
			last = r
		}
	}

	return last, nil
}

func (m *Memory) Hosts(ctx context.Context) ([]string, error) {
	m.RLock()
	defer m.RUnlock()

	keys := []string{}
	for k := range m.history {
		keys = append(keys, k)
	}

	return keys, nil
}

func (m *Memory) AddIncident(ctx context.Context, hostID string, e *types.Incident) error {
	m.Lock()
	defer m.Unlock()

	if _, ok := m.incidents[hostID]; !ok {
		m.incidents[hostID] = make([]*types.Incident, 0)
	}

	e.ID = len(m.incidents[hostID]) + 1
	m.incidents[hostID] = append(m.incidents[hostID], e)

	return nil
}
func (m *Memory) EndIncident(ctx context.Context, hostID string, eventID int, ts time.Time) error {
	m.Lock()
	defer m.Unlock()

	if _, ok := m.incidents[hostID]; !ok {
		return nil
	}

	for _, e := range m.incidents[hostID] {
		if e.ID != eventID {
			continue
		}
		e.EndTS = &ts
		break
	}

	return nil
}
func (m *Memory) DeleteIncident(ctx context.Context, hostID string, eventID int) error {
	m.Lock()
	defer m.Unlock()

	if _, ok := m.incidents[hostID]; !ok {
		return nil
	}

	for i, e := range m.incidents[hostID] {
		if e.ID == eventID {
			m.incidents[hostID] = append(m.incidents[hostID][:i], m.incidents[hostID][i+1:]...)
			break
		}
	}

	return nil
}
func (m *Memory) FindIncidents(ctx context.Context, hostID string, skip, limit int) ([]*types.Incident, error) {
	m.RLock()
	defer m.RUnlock()

	if _, ok := m.incidents[hostID]; !ok {
		return []*types.Incident{}, nil
	}

	res := make([]*types.Incident, 0, len(m.incidents[hostID]))
	for _, e := range m.incidents[hostID] {
		res = append(res, e)
	}
	for i, j := 0, len(res)-1; i < j; i, j = i+1, j-1 {
		res[i], res[j] = res[j], res[i]
	}

	if skip > 0 {
		if len(res) < skip {
			return []*types.Incident{}, nil
		}
		res = res[skip:]
	}
	if limit > 0 && len(res) > limit {
		res = res[:limit]
	}

	return res, nil
}
