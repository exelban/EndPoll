package store

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/exelban/EndPoll/types"
	"github.com/stretchr/testify/require"
)

func newTestSQLite(t *testing.T) *SQLite {
	t.Helper()

	s, err := NewSQLite(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, s.Close())
	})
	return s
}

func TestSQLite_Persistence(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "history ?#%.db")
	ts := time.Date(2026, 1, 2, 3, 4, 5, 123456789, time.UTC)
	expiry := ts.AddDate(0, 1, 0)
	r := &types.HttpResponse{
		Timestamp:     ts,
		Time:          100 * time.Millisecond,
		Code:          200,
		Body:          "ok",
		Status:        true,
		StatusType:    types.UP,
		DNS:           time.Millisecond,
		TLSHandshake:  2 * time.Millisecond,
		Connect:       3 * time.Millisecond,
		TTFB:          4 * time.Millisecond,
		SSLCertExpiry: &expiry,
		SSLIssuer:     "issuer",
		TLSVersion:    "TLS 1.3",
	}
	e := &types.Incident{
		StartTS: ts,
		Details: types.IncidentDetails{StatusCode: 503, Response: "unavailable", TS: ts},
	}

	s, err := NewSQLite(ctx, path)
	require.NoError(t, err)
	require.NoError(t, s.AddResponse(ctx, "host", r))
	require.NoError(t, s.AddIncident(ctx, "host", e))
	end := ts.Add(time.Hour)
	require.NoError(t, s.EndIncident(ctx, "host", e.ID, end))
	require.NoError(t, s.Close())

	s, err = NewSQLite(ctx, path)
	require.NoError(t, err)

	history, err := s.FindResponses(ctx, "host")
	require.NoError(t, err)
	require.Equal(t, []*types.HttpResponse{r}, history)
	last, err := s.LastResponse(ctx, "host")
	require.NoError(t, err)
	require.Equal(t, r, last)
	incidents, err := s.FindIncidents(ctx, "host", 0, 0)
	require.NoError(t, err)
	expected := *e
	expected.EndTS = &end
	require.Equal(t, []*types.Incident{&expected}, incidents)
	require.NoError(t, s.DeleteIncident(ctx, "host", e.ID))
	require.NoError(t, s.Close())

	s, err = NewSQLite(ctx, path)
	require.NoError(t, err)
	defer s.Close()

	next := &types.Incident{StartTS: end}
	require.NoError(t, s.AddIncident(ctx, "host", next))
	require.Equal(t, e.ID+1, next.ID)
	other := &types.Incident{StartTS: end}
	require.NoError(t, s.AddIncident(ctx, "other", other))
	require.Equal(t, 1, other.ID)

	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestSQLite_Empty(t *testing.T) {
	ctx := context.Background()
	s := newTestSQLite(t)

	history, err := s.FindResponses(ctx, "missing")
	require.NoError(t, err)
	require.Equal(t, []*types.HttpResponse{}, history)
	last, err := s.LastResponse(ctx, "missing")
	require.NoError(t, err)
	require.Nil(t, last)
	incidents, err := s.FindIncidents(ctx, "missing", 0, 10)
	require.NoError(t, err)
	require.Equal(t, []*types.Incident{}, incidents)
	require.NoError(t, s.DeleteResponse(ctx, "missing", []time.Time{time.Now()}))
	require.NoError(t, s.DeleteIncident(ctx, "missing", 1))
	require.NoError(t, s.EndIncident(ctx, "missing", 1, time.Now()))
	require.NoError(t, s.AggregateResponses(ctx, "missing", time.Now()))

	require.NoError(t, s.AddIncident(ctx, "incident-only", &types.Incident{StartTS: time.Now()}))
	hosts, err := s.Hosts(ctx)
	require.NoError(t, err)
	require.Equal(t, []string{}, hosts)
	incidents, err = s.FindIncidents(ctx, "incident-only", 2, 10)
	require.NoError(t, err)
	require.Empty(t, incidents)
}

