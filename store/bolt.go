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
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, fmt.Errorf("failed to open bolt database: %w", err)
	}

	b := &Bolt{
		conn: db,
	}
	if err := b.migrateResponses(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate response keys: %w", err)
	}

	return b, nil
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
		return bucket.Put(responseKey(r.Timestamp), data)
	})
}
func (b *Bolt) DeleteResponse(ctx context.Context, hostID string, keys []time.Time) error {
	return b.conn.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(hostID))
		if bucket == nil {
			return nil
		}

		for _, key := range keys {
			if err := bucket.Delete(responseKey(key)); err != nil {
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

func (b *Bolt) AggregateResponses(ctx context.Context, hostID string, day time.Time) error {
	return b.conn.Update(func(tx *bolt.Tx) error {
		if err := ctx.Err(); err != nil {
			return err
		}

		bucket := tx.Bucket([]byte(hostID))
		if bucket == nil {
			return nil
		}

		end := day.AddDate(0, 0, 1)
		responses := make([]*types.HttpResponse, 0)
		c := bucket.Cursor()
		for k, v := c.Seek(responseKey(day)); k != nil; k, v = c.Next() {
			var r types.HttpResponse
			if err := json.Unmarshal(v, &r); err != nil {
				return err
			}
			if !r.Timestamp.Before(end) {
				break
			}
			responses = append(responses, &r)
		}
		if len(responses) == 0 {
			return nil
		}

		data, err := json.Marshal(AggregateDay(day, responses))
		if err != nil {
			return err
		}
		for _, r := range responses {
			if err := bucket.Delete(responseKey(r.Timestamp)); err != nil {
				return err
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		return bucket.Put(responseKey(day), data)
	})
}

func (b *Bolt) migrateResponses(ctx context.Context) error {
	return b.conn.Update(func(tx *bolt.Tx) error {
		return tx.ForEach(func(name []byte, bucket *bolt.Bucket) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if strings.HasPrefix(string(name), "e-") && bucket.Sequence() > 0 {
				return nil
			}

			keys := make([][]byte, 0)
			if err := bucket.ForEach(func(k, v []byte) error {
				if len(k) == 8 {
					keys = append(keys, append([]byte{}, k...))
				}
				return nil
			}); err != nil {
				return err
			}

			for _, key := range keys {
				data := append([]byte{}, bucket.Get(key)...)
				var r types.HttpResponse
				if err := json.Unmarshal(data, &r); err != nil {
					return err
				}
				if err := bucket.Put(responseKey(r.Timestamp), data); err != nil {
					return err
				}
				if err := bucket.Delete(key); err != nil {
					return err
				}
			}

			return nil
		})
	})
}

func (b *Bolt) Hosts(ctx context.Context) ([]string, error) {
	keys := []string{}

	return keys, b.conn.View(func(tx *bolt.Tx) error {
		return tx.ForEach(func(name []byte, bucket *bolt.Bucket) error {
			if !strings.HasPrefix(string(name), "e-") || bucket.Sequence() == 0 {
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

		eventID, err := bucket.NextSequence()
		if err != nil {
			return err
		}
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

		key := itob(eventID)
		data := bucket.Get(key)
		if data == nil {
			return nil
		}
		var e types.Incident
		if err := json.Unmarshal(data, &e); err != nil {
			return err
		}

		e.EndTS = &ts
		data, err := json.Marshal(e)
		if err != nil {
			return err
		}

		return bucket.Put(key, data)
	})
}
func (b *Bolt) DeleteIncident(ctx context.Context, hostID string, eventID int) error {
	return b.conn.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(fmt.Sprintf("e-%s", hostID)))
		if bucket == nil {
			return nil
		}

		return bucket.Delete(itob(eventID))
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

func responseKey(ts time.Time) []byte {
	b := make([]byte, 12)
	binary.BigEndian.PutUint64(b, uint64(ts.Unix()))
	binary.BigEndian.PutUint32(b[8:], uint32(ts.Nanosecond()))
	return b
}
