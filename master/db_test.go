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

func TestGroupsRoundTrip(t *testing.T) {
	if err := openDB(filepath.Join(t.TempDir(), "t.db")); err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	groupsList = []string{"默认分组", "香港"}
	if err := saveGroups(""); err != nil {
		t.Fatal(err)
	}
	groupsList = nil
	if err := loadGroups(""); err != nil {
		t.Fatal(err)
	}
	if len(groupsList) != 2 || groupsList[1] != "香港" {
		t.Fatalf("groups 往返错: %v", groupsList)
	}
}

func TestMonitorsRoundTrip(t *testing.T) {
	if err := openDB(filepath.Join(t.TempDir(), "t.db")); err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	monitors = map[string]*Monitor{}
	monitors["m1"] = &Monitor{ID: "m1", Name: "web", Type: "http", Target: "http://x", Interval: 30, AgentID: "n1"}
	if err := saveMonitors(""); err != nil {
		t.Fatal(err)
	}
	monitors = map[string]*Monitor{}
	if err := loadMonitors(""); err != nil {
		t.Fatal(err)
	}
	if m := monitors["m1"]; m == nil || m.Target != "http://x" || m.AgentID != "n1" {
		t.Fatalf("monitors 往返错: %+v", m)
	}
}

func TestAlertsRoundTrip(t *testing.T) {
	if err := openDB(filepath.Join(t.TempDir(), "t.db")); err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	alertConfig = AlertConfig{CPUPercent: 70, MemPercent: 75, DiskPercent: 88, OfflineMinutes: 9, Enabled: true}
	if err := saveAlerts(""); err != nil {
		t.Fatal(err)
	}
	alertConfig = AlertConfig{}
	if err := loadAlerts(""); err != nil {
		t.Fatal(err)
	}
	if alertConfig.CPUPercent != 70 || alertConfig.DiskPercent != 88 || !alertConfig.Enabled {
		t.Fatalf("alerts 往返错: %+v", alertConfig)
	}
}
