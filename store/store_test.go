package store

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/exelban/EndPoll/types"
	"github.com/stretchr/testify/require"
)

func TestStore_AddResponse(t *testing.T) {
	ctx := context.Background()
	list := map[string]func() Interface{
		"memory": func() Interface {
			return NewMemory(ctx)
		},
		"bolt": func() Interface {
			file, err := os.CreateTemp("", "test.db")
			require.NoError(t, err)
			defer os.RemoveAll(file.Name())

			b, err := NewBolt(ctx, file.Name())
			require.NoError(t, err)
			require.NotNil(t, b)

			return b
		},
	}
	now := time.Now()

	for name, f := range list {
		t.Run(name, func(t *testing.T) {
			s := f()
			count := rand.Intn(100-30) + 30
			for i := 0; i < count; i++ {
				require.NoError(t, s.AddResponse(ctx, "test", &types.HttpResponse{Code: i, Timestamp: now.Add(-time.Minute * time.Duration(i))}))
			}
			h, err := s.FindResponses(ctx, "test")
			require.NoError(t, err)
			require.Equal(t, count, len(h))
		})
	}
}

func TestStore_ResponseHistory(t *testing.T) {
	ctx := context.Background()
	list := map[string]func() Interface{
		"memory": func() Interface {
			return NewMemory(ctx)
		},
		"bolt": func() Interface {
			file, err := os.CreateTemp("", "test.db")
			require.NoError(t, err)
			defer os.RemoveAll(file.Name())

			b, err := NewBolt(ctx, file.Name())
			require.NoError(t, err)
			require.NotNil(t, b)

			return b
		},
	}

	for name, f := range list {
		t.Run(name, func(t *testing.T) {
			s := f()
			count := rand.Intn(500-100) + 100
			now := time.Now()
			wg := sync.WaitGroup{}
			wg.Add(count)
			for i := 0; i < count; i++ {
				go func(i int) {
					defer wg.Done()
					_ = s.AddResponse(ctx, "test", &types.HttpResponse{Code: i, Timestamp: now.Add(-time.Minute * time.Duration(i))})
				}(i)
			}
			wg.Wait()
			history, err := s.FindResponses(ctx, "test")
			require.NoError(t, err)
			require.Equal(t, count, len(history))

			for i, h := range history {
				require.Equal(t, count-i-1, h.Code)
			}

			require.Equal(t, now.Unix(), history[len(history)-1].Timestamp.Unix())
		})
	}
}

func TestStore_DeleteResponse(t *testing.T) {
	ctx := context.Background()
	list := map[string]func() Interface{
		"memory": func() Interface {
			return NewMemory(ctx)
		},
		"bolt": func() Interface {
			file, err := os.CreateTemp("", "test.db")
			require.NoError(t, err)
			defer os.RemoveAll(file.Name())

			b, err := NewBolt(ctx, file.Name())
			require.NoError(t, err)
			require.NotNil(t, b)

			return b
		},
	}
	now := time.Now()

	for name, f := range list {
		t.Run(name, func(t *testing.T) {
			s := f()
			count := rand.Intn(100-30) + 30
			for i := 0; i < count; i++ {
				require.NoError(t, s.AddResponse(ctx, "test", &types.HttpResponse{Code: i, Timestamp: now.Add(-time.Minute * time.Duration(i))}))
			}
			responses, err := s.FindResponses(ctx, "test")
			require.NoError(t, err)
			require.Equal(t, count, len(responses))

			t.Run("delete half of the responses", func(t *testing.T) {
				half := count / 2
				keys := make([]time.Time, 0)
				for i := 0; i < half; i++ {
					keys = append(keys, responses[i].Timestamp)
				}
				require.NoError(t, s.DeleteResponse(ctx, "test", keys))

				responses, err = s.FindResponses(ctx, "test")
				require.NoError(t, err)
				require.Equal(t, count-half, len(responses))
			})

			t.Run("delete all responses", func(t *testing.T) {
				keys := make([]time.Time, 0)
				for i := 0; i < len(responses); i++ {
					keys = append(keys, responses[i].Timestamp)
				}
				require.NoError(t, s.DeleteResponse(ctx, "test", keys))

				responses, err = s.FindResponses(ctx, "test")
				require.NoError(t, err)
				require.Empty(t, responses)
			})
		})
	}
}

