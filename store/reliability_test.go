package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/exelban/EndPoll/types"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

func TestStore_AggregationPreservesHistory(t *testing.T) {
	for _, typ := range []string{"memory", "bolt", "sqlite"} {
		t.Run(typ, func(t *testing.T) {
			ctx := context.Background()
			s, err := New(ctx, typ, t.TempDir())
			require.NoError(t, err)
			defer s.Close()

			now := time.Now()
			today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
			day := today.AddDate(0, 0, -1)
			first := day.Add(time.Hour)

			for _, ts := range []time.Time{first, first.Add(time.Millisecond)} {
				require.NoError(t, s.AddResponse(ctx, "host", &types.HttpResponse{
					Timestamp:  ts,
					StatusType: types.UP,
					Time:       100 * time.Millisecond,
				}))
			}

			history, err := s.FindResponses(ctx, "host")
			require.NoError(t, err)
			require.Len(t, history, 2)

			cancelled, cancel := context.WithCancel(ctx)
			cancel()
			require.ErrorIs(t, s.AggregateResponses(cancelled, "host", day), context.Canceled)

			history, err = s.FindResponses(ctx, "host")
			require.NoError(t, err)
			require.Len(t, history, 2)

			require.NoError(t, Aggregate(ctx, s))
			require.NoError(t, s.AddResponse(ctx, "host", &types.HttpResponse{
				Timestamp:  first.Add(time.Second),
				StatusType: types.DOWN,
				Time:       400 * time.Millisecond,
			}))
			require.NoError(t, s.AddResponse(ctx, "host", &types.HttpResponse{
				Timestamp:  today,
				StatusType: types.UP,
			}))
			require.NoError(t, Aggregate(ctx, s))
			require.NoError(t, Aggregate(ctx, s))

			history, err = s.FindResponses(ctx, "host")
			require.NoError(t, err)
			require.Len(t, history, 2)
			require.Equal(t, 3, history[0].Count)
			require.InDelta(t, 2.0/3, history[0].Uptime, 0.0001)
			require.Equal(t, 200*time.Millisecond, history[0].Time)
			require.False(t, history[1].IsAggregated)
		})
	}
}

func TestBolt_LegacyResponseKeys(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "test.db")
	ts := time.Now().Truncate(time.Second).Add(100 * time.Millisecond)
	hostID := "e-host"

	db, err := bolt.Open(path, 0600, nil)
	require.NoError(t, err)
	require.NoError(t, db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(hostID))
		if err != nil {
			return err
		}
		data, err := json.Marshal(&types.HttpResponse{
			Timestamp:  ts,
			StatusType: types.UP,
		})
		if err != nil {
			return err
		}
		return bucket.Put(itob(int(ts.Unix())), data)
	}))
	require.NoError(t, db.Close())

	s, err := NewBolt(ctx, path)
	require.NoError(t, err)
	require.NoError(t, s.AddResponse(ctx, hostID, &types.HttpResponse{
		Timestamp:  ts.Add(time.Millisecond),
		StatusType: types.DOWN,
	}))
	require.NoError(t, s.AddIncident(ctx, hostID, &types.Incident{StartTS: ts}))
	require.NoError(t, s.Close())

	s, err = NewBolt(ctx, path)
	require.NoError(t, err)
	defer s.Close()

	hosts, err := s.Hosts(ctx)
	require.NoError(t, err)
	require.Equal(t, []string{hostID}, hosts)

	incidents, err := s.FindIncidents(ctx, hostID, 0, 0)
	require.NoError(t, err)
	require.Len(t, incidents, 1)
	require.Equal(t, ts, incidents[0].StartTS)

	history, err := s.FindResponses(ctx, hostID)
	require.NoError(t, err)
	require.Len(t, history, 2)

	last, err := s.LastResponse(ctx, hostID)
	require.NoError(t, err)
	require.Equal(t, types.DOWN, last.StatusType)

	require.NoError(t, s.DeleteResponse(ctx, hostID, []time.Time{ts}))
	history, err = s.FindResponses(ctx, hostID)
	require.NoError(t, err)
	require.Len(t, history, 1)
	require.Equal(t, types.DOWN, history[0].StatusType)
}

func TestBolt_AggregationRollback(t *testing.T) {
	ctx := context.Background()
	s, err := NewBolt(ctx, filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer s.Close()

	day := time.Now().AddDate(0, 0, -1).Truncate(24 * time.Hour)
	ts := day.Add(time.Hour)
	require.NoError(t, s.AddResponse(ctx, "host", &types.HttpResponse{
		Timestamp:  ts,
		StatusType: types.UP,
	}))
	require.NoError(t, s.conn.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("host")).Put(responseKey(ts.Add(time.Second)), []byte("broken record"))
	}))

	require.Error(t, s.AggregateResponses(ctx, "host", day))
	require.NoError(t, s.conn.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("host"))
		require.NotNil(t, bucket.Get(responseKey(ts)))
		require.Equal(t, []byte("broken record"), bucket.Get(responseKey(ts.Add(time.Second))))
		require.Nil(t, bucket.Get(responseKey(day)))
		return nil
	}))
}