func TestSQLite_ResponseTimestamps(t *testing.T) {
	ctx := context.Background()
	s := newTestSQLite(t)
	hostID := "host'; DROP TABLE responses; --"
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for _, ts := range []time.Time{ts.Add(time.Nanosecond), ts, time.Time{}} {
		require.NoError(t, s.AddResponse(ctx, hostID, &types.HttpResponse{Timestamp: ts, Code: 200}))
	}
	other := &types.HttpResponse{Timestamp: ts, Code: 503}
	require.NoError(t, s.AddResponse(ctx, "other", other))
	local := ts.In(time.FixedZone("offset", 2*60*60))
	require.NoError(t, s.AddResponse(ctx, hostID, &types.HttpResponse{Timestamp: local, Code: 201}))

	history, err := s.FindResponses(ctx, hostID)
	require.NoError(t, err)
	require.Len(t, history, 3)
	require.True(t, history[0].Timestamp.IsZero())
	require.True(t, history[1].Timestamp.Equal(ts))
	require.Equal(t, 201, history[1].Code)
	require.True(t, history[2].Timestamp.Equal(ts.Add(time.Nanosecond)))
	last, err := s.LastResponse(ctx, hostID)
	require.NoError(t, err)
	require.Equal(t, history[2], last)

	require.NoError(t, s.DeleteResponse(ctx, hostID, []time.Time{local, time.Time{}}))
	history, err = s.FindResponses(ctx, hostID)
	require.NoError(t, err)
	require.Equal(t, []*types.HttpResponse{last}, history)
	history, err = s.FindResponses(ctx, "other")
	require.NoError(t, err)
	require.Equal(t, []*types.HttpResponse{other}, history)
}

func TestSQLite_AggregationRollback(t *testing.T) {
	ctx := context.Background()
	s := newTestSQLite(t)
	day := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	r := &types.HttpResponse{Timestamp: day.Add(time.Hour), StatusType: types.UP}
	require.NoError(t, s.AddResponse(ctx, "host", r))
	_, err := s.conn.ExecContext(ctx, `
		CREATE TRIGGER reject_insert BEFORE INSERT ON responses
		BEGIN SELECT RAISE(ABORT, 'test write failure'); END
	`)
	require.NoError(t, err)

	require.ErrorContains(t, s.AggregateResponses(ctx, "host", day), "test write failure")
	history, err := s.FindResponses(ctx, "host")
	require.NoError(t, err)
	require.Equal(t, []*types.HttpResponse{r}, history)

	_, err = s.conn.ExecContext(ctx, "DROP TRIGGER reject_insert")
	require.NoError(t, err)
	require.NoError(t, s.AggregateResponses(ctx, "host", day))
	history, err = s.FindResponses(ctx, "host")
	require.NoError(t, err)
	require.Len(t, history, 1)
	require.True(t, history[0].IsAggregated)
	require.Equal(t, 1, history[0].Count)
}

func TestSQLite_AggregationDayBoundaries(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)

	for _, day := range []time.Time{
		time.Date(2026, 3, 8, 0, 0, 0, 0, loc),
		time.Date(2026, 11, 1, 0, 0, 0, 0, loc),
	} {
		t.Run(day.Format("2006-01-02"), func(t *testing.T) {
			ctx := context.Background()
			s := newTestSQLite(t)
			end := day.AddDate(0, 0, 1)
			for _, ts := range []time.Time{day.Add(-time.Nanosecond), day, end.Add(-time.Nanosecond), end} {
				require.NoError(t, s.AddResponse(ctx, "host", &types.HttpResponse{Timestamp: ts, StatusType: types.UP}))
			}
			require.NoError(t, s.AddResponse(ctx, "other", &types.HttpResponse{Timestamp: day, StatusType: types.DOWN}))
			require.NoError(t, s.AggregateResponses(ctx, "host", day))
			require.NoError(t, s.AggregateResponses(ctx, "host", day))

			history, err := s.FindResponses(ctx, "host")
			require.NoError(t, err)
			require.Len(t, history, 3)
			require.True(t, history[0].Timestamp.Equal(day.Add(-time.Nanosecond)))
			require.False(t, history[0].IsAggregated)
			require.True(t, history[1].Timestamp.Equal(day))
			require.True(t, history[1].IsAggregated)
			require.Equal(t, 2, history[1].Count)
			require.True(t, history[2].Timestamp.Equal(end))
			require.False(t, history[2].IsAggregated)
			history, err = s.FindResponses(ctx, "other")
			require.NoError(t, err)
			require.Len(t, history, 1)
			require.False(t, history[0].IsAggregated)
		})
	}
}

