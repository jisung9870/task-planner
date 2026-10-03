package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A vault config written before due/estimate/rollover were retired still
// loads; it says which keys and views no longer do anything.
func TestLoadWarnsAboutRetiredKeysAndViews(t *testing.T) {
	dir := t.TempDir()
	raw := `wip_limit: 3
auto_rollover: true
rollover_warn_at: 3
due_soon_days: 3
daily_capacity: 6h
views:
- name: 진행중
  query: status:doing
- name: 마감 임박
  query: is:duesoon
- name: 반복 이월
  query: is:carried rollover>2
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	all := strings.Join(cfg.Warnings, "\n")
	for _, want := range []string{"auto_rollover", "daily_capacity", "마감 임박", "반복 이월"} {
		if !strings.Contains(all, want) {
			t.Errorf("warning missing %q:\n%s", want, all)
		}
	}
	if strings.Contains(all, "진행중") {
		t.Errorf("a live view was flagged:\n%s", all)
	}
	if cfg.StaleDays != 5 || cfg.BackfillUnder.String() != "10m" {
		t.Errorf("defaults: stale=%d backfill=%s", cfg.StaleDays, cfg.BackfillUnder)
	}
}

func TestLoadCleanConfigHasNoWarnings(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("wip_limit: 3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Warnings) != 0 {
		t.Fatalf("warnings = %v", cfg.Warnings)
	}
}