func TestMemory_RecordCopies(t *testing.T) {
	ctx := context.Background()
	s := NewMemory(ctx)
	ts := time.Now()
	expiry := ts.Add(time.Hour)

	r := &types.HttpResponse{
		Timestamp:     ts,
		Bytes:         []byte("ok"),
		SSLCertExpiry: &expiry,
	}
	require.NoError(t, s.AddResponse(ctx, "host", r))

	r.Bytes[0] = 'x'
	expiry = ts
	last, err := s.LastResponse(ctx, "host")
	require.NoError(t, err)
	require.Equal(t, []byte("ok"), last.Bytes)
	require.Equal(t, ts.Add(time.Hour), *last.SSLCertExpiry)

	*last.SSLCertExpiry = ts
	history, err := s.FindResponses(ctx, "host")
	require.NoError(t, err)
	require.Equal(t, ts.Add(time.Hour), *history[0].SSLCertExpiry)

	e := &types.Incident{StartTS: ts}
	require.NoError(t, s.AddIncident(ctx, "host", e))

	e.Text = "changed"
	incidents, err := s.FindIncidents(ctx, "host", 0, 0)
	require.NoError(t, err)
	require.Empty(t, incidents[0].Text)

	require.NoError(t, s.EndIncident(ctx, "host", e.ID, ts.Add(time.Minute)))
	require.Nil(t, incidents[0].EndTS)

	incidents, err = s.FindIncidents(ctx, "host", 0, 0)
	require.NoError(t, err)

	*incidents[0].EndTS = ts
	incidents, err = s.FindIncidents(ctx, "host", 0, 0)
	require.NoError(t, err)
	require.Equal(t, ts.Add(time.Minute), *incidents[0].EndTS)
}

func TestStore_IncidentUpdates(t *testing.T) {
	for _, typ := range []string{"memory", "bolt", "sqlite"} {
		t.Run(typ, func(t *testing.T) {
			ctx := context.Background()
			s, err := New(ctx, typ, t.TempDir())
			require.NoError(t, err)
			defer s.Close()

			ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
			first := &types.Incident{StartTS: ts}
			second := &types.Incident{
				StartTS: ts.Add(time.Minute),
				Details: types.IncidentDetails{StatusCode: 503, Response: "unavailable", TS: ts},
			}
			require.NoError(t, s.AddIncident(ctx, "host", first))
			require.NoError(t, s.AddIncident(ctx, "host", second))

			end := ts.Add(time.Hour)
			require.NoError(t, s.EndIncident(ctx, "missing", first.ID, end))
			require.NoError(t, s.EndIncident(ctx, "host", second.ID+1, end))
			require.NoError(t, s.DeleteIncident(ctx, "missing", first.ID))
			require.NoError(t, s.DeleteIncident(ctx, "host", second.ID+1))
			require.NoError(t, s.EndIncident(ctx, "host", second.ID, end))

			incidents, err := s.FindIncidents(ctx, "host", 0, 0)
			require.NoError(t, err)
			expected := *second
			expected.EndTS = &end
			require.Equal(t, []*types.Incident{&expected, first}, incidents)

			require.NoError(t, s.DeleteIncident(ctx, "host", second.ID))
			require.NoError(t, s.DeleteIncident(ctx, "host", second.ID))
			require.NoError(t, s.EndIncident(ctx, "host", second.ID, end))
			incidents, err = s.FindIncidents(ctx, "host", 0, 0)
			require.NoError(t, err)
			require.Equal(t, []*types.Incident{first}, incidents)

			third := &types.Incident{StartTS: end}
			require.NoError(t, s.AddIncident(ctx, "host", third))
			require.Equal(t, second.ID+1, third.ID)
		})
	}
}

func TestBolt_EndIncident_InvalidRecord(t *testing.T) {
	ctx := context.Background()
	s, err := NewBolt(ctx, filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	defer s.Close()

	e := &types.Incident{StartTS: time.Now()}
	require.NoError(t, s.AddIncident(ctx, "host", e))
	require.NoError(t, s.conn.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("e-host")).Put(itob(e.ID), []byte("broken record"))
	}))

	var decodeErr *json.SyntaxError
	require.ErrorAs(t, s.EndIncident(ctx, "host", e.ID, time.Now()), &decodeErr)
	require.NoError(t, s.conn.View(func(tx *bolt.Tx) error {
		require.Equal(t, []byte("broken record"), tx.Bucket([]byte("e-host")).Get(itob(e.ID)))
		return nil
	}))
	require.NoError(t, s.DeleteIncident(ctx, "host", e.ID))
}
