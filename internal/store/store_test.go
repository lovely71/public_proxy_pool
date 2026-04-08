package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSQLiteDSN_AppendsPragmasForFileDB(t *testing.T) {
	got := sqliteDSN("/tmp/proxypool.db", 15*time.Second, 100*1024*1024, 256)

	for _, want := range []string{
		"/tmp/proxypool.db?",
		"_pragma=busy_timeout%2815000%29",
		"_pragma=foreign_keys%28ON%29",
		"_pragma=journal_mode%28WAL%29",
		"_pragma=journal_size_limit%28104857600%29",
		"_pragma=wal_autocheckpoint%28256%29",
		"_pragma=synchronous%28NORMAL%29",
		"_pragma=temp_store%28MEMORY%29",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("dsn missing %q: %s", want, got)
		}
	}
}

func TestNormalizeOpenOptions_UsesSingleConnForMemoryDB(t *testing.T) {
	got := normalizeOpenOptions(":memory:", OpenOptions{
		MaxOpenConns: 8,
		BusyTimeout:  15 * time.Second,
	})

	if got.MaxOpenConns != 1 {
		t.Fatalf("memory db should force a single connection, got %d", got.MaxOpenConns)
	}
	if got.BusyTimeout != 15*time.Second {
		t.Fatalf("busy timeout changed unexpectedly: %s", got.BusyTimeout)
	}
	if got.WALSizeLimitBytes != 0 {
		t.Fatalf("memory db should disable wal size limits, got %d", got.WALSizeLimitBytes)
	}
	if got.WALAutoCheckpoint != 0 {
		t.Fatalf("memory db should disable wal autocheckpoint, got %d", got.WALAutoCheckpoint)
	}
}

func TestGetStats_EmptyDB(t *testing.T) {
	st, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()

	if err := st.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	stats, err := st.GetStats(context.Background(), time.Now(), 0)
	if err != nil {
		t.Fatalf("get stats: %v", err)
	}

	if *stats != (Stats{}) {
		t.Fatalf("expected zero stats, got %#v", *stats)
	}
}

func TestPruneInvalidNodesBefore_RemovesOnlyOldInvalidNodes(t *testing.T) {
	st, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()

	if err := st.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	now := time.Now()
	_, err = st.UpsertNodes(context.Background(), now, []NodeUpsert{
		{Kind: "proxy", Protocol: "http", Fingerprint: "invalid-old", Host: "1.1.1.1", Port: 80, RawURI: "http://1.1.1.1:80", LastSource: 1},
		{Kind: "proxy", Protocol: "http", Fingerprint: "invalid-fresh", Host: "2.2.2.2", Port: 80, RawURI: "http://2.2.2.2:80", LastSource: 1},
		{Kind: "proxy", Protocol: "http", Fingerprint: "valid-old", Host: "3.3.3.3", Port: 80, RawURI: "http://3.3.3.3:80", LastSource: 1},
	})
	if err != nil {
		t.Fatalf("upsert nodes: %v", err)
	}

	mustExec(t, st, `UPDATE nodes SET status='invalid', last_seen_at=? WHERE fingerprint='invalid-old'`, now.Add(-4*24*time.Hour).Unix())
	mustExec(t, st, `UPDATE nodes SET status='invalid', last_seen_at=? WHERE fingerprint='invalid-fresh'`, now.Add(-2*24*time.Hour).Unix())
	mustExec(t, st, `UPDATE nodes SET status='valid', last_seen_at=? WHERE fingerprint='valid-old'`, now.Add(-10*24*time.Hour).Unix())

	pruned, err := st.PruneInvalidNodesBefore(context.Background(), now.Add(-3*24*time.Hour).Unix())
	if err != nil {
		t.Fatalf("prune invalid nodes: %v", err)
	}
	if pruned != 1 {
		t.Fatalf("expected 1 pruned node, got %d", pruned)
	}

	if _, err := st.GetNodeByFingerprint(context.Background(), "invalid-old"); err != ErrNotFound {
		t.Fatalf("expected invalid-old to be pruned, got %v", err)
	}
	if _, err := st.GetNodeByFingerprint(context.Background(), "invalid-fresh"); err != nil {
		t.Fatalf("expected invalid-fresh to remain, got %v", err)
	}
	if _, err := st.GetNodeByFingerprint(context.Background(), "valid-old"); err != nil {
		t.Fatalf("expected valid-old to remain, got %v", err)
	}
}

func TestEnforceWALSizeLimit_TruncatesOversizedWAL(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "proxypool.db")
	st, err := OpenWithOptions(dbPath, OpenOptions{
		MaxOpenConns:      1,
		BusyTimeout:       5 * time.Second,
		WALSizeLimitBytes: 1024,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()

	if err := st.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	payload := strings.Repeat("x", 4096)
	for i := 0; i < 200; i++ {
		if _, err := st.DB().ExecContext(
			context.Background(),
			`INSERT INTO meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
			fmt.Sprintf("k-%d", i),
			payload,
		); err != nil {
			t.Fatalf("seed wal data: %v", err)
		}
	}

	before := fileSize(t, dbPath+"-wal", false)
	if before <= 1024 {
		t.Fatalf("expected wal file to exceed the limit, got %d bytes", before)
	}

	if err := st.EnforceWALSizeLimit(context.Background()); err != nil {
		t.Fatalf("enforce wal size limit: %v", err)
	}

	after := fileSize(t, dbPath+"-wal", true)
	if after > 1024 {
		t.Fatalf("expected wal file to be truncated to 1024 bytes or less, got %d bytes", after)
	}
}

func fileSize(t *testing.T, path string, allowMissing bool) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		if allowMissing && os.IsNotExist(err) {
			return 0
		}
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.Size()
}

func mustExec(t *testing.T, st *Store, query string, args ...any) {
	t.Helper()
	if _, err := st.DB().ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}
