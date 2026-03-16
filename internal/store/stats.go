package store

import (
	"context"
	"time"
)

type Stats struct {
	NodesTotal     int64
	NodesValid     int64
	NodesInvalid   int64
	NodesUnknown   int64
	NodesFreshOK   int64
	SourcesTotal   int64
	SourcesEnabled int64
}

func (s *Store) GetStats(ctx context.Context, now time.Time, freshWithin time.Duration) (*Stats, error) {
	out := &Stats{}
	freshSeconds := int64(freshWithin.Seconds())
	if freshSeconds <= 0 {
		freshSeconds = int64((5 * time.Minute).Seconds())
	}

	if err := s.db.QueryRowContext(ctx, `
SELECT
  COUNT(1),
  COALESCE(SUM(CASE WHEN status='valid' THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN status='invalid' THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN status='unknown' THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN status='valid' AND last_checked_at>=? AND ban_until<=? THEN 1 ELSE 0 END), 0)
FROM nodes
`, now.Unix()-freshSeconds, now.Unix()).Scan(
		&out.NodesTotal,
		&out.NodesValid,
		&out.NodesInvalid,
		&out.NodesUnknown,
		&out.NodesFreshOK,
	); err != nil {
		return nil, err
	}

	if err := s.db.QueryRowContext(ctx, `
SELECT
  COUNT(1),
  COALESCE(SUM(CASE WHEN enabled=1 THEN 1 ELSE 0 END), 0)
FROM sources
`).Scan(&out.SourcesTotal, &out.SourcesEnabled); err != nil {
		return nil, err
	}
	return out, nil
}