func TestStore_Hosts(t *testing.T) {
	ctx := context.Background()
	list := map[string]func() Interface{
		"memory": func() Interface {
			return NewMemory(ctx)
		},
		"bolt": func() Interface {
			file, err := os.CreateTemp("", "test.db")
			require.NoError(t, err)
			defer os.RemoveAll(file.Name())

			b, err := NewBolt(ctx, file.Name())
			require.NoError(t, err)
			require.NotNil(t, b)

			return b
		},
	}
	now := time.Now()

	for name, f := range list {
		t.Run(name, func(t *testing.T) {
			s := f()
			count := rand.Intn(100-30) + 30
			for i := 0; i < count; i++ {
				hostID := fmt.Sprintf("host-%d", i)
				require.NoError(t, s.AddResponse(ctx, hostID, &types.HttpResponse{Timestamp: now.Add(-time.Minute * time.Duration(i))}))
			}
			hosts, err := s.Hosts(ctx)
			require.NoError(t, err)
			require.Equal(t, count, len(hosts))
		})
	}
}

func TestStore_AddEvent(t *testing.T) {
	ctx := context.Background()
	list := map[string]func() Interface{
		"memory": func() Interface {
			return NewMemory(ctx)
		},
		"bolt": func() Interface {
			file, err := os.CreateTemp("", "test.db")
			require.NoError(t, err)
			defer os.RemoveAll(file.Name())

			b, err := NewBolt(ctx, file.Name())
			require.NoError(t, err)
			require.NotNil(t, b)

			return b
		},
	}
	now := time.Now()

	for name, f := range list {
		t.Run(name, func(t *testing.T) {
			s := f()
			count := rand.Intn(100-30) + 30
			for i := 0; i < count; i++ {
				require.NoError(t, s.AddIncident(ctx, "test", &types.Incident{
					StartTS: now.Add(-time.Minute * time.Duration(i)),
					EndTS:   &now,
				}))
				time.Sleep(time.Millisecond)
			}
			e, err := s.FindIncidents(ctx, "test", 0, 0)
			require.NoError(t, err)
			require.Equal(t, count, len(e))

			eventToFinish := e[3]
			require.NoError(t, s.EndIncident(ctx, "test", eventToFinish.ID, now))
			e, err = s.FindIncidents(ctx, "test", -1, -1)
			require.NoError(t, err)
			require.NotNil(t, e[3].EndTS)
			require.Equal(t, now.Unix(), e[3].EndTS.Unix())
		})
	}
}

func TestStore_FindEvents(t *testing.T) {
	ctx := context.Background()
	list := map[string]func() Interface{
		"memory": func() Interface {
			return NewMemory(ctx)
		},
		"bolt": func() Interface {
			file, err := os.CreateTemp("", "test.db")
			require.NoError(t, err)
			defer os.RemoveAll(file.Name())

			b, err := NewBolt(ctx, file.Name())
			require.NoError(t, err)
			require.NotNil(t, b)

			return b
		},
	}
	now := time.Now()

	for name, f := range list {
		t.Run(name, func(t *testing.T) {
			s := f()
			count := rand.Intn(100-30) + 30
			for i := 0; i < count; i++ {
				require.NoError(t, s.AddIncident(ctx, "test", &types.Incident{
					StartTS: now.Add(-time.Minute * time.Duration(i)),
					EndTS:   &now,
				}))
				time.Sleep(time.Millisecond)
			}

			t.Run("no skip and no limit", func(t *testing.T) {
				events, err := s.FindIncidents(ctx, "test", 0, 0)
				require.NoError(t, err)
				require.Equal(t, count, len(events))
				require.Equal(t, count, events[0].ID)
				require.Equal(t, 1, events[len(events)-1].ID)
			})

			t.Run("no skip with limit", func(t *testing.T) {
				events, err := s.FindIncidents(ctx, "test", 0, 10)
				require.NoError(t, err)
				require.Equal(t, 10, len(events))
				require.Equal(t, count, events[0].ID)
				require.Equal(t, count-9, events[len(events)-1].ID)
			})

			t.Run("with skip no limit", func(t *testing.T) {
				events, err := s.FindIncidents(ctx, "test", 5, 0)
				require.NoError(t, err)
				require.Equal(t, count-5, len(events))
				require.Equal(t, count-5, events[0].ID)
				require.Equal(t, 1, events[len(events)-1].ID)
			})

			t.Run("with skip and limit", func(t *testing.T) {
				limitedAndSkipped, err := s.FindIncidents(ctx, "test", 5, 3)
				require.NoError(t, err)
				require.Equal(t, 3, len(limitedAndSkipped))
				require.Equal(t, count-5, limitedAndSkipped[0].ID)
				require.Equal(t, count-7, limitedAndSkipped[len(limitedAndSkipped)-1].ID)
			})

			t.Run("get last event", func(t *testing.T) {
				e, err := s.FindIncidents(ctx, "test", 0, 1)
				require.NoError(t, err)
				require.Len(t, e, 1)
				require.Equal(t, count, e[0].ID)
			})
		})
	}
}