func TestSQLite_ConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "test.db")
	first, err := NewSQLite(ctx, path)
	require.NoError(t, err)
	defer first.Close()
	second, err := NewSQLite(ctx, path)
	require.NoError(t, err)
	defer second.Close()
	stores := []*SQLite{first, second}
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	count := 40
	errs := make(chan error, count)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s := stores[i%len(stores)]
			r := &types.HttpResponse{Timestamp: ts.Add(time.Duration(i)), StatusType: types.UP}
			if err := s.AddResponse(ctx, "host", r); err != nil {
				errs <- err
				return
			}
			e := &types.Incident{StartTS: r.Timestamp}
			if err := s.AddIncident(ctx, "host", e); err != nil {
				errs <- err
				return
			}
			errs <- s.EndIncident(ctx, "host", e.ID, r.Timestamp.Add(time.Minute))
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	history, err := first.FindResponses(ctx, "host")
	require.NoError(t, err)
	require.Len(t, history, count)
	incidents, err := second.FindIncidents(ctx, "host", 0, 0)
	require.NoError(t, err)
	require.Len(t, incidents, count)
	for i, e := range incidents {
		require.Equal(t, count-i, e.ID)
		require.NotNil(t, e.EndTS)
	}
}

func TestSQLite_CancelledContext(t *testing.T) {
	s := newTestSQLite(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ts := time.Now()

	require.ErrorIs(t, s.AddResponse(ctx, "host", &types.HttpResponse{Timestamp: ts}), context.Canceled)
	require.ErrorIs(t, s.DeleteResponse(ctx, "host", []time.Time{ts}), context.Canceled)
	_, err := s.FindResponses(ctx, "host")
	require.ErrorIs(t, err, context.Canceled)
	_, err = s.LastResponse(ctx, "host")
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, s.AggregateResponses(ctx, "host", ts), context.Canceled)
	_, err = s.Hosts(ctx)
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, s.AddIncident(ctx, "host", &types.Incident{StartTS: ts}), context.Canceled)
	require.ErrorIs(t, s.EndIncident(ctx, "host", 1, ts), context.Canceled)
	require.ErrorIs(t, s.DeleteIncident(ctx, "host", 1), context.Canceled)
	_, err = s.FindIncidents(ctx, "host", 0, 0)
	require.ErrorIs(t, err, context.Canceled)
}

func TestSQLite_New(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "nested", "data")
	s, err := New(ctx, "sqlite", dir)
	require.NoError(t, err)
	require.IsType(t, &SQLite{}, s.(*scheduledStore).Interface)
	require.FileExists(t, filepath.Join(dir, "endpoll.sqlite.db"))

	day := time.Now().AddDate(0, 0, -2).Truncate(24 * time.Hour)
	require.NoError(t, s.AddResponse(ctx, "host", &types.HttpResponse{Timestamp: day.Add(time.Hour), StatusType: types.UP}))
	require.NoError(t, s.Close())
	s, err = New(ctx, "sqlite", dir)
	require.NoError(t, err)

	history, err := s.FindResponses(ctx, "host")
	require.NoError(t, err)
	require.Len(t, history, 1)
	require.True(t, history[0].IsAggregated)
	require.NoError(t, s.Close())

	for _, name := range []string{"endpoll.db", "jam.db"} {
		bolt, err := NewBolt(ctx, filepath.Join(dir, name))
		require.NoError(t, err)
		r := &types.HttpResponse{Timestamp: day, StatusType: types.DOWN}
		require.NoError(t, bolt.AddResponse(ctx, "bolt-host", r))

		s, err := New(ctx, "sqlite", dir)
		require.NoError(t, err)
		hosts, err := s.Hosts(ctx)
		require.NoError(t, err)
		require.Equal(t, []string{"host"}, hosts)
		require.NoError(t, s.Close())
		history, err := bolt.FindResponses(ctx, "bolt-host")
		require.NoError(t, err)
		require.Equal(t, []*types.HttpResponse{r}, history)
		require.NoError(t, bolt.Close())
	}
}

func TestSQLite_OpenError(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	_, err := NewSQLite(ctx, dir)
	require.Error(t, err)

	path := filepath.Join(dir, "invalid.db")
	require.NoError(t, os.WriteFile(path, []byte("not a database"), 0600))
	_, err = NewSQLite(ctx, path)
	require.Error(t, err)
	_, err = New(ctx, "sqlite", path)
	require.Error(t, err)

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	path = filepath.Join(dir, "cancelled.db")
	_, err = NewSQLite(cancelled, path)
	require.ErrorIs(t, err, context.Canceled)
	s, err := NewSQLite(ctx, path)
	require.NoError(t, err)
	require.NoError(t, s.Close())
}
