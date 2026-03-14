package store

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/exelban/EndPoll/types"
	bolt "go.etcd.io/bbolt"
)

type Bolt struct {
	conn *bolt.DB
}

func NewBolt(ctx context.Context, path string) (*Bolt, error) {
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open bolt database: %w", err)
	}

	return &Bolt{
		conn: db,
	}, nil
}
func (b *Bolt) Close() error {
	return b.conn.Close()
}

func (b *Bolt) AddResponse(ctx context.Context, hostID string, r *types.HttpResponse) error {
	return b.conn.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(hostID))
		if err != nil {
			return err
		}
		data, err := json.Marshal(r)
		if err != nil {
			return err
		}
		return bucket.Put(itob(int(r.Timestamp.UTC().Unix())), data)
	})
}
func (b *Bolt) DeleteResponse(ctx context.Context, hostID string, keys []time.Time) error {
	return b.conn.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(hostID))
		if bucket == nil {
			return nil
		}

		for _, key := range keys {
			if err := bucket.Delete(itob(int(key.UTC().Unix()))); err != nil {
				return err
			}
		}

		return nil
	})
}
func (b *Bolt) FindResponses(ctx context.Context, hostID string) ([]*types.HttpResponse, error) {
	res := []*types.HttpResponse{}

	err := b.conn.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(hostID))
		if bucket == nil {
			return nil
		}
		return bucket.ForEach(func(k, v []byte) error {
			var r types.HttpResponse
			if err := json.Unmarshal(v, &r); err != nil {
				return err
			}
			res = append(res, &r)
			return nil
		})
	})
	sort.Slice(res, func(i, j int) bool {
		return res[i].Timestamp.Before(res[j].Timestamp)
	})

	return res, err
}
func (b *Bolt) LastResponse(ctx context.Context, hostID string) (*types.HttpResponse, error) {
	var res *types.HttpResponse

	err := b.conn.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(hostID))
		if bucket == nil {
			return nil
		}
		c := bucket.Cursor()
		k, v := c.Last()
		if k == nil {
			return nil
		}
		var r types.HttpResponse
		if err := json.Unmarshal(v, &r); err != nil {
			return err
		}
		res = &r
		return nil
	})

	return res, err
}

func (b *Bolt) Hosts(ctx context.Context) ([]string, error) {
	keys := []string{}

	return keys, b.conn.View(func(tx *bolt.Tx) error {
		return tx.ForEach(func(name []byte, _ *bolt.Bucket) error {
			if !strings.HasPrefix(string(name), "e-") {
				keys = append(keys, string(name))
			}
			return nil
		})
	})
}

func (b *Bolt) AddIncident(ctx context.Context, hostID string, e *types.Incident) error {
	return b.conn.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(fmt.Sprintf("e-%s", hostID)))
		if err != nil {
			return err
		}

		eventID, _ := bucket.NextSequence()
		e.ID = int(eventID)
		data, err := json.Marshal(e)
		if err != nil {
			return err
		}

		return bucket.Put(itob(e.ID), data)
	})
}
func (b *Bolt) EndIncident(ctx context.Context, hostID string, eventID int, ts time.Time) error {
	return b.conn.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(fmt.Sprintf("e-%s", hostID)))
		if bucket == nil {
			return nil
		}

		c := bucket.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			if int(binary.BigEndian.Uint64(k)) != eventID {
				continue
			}
			var e types.Incident
			if err := json.Unmarshal(v, &e); err != nil {
				return err
			}

			e.EndTS = &ts
			data, err := json.Marshal(e)
			if err != nil {
				return err
			}

			if err := bucket.Put(k, data); err != nil {
				return err
			}

			break
		}

		return nil
	})
}
func (b *Bolt) DeleteIncident(ctx context.Context, hostID string, eventID int) error {
	return b.conn.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(fmt.Sprintf("e-%s", hostID)))
		if bucket == nil {
			return nil
		}

		c := bucket.Cursor()
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			if int(binary.BigEndian.Uint64(k)) != eventID {
				continue
			}
			return bucket.Delete(k)
		}

		return nil
	})
}
func (b *Bolt) FindIncidents(ctx context.Context, hostID string, skip, limit int) ([]*types.Incident, error) {
	res := []*types.Incident{}
	err := b.conn.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(fmt.Sprintf("e-%s", hostID)))
		if bucket == nil {
			return nil
		}
		return bucket.ForEach(func(k, v []byte) error {
			var e types.Incident
			if err := json.Unmarshal(v, &e); err != nil {
				return err
			}
			res = append(res, &e)
			return nil
		})
	})
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

	return res, err
}

func itob(v int) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(v))
	return b
}
