package store

import (
	"context"
	"errors"
	"os"
	"strconv"
)

type walCheckpointResult struct {
	Busy         int64
	LogFrames    int64
	Checkpointed int64
}

func (s *Store) EnforceWALSizeLimit(ctx context.Context) error {
	if s == nil || s.db == nil || s.sqlitePath == "" || s.walSizeLimitBytes <= 0 {
		return nil
	}

	size, err := s.walSizeBytes()
	if err != nil {
		return err
	}
	if size <= s.walSizeLimitBytes {
		return nil
	}

	if err := s.applyJournalSizeLimit(ctx); err != nil {
		return err
	}

	if _, err := s.runWALCheckpoint(ctx, "PASSIVE"); err != nil {
		return err
	}
	size, err = s.walSizeBytes()
	if err != nil || size <= s.walSizeLimitBytes {
		return err
	}

	if _, err := s.runWALCheckpoint(ctx, "RESTART"); err != nil {
		return err
	}
	size, err = s.walSizeBytes()
	if err != nil || size <= s.walSizeLimitBytes {
		return err
	}

	if _, err := s.runWALCheckpoint(ctx, "TRUNCATE"); err != nil {
		return err
	}
	return nil
}

func (s *Store) walSizeBytes() (int64, error) {
	info, err := os.Stat(s.sqlitePath + "-wal")
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	return info.Size(), nil
}

func (s *Store) applyJournalSizeLimit(ctx context.Context) error {
	var applied int64
	return s.db.QueryRowContext(
		ctx,
		`PRAGMA journal_size_limit=`+strconv.FormatInt(s.walSizeLimitBytes, 10),
	).Scan(&applied)
}

func (s *Store) runWALCheckpoint(ctx context.Context, mode string) (walCheckpointResult, error) {
	var out walCheckpointResult
	err := s.db.QueryRowContext(ctx, `PRAGMA wal_checkpoint(`+mode+`)`).Scan(&out.Busy, &out.LogFrames, &out.Checkpointed)
	return out, err
}
