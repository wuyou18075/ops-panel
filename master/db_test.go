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

func TestAgentsRoundTrip(t *testing.T) {
	if err := openDB(filepath.Join(t.TempDir(), "t.db")); err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	agents = map[string]*AgentRecord{}
	rec := NewAgentRecord()
	rec.AgentID = "node-x"
	rec.Name = "n1"
	rec.Prefs.Group = "g1"
	agents[rec.AgentID] = rec
	if err := saveAgents(""); err != nil {
		t.Fatal(err)
	}
	agents = map[string]*AgentRecord{}
	if err := loadAgents(""); err != nil {
		t.Fatal(err)
	}
	got := agents["node-x"]
	if got == nil || got.Name != "n1" || got.Prefs.Group != "g1" {
		t.Fatalf("往返丢失: %+v", got)
	}
}
