package store

import (
	"context"
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"time"

	"github.com/exelban/EndPoll/pkg"
	"github.com/exelban/EndPoll/types"
)

type Interface interface {
	// AddResponse adds a new response to the history of the given ID.
	// DeleteResponse deletes the given keys from the history of the given ID.
	// FindResponses returns the history of the given ID.
	AddResponse(ctx context.Context, hostID string, r *types.HttpResponse) error
	DeleteResponse(ctx context.Context, hostID string, keys []time.Time) error
	FindResponses(ctx context.Context, hostID string) ([]*types.HttpResponse, error)
	LastResponse(ctx context.Context, hostID string) (*types.HttpResponse, error)
	AggregateResponses(ctx context.Context, hostID string, day time.Time) error

	// Hosts returns a list of all hosts that has any responses in the store.
	Hosts(ctx context.Context) ([]string, error)

	// AddIncident puts a new incident to the store.
	// EndIncident marks the incident as finished by setting the end time.
	// DeleteIncident removes the incident from the store.
	// FindIncidents returns the list of incidents for the ID.
	AddIncident(ctx context.Context, hostID string, e *types.Incident) error
	EndIncident(ctx context.Context, hostID string, eventID int, ts time.Time) error
	DeleteIncident(ctx context.Context, hostID string, eventID int) error
	FindIncidents(ctx context.Context, hostID string, skip, limit int) ([]*types.Incident, error)

	Close() error
}

func New(ctx context.Context, typ, path string) (Interface, error) {
	var store Interface

	switch typ {
	case "memory":
		store = NewMemory(ctx)
		log.Printf("[INFO] using memory storage")
	case "sqlite":
		if err := os.MkdirAll(path, 0755); err != nil {
			return nil, fmt.Errorf("failed to create data directory: %w", err)
		}
		dbFilePath := fmt.Sprintf("%s/%s", path, "endpoll.sqlite.db")
		s, err := NewSQLite(ctx, dbFilePath)
		if err != nil {
			return nil, err
		}
		store = s

		log.Printf("[INFO] using sqlite storage at %s", dbFilePath)
	default:
		dbPath := path
		if err := os.MkdirAll(dbPath, 0755); err != nil {
			return nil, fmt.Errorf("failed to create data directory: %w", err)
		}
		dbFilePath := fmt.Sprintf("%s/%s", dbPath, "endpoll.db")
		if legacyPath := fmt.Sprintf("%s/%s", dbPath, "jam.db"); fileExists(legacyPath) {
			dbFilePath = legacyPath
		}

		s, err := NewBolt(ctx, dbFilePath)
		if err != nil {
			return nil, err
		}
		store = s

		log.Printf("[INFO] using bolt storage at %s", dbFilePath)
	}

	if err := Aggregate(ctx, store); err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("failed to aggregate: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	s := &scheduledStore{
		Interface: store,
		cancel:    cancel,
		done:      make(chan struct{}),
	}

	tk := time.NewTicker(hoursToMidnight())
	go func() {
		defer close(s.done)
		defer tk.Stop()

		for {
			select {
			case <-tk.C:
				if err := Aggregate(ctx, store); err != nil {
					log.Printf("[ERROR] failed to aggregate: %v", err)
				}
				nextRun := hoursToMidnight()
				tk.Reset(nextRun)
				log.Printf("[INFO] next aggregation in %v", nextRun)
			case <-ctx.Done():
				return
			}
		}
	}()

	return s, nil
}

type scheduledStore struct {
	Interface
	cancel context.CancelFunc
	done   chan struct{}
}

func (s *scheduledStore) Close() error {
	s.cancel()
	<-s.done
	return s.Interface.Close()
}

func Aggregate(ctx context.Context, s Interface) error {
	log.Printf("[INFO] aggregating data")
	start := time.Now()

	hosts, err := s.Hosts(ctx)
	if err != nil {
		return fmt.Errorf("failed to get keys: %w", err)
	}

	now := time.Now()
	today := pkg.StartOfDay(now)
	for _, hostID := range hosts {
		days := make(map[time.Time]bool)

		history, err := s.FindResponses(ctx, hostID)
		if err != nil {
			return fmt.Errorf("failed to get history for %s: %w", hostID, err)
		}

		for _, r := range history {
			if !r.Timestamp.Before(today) || r.IsAggregated {
				continue
			}
			day := pkg.StartOfDay(r.Timestamp)
			days[day] = true
		}

		for day := range days {
			if err := s.AggregateResponses(ctx, hostID, day); err != nil {
				return err
			}
		}
	}

	log.Printf("[INFO] aggregation took %v", time.Since(start))
	return nil
}
func AggregateDay(ts time.Time, responses []*types.HttpResponse) *types.HttpResponse {
	aggregation := &types.HttpResponse{
		Timestamp:    ts,
		IsAggregated: true,
		Uptime:       0,
		Count:        0,
		Time:         0,
	}

	for _, r := range responses {
		if r.IsAggregated {
			aggregation.Count += r.Count
			aggregation.Uptime += r.Uptime * float64(r.Count)
			aggregation.Time += r.Time * time.Duration(r.Count)
			continue
		}
		aggregation.Count++
		if r.StatusType == types.UP {
			aggregation.Uptime++
		}
		aggregation.Time += r.Time
	}

	if aggregation.Count == 0 {
		aggregation.StatusType = types.Unknown
		return aggregation
	}

	aggregation.Uptime = aggregation.Uptime / float64(aggregation.Count)
	aggregation.Time = aggregation.Time / time.Duration(aggregation.Count)

	if aggregation.Uptime > 0.95 {
		aggregation.StatusType = types.UP
	} else if aggregation.Uptime > 0.5 {
		aggregation.StatusType = types.DEGRADED
	} else {
		aggregation.StatusType = types.DOWN
	}

	return aggregation
}

func GenerateHistory(s Interface, start time.Time, id string) int {
	ctx := context.Background()
	now := time.Now()
	day := pkg.StartOfDay(now)
	count := rand.IntN(10000-3000) + 3000
	interval := now.Sub(start) / time.Duration(count)
	today := 0

	for i := 0; i < count; i++ {
		var status types.StatusType
		switch randInt(1, 3) {
		case 1:
			status = types.UP
		default:
			status = types.DOWN
		}
		r := &types.HttpResponse{
			Timestamp:  start.Add(interval * time.Duration(i)),
			Code:       i,
			StatusType: status,
			Time:       time.Duration(randInt(1, 100)) * time.Millisecond,
		}
		if r.Timestamp.After(day) {
			today++
		}
		if err := s.AddResponse(ctx, id, r); err != nil {
			log.Printf("[ERROR] failed to add response: %v", err)
		}
	}

	return today
}

func randInt(min, max int) int {
	return rand.IntN(max-min) + min
}

func hoursToMidnight() time.Duration {
	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 10, 0, 0, now.Location())
	return time.Until(next)
}
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
