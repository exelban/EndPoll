package monitor

import (
	"context"
	"math/rand/v2"
	"net/http"
	"testing"
	"time"

	"github.com/exelban/EndPoll/store"
	"github.com/exelban/EndPoll/types"
	"github.com/stretchr/testify/require"
)

func TestMonitor_Stats(t *testing.T) {
	ctx := context.Background()
	interval := time.Hour
	group := "group"

	t.Run("no hosts", func(t *testing.T) {
		m := Monitor{}
		s, err := m.Stats(ctx)
		require.NoError(t, err)
		require.NotNil(t, s)
		require.False(t, s.IsHost)
		require.Equal(t, types.Unknown, s.Status)
	})
	t.Run("no history", func(t *testing.T) {
		m := Monitor{
			Store: store.NewMemory(ctx),
			watchers: map[string]*watcher{
				"test": {
					host: &types.Host{
						ID:       "test",
						Interval: &interval,
						Group:    &group,
					},
					status: types.UP,
				},
			},
		}

		s, err := m.Stats(ctx)
		require.NoError(t, err)
		require.NotNil(t, s)
		require.False(t, s.IsHost)
		require.Len(t, s.Hosts, 1)
	})

	t.Run("not full history", func(t *testing.T) {
		m := Monitor{
			Store: store.NewMemory(ctx),
			watchers: map[string]*watcher{
				"test": {
					host: &types.Host{
						ID:       "test",
						Interval: &interval,
					},
					status: types.UP,
				},
			},
		}

		for _, r := range generateDays(30) {
			_ = m.Store.AddResponse(ctx, "test", r)
		}
		require.NoError(t, store.Aggregate(ctx, m.Store))

		history, err := m.Store.FindResponses(ctx, "test")
		require.NoError(t, err)
		require.NotEmpty(t, history)

		s, err := m.Stats(ctx)
		require.NoError(t, err)
		require.NotNil(t, s)
		require.False(t, s.IsHost)
		require.Len(t, s.Hosts, 1)

		host := s.Hosts[0]
		require.Len(t, host.Chart.Points, 91)
		require.Len(t, host.Chart.Intervals, 3)

		for _, p := range host.Chart.Points[:60] {
			require.Equal(t, types.Unknown, p.Status)
		}

		for i, p := range host.Chart.Points[60:90] {
			response := history[i]
			require.Equal(t, response.StatusType, p.Status)
			require.Equal(t, p.Timestamp, response.Timestamp.Format("2006-01-02"))
		}
	})

	t.Run("one host", func(t *testing.T) {
		m := Monitor{
			Store: store.NewMemory(ctx),
			watchers: map[string]*watcher{
				"test": {
					host: &types.Host{
						ID:       "test",
						Interval: &interval,
					},
					status: types.UP,
				},
			},
		}

		now := time.Now()
		for i := 0; i < rand.IntN(1000-100)+100; i++ {
			_ = m.Store.AddResponse(ctx, "test", generateResponse(now.Add(-time.Duration(i)*time.Minute)))
		}
		for _, r := range generateDays(rand.IntN(1000-100) + 100) {
			_ = m.Store.AddResponse(ctx, "test", r)
		}
		require.NoError(t, store.Aggregate(ctx, m.Store))

		history, err := m.Store.FindResponses(ctx, "test")
		require.NoError(t, err)
		require.NotEmpty(t, history)
		startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		days := make([]*types.HttpResponse, 0)
		today := make([]*types.HttpResponse, 0)
		for _, r := range history {
			if r.Timestamp.After(startOfDay) {
				today = append(today, r)
			} else {
				days = append(days, r)
			}
		}
		history = days[len(days)-90:]
		history = append(history, store.AggregateDay(startOfDay, today))
		require.Len(t, history, 91)

		s, err := m.Stats(ctx)
		require.NoError(t, err)
		require.NotNil(t, s)
		require.False(t, s.IsHost)
		require.Len(t, s.Hosts, 1)

		host := s.Hosts[0]
		require.Len(t, host.Chart.Points, len(history))
		require.Len(t, host.Chart.Intervals, 3)

		for i, p := range host.Chart.Points {
			response := history[len(history)-91+i]
			require.Equal(t, response.StatusType, p.Status)
			require.Equal(t, p.Timestamp, response.Timestamp.Format("2006-01-02"))
		}

		require.Equal(t, "90d", host.Chart.Intervals[0])
		require.Equal(t, "60d", host.Chart.Intervals[1])
		require.Equal(t, "30d", host.Chart.Intervals[2])
	})
	t.Run("one group", func(t *testing.T) {
		m := Monitor{
			Store: store.NewMemory(ctx),
			watchers: map[string]*watcher{
				"test": {
					host: &types.Host{
						ID:       "test",
						Interval: &interval,
						Group:    &group,
					},
					status: types.UP,
				},
			},
		}

		now := time.Now()
		for i := 0; i < rand.IntN(1000-100)+100; i++ {
			_ = m.Store.AddResponse(ctx, "test", generateResponse(now.Add(-time.Duration(i)*time.Minute)))
		}
		for _, r := range generateDays(rand.IntN(1000-100) + 100) {
			_ = m.Store.AddResponse(ctx, "test", r)
		}
		require.NoError(t, store.Aggregate(ctx, m.Store))

		history, err := m.Store.FindResponses(ctx, "test")
		require.NoError(t, err)
		require.NotEmpty(t, history)
		startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		days := make([]*types.HttpResponse, 0)
		today := make([]*types.HttpResponse, 0)
		for _, r := range history {
			if r.Timestamp.After(startOfDay) {
				today = append(today, r)
			} else {
				days = append(days, r)
			}
		}
		history = days[len(days)-90:]
		history = append(history, store.AggregateDay(startOfDay, today))
		require.Len(t, history, 91)

		s, err := m.Stats(ctx)
		require.NoError(t, err)
		require.NotNil(t, s)
		require.False(t, s.IsHost)
		require.Len(t, s.Hosts, 1)

		host := s.Hosts[0]
		require.Len(t, host.Chart.Points, len(history))
		require.Len(t, host.Chart.Intervals, 3)

		for i, p := range host.Chart.Points {
			response := history[len(history)-91+i]
			require.Equal(t, response.StatusType, p.Status)
			require.Equal(t, p.Timestamp, response.Timestamp.Format("2006-01-02"))
		}

		require.Equal(t, "90d", host.Chart.Intervals[0])
		require.Equal(t, "60d", host.Chart.Intervals[1])
		require.Equal(t, "30d", host.Chart.Intervals[2])
	})

	t.Run("order of hosts", func(t *testing.T) {
		storage := store.NewMemory(ctx)

		m := Monitor{
			Store: storage,
			watchers: map[string]*watcher{
				"test-1": {
					host: &types.Host{
						ID:       "test-1",
						Interval: &interval,
						Index:    0,
					},
				},
				"test-2": {
					host: &types.Host{
						ID:       "test-2",
						Interval: &interval,
						Index:    1,
					},
				},
				"test-3": {
					host: &types.Host{
						ID:       "test-3",
						Interval: &interval,
						Index:    2,
					},
				},
			},
		}

		s, err := m.Stats(ctx)
		require.NoError(t, err)
		require.NotNil(t, s)

		require.Equal(t, "test-1", s.Hosts[0].ID)
		require.Equal(t, "test-2", s.Hosts[1].ID)
		require.Equal(t, "test-3", s.Hosts[2].ID)

		m.watchers["test-1"].host.Index = 1
		m.watchers["test-2"].host.Index = 2
		m.watchers["test-3"].host.Index = 0

		s, err = m.Stats(ctx)
		require.NoError(t, err)
		require.NotNil(t, s)

		require.Equal(t, "test-3", s.Hosts[0].ID)
		require.Equal(t, "test-1", s.Hosts[1].ID)
		require.Equal(t, "test-2", s.Hosts[2].ID)
	})
}