func TestStore_aggregation(t *testing.T) {
	t.Run("one day history", func(t *testing.T) {
		ctx := context.Background()
		s, err := New(ctx, "memory", "", &types.Cfg{})
		require.NoError(t, err)
		require.NotNil(t, s)

		start := time.Now().Add(-24 * time.Hour).Truncate(time.Hour * 24)
		today := GenerateHistory(s, start, "test")

		require.NoError(t, Aggregate(ctx, s))

		history, err := s.FindResponses(ctx, "test")
		require.NoError(t, err)
		require.Equal(t, today+1, len(history))
	})
	t.Run("random days history back", func(t *testing.T) {
		ctx := context.Background()
		s, err := New(ctx, "memory", "", &types.Cfg{})
		require.NoError(t, err)
		require.NotNil(t, s)

		days := rand.Intn(1000-500) + 500
		start := time.Now().Add(-24 * time.Hour * time.Duration(days)).Truncate(time.Hour * 24)
		today := GenerateHistory(s, start, "test")

		require.NoError(t, Aggregate(ctx, s))

		history, err := s.FindResponses(ctx, "test")
		require.NoError(t, err)
		require.Equal(t, today+days, len(history))
	})
	t.Run("uptime, status and responseTime type per day", func(t *testing.T) {
		ctx := context.Background()
		s, err := New(ctx, "memory", "", &types.Cfg{})
		require.NoError(t, err)
		require.NotNil(t, s)

		daysNum := rand.Intn(100-50) + 50
		start := time.Now().Add(-24 * time.Hour * time.Duration(daysNum)).Truncate(time.Hour * 24)
		_ = GenerateHistory(s, start, "test")

		history, err := s.FindResponses(ctx, "test")
		require.NoError(t, err)

		days := make(map[time.Time][]*types.HttpResponse)
		for _, h := range history {
			key := time.Date(h.Timestamp.Year(), h.Timestamp.Month(), h.Timestamp.Day(), 0, 0, 0, 0, h.Timestamp.Location())
			days[key] = append(days[key], h)
		}
		require.Equal(t, daysNum+1, len(days))

		type stats struct {
			responseTime time.Duration
			up           int
			count        int
			uptime       float64
		}
		statistics := make(map[time.Time]stats)
		for ts, res := range days {
			stat := stats{
				responseTime: 0,
				up:           0,
				count:        len(res),
			}
			for _, r := range res {
				stat.responseTime += r.Time
				if r.StatusType == types.UP {
					stat.up++
				}
			}
			stat.responseTime = stat.responseTime / time.Duration(len(res))
			stat.uptime = float64(stat.up) / float64(len(res))
			statistics[ts] = stat
		}

		require.NoError(t, Aggregate(ctx, s))

		history, err = s.FindResponses(ctx, "test")
		require.NoError(t, err)

		for _, h := range history {
			if h.Uptime == 0 {
				continue
			}
			stat, ok := statistics[h.Timestamp]
			require.True(t, ok)
			require.Equal(t, stat.responseTime, h.Time)
			require.Equal(t, stat.uptime, h.Uptime)
			require.Equal(t, stat.count, h.Count)
			if h.Uptime > 0.95 {
				require.Equal(t, types.UP, h.StatusType)
			} else if h.Uptime > 0.5 {
				require.Equal(t, types.DEGRADED, h.StatusType)
			} else {
				require.Equal(t, types.DOWN, h.StatusType)
			}
		}
	})
}

