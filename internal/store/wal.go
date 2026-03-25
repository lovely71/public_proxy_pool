package store

import (
	"context"
	"errors"
	"os"
	"strconv"
)

func (s *Store) EnforceWALSizeLimit(ctx context.Context) error {
	if s == nil || s.db == nil || s.sqlitePath == "" || s.walSizeLimitBytes <= 0 {
		return nil
	}

	info, err := os.Stat(s.sqlitePath + "-wal")
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if info.Size() <= s.walSizeLimitBytes {
		return nil
	}

	var applied int64
	if err := s.db.QueryRowContext(
		ctx,
		`PRAGMA journal_size_limit=`+strconv.FormatInt(s.walSizeLimitBytes, 10),
	).Scan(&applied); err != nil {
		return err
	}

	var busy int64
	if err := s.db.QueryRowContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`).Scan(&busy, new(int64), new(int64)); err != nil {
		return err
	}
	if busy > 0 {
		// A busy reader may postpone truncation. The next maintenance tick will retry.
		return nil
	}

	return nil
}
