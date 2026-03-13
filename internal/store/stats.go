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
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM nodes`).Scan(&out.NodesTotal); err != nil {
		return nil, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM nodes WHERE status='valid'`).Scan(&out.NodesValid); err != nil {
		return nil, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM nodes WHERE status='invalid'`).Scan(&out.NodesInvalid); err != nil {
		return nil, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM nodes WHERE status='unknown'`).Scan(&out.NodesUnknown); err != nil {
		return nil, err
	}
	freshSeconds := int64(freshWithin.Seconds())
	if freshSeconds <= 0 {
		freshSeconds = int64((5 * time.Minute).Seconds())
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM nodes WHERE status='valid' AND last_checked_at>=? AND ban_until<=?`, now.Unix()-freshSeconds, now.Unix()).Scan(&out.NodesFreshOK); err != nil {
		return nil, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM sources`).Scan(&out.SourcesTotal); err != nil {
		return nil, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM sources WHERE enabled=1`).Scan(&out.SourcesEnabled); err != nil {
		return nil, err
	}
	return out, nil
}

