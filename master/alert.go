package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type AlertEvent struct {
	ID      int64  `json:"id"`
	TS      int64  `json:"ts"`
	AgentID string `json:"agent_id"`
	Kind    string `json:"kind"`
	Title   string `json:"title"`
	Detail  string `json:"detail"`
}

func insertAlertEvent(agentID, kind, title, detail string) int64 {
	r, err := db.Exec("INSERT INTO alert_events(ts,agent_id,kind,title,detail) VALUES(?,?,?,?,?)", time.Now().Unix(), agentID, kind, title, detail)
	if err != nil {
		return 0
	}
	id, _ := r.LastInsertId()
	db.Exec("DELETE FROM alert_events WHERE id NOT IN (SELECT id FROM alert_events ORDER BY id DESC LIMIT 1000)")
	return id
}
func handleAlertEvents(w http.ResponseWriter, r *http.Request) {
	if !operatorAuthorized(r) {
		http.Error(w, "未授权", 401)
		return
	}
	rows, err := db.Query("SELECT id,ts,agent_id,kind,title,detail FROM alert_events ORDER BY id DESC LIMIT 500")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	out := []AlertEvent{}
	for rows.Next() {
		var e AlertEvent
		rows.Scan(&e.ID, &e.TS, &e.AgentID, &e.Kind, &e.Title, &e.Detail)
		out = append(out, e)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

// alert.go —— CPU/内存/磁盘/离线阈值真实告警。
// ingestStat 写入每节点最新指标与最后在线时间；alertLoop 周期比对阈值经 TG 告警。

type statSample struct{ CPU, Mem, Disk float64 }

var (
	metricMu   sync.Mutex
	latestStat = map[string]statSample{}
	lastSeen   = map[string]time.Time{} // 断连不删，供离线判断
)

// evalAlerts 返回本轮应发送的告警文案（纯函数，便于测试）。
func evalAlerts(cfg AlertConfig, ids []string, latest map[string]statSample, seen map[string]time.Time, now time.Time) []string {
	if !cfg.Enabled {
		return nil
	}
	var out []string
	for _, id := range ids {
		name := alertName(id)
		if t, ok := seen[id]; ok && now.Sub(t) > time.Duration(cfg.OfflineMinutes)*time.Minute {
			out = append(out, fmt.Sprintf("🔴 %s 离线超过 %d 分钟", name, cfg.OfflineMinutes))
			continue // 离线则不再报资源阈值
		}
		s, ok := latest[id]
		if !ok {
			continue
		}
		if s.CPU >= float64(cfg.CPUPercent) {
			out = append(out, fmt.Sprintf("⚠️ %s CPU %.0f%% 超阈值 %d%%", name, s.CPU, cfg.CPUPercent))
		}
		if s.Mem >= float64(cfg.MemPercent) {
			out = append(out, fmt.Sprintf("⚠️ %s 内存 %.0f%% 超阈值 %d%%", name, s.Mem, cfg.MemPercent))
		}
		if s.Disk >= float64(cfg.DiskPercent) {
			out = append(out, fmt.Sprintf("⚠️ %s 磁盘 %.0f%% 超阈值 %d%%", name, s.Disk, cfg.DiskPercent))
		}
	}
	return out
}

func alertName(id string) string {
	if r := AgentByID(id); r != nil && r.Name != "" {
		return r.Name
	}
	return id
}

var (
	alertFireMu   sync.Mutex
	alertFired    = map[string]bool{} // 文案 -> 上轮已发，去重防刷屏
	breachSince   = map[string]time.Time{}
	incidentCount = map[string]int{}
	incidentLast  = map[string]time.Time{}
)

func alertLoop() {
	for range time.Tick(60 * time.Second) {
		alertsMu.RLock()
		cfg := alertConfig
		alertsMu.RUnlock()

		metricMu.Lock()
		ls := make(map[string]statSample, len(latestStat))
		for k, v := range latestStat {
			ls[k] = v
		}
		sn := make(map[string]time.Time, len(lastSeen))
		for k, v := range lastSeen {
			sn[k] = v
		}
		metricMu.Unlock()

		agentsMu.RLock()
		ids := make([]string, 0, len(agents))
		for id := range agents {
			ids = append(ids, id)
		}
		agentsMu.RUnlock()

		alertFireMu.Lock()
		now := time.Now()
		active := map[string]bool{}
		for _, id := range ids {
			name := alertName(id)
			offKey := id + "|offline"
			if t, ok := sn[id]; ok && now.Sub(t) > time.Duration(cfg.OfflineMinutes)*time.Minute {
				active[offKey] = true
				if !alertFired[offKey] {
					m := fmt.Sprintf("🔴 %s 离线超过 %d 分钟", name, cfg.OfflineMinutes)
					sendTGAlert(m)
					insertAlertEvent(id, "offline", "节点掉线", m)
				}
				continue
			}
			s, ok := ls[id]
			if !ok {
				continue
			}
			checks := []struct {
				k, label string
				v        float64
				limit    int
			}{{"cpu", "CPU", s.CPU, cfg.CPUPercent}, {"memory", "内存", s.Mem, cfg.MemPercent}}
			for _, c := range checks {
				key := id + "|" + c.k
				if c.v >= float64(c.limit) {
					active[key] = true
					if breachSince[key].IsZero() {
						breachSince[key] = now
					}
					if now.Sub(breachSince[key]) >= 3*time.Minute && incidentCount[key] < 3 && (incidentLast[key].IsZero() || now.Sub(incidentLast[key]) >= 3*time.Minute) {
						m := fmt.Sprintf("⚠️ %s %s %.0f%% 连续超过阈值 %d%%", name, c.label, c.v, c.limit)
						sendTGAlert(m)
						eid := insertAlertEvent(id, c.k, c.label+"警报", m)
						if eid > 0 {
							requestProcessSnapshot(id, eid)
						}
						incidentCount[key]++
						incidentLast[key] = now
					}
				} else {
					delete(breachSince, key)
					delete(incidentCount, key)
					delete(incidentLast, key)
				}
			}
		}
		alertFired = active
		alertFireMu.Unlock()
	}
}
