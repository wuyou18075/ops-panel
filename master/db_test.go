package main

import (
	"path/filepath"
	"testing"
)

func TestOpenDB_CreatesSchema(t *testing.T) {
	p := filepath.Join(t.TempDir(), "t.db")
	if err := openDB(p); err != nil {
		t.Fatalf("openDB: %v", err)
	}
	defer db.Close()
	for _, tbl := range []string{"agents", "agent_groups", "monitors", "alerts", "traffic_daily", "panel_login", "ssh_login", "ssh_fail_reset"} {
		if _, err := db.Exec("SELECT count(*) FROM " + tbl); err != nil {
			t.Errorf("表 %s 不可用: %v", tbl, err)
		}
	}
}