func TestMonitor_StatsByID(t *testing.T) {
	ctx := context.Background()
	interval := time.Second

	t.Run("host not found", func(t *testing.T) {
		m := Monitor{}
		s, err := m.StatsByID(ctx, "not found", false)
		require.Nil(t, s)
		require.Error(t, err)
	})
	t.Run("no history", func(t *testing.T) {
		m := Monitor{
			Store: store.NewMemory(ctx),
			watchers: map[string]*watcher{
				"host": {
					host: &types.Host{
						ID:       "host",
						URL:      "host",
						Interval: &interval,
					},
				},
			},
		}

		now := time.Now()
		s, err := m.StatsByID(ctx, "host", false)
		require.NoError(t, err)
		require.NotNil(t, s)

		require.Equal(t, true, s.IsHost)
		require.Equal(t, types.Unknown, s.Status)
		require.Len(t, s.Hosts, 1)

		host := s.Hosts[0]
		require.Equal(t, types.Unknown, host.Status)
		require.NotEmpty(t, host.Chart)
		require.NotEmpty(t, host.Chart.Points)
		require.Len(t, host.Chart.Points, 90)
		require.Equal(t, 0, host.Uptime)
		require.Equal(t, "0s", host.ResponseTime)

		firstPoint := now.Add(-interval * 90)
		lastPoint := now.Add(-interval)
		require.Equal(t, firstPoint.Unix(), host.Chart.Points[0].TS.Unix())
		require.Equal(t, lastPoint.Unix(), host.Chart.Points[len(host.Chart.Points)-1].TS.Unix())

		for _, p := range host.Chart.Points {
			require.Equal(t, types.Unknown, p.Status)
		}

		require.NotEmpty(t, host.Chart.Intervals)
		require.Len(t, host.Chart.Intervals, 3)
		require.Equal(t, "2m", host.Chart.Intervals[0])
		require.Equal(t, "1m", host.Chart.Intervals[1])
		require.Equal(t, "30s", host.Chart.Intervals[2])

		//require.NotEmpty(t, host.Details)
		//require.NotEmpty(t, host.Details.Uptime)
		//require.Equal(t, "0", host.Details.Uptime[0])
		//require.Equal(t, "0", host.Details.Uptime[1])
		//require.Equal(t, "0", host.Details.Uptime[2])
		//require.NotEmpty(t, host.Details.ResponseTime)
		//require.Equal(t, "0ns", host.Details.ResponseTime[0])
		//require.Equal(t, "0ns", host.Details.ResponseTime[1])
		//require.Equal(t, "0ns", host.Details.ResponseTime[2])
	})
	t.Run("not full history", func(t *testing.T) {
		m := Monitor{
			Store: store.NewMemory(ctx),
			watchers: map[string]*watcher{
				"host": {
					host: &types.Host{
						ID:       "host",
						URL:      "host",
						Interval: &interval,
					},
				},
			},
		}

		responses := []*types.HttpResponse{}
		responseTime := time.Duration(0)
		for i := 0; i < 30; i++ {
			r := generateResponse(time.Now().Add(-time.Duration(i) * time.Second))
			_ = m.Store.AddResponse(ctx, "host", r)
			responses = append(responses, r)
			responseTime += r.Time
		}
		responseTime = responseTime / time.Duration(90)
		require.NoError(t, store.Aggregate(ctx, m.Store))

		s, err := m.StatsByID(ctx, "host", false)
		require.NoError(t, err)
		require.NotNil(t, s)

		host := s.Hosts[0]
		require.Equal(t, types.Unknown, host.Status)
		require.NotEmpty(t, host.Chart)
		require.NotEmpty(t, host.Chart.Points)
		require.Len(t, host.Chart.Points, 90)
		require.Equal(t, responseTime.Truncate(time.Millisecond).String(), host.ResponseTime)

		now := time.Now()
		for i, p := range host.Chart.Points[:60] {
			ts := now.Add(-interval*90 + interval*time.Duration(i))
			require.Equal(t, types.Unknown, p.Status)
			require.Equal(t, ts.Format("2006-01-02 15:04:05"), p.Timestamp)
		}

		for i, p := range host.Chart.Points[60:] {
			response := responses[29-i]
			require.Equal(t, response.StatusType, p.Status)
			require.Equal(t, p.Timestamp, response.Timestamp.Format("2006-01-02 15:04:05"))
		}
	})

	t.Run("few hours", func(t *testing.T) {
		m := Monitor{
			Store: store.NewMemory(ctx),
			watchers: map[string]*watcher{
				"host": {
					host: &types.Host{
						ID:       "host",
						URL:      "host",
						Interval: &interval,
					},
				},
			},
		}

		responses := generateHours(10)
		for _, r := range responses {
			_ = m.Store.AddResponse(ctx, "host", r)
		}
		require.NoError(t, store.Aggregate(ctx, m.Store))

		responseTime := time.Duration(0)
		for _, r := range responses[len(responses)-90:] {
			responseTime += r.Time
		}
		responseTime = responseTime / time.Duration(90)

		s, err := m.StatsByID(ctx, "host", false)
		require.NoError(t, err)
		require.NotNil(t, s)

		host := s.Hosts[0]
		require.Equal(t, types.Unknown, host.Status)
		require.NotEmpty(t, host.Chart)
		require.NotEmpty(t, host.Chart.Points)
		require.Len(t, host.Chart.Points, 90)
		require.Equal(t, responseTime.Truncate(time.Millisecond).String(), host.ResponseTime)

		for i, p := range host.Chart.Points {
			response := responses[len(responses)-90+i]
			require.Equal(t, response.StatusType, p.Status)
			require.Equal(t, p.Timestamp, response.Timestamp.Format("2006-01-02 15:04:05"))
		}
	})
	t.Run("few days history only", func(t *testing.T) {
		m := Monitor{
			Store: store.NewMemory(ctx),
			watchers: map[string]*watcher{
				"host": {
					host: &types.Host{
						ID:       "host",
						URL:      "host",
						Interval: &interval,
					},
				},
			},
		}

		raw := generateDays(90)
		for _, r := range raw {
			_ = m.Store.AddResponse(ctx, "host", r)
		}
		require.NoError(t, store.Aggregate(ctx, m.Store))
		history, err := m.Store.FindResponses(ctx, "host")
		require.NoError(t, err)
		require.Equal(t, 90, len(history))

		responseTime := time.Duration(0)
		for _, r := range history[len(history)-90:] {
			responseTime += r.Time
		}
		responseTime = responseTime / time.Duration(90)

		s, err := m.StatsByID(ctx, "host", false)
		require.NoError(t, err)
		require.NotNil(t, s)

		require.Equal(t, true, s.IsHost)
		require.Equal(t, types.Unknown, s.Status)
		require.Len(t, s.Hosts, 1)

		host := s.Hosts[0]
		require.Equal(t, types.Unknown, host.Status)
		require.NotEmpty(t, host.Chart)
		require.NotEmpty(t, host.Chart.Points)
		require.Len(t, host.Chart.Points, 90)
		require.Equal(t, responseTime.Truncate(time.Millisecond).String(), host.ResponseTime)

		for i, p := range host.Chart.Points {
			response := history[len(history)-90+i]
			require.Equal(t, response.StatusType, p.Status)
			require.Equal(t, p.Timestamp, response.Timestamp.Format("2006-01-02"))
		}
	})
	t.Run("mixed history", func(t *testing.T) {
		m := Monitor{
			Store: store.NewMemory(ctx),
			watchers: map[string]*watcher{
				"host": {
					host: &types.Host{
						ID:       "host",
						URL:      "host",
						Interval: &interval,
					},
				},
			},
		}

		now := time.Now()
		for i := 0; i < 45; i++ {
			_ = m.Store.AddResponse(ctx, "host", generateResponse(now.Add(-time.Duration(i)*time.Minute)))
		}
		for _, r := range generateDays(90) {
			_ = m.Store.AddResponse(ctx, "host", r)
		}
		require.NoError(t, store.Aggregate(ctx, m.Store))
		history, err := m.Store.FindResponses(ctx, "host")
		require.NoError(t, err)
		history = history[len(history)-90:]
		require.Equal(t, 90, len(history))

		responseTime := time.Duration(0)
		for _, r := range history {
			responseTime += r.Time
		}
		responseTime = responseTime / time.Duration(90)

		s, err := m.StatsByID(ctx, "host", false)
		require.NoError(t, err)
		require.NotNil(t, s)

		host := s.Hosts[0]
		require.Equal(t, types.Unknown, host.Status)
		require.NotEmpty(t, host.Chart)
		require.NotEmpty(t, host.Chart.Points)
		require.Len(t, host.Chart.Points, 90)
		require.Equal(t, responseTime.Truncate(time.Millisecond).String(), host.ResponseTime)

		for i, p := range host.Chart.Points[:45] {
			response := history[len(history)-90+i]
			require.Equal(t, response.StatusType, p.Status)
			require.Equal(t, p.Timestamp, response.Timestamp.Format("2006-01-02"))
		}
		for i, p := range host.Chart.Points[45:] {
			response := history[len(history)-45+i]
			require.Equal(t, response.StatusType, p.Status)
			require.Equal(t, p.Timestamp, response.Timestamp.Format("2006-01-02 15:04:05"))
		}
	})

	t.Run("day", func(t *testing.T) {
		m := Monitor{
			Store: store.NewMemory(ctx),
			watchers: map[string]*watcher{
				"host": {
					host: &types.Host{
						ID:       "host",
						URL:      "host",
						Interval: &interval,
					},
				},
			},
		}

		now := time.Now()
		for i := 0; i < 45; i++ {
			_ = m.Store.AddResponse(ctx, "host", generateResponse(now.Add(-time.Duration(i)*time.Minute)))
		}
		for _, r := range generateDays(90) {
			_ = m.Store.AddResponse(ctx, "host", r)
		}
		require.NoError(t, store.Aggregate(ctx, m.Store))
		history, err := m.Store.FindResponses(ctx, "host")
		require.NoError(t, err)
		history = history[len(history)-90:]
		require.Equal(t, 90, len(history))

		responseTime := time.Duration(0)
		for _, r := range history {
			responseTime += r.Time
		}
		responseTime = responseTime / time.Duration(90)

		s, err := m.StatsByID(ctx, "host", true)
		require.NoError(t, err)
		require.NotNil(t, s)

		host := s.Hosts[0]
		require.Equal(t, types.Unknown, host.Status)
		require.NotEmpty(t, host.Chart)
		require.NotEmpty(t, host.Chart.Points)
		//require.Len(t, host.Chart.Points, 90)
		//require.Equal(t, responseTime.Truncate(time.Millisecond).String(), host.ResponseTime)

		require.Equal(t, now.Add(-time.Hour*24*90).Format("2006-01-02"), host.Chart.Points[0].Timestamp)
		require.Equal(t, now.Format("2006-01-02"), host.Chart.Points[len(host.Chart.Points)-1].Timestamp)

		//for i, p := range host.Chart.Points[:45] {
		//	response := history[len(history)-90+i]
		//	require.Equal(t, response.StatusType, p.Status)
		//	require.Equal(t, p.Timestamp, response.Timestamp.Format("2006-01-02"))
		//}
		//for i, p := range host.Chart.Points[45:] {
		//	response := history[len(history)-45+i]
		//	require.Equal(t, response.StatusType, p.Status)
		//	require.Equal(t, p.Timestamp, response.Timestamp.Format("2006-01-02 15:04:05"))
		//}
	})
}

