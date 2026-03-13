package core

import (
	"testing"
	"time"
)

func TestBackoffUntil(t *testing.T) {
	now := time.Date(2026, 3, 13, 12, 0, 0, 0, time.UTC)

	if got := backoffUntil(now, now.Unix()); got != now.Add(5*time.Minute).Unix() {
		t.Fatalf("case1 got=%d want=%d", got, now.Add(5*time.Minute).Unix())
	}

	if got := backoffUntil(now, now.Add(2*time.Minute).Unix()); got != now.Add(15*time.Minute).Unix() {
		t.Fatalf("case2 got=%d want=%d", got, now.Add(15*time.Minute).Unix())
	}

	if got := backoffUntil(now, now.Add(20*time.Minute).Unix()); got != now.Add(1*time.Hour).Unix() {
		t.Fatalf("case3 got=%d want=%d", got, now.Add(1*time.Hour).Unix())
	}

	if got := backoffUntil(now, now.Add(2*time.Hour).Unix()); got != now.Add(1*time.Hour).Unix() {
		t.Fatalf("case4 got=%d want=%d", got, now.Add(1*time.Hour).Unix())
	}
}

