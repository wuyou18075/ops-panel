package main

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// history.go —— 每个 agent 的内存时序（1 分钟聚合桶，保留 24h）。
// best-effort：master 重启后清空。若需持久化可后续加 JSON 快照。

const maxHistPoints = 1440 // 24h × 60min

type histBucket struct {
	min                       int64   // unix 分钟
	n                         int     // 采样计数（用于求平均）
	cpu, mem, disk, up, down  float64 // 各指标的累加和
}

type agentHist struct {
	ring []*histBucket
}

var (
	histMu sync.Mutex
	hist   = map[string]*agentHist{}
)

// recordHistory 把一次采样并入当前分钟桶。
func recordHistory(agentID string, cpu, mem, disk, up, down float64) {
	histMu.Lock()
	defer histMu.Unlock()
	h := hist[agentID]
	if h == nil {
		h = &agentHist{}
		hist[agentID] = h
	}
	m := time.Now().Unix() / 60
	if n := len(h.ring); n > 0 && h.ring[n-1].min == m {
		b := h.ring[n-1]
		b.n++
		b.cpu += cpu
		b.mem += mem
		b.disk += disk
		b.up += up
		b.down += down
		return
	}
	h.ring = append(h.ring, &histBucket{min: m, n: 1, cpu: cpu, mem: mem, disk: disk, up: up, down: down})
	if len(h.ring) > maxHistPoints {
		// 丢弃最旧的一批（每分钟至多触发一次，成本可忽略）
		h.ring = append([]*histBucket(nil), h.ring[len(h.ring)-maxHistPoints:]...)
	}
}

// HistPoint 是对外的每分钟平均值。
type HistPoint struct {
	T    int64   `json:"t"`    // unix 秒（分钟对齐）
	CPU  float64 `json:"cpu"`
	Mem  float64 `json:"mem"`
	Disk float64 `json:"disk"`
	Up   float64 `json:"up"`   // 上行字节/秒（分钟均值）
	Down float64 `json:"down"` // 下行字节/秒（分钟均值）
}

func historySeries(agentID string) []HistPoint {
	histMu.Lock()
	defer histMu.Unlock()
	h := hist[agentID]
	if h == nil {
		return []HistPoint{}
	}
	out := make([]HistPoint, 0, len(h.ring))
	for _, b := range h.ring {
		n := float64(b.n)
		if n == 0 {
			n = 1
		}
		out = append(out, HistPoint{
			T:    b.min * 60,
			CPU:  round1(b.cpu / n),
			Mem:  round1(b.mem / n),
			Disk: round1(b.disk / n),
			Up:   b.up / n,
			Down: b.down / n,
		})
	}
	return out
}

func round1(v float64) float64 { return float64(int64(v*10+0.5)) / 10 }

// GET /api/history?agent_id=xxx —— 返回该节点 24h 分钟级序列（查看态公开）。
func handleHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := sanitizeAgentID(r.URL.Query().Get("agent_id"))
	if id == "" {
		http.Error(w, "缺少 agent_id", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(historySeries(id))
}
