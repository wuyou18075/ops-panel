package main

import (
	"strings"
	"testing"
	"time"
)

func TestIngestStat_TracksLatest(t *testing.T) {
	latestStat = map[string]statSample{}
	lastSeen = map[string]time.Time{}
	rec := &AgentRecord{AgentID: "n1", Prefs: AgentPreferences{Interval: 2}}
	ingestStat("n1", rec, `{"cpu":91.5,"mem":40,"disk":20,"net_sent":1,"net_recv":2}`)
	s, ok := latestStat["n1"]
	if !ok || s.CPU != 91.5 {
		t.Fatalf("最新指标未记录: %+v", s)
	}
	if lastSeen["n1"].IsZero() {
		t.Errorf("lastSeen 未更新")
	}
}

func TestEvalAlerts(t *testing.T) {
	cfg := AlertConfig{CPUPercent: 80, MemPercent: 80, DiskPercent: 90, OfflineMinutes: 5, Enabled: true}
	now := time.Now()
	latest := map[string]statSample{"n1": {CPU: 95, Mem: 10, Disk: 10}}
	seen := map[string]time.Time{"n1": now, "n2": now.Add(-10 * time.Minute)}
	ids := []string{"n1", "n2"}
	msgs := evalAlerts(cfg, ids, latest, seen, now)
	joined := strings.Join(msgs, "\n")
	if !strings.Contains(joined, "n1") || !strings.Contains(joined, "CPU") {
		t.Errorf("应含 n1 CPU 告警: %v", msgs)
	}
	if !strings.Contains(joined, "n2") || !strings.Contains(joined, "离线") {
		t.Errorf("应含 n2 离线告警: %v", msgs)
	}
}

func TestEvalAlerts_DisabledSilent(t *testing.T) {
	cfg := AlertConfig{CPUPercent: 80, Enabled: false}
	msgs := evalAlerts(cfg, []string{"n1"}, map[string]statSample{"n1": {CPU: 99}}, map[string]time.Time{"n1": time.Now()}, time.Now())
	if len(msgs) != 0 {
		t.Errorf("关闭时不应告警: %v", msgs)
	}
}