func TestGenIntervals_FewPoints(t *testing.T) {
	t.Run("less than 61 points returns empty intervals", func(t *testing.T) {
		points := make([]*types.Point, 10)
		for i := range points {
			points[i] = &types.Point{TS: time.Now()}
		}
		intervals := genIntervals(points)
		require.Len(t, intervals, 3)
		require.Equal(t, "", intervals[0])
		require.Equal(t, "", intervals[1])
		require.Equal(t, "", intervals[2])
	})

	t.Run("exactly 61 points works", func(t *testing.T) {
		now := time.Now()
		points := make([]*types.Point, 61)
		for i := range points {
			points[i] = &types.Point{TS: now.Add(-time.Duration(61-i) * time.Hour * 24)}
		}
		intervals := genIntervals(points)
		require.Len(t, intervals, 3)
		for _, interval := range intervals {
			require.NotEmpty(t, interval)
		}
	})

	t.Run("91 points works", func(t *testing.T) {
		now := time.Now()
		points := make([]*types.Point, 91)
		for i := range points {
			points[i] = &types.Point{TS: now.Add(-time.Duration(91-i) * time.Hour * 24)}
		}
		intervals := genIntervals(points)
		require.Len(t, intervals, 3)
		require.Contains(t, intervals[0], "d")
		require.Contains(t, intervals[1], "d")
		require.Contains(t, intervals[2], "d")
	})
}

