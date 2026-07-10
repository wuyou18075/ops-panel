package main

import (
	"path/filepath"
	"testing"
	"time"
)

func TestTrafficSnapshot_SplitInOut(t *testing.T) {
	agents = map[string]*AgentRecord{"n1": {AgentID: "n1", Name: "n1", Prefs: AgentPreferences{TrackTraffic: true}}}
	traffic = map[string]*TrafficDay{}
	today := time.Now().Format("2006-01-02")
	traffic["n1|"+today] = &TrafficDay{Date: today, Sent: 100, Recv: 300}
	out := trafficStatsSnapshot()
	if len(out) != 1 {
		t.Fatalf("want 1, got %d", len(out))
	}
	s := out[0]
	if s.TodaySent != 100 || s.TodayRecv != 300 {
		t.Errorf("今日入出错: %+v", s)
	}
	if s.MonthSent != 100 || s.MonthRecv != 300 {
		t.Errorf("本月入出错: %+v", s)
	}
	if s.Today != 400 || s.ThisMonth != 400 {
		t.Errorf("合计错: %+v", s)
	}
}

func TestTrafficPersistRoundTrip(t *testing.T) {
	if err := openDB(filepath.Join(t.TempDir(), "t.db")); err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	traffic = map[string]*TrafficDay{}
	d := "2026-07-10"
	traffic["n1|"+d] = &TrafficDay{Date: d, Sent: 5, Recv: 7}
	if err := persistTrafficOnce(); err != nil {
		t.Fatal(err)
	}
	traffic = map[string]*TrafficDay{}
	if err := loadTrafficFromDB(); err != nil {
		t.Fatal(err)
	}
	got := traffic["n1|"+d]
	if got == nil || got.Sent != 5 || got.Recv != 7 {
		t.Fatalf("往返错: %+v", got)
	}
}
