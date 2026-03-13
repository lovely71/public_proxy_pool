package store

import (
	"context"
)

func (s *Store) PruneChecksBefore(ctx context.Context, cutoffUnix int64) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM checks WHERE checked_at < ?`, cutoffUnix)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (s *Store) PruneIPFactsBefore(ctx context.Context, cutoffUnix int64) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM ip_facts WHERE updated_at < ?`, cutoffUnix)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

