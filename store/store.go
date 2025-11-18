package store

import (
	"context"
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"time"

	"github.com/exelban/JAM/types"
)

type Interface interface {
	// AddResponse adds a new response to the history of the given ID.
	// DeleteResponse deletes the given keys from the history of the given ID.
	// FindResponses returns the history of the given ID.
	AddResponse(ctx context.Context, hostID string, r *types.HttpResponse) error
	DeleteResponse(ctx context.Context, hostID string, keys []time.Time) error
	FindResponses(ctx context.Context, hostID string) ([]*types.HttpResponse, error)
	LastResponse(ctx context.Context, hostID string) (*types.HttpResponse, error)

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

func New(ctx context.Context, typ, path string, cfg *types.Cfg) (Interface, error) {
	var store Interface

	switch typ {
	case "memory":
		store = NewMemory(ctx)
		log.Printf("[INFO] using memory storage")
	default:
		dbPath := path
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			if err := os.Mkdir(dbPath, 0755); err != nil {
				return nil, fmt.Errorf("failed to create data directory: %w", err)
			}
		}
		dbFilePath := fmt.Sprintf("%s/%s", dbPath, "jam.db")

		s, err := NewBolt(ctx, dbFilePath)
		if err != nil {
			return nil, err
		}
		store = s

		log.Printf("[INFO] using bolt storage at %s", dbFilePath)
	}

	if err := Aggregate(ctx, store); err != nil {
		return nil, fmt.Errorf("failed to aggregate: %w", err)
	}

	tk := time.NewTicker(hoursToMidnight())
	go func() {
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
				tk.Stop()
				return
			}
		}
	}()

	return store, nil
}

func Aggregate(ctx context.Context, s Interface) error {
	log.Printf("[INFO] aggregating data")
	start := time.Now()

	hosts, err := s.Hosts(ctx)
	if err != nil {
		return fmt.Errorf("failed to get keys: %w", err)
	}

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	for _, hostID := range hosts {
		days := make(map[time.Time][]*types.HttpResponse)

		history, err := s.FindResponses(ctx, hostID)
		if err != nil {
			return fmt.Errorf("failed to get history for %s: %w", hostID, err)
		}

		for _, r := range history {
			if r.Timestamp.After(today) || r.IsAggregated {
				continue
			}
			y, m, d := r.Timestamp.Date()
			day := time.Date(y, m, d, 0, 0, 0, 0, r.Timestamp.Location())
			if _, ok := days[day]; !ok {
				days[day] = make([]*types.HttpResponse, 0)
			}
			days[day] = append(days[day], r)
		}

		for day, responses := range days {
			aggregation := AggregateDay(day, responses)

			toDelete := make([]time.Time, 0)
			for _, r := range responses {
				toDelete = append(toDelete, r.Timestamp)
			}

			if err := s.DeleteResponse(ctx, hostID, toDelete); err != nil {
				return err
			}
			if err := s.AddResponse(ctx, hostID, aggregation); err != nil {
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
		Count:        len(responses),
		Time:         0,
	}
	if len(responses) == 0 {
		return aggregation
	}

	for _, r := range responses {
		if r.StatusType != types.DOWN {
			aggregation.Uptime++
		}
		aggregation.Time += r.Time
	}

	aggregation.Uptime = aggregation.Uptime / float64(len(responses))
	aggregation.Time = aggregation.Time / time.Duration(len(responses))

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
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
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
	return time.Until(time.Now().Truncate(24 * time.Hour).Add(24 * time.Hour).Add(time.Minute * 10))
}
