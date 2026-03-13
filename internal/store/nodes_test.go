package store

import (
	"context"
	"testing"
	"time"
)

func TestQueryFreshValidNodes_VerifyFreshWithin(t *testing.T) {
	st, err := Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()
	if err := st.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	now := time.Now()
	ctx := context.Background()

	_, err = st.UpsertNodes(ctx, now, []NodeUpsert{
		{Kind: "proxy", Protocol: "http", Fingerprint: "fp1", Host: "1.1.1.1", Port: 80, RawURI: "http://1.1.1.1:80"},
		{Kind: "proxy", Protocol: "http", Fingerprint: "fp2", Host: "2.2.2.2", Port: 80, RawURI: "http://2.2.2.2:80"},
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	n1, err := st.GetNodeByFingerprint(ctx, "fp1")
	if err != nil {
		t.Fatalf("get fp1: %v", err)
	}
	n2, err := st.GetNodeByFingerprint(ctx, "fp2")
	if err != nil {
		t.Fatalf("get fp2: %v", err)
	}

	if err := st.ApplyNodeCheck(ctx, n1.ID, NodeCheckUpdate{
		CheckedAt:   now.Unix(),
		OK:          true,
		LatencyMS:   100,
		ExitIP:      "9.9.9.9",
		Country:     "US",
		ASN:         "AS123",
		Anonymity:   "elite",
		PurityScore: 80,
		Error:       "",
		Score:       900,
	}); err != nil {
		t.Fatalf("apply check n1: %v", err)
	}

	stale := now.Add(-10 * time.Minute).Unix()
	if err := st.ApplyNodeCheck(ctx, n2.ID, NodeCheckUpdate{
		CheckedAt:   stale,
		OK:          true,
		LatencyMS:   100,
		ExitIP:      "8.8.8.8",
		Country:     "JP",
		ASN:         "AS456",
		Anonymity:   "elite",
		PurityScore: 80,
		Error:       "",
		Score:       800,
	}); err != nil {
		t.Fatalf("apply check n2: %v", err)
	}

	got, err := st.QueryFreshValidNodes(ctx, now, NodeFilter{
		FreshWithin: 5 * time.Minute,
		Verify:      true,
	}, 10)
	if err != nil {
		t.Fatalf("query verify: %v", err)
	}
	if len(got) != 1 || got[0].Fingerprint != "fp1" {
		t.Fatalf("verify want only fp1, got=%v", got)
	}

	got2, err := st.QueryFreshValidNodes(ctx, now, NodeFilter{
		Verify: false,
	}, 10)
	if err != nil {
		t.Fatalf("query no-verify: %v", err)
	}
	if len(got2) != 2 {
		t.Fatalf("no-verify want 2, got %d", len(got2))
	}
}

