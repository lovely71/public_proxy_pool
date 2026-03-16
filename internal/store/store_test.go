package store

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestSQLiteDSN_AppendsPragmasForFileDB(t *testing.T) {
	got := sqliteDSN("/tmp/proxypool.db", 15*time.Second)

	for _, want := range []string{
		"/tmp/proxypool.db?",
		"_pragma=busy_timeout%2815000%29",
		"_pragma=foreign_keys%28ON%29",
		"_pragma=journal_mode%28WAL%29",
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
