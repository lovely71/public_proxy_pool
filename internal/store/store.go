package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db                *sql.DB
	sqlitePath        string
	walSizeLimitBytes int64
}

type OpenOptions struct {
	MaxOpenConns      int
	BusyTimeout       time.Duration
	WALSizeLimitBytes int64
	WALAutoCheckpoint int
}

func Open(path string) (*Store, error) {
	return OpenWithOptions(path, OpenOptions{})
}

func OpenWithOptions(path string, opts OpenOptions) (*Store, error) {
	if path != "" && !isMemorySQLitePath(path) {
		dir := filepath.Dir(path)
		if dir != "." && dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, err
			}
		}
	}
	opts = normalizeOpenOptions(path, opts)
	dsn := path
	if !isMemorySQLitePath(path) {
		dsn = sqliteDSN(path, opts.BusyTimeout, opts.WALSizeLimitBytes, opts.WALAutoCheckpoint)
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(opts.MaxOpenConns)
	db.SetMaxIdleConns(opts.MaxOpenConns)

	st := &Store{
		db:                db,
		sqlitePath:        path,
		walSizeLimitBytes: opts.WALSizeLimitBytes,
	}
	if isMemorySQLitePath(path) {
		if err := st.initPragmas(opts.BusyTimeout); err != nil {
			_ = db.Close()
			return nil, err
		}
	}
	return st, nil
}

func normalizeOpenOptions(path string, opts OpenOptions) OpenOptions {
	if opts.MaxOpenConns <= 0 {
		opts.MaxOpenConns = 1
	}
	if opts.BusyTimeout <= 0 {
		opts.BusyTimeout = 5 * time.Second
	}
	if opts.WALSizeLimitBytes <= 0 {
		opts.WALSizeLimitBytes = 100 * 1024 * 1024
	}
	if opts.WALAutoCheckpoint <= 0 {
		opts.WALAutoCheckpoint = 256
	}
	if isMemorySQLitePath(path) {
		// Each :memory: connection gets its own isolated database, so keep tests
		// and in-memory usage on a single shared connection.
		opts.MaxOpenConns = 1
		opts.WALSizeLimitBytes = 0
		opts.WALAutoCheckpoint = 0
	}
	return opts
}

func isMemorySQLitePath(path string) bool {
	switch path {
	case ":memory:", "file::memory:":
		return true
	}
	return strings.HasPrefix(path, "file::memory:?")
}

func sqliteDSN(path string, busyTimeout time.Duration, walSizeLimitBytes int64, walAutoCheckpoint int) string {
	args := []string{
		"_pragma=" + pragmaValue("busy_timeout", strconv.FormatInt(busyTimeout.Milliseconds(), 10)),
		"_pragma=" + pragmaValue("foreign_keys", "ON"),
		"_pragma=" + pragmaValue("journal_mode", "WAL"),
		"_pragma=" + pragmaValue("journal_size_limit", strconv.FormatInt(walSizeLimitBytes, 10)),
		"_pragma=" + pragmaValue("wal_autocheckpoint", strconv.Itoa(walAutoCheckpoint)),
		"_pragma=" + pragmaValue("synchronous", "NORMAL"),
		"_pragma=" + pragmaValue("temp_store", "MEMORY"),
	}

	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return path + sep + strings.Join(args, "&")
}

func pragmaValue(name, value string) string {
	return name + "%28" + value + "%29"
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

func (s *Store) initPragmas(busyTimeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.db.ExecContext(ctx, `
PRAGMA synchronous=NORMAL;
PRAGMA temp_store=MEMORY;
PRAGMA foreign_keys=ON;
`)
	if err != nil {
		return err
	}
	if busyTimeout > 0 {
		_, err = s.db.ExecContext(ctx, `PRAGMA busy_timeout=`+strconv.FormatInt(busyTimeout.Milliseconds(), 10))
	}
	if err != nil {
		return err
	}
	return nil
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
