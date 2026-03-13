package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	if path != "" && path != ":memory:" && path != "file::memory:" {
		dir := filepath.Dir(path)
		if dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, err
			}
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // SQLite: keep simple, avoid writer contention on 1c1g.
	db.SetMaxIdleConns(1)

	st := &Store{db: db}
	if err := st.initPragmas(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return st, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) initPragmas() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.db.ExecContext(ctx, `
PRAGMA journal_mode=WAL;
PRAGMA synchronous=NORMAL;
PRAGMA temp_store=MEMORY;
PRAGMA foreign_keys=ON;
PRAGMA busy_timeout=5000;
`)
	return err
}

func unixNow() int64 { return time.Now().Unix() }

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func intToBool(v int64) bool { return v != 0 }

var ErrNotFound = errors.New("not found")

func wrapErr(op string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", op, err)
}