func TestGenerateGroupStatus(t *testing.T) {
	t.Run("all up", func(t *testing.T) {
		hosts := []types.Stat{
			{Status: types.UP},
			{Status: types.UP},
		}
		require.Equal(t, types.UP, generateGroupStatus(&hosts, nil))
	})

	t.Run("all down", func(t *testing.T) {
		hosts := []types.Stat{
			{Status: types.DOWN},
			{Status: types.DOWN},
		}
		require.Equal(t, types.DOWN, generateGroupStatus(&hosts, nil))
	})

	t.Run("mixed up and down is degraded", func(t *testing.T) {
		hosts := []types.Stat{
			{Status: types.UP},
			{Status: types.DOWN},
		}
		require.Equal(t, types.DEGRADED, generateGroupStatus(&hosts, nil))
	})

	t.Run("up with unknown is up", func(t *testing.T) {
		hosts := []types.Stat{
			{Status: types.UP},
			{Status: types.Unknown},
		}
		require.Equal(t, types.UP, generateGroupStatus(&hosts, nil))
	})

	t.Run("all unknown", func(t *testing.T) {
		hosts := []types.Stat{
			{Status: types.Unknown},
			{Status: types.Unknown},
		}
		require.Equal(t, types.Unknown, generateGroupStatus(&hosts, nil))
	})

	t.Run("degraded present", func(t *testing.T) {
		hosts := []types.Stat{
			{Status: types.UP},
			{Status: types.DEGRADED},
		}
		require.Equal(t, types.DEGRADED, generateGroupStatus(&hosts, nil))
	})

	t.Run("with point index", func(t *testing.T) {
		points := make([]*types.Point, 91)
		for i := range points {
			points[i] = &types.Point{Status: types.UP}
		}
		points[5] = &types.Point{Status: types.DOWN}

		hosts := []types.Stat{
			{Chart: types.Chart{Points: points}},
		}
		idx := 5
		require.Equal(t, types.DOWN, generateGroupStatus(&hosts, &idx))
		idx = 0
		require.Equal(t, types.UP, generateGroupStatus(&hosts, &idx))
	})
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		input    time.Duration
		expected string
	}{
		{0, "0ns"},
		{500 * time.Nanosecond, "0ns"},
		{5 * time.Millisecond, "5ms"},
		{500 * time.Millisecond, "500ms"},
		{5 * time.Second, "5s"},
		{90 * time.Second, "1m"},
		{5 * time.Minute, "5m"},
		{2 * time.Hour, "2h"},
		{48 * time.Hour, "2d"},
		{72*time.Hour + 30*time.Minute, "3d"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			require.Equal(t, tt.expected, formatDuration(tt.input))
		})
	}
}

