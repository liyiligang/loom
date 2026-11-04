package storage

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/liyiligang/loom/internal/model"
	_ "modernc.org/sqlite"
)

//go:embed migrations/001_initial.sql
var initialMigration string

type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.initialize(); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize database: %w", err)
	}
	return s, nil
}

func (s *Store) initialize() error {
	if _, err := s.db.Exec("PRAGMA foreign_keys = ON; PRAGMA journal_mode = WAL; PRAGMA busy_timeout = 5000;"); err != nil {
		return err
	}
	var exists int
	if err := s.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name = 'schema_migrations'").Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.Exec(initialMigration); err != nil {
			return err
		}
		return tx.Commit()
	}
	var version int
	if err := s.db.QueryRow("SELECT max(version) FROM schema_migrations").Scan(&version); err != nil {
		return err
	}
	if version != 1 {
		return fmt.Errorf("unsupported schema version %d", version)
	}
	return nil
}

func load[T any](db *sql.DB, table string) ([]*T, error) {
	rows, err := db.Query("SELECT data FROM " + table + " ORDER BY rowid")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]*T, 0)
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var value T
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, err
		}
		result = append(result, &value)
	}
	return result, rows.Err()
}

func (s *Store) Load() (model.Data, error) {
	var data model.Data
	var err error
	if data.Endpoints, err = load[model.Endpoint](s.db, "endpoints"); err != nil {
		return data, err
	}
	if data.Sessions, err = load[model.Session](s.db, "sessions"); err != nil {
		return data, err
	}
	if data.Turns, err = load[model.Turn](s.db, "turns"); err != nil {
		return data, err
	}
	if data.Entries, err = load[model.Entry](s.db, "entries"); err != nil {
		return data, err
	}
	records, err := load[model.InteractionRecord](s.db, "interactions")
	if err != nil {
		return data, err
	}
	for _, record := range records {
		record.RequestID, record.Generation = record.NativeID, record.Connection
		data.Interactions = append(data.Interactions, &record.Interaction)
	}
	return data, nil
}

// Commit atomically saves projections and their notification cursor.
func (s *Store) Commit(eventType, resource string, values ...any) (int64, error) {
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	for _, value := range values {
		if err := save(tx, value); err != nil {
			return 0, err
		}
	}
	result, err := tx.Exec("INSERT INTO events(type, resource, created_at) VALUES (?, ?, ?)", eventType, resource, model.Now())
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

func save(tx *sql.Tx, value any) error {
	stored := value
	if interaction, ok := value.(*model.Interaction); ok {
		stored = model.InteractionRecord{Interaction: *interaction, NativeID: interaction.RequestID, Connection: interaction.Generation}
	}
	raw, err := json.Marshal(stored)
	if err != nil {
		return err
	}
	var query string
	var args []any
	switch v := value.(type) {
	case *model.Endpoint:
		query = "INSERT INTO endpoints VALUES (?, ?) ON CONFLICT(id) DO UPDATE SET data=excluded.data"
		args = []any{v.ID, raw}
	case *model.Session:
		query = "INSERT INTO sessions VALUES (?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET data=excluded.data"
		args = []any{v.ID, v.EndpointID, v.ProviderID, raw}
	case *model.Turn:
		query = "INSERT INTO turns VALUES (?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET data=excluded.data"
		args = []any{v.ID, v.SessionID, v.ClientRequestID, raw}
	case *model.Entry:
		query = "INSERT INTO entries VALUES (?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET data=excluded.data"
		args = []any{v.ID, v.SessionID, v.TurnID, raw}
	case *model.Interaction:
		query = "INSERT INTO interactions VALUES (?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET data=excluded.data"
		args = []any{v.ID, v.SessionID, v.TurnID, raw}
	default:
		return fmt.Errorf("unsupported projection %T", value)
	}
	_, err = tx.Exec(query, args...)
	return err
}

func (s *Store) Cursor() (int64, error) {
	var id int64
	err := s.db.QueryRow("SELECT coalesce(max(id), 0) FROM events").Scan(&id)
	return id, err
}

func (s *Store) Events(after int64) ([]model.Event, error) {
	rows, err := s.db.Query("SELECT id, type, resource, created_at FROM events WHERE id > ? ORDER BY id LIMIT 100", after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]model.Event, 0)
	for rows.Next() {
		var event model.Event
		if err := rows.Scan(&event.ID, &event.Type, &event.Resource, &event.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, event)
	}
	return result, rows.Err()
}

func (s *Store) Close() error { return s.db.Close() }
