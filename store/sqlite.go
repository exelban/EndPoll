package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/exelban/EndPoll/types"
	_ "modernc.org/sqlite" // sqlite driver
)

type SQLite struct {
	conn *sql.DB
}

func NewSQLite(ctx context.Context, path string) (*SQLite, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve sqlite path: %w", err)
	}
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, err
	}

	u := url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	q := u.Query()
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Set("_txlock", "immediate")
	u.RawQuery = q.Encode()

	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}
	// Serialize access so concurrent checks share one SQLite writer.
	db.SetMaxOpenConns(1)

	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS responses (
			host_id TEXT NOT NULL,
			timestamp INTEGER NOT NULL,
			nanosecond INTEGER NOT NULL,
			data BLOB NOT NULL,
			PRIMARY KEY (host_id, timestamp, nanosecond)
		) WITHOUT ROWID;
		CREATE TABLE IF NOT EXISTS incident_sequences (
			host_id TEXT PRIMARY KEY,
			sequence INTEGER NOT NULL
		) WITHOUT ROWID;
		CREATE TABLE IF NOT EXISTS incidents (
			host_id TEXT NOT NULL,
			id INTEGER NOT NULL,
			data BLOB NOT NULL,
			PRIMARY KEY (host_id, id)
		) WITHOUT ROWID;
	`)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to initialize sqlite database: %w", err)
	}

	return &SQLite{conn: db}, nil
}
func (s *SQLite) Close() error {
	return s.conn.Close()
}

func (s *SQLite) AddResponse(ctx context.Context, hostID string, r *types.HttpResponse) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	_, err = s.conn.ExecContext(ctx, `
		INSERT INTO responses (host_id, timestamp, nanosecond, data) VALUES (?, ?, ?, ?)
		ON CONFLICT (host_id, timestamp, nanosecond) DO UPDATE SET data = excluded.data
	`, hostID, r.Timestamp.Unix(), r.Timestamp.Nanosecond(), data)
	return err
}
func (s *SQLite) DeleteResponse(ctx context.Context, hostID string, keys []time.Time) error {
	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, key := range keys {
		if _, err := tx.ExecContext(ctx, "DELETE FROM responses WHERE host_id = ? AND timestamp = ? AND nanosecond = ?", hostID, key.Unix(), key.Nanosecond()); err != nil {
			return err
		}
	}

	return tx.Commit()
}
func (s *SQLite) FindResponses(ctx context.Context, hostID string) ([]*types.HttpResponse, error) {
	rows, err := s.conn.QueryContext(ctx, "SELECT data FROM responses WHERE host_id = ? ORDER BY timestamp, nanosecond", hostID)
	if err != nil {
		return nil, err
	}
	return sqliteResponses(rows)
}
func (s *SQLite) LastResponse(ctx context.Context, hostID string) (*types.HttpResponse, error) {
	var data []byte
	err := s.conn.QueryRowContext(ctx, "SELECT data FROM responses WHERE host_id = ? ORDER BY timestamp DESC, nanosecond DESC LIMIT 1", hostID).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var r types.HttpResponse
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}

	return &r, nil
}

func (s *SQLite) AggregateResponses(ctx context.Context, hostID string, day time.Time) error {
	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	end := day.AddDate(0, 0, 1)
	rows, err := tx.QueryContext(ctx, `
		SELECT data FROM responses WHERE host_id = ?
		AND (timestamp, nanosecond) >= (?, ?) AND (timestamp, nanosecond) < (?, ?)
		ORDER BY timestamp, nanosecond
	`, hostID, day.Unix(), day.Nanosecond(), end.Unix(), end.Nanosecond())
	if err != nil {
		return err
	}
	responses, err := sqliteResponses(rows)
	if err != nil {
		return err
	}
	if len(responses) == 0 {
		return nil
	}

	data, err := json.Marshal(AggregateDay(day, responses))
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM responses WHERE host_id = ?
		AND (timestamp, nanosecond) >= (?, ?) AND (timestamp, nanosecond) < (?, ?)
	`, hostID, day.Unix(), day.Nanosecond(), end.Unix(), end.Nanosecond()); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO responses (host_id, timestamp, nanosecond, data) VALUES (?, ?, ?, ?)", hostID, day.Unix(), day.Nanosecond(), data); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *SQLite) Hosts(ctx context.Context) ([]string, error) {
	rows, err := s.conn.QueryContext(ctx, "SELECT DISTINCT host_id FROM responses ORDER BY host_id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	keys := []string{}
	for rows.Next() {
		var hostID string
		if err := rows.Scan(&hostID); err != nil {
			return nil, err
		}
		keys = append(keys, hostID)
	}

	return keys, rows.Err()
}

func (s *SQLite) AddIncident(ctx context.Context, hostID string, e *types.Incident) error {
	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var eventID int
	err = tx.QueryRowContext(ctx, `
		INSERT INTO incident_sequences (host_id, sequence) VALUES (?, 1)
		ON CONFLICT (host_id) DO UPDATE SET sequence = sequence + 1
		RETURNING sequence
	`, hostID).Scan(&eventID)
	if err != nil {
		return err
	}
	incident := *e
	incident.ID = eventID
	data, err := json.Marshal(&incident)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO incidents (host_id, id, data) VALUES (?, ?, ?)", hostID, eventID, data); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	e.ID = eventID

	return nil
}
func (s *SQLite) EndIncident(ctx context.Context, hostID string, eventID int, ts time.Time) error {
	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var data []byte
	err = tx.QueryRowContext(ctx, "SELECT data FROM incidents WHERE host_id = ? AND id = ?", hostID, eventID).Scan(&data)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	var e types.Incident
	if err := json.Unmarshal(data, &e); err != nil {
		return err
	}

	e.EndTS = &ts
	data, err = json.Marshal(&e)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE incidents SET data = ? WHERE host_id = ? AND id = ?", data, hostID, eventID); err != nil {
		return err
	}

	return tx.Commit()
}
func (s *SQLite) DeleteIncident(ctx context.Context, hostID string, eventID int) error {
	_, err := s.conn.ExecContext(ctx, "DELETE FROM incidents WHERE host_id = ? AND id = ?", hostID, eventID)
	return err
}
func (s *SQLite) FindIncidents(ctx context.Context, hostID string, skip, limit int) ([]*types.Incident, error) {
	if skip < 0 {
		skip = 0
	}
	if limit <= 0 {
		limit = -1
	}
	rows, err := s.conn.QueryContext(ctx, "SELECT data FROM incidents WHERE host_id = ? ORDER BY id DESC LIMIT ? OFFSET ?", hostID, limit, skip)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	res := []*types.Incident{}
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var e types.Incident
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		res = append(res, &e)
	}

	return res, rows.Err()
}

func sqliteResponses(rows *sql.Rows) ([]*types.HttpResponse, error) {
	defer rows.Close()

	res := []*types.HttpResponse{}
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var r types.HttpResponse
		if err := json.Unmarshal(data, &r); err != nil {
			return nil, err
		}
		res = append(res, &r)
	}

	return res, rows.Err()
}