func TestProcessIncidents(t *testing.T) {
	t.Run("open incident", func(t *testing.T) {
		incidents := []*types.Incident{
			{
				StartTS: time.Now().Add(-1 * time.Hour),
				Details: types.IncidentDetails{
					StatusCode: 500,
				},
			},
		}
		processIncidents(incidents)
		require.Contains(t, incidents[0].Text, "down for")
		require.NotEmpty(t, incidents[0].Start)
		require.Empty(t, incidents[0].End)
		require.Equal(t, "Internal Server Error", incidents[0].Details.StatusText)
	})

	t.Run("closed incident", func(t *testing.T) {
		start := time.Now().Add(-2 * time.Hour)
		end := time.Now().Add(-1 * time.Hour)
		incidents := []*types.Incident{
			{
				StartTS: start,
				EndTS:   &end,
				Details: types.IncidentDetails{
					StatusCode: 522,
				},
			},
		}
		processIncidents(incidents)
		require.Contains(t, incidents[0].Text, "was down for")
		require.NotEmpty(t, incidents[0].Duration)
		require.NotEmpty(t, incidents[0].End)
		require.Equal(t, "Connection timed out", incidents[0].Details.StatusText)
	})

	t.Run("custom status codes", func(t *testing.T) {
		codes := map[int]string{
			521: "Web server is down",
			522: "Connection timed out",
			523: "Origin is unreachable",
			404: http.StatusText(404),
		}
		for code, expected := range codes {
			incidents := []*types.Incident{
				{
					StartTS: time.Now(),
					Details: types.IncidentDetails{StatusCode: code},
				},
			}
			processIncidents(incidents)
			require.Equal(t, expected, incidents[0].Details.StatusText)
		}
	})
}

