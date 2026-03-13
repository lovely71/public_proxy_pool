package store

import (
	"context"
	"database/sql"
	"time"
)

func (s *Store) SourceCount(ctx context.Context) (int, error) {
	row := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM sources`)
	var n int
	if err := row.Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func (s *Store) UpsertSource(ctx context.Context, src Source) (int64, error) {
	if src.NextFetchAt <= 0 {
		src.NextFetchAt = unixNow()
	}
	res, err := s.db.ExecContext(ctx, `
INSERT INTO sources(name,type,url,parser,default_scheme,repo_url,update_hint,enabled,interval_sec,next_fetch_at)
VALUES(?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(name) DO UPDATE SET
  type=excluded.type,
  url=excluded.url,
  parser=excluded.parser,
  default_scheme=excluded.default_scheme,
  repo_url=excluded.repo_url,
  update_hint=excluded.update_hint
`, src.Name, src.Type, src.URL, src.Parser, src.DefaultScheme, src.RepoURL, src.UpdateHint, boolToInt(src.Enabled), src.IntervalSec, src.NextFetchAt)
	if err != nil {
		return 0, err
	}
	if id, err := res.LastInsertId(); err == nil && id != 0 {
		return id, nil
	}
	row := s.db.QueryRowContext(ctx, `SELECT id FROM sources WHERE name=?`, src.Name)
	var id int64
	if err := row.Scan(&id); err != nil {
		return 0, err
	}
	return id, nil
}

func (s *Store) ListSources(ctx context.Context) ([]Source, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id,name,type,url,parser,default_scheme,repo_url,update_hint,enabled,interval_sec,next_fetch_at,backoff_until,last_fetch_at,etag,last_modified,ema_valid_yield,ema_avg_score,last_error,fetch_ok_total,fetch_fail_total,fetched_total,fetched_not_modified_total
FROM sources ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Source
	for rows.Next() {
		var src Source
		var enabled int64
		if err := rows.Scan(
			&src.ID, &src.Name, &src.Type, &src.URL, &src.Parser, &src.DefaultScheme, &src.RepoURL, &src.UpdateHint,
			&enabled, &src.IntervalSec, &src.NextFetchAt, &src.BackoffUntil, &src.LastFetchAt, &src.ETag, &src.LastModified,
			&src.EMAYield, &src.EMAAvgScore, &src.LastError,
			&src.FetchOKTotal, &src.FetchFailTotal, &src.FetchedTotal, &src.FetchedNotModifiedTotal,
		); err != nil {
			return nil, err
		}
		src.Enabled = intToBool(enabled)
		out = append(out, src)
	}
	return out, rows.Err()
}

func (s *Store) GetSourcesDue(ctx context.Context, now time.Time, limit int) ([]Source, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id,name,type,url,parser,default_scheme,repo_url,update_hint,enabled,interval_sec,next_fetch_at,backoff_until,last_fetch_at,etag,last_modified,ema_valid_yield,ema_avg_score,last_error,fetch_ok_total,fetch_fail_total,fetched_total,fetched_not_modified_total
FROM sources
WHERE enabled=1 AND (backoff_until=0 OR backoff_until<=?) AND (next_fetch_at=0 OR next_fetch_at<=?)
ORDER BY (ema_valid_yield * (CASE WHEN ema_avg_score<=0 THEN 0.01 ELSE ema_avg_score END)) DESC, last_fetch_at ASC
LIMIT ?`, now.Unix(), now.Unix(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Source
	for rows.Next() {
		var src Source
		var enabled int64
		if err := rows.Scan(
			&src.ID, &src.Name, &src.Type, &src.URL, &src.Parser, &src.DefaultScheme, &src.RepoURL, &src.UpdateHint,
			&enabled, &src.IntervalSec, &src.NextFetchAt, &src.BackoffUntil, &src.LastFetchAt, &src.ETag, &src.LastModified,
			&src.EMAYield, &src.EMAAvgScore, &src.LastError,
			&src.FetchOKTotal, &src.FetchFailTotal, &src.FetchedTotal, &src.FetchedNotModifiedTotal,
		); err != nil {
			return nil, err
		}
		src.Enabled = intToBool(enabled)
		out = append(out, src)
	}
	return out, rows.Err()
}

func (s *Store) UpdateSourceFetchMeta(ctx context.Context, srcID int64, meta FetchMetaUpdate) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE sources SET
  last_fetch_at=?,
  etag=?,
  last_modified=?,
  last_error=?,
  next_fetch_at=?,
  backoff_until=?,
  fetch_ok_total=fetch_ok_total+?,
  fetch_fail_total=fetch_fail_total+?,
  fetched_total=fetched_total+?,
  fetched_not_modified_total=fetched_not_modified_total+?
WHERE id=?`,
		meta.LastFetchAt,
		meta.ETag,
		meta.LastModified,
		meta.LastError,
		meta.NextFetchAt,
		meta.BackoffUntil,
		meta.FetchOKInc,
		meta.FetchFailInc,
		meta.FetchedInc,
		meta.NotModifiedInc,
		srcID,
	)
	return err
}

type FetchMetaUpdate struct {
	LastFetchAt    int64
	ETag           string
	LastModified   string
	LastError      string
	NextFetchAt    int64
	BackoffUntil   int64
	FetchOKInc     int64
	FetchFailInc   int64
	FetchedInc     int64
	NotModifiedInc int64
}

func (s *Store) SetSourceEnabled(ctx context.Context, srcID int64, enabled bool) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sources SET enabled=? WHERE id=?`, boolToInt(enabled), srcID)
	return err
}

func (s *Store) UpdateSourceEMA(ctx context.Context, srcID int64, emaYield, emaAvgScore float64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sources SET ema_valid_yield=?, ema_avg_score=? WHERE id=?`, emaYield, emaAvgScore, srcID)
	return err
}

func (s *Store) GetSourceByID(ctx context.Context, id int64) (*Source, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id,name,type,url,parser,default_scheme,repo_url,update_hint,enabled,interval_sec,next_fetch_at,backoff_until,last_fetch_at,etag,last_modified,ema_valid_yield,ema_avg_score,last_error,fetch_ok_total,fetch_fail_total,fetched_total,fetched_not_modified_total
FROM sources WHERE id=?`, id)
	var src Source
	var enabled int64
	if err := row.Scan(
		&src.ID, &src.Name, &src.Type, &src.URL, &src.Parser, &src.DefaultScheme, &src.RepoURL, &src.UpdateHint,
		&enabled, &src.IntervalSec, &src.NextFetchAt, &src.BackoffUntil, &src.LastFetchAt, &src.ETag, &src.LastModified,
		&src.EMAYield, &src.EMAAvgScore, &src.LastError,
		&src.FetchOKTotal, &src.FetchFailTotal, &src.FetchedTotal, &src.FetchedNotModifiedTotal,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	src.Enabled = intToBool(enabled)
	return &src, nil
}
