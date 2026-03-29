package core

import (
	"context"
	"testing"
	"time"

	"github.com/qiyiyun/public_proxy_pool/internal/config"
	"github.com/qiyiyun/public_proxy_pool/internal/store"
)

func TestCleanupOnce_PrunesOldInvalidNodes(t *testing.T) {
	st, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()

	if err := st.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	now := time.Now()
	_, err = st.UpsertNodes(context.Background(), now, []store.NodeUpsert{
		{Kind: "proxy", Protocol: "http", Fingerprint: "invalid-old", Host: "1.1.1.1", Port: 80, RawURI: "http://1.1.1.1:80", LastSource: 1},
		{Kind: "proxy", Protocol: "http", Fingerprint: "invalid-fresh", Host: "2.2.2.2", Port: 80, RawURI: "http://2.2.2.2:80", LastSource: 1},
	})
	if err != nil {
		t.Fatalf("upsert nodes: %v", err)
	}

	if _, err := st.DB().ExecContext(context.Background(), `UPDATE nodes SET status='invalid', last_seen_at=? WHERE fingerprint='invalid-old'`, now.Add(-4*24*time.Hour).Unix()); err != nil {
		t.Fatalf("update invalid-old: %v", err)
	}
	if _, err := st.DB().ExecContext(context.Background(), `UPDATE nodes SET status='invalid', last_seen_at=? WHERE fingerprint='invalid-fresh'`, now.Add(-2*24*time.Hour).Unix()); err != nil {
		t.Fatalf("update invalid-fresh: %v", err)
	}

	s := NewSupervisor(st, nil, &config.Config{
		ChecksRetention:      30 * 24 * time.Hour,
		InvalidNodeRetention: 72 * time.Hour,
		CleanupInterval:      6 * time.Hour,
	})
	if err := s.cleanupOnce(context.Background()); err != nil {
		t.Fatalf("cleanupOnce: %v", err)
	}

	if _, err := st.GetNodeByFingerprint(context.Background(), "invalid-old"); err != store.ErrNotFound {
		t.Fatalf("expected invalid-old to be pruned, got %v", err)
	}
	if _, err := st.GetNodeByFingerprint(context.Background(), "invalid-fresh"); err != nil {
		t.Fatalf("expected invalid-fresh to remain, got %v", err)
	}
}