func TestMonitor_ResponseTime(t *testing.T) {
	ctx := context.Background()
	interval := time.Second

	t.Run("no history", func(t *testing.T) {
		m := Monitor{
			Store: store.NewMemory(ctx),
			watchers: map[string]*watcher{
				"host": {host: &types.Host{ID: "host", Interval: &interval}},
			},
		}
		x, y, err := m.ResponseTime(ctx, "host")
		require.NoError(t, err)
		require.Empty(t, x)
		require.Empty(t, y)
	})

	t.Run("aggregates by day", func(t *testing.T) {
		m := Monitor{
			Store: store.NewMemory(ctx),
			watchers: map[string]*watcher{
				"host": {host: &types.Host{ID: "host", Interval: &interval}},
			},
		}
		now := time.Now()
		// Add responses across 3 days
		for i := 0; i < 3; i++ {
			day := now.AddDate(0, 0, -i)
			for j := 0; j < 5; j++ {
				_ = m.Store.AddResponse(ctx, "host", &types.HttpResponse{
					Timestamp: day.Add(time.Duration(j) * time.Minute),
					Time:      time.Duration(100*(i+1)) * time.Millisecond,
				})
			}
		}

		x, y, err := m.ResponseTime(ctx, "host")
		require.NoError(t, err)
		require.Equal(t, 3, len(x))
		require.Equal(t, 3, len(y))

		// values should be sorted by time
		require.True(t, x[0].Before(x[1]))
		require.True(t, x[1].Before(x[2]))
	})
}

