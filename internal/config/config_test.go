package config

import "testing"

func TestParseSizeBytes(t *testing.T) {
	t.Parallel()

	tests := map[string]int64{
		"100m": 100 * 1024 * 1024,
		"64MB": 64 * 1024 * 1024,
		"2g":   2 * 1024 * 1024 * 1024,
		"512k": 512 * 1024,
		"4096": 4096,
	}

	for raw, want := range tests {
		raw := raw
		want := want
		t.Run(raw, func(t *testing.T) {
			t.Parallel()

			got, err := parseSizeBytes(raw)
			if err != nil {
				t.Fatalf("parseSizeBytes(%q): %v", raw, err)
			}
			if got != want {
				t.Fatalf("parseSizeBytes(%q) = %d, want %d", raw, got, want)
			}
		})
	}
}

func TestEnvSizeBytes_FallsBackToDefaultOnInvalidValue(t *testing.T) {
	t.Setenv("SQLITE_WAL_SIZE_LIMIT", "bad-value")

	got := envSizeBytes("SQLITE_WAL_SIZE_LIMIT", 1234)
	if got != 1234 {
		t.Fatalf("envSizeBytes should fall back to default, got %d", got)
	}
}

func TestLoad_DerivesSQLiteWALHardLimitFromSizeLimit(t *testing.T) {
	t.Setenv("SQLITE_WAL_SIZE_LIMIT", "100m")
	t.Setenv("SQLITE_WAL_HARD_LIMIT", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SQLiteWALHardLimitBytes != 200*1024*1024 {
		t.Fatalf("expected hard limit 200MiB, got %d", cfg.SQLiteWALHardLimitBytes)
	}
}

func TestLoad_ClampsSQLiteWALHardLimitAtLeastSizeLimit(t *testing.T) {
	t.Setenv("SQLITE_WAL_SIZE_LIMIT", "100m")
	t.Setenv("SQLITE_WAL_HARD_LIMIT", "50m")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SQLiteWALHardLimitBytes != 100*1024*1024 {
		t.Fatalf("expected hard limit to clamp to size limit, got %d", cfg.SQLiteWALHardLimitBytes)
	}
}
