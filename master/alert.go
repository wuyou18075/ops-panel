package main

import (
	"fmt"
	"sync"
	"time"
)

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
	alertFireMu sync.Mutex
	alertFired  = map[string]bool{} // 文案 -> 上轮已发，去重防刷屏
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

		msgs := evalAlerts(cfg, ids, ls, sn, time.Now())
		cur := map[string]bool{}
		for _, m := range msgs {
			cur[m] = true
		}
		alertFireMu.Lock()
		for _, m := range msgs {
			if !alertFired[m] {
				sendTGAlert(m)
			}
		}
		alertFired = cur // 未再出现的告警清除，恢复后可再报
		alertFireMu.Unlock()
	}
}