func TestGetDetails(t *testing.T) {
	t.Run("no responses", func(t *testing.T) {
		d := getDetails([]*types.HttpResponse{}, []*types.Incident{})
		require.NotNil(t, d)
		require.Nil(t, d.LastOutage)
		require.Nil(t, d.SSL)
	})

	t.Run("with SSL cert", func(t *testing.T) {
		expiry := time.Now().Add(30 * 24 * time.Hour)
		responses := []*types.HttpResponse{
			{
				Timestamp:     time.Now(),
				SSLCertExpiry: &expiry,
				StatusType:    types.UP,
			},
		}
		d := getDetails(responses, []*types.Incident{})
		require.NotNil(t, d.SSL)
		require.Equal(t, 29, d.SSL.ExpireInDays) // approximately 30 days
	})

	t.Run("with incidents", func(t *testing.T) {
		end := time.Now().Add(-1 * time.Hour)
		incidents := []*types.Incident{
			{
				StartTS:  time.Now().Add(-2 * time.Hour),
				EndTS:    &end,
				Duration: "1h",
			},
		}
		d := getDetails([]*types.HttpResponse{}, incidents)
		require.NotNil(t, d.LastOutage)
		require.NotEmpty(t, d.LastOutage.Since)
	})

	t.Run("30 day uptime calculation", func(t *testing.T) {
		now := time.Now()
		responses := make([]*types.HttpResponse, 0)
		for i := 0; i < 100; i++ {
			s := types.UP
			if i < 10 {
				s = types.DOWN
			}
			responses = append(responses, &types.HttpResponse{
				Timestamp:  now.Add(-time.Duration(i) * time.Hour),
				StatusType: s,
				Time:       50 * time.Millisecond,
			})
		}

		d := getDetails(responses, []*types.Incident{})
		require.NotNil(t, d)
		require.NotEmpty(t, d.Uptime)
		require.NotEmpty(t, d.ResponseTime)
	})
}

func generateDays(days int) []*types.HttpResponse {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day()-days, 0, 0, 0, 0, now.Location())
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	count := rand.IntN(10000-3000) + 3000
	interval := now.Sub(start) / time.Duration(count)
	res := []*types.HttpResponse{}

	for i := 0; i < count; i++ {
		r := generateResponse(start.Add(interval * time.Duration(i)))
		if r.Timestamp.After(day) {
			continue
		}
		res = append(res, r)
	}

	return res
}
func generateHours(hours int) []*types.HttpResponse {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), now.Hour()-hours, 0, 0, 0, now.Location())
	count := rand.IntN(1000-300) + 300
	interval := now.Sub(start) / time.Duration(count)
	res := []*types.HttpResponse{}

	for i := 0; i < count; i++ {
		res = append(res, generateResponse(start.Add(interval*time.Duration(i))))
	}

	return res
}

func generateResponse(ts time.Time) *types.HttpResponse {
	var status types.StatusType
	switch randInt(1, 3) {
	case 1:
		status = types.UP
	case 2:
		status = types.DOWN
	default:
		status = types.Unknown
	}

	return &types.HttpResponse{
		Timestamp:  ts,
		StatusType: status,
		Time:       time.Duration(randInt(1, 100)) * time.Millisecond,
	}
}

func randInt(min, max int) int {
	return rand.IntN(max-min) + min
}
