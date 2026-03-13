package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const schemaVersion = 1

func (s *Store) Migrate() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if _, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS meta (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
`); err != nil {
		return wrapErr("create meta", err)
	}

	current, err := s.getSchemaVersion(ctx)
	if err != nil {
		return wrapErr("get schema version", err)
	}
	if current == schemaVersion {
		return nil
	}
	if current != 0 && current != schemaVersion {
		return fmt.Errorf("unsupported schema version: %d (want %d)", current, schemaVersion)
	}

	if err := s.migrateToV1(ctx); err != nil {
		return wrapErr("migrate v1", err)
	}
	if err := s.setSchemaVersion(ctx, schemaVersion); err != nil {
		return wrapErr("set schema version", err)
	}
	return nil
}

func (s *Store) getSchemaVersion(ctx context.Context) (int, error) {
	row := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key='schema_version'`)
	var raw string
	switch err := row.Scan(&raw); err {
	case nil:
		var v int
		_, _ = fmt.Sscanf(raw, "%d", &v)
		return v, nil
	case sql.ErrNoRows:
		return 0, nil
	default:
		return 0, err
	}
}

func (s *Store) setSchemaVersion(ctx context.Context, v int) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO meta(key,value) VALUES('schema_version', ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, fmt.Sprintf("%d", v))
	return err
}

func (s *Store) migrateToV1(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS sources (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL UNIQUE,
  type TEXT NOT NULL,
  url TEXT NOT NULL,
  parser TEXT NOT NULL DEFAULT 'generic',
  default_scheme TEXT NOT NULL DEFAULT 'http',
  repo_url TEXT NOT NULL DEFAULT '',
  update_hint TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1,
  interval_sec INTEGER NOT NULL DEFAULT 3600,
  next_fetch_at INTEGER NOT NULL DEFAULT 0,
  backoff_until INTEGER NOT NULL DEFAULT 0,
  last_fetch_at INTEGER NOT NULL DEFAULT 0,
  etag TEXT NOT NULL DEFAULT '',
  last_modified TEXT NOT NULL DEFAULT '',
  ema_valid_yield REAL NOT NULL DEFAULT 0.0,
  ema_avg_score REAL NOT NULL DEFAULT 0.0,
  last_error TEXT NOT NULL DEFAULT '',
  fetch_ok_total INTEGER NOT NULL DEFAULT 0,
  fetch_fail_total INTEGER NOT NULL DEFAULT 0,
  fetched_total INTEGER NOT NULL DEFAULT 0,
  fetched_not_modified_total INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS nodes (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  kind TEXT NOT NULL,
  protocol TEXT NOT NULL,
  fingerprint TEXT NOT NULL,
  host TEXT NOT NULL,
  port INTEGER NOT NULL,
  username TEXT NOT NULL DEFAULT '',
  password TEXT NOT NULL DEFAULT '',
  raw_uri TEXT NOT NULL,
  name TEXT NOT NULL DEFAULT '',
  last_source_id INTEGER NOT NULL DEFAULT 0,
  first_seen_at INTEGER NOT NULL DEFAULT 0,
  last_seen_at INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'unknown', -- unknown|valid|invalid
  last_checked_at INTEGER NOT NULL DEFAULT 0,
  last_ok_at INTEGER NOT NULL DEFAULT 0,
  latency_ms INTEGER NOT NULL DEFAULT 0,
  exit_ip TEXT NOT NULL DEFAULT '',
  country TEXT NOT NULL DEFAULT '',
  asn TEXT NOT NULL DEFAULT '',
  anonymity TEXT NOT NULL DEFAULT '',
  purity_score INTEGER NOT NULL DEFAULT 0,
  score REAL NOT NULL DEFAULT 0.0,
  ok_count INTEGER NOT NULL DEFAULT 0,
  fail_count INTEGER NOT NULL DEFAULT 0,
  fail_streak INTEGER NOT NULL DEFAULT 0,
  ban_until INTEGER NOT NULL DEFAULT 0,
  last_error TEXT NOT NULL DEFAULT '',
  UNIQUE(fingerprint)
);

CREATE INDEX IF NOT EXISTS idx_nodes_status_checked_score ON nodes(status, last_checked_at, score);
CREATE INDEX IF NOT EXISTS idx_nodes_country ON nodes(country);
CREATE INDEX IF NOT EXISTS idx_nodes_protocol ON nodes(protocol);
CREATE INDEX IF NOT EXISTS idx_nodes_ban_until ON nodes(ban_until);
CREATE INDEX IF NOT EXISTS idx_nodes_last_source ON nodes(last_source_id);
CREATE INDEX IF NOT EXISTS idx_nodes_fingerprint ON nodes(fingerprint);

CREATE TABLE IF NOT EXISTS checks (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  node_id INTEGER NOT NULL,
  checked_at INTEGER NOT NULL,
  ok INTEGER NOT NULL,
  latency_ms INTEGER NOT NULL,
  exit_ip TEXT NOT NULL,
  country TEXT NOT NULL,
  anonymity TEXT NOT NULL,
  purity_score INTEGER NOT NULL,
  error TEXT NOT NULL,
  FOREIGN KEY(node_id) REFERENCES nodes(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_checks_node_checked ON checks(node_id, checked_at);

CREATE TABLE IF NOT EXISTS ip_facts (
  ip TEXT PRIMARY KEY,
  updated_at INTEGER NOT NULL,
  country TEXT NOT NULL,
  proxy INTEGER NOT NULL,
  hosting INTEGER NOT NULL,
  mobile INTEGER NOT NULL
);
`);
	return err
}