func TestAggregateDay(t *testing.T) {
	ts := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	t.Run("empty responses", func(t *testing.T) {
		r := AggregateDay(ts, []*types.HttpResponse{})
		require.True(t, r.IsAggregated)
		require.Equal(t, 0, r.Count)
		require.Equal(t, float64(0), r.Uptime)
		require.Equal(t, ts, r.Timestamp)
	})

	t.Run("all up", func(t *testing.T) {
		responses := []*types.HttpResponse{
			{StatusType: types.UP, Time: 100 * time.Millisecond},
			{StatusType: types.UP, Time: 200 * time.Millisecond},
			{StatusType: types.UP, Time: 300 * time.Millisecond},
		}
		r := AggregateDay(ts, responses)
		require.True(t, r.IsAggregated)
		require.Equal(t, 3, r.Count)
		require.Equal(t, float64(1), r.Uptime)
		require.Equal(t, types.UP, r.StatusType)
		require.Equal(t, 200*time.Millisecond, r.Time)
	})

	t.Run("all down", func(t *testing.T) {
		responses := []*types.HttpResponse{
			{StatusType: types.DOWN, Time: 100 * time.Millisecond},
			{StatusType: types.DOWN, Time: 200 * time.Millisecond},
		}
		r := AggregateDay(ts, responses)
		require.Equal(t, float64(0), r.Uptime)
		require.Equal(t, types.DOWN, r.StatusType)
	})

	t.Run("mixed status produces degraded", func(t *testing.T) {
		responses := make([]*types.HttpResponse, 0)
		for i := 0; i < 10; i++ {
			s := types.UP
			if i < 3 {
				s = types.DOWN
			}
			responses = append(responses, &types.HttpResponse{StatusType: s, Time: 50 * time.Millisecond})
		}
		r := AggregateDay(ts, responses)
		require.Equal(t, float64(7)/float64(10), r.Uptime)
		require.Equal(t, types.DEGRADED, r.StatusType)
	})

	t.Run("unknown counts as not up", func(t *testing.T) {
		responses := []*types.HttpResponse{
			{StatusType: types.Unknown, Time: 100 * time.Millisecond},
			{StatusType: types.Unknown, Time: 100 * time.Millisecond},
		}
		r := AggregateDay(ts, responses)
		require.Equal(t, float64(0), r.Uptime)
		require.Equal(t, types.DOWN, r.StatusType)
	})

	t.Run("just above 95% is UP", func(t *testing.T) {
		responses := make([]*types.HttpResponse, 100)
		for i := range responses {
			s := types.UP
			if i < 4 {
				s = types.DOWN
			}
			responses[i] = &types.HttpResponse{StatusType: s, Time: time.Millisecond}
		}
		r := AggregateDay(ts, responses)
		require.Equal(t, types.UP, r.StatusType)
	})

	t.Run("exactly 95% is DEGRADED", func(t *testing.T) {
		responses := make([]*types.HttpResponse, 100)
		for i := range responses {
			s := types.UP
			if i < 5 {
				s = types.DOWN
			}
			responses[i] = &types.HttpResponse{StatusType: s, Time: time.Millisecond}
		}
		r := AggregateDay(ts, responses)
		require.Equal(t, types.DEGRADED, r.StatusType)
	})
}

func TestStore_LastResponse(t *testing.T) {
	ctx := context.Background()

	stores := map[string]func() Interface{
		"memory": func() Interface {
			return NewMemory(ctx)
		},
		"bolt": func() Interface {
			file, err := os.CreateTemp("", "test_last_*.db")
			require.NoError(t, err)
			defer os.RemoveAll(file.Name())
			b, err := NewBolt(ctx, file.Name())
			require.NoError(t, err)
			return b
		},
	}

	for name, f := range stores {
		t.Run(name, func(t *testing.T) {
			t.Run("no responses", func(t *testing.T) {
				s := f()
				resp, err := s.LastResponse(ctx, "nonexistent")
				require.NoError(t, err)
				require.Nil(t, resp)
			})

			t.Run("returns most recent", func(t *testing.T) {
				s := f()
				now := time.Now()
				require.NoError(t, s.AddResponse(ctx, "host", &types.HttpResponse{
					Timestamp: now.Add(-2 * time.Minute), Code: 200,
				}))
				require.NoError(t, s.AddResponse(ctx, "host", &types.HttpResponse{
					Timestamp: now.Add(-1 * time.Minute), Code: 201,
				}))
				require.NoError(t, s.AddResponse(ctx, "host", &types.HttpResponse{
					Timestamp: now, Code: 202,
				}))

				resp, err := s.LastResponse(ctx, "host")
				require.NoError(t, err)
				require.NotNil(t, resp)
				require.Equal(t, 202, resp.Code)
			})
		})
	}
}

func TestStore_DeleteIncident(t *testing.T) {
	ctx := context.Background()

	stores := map[string]func() Interface{
		"memory": func() Interface {
			return NewMemory(ctx)
		},
		"bolt": func() Interface {
			file, err := os.CreateTemp("", "test_delinc_*.db")
			require.NoError(t, err)
			defer os.RemoveAll(file.Name())
			b, err := NewBolt(ctx, file.Name())
			require.NoError(t, err)
			return b
		},
	}
	now := time.Now()

	for name, f := range stores {
		t.Run(name, func(t *testing.T) {
			s := f()
			for i := 0; i < 5; i++ {
				require.NoError(t, s.AddIncident(ctx, "host", &types.Incident{
					StartTS: now.Add(-time.Duration(i) * time.Minute),
				}))
				time.Sleep(time.Millisecond)
			}

			incidents, err := s.FindIncidents(ctx, "host", 0, 0)
			require.NoError(t, err)
			require.Len(t, incidents, 5)

			// delete the 3rd incident
			require.NoError(t, s.DeleteIncident(ctx, "host", incidents[2].ID))

			incidents, err = s.FindIncidents(ctx, "host", 0, 0)
			require.NoError(t, err)
			require.Len(t, incidents, 4)
		})
	}
}

func TestStore_DeleteIncident_NonExistent(t *testing.T) {
	ctx := context.Background()

	t.Run("memory", func(t *testing.T) {
		s := NewMemory(ctx)
		require.NoError(t, s.DeleteIncident(ctx, "nonexistent", 999))
	})

	t.Run("bolt", func(t *testing.T) {
		file, err := os.CreateTemp("", "test_delne_*.db")
		require.NoError(t, err)
		defer os.RemoveAll(file.Name())
		s, err := NewBolt(ctx, file.Name())
		require.NoError(t, err)
		require.NoError(t, s.DeleteIncident(ctx, "nonexistent", 999))
	})
}

func TestStore_FindResponses_Empty(t *testing.T) {
	ctx := context.Background()

	t.Run("memory", func(t *testing.T) {
		s := NewMemory(ctx)
		resp, err := s.FindResponses(ctx, "nonexistent")
		require.NoError(t, err)
		require.Empty(t, resp)
	})

	t.Run("bolt", func(t *testing.T) {
		file, err := os.CreateTemp("", "test_findempty_*.db")
		require.NoError(t, err)
		defer os.RemoveAll(file.Name())
		s, err := NewBolt(ctx, file.Name())
		require.NoError(t, err)

		resp, err := s.FindResponses(ctx, "nonexistent")
		require.NoError(t, err)
		require.Empty(t, resp)
	})
}

func TestStore_FindIncidents_Empty(t *testing.T) {
	ctx := context.Background()

	t.Run("memory", func(t *testing.T) {
		s := NewMemory(ctx)
		inc, err := s.FindIncidents(ctx, "nonexistent", 0, 10)
		require.NoError(t, err)
		require.Empty(t, inc)
	})

	t.Run("bolt", func(t *testing.T) {
		file, err := os.CreateTemp("", "test_findinc_*.db")
		require.NoError(t, err)
		defer os.RemoveAll(file.Name())
		s, err := NewBolt(ctx, file.Name())
		require.NoError(t, err)

		inc, err := s.FindIncidents(ctx, "nonexistent", 0, 10)
		require.NoError(t, err)
		require.Empty(t, inc)
	})
}

func TestHoursToMidnight(t *testing.T) {
	d := hoursToMidnight()
	require.Greater(t, d, time.Duration(0))
	require.LessOrEqual(t, d, 24*time.Hour+11*time.Minute)
}
