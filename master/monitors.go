package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// monitors.go —— Ping/延迟监控（Nezha 风格）。
// operator 配置探测目标 → master 经 agent WS 下发 monitor_config →
// agent 定时探测并上报 probe_result → master 存最新值 + 短历史 → 前端「服务监控」页。

type Monitor struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`     // tcp | http | icmp
	Target   string `json:"target"`   // tcp: host:port · http: url · icmp: host
	Interval int    `json:"interval"` // 探测间隔（秒）
	AgentID  string `json:"agent_id"` // 由哪个节点执行探测（探测视角）
}

type ProbeResult struct {
	MonitorID string  `json:"monitor_id"`
	Up        bool    `json:"up"`
	LatencyMs float64 `json:"latency_ms"`
	TS        int64   `json:"ts"`
}

const (
	monitorsFile    = "monitors.json"
	maxProbeHistory = 120
)

type probeState struct {
	latest  ProbeResult
	history []ProbeResult
}

var (
	monitorsMu  sync.RWMutex
	monitors    = map[string]*Monitor{}
	probeStates = map[string]*probeState{}
)

// ── 持久化 ──

func loadMonitors(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	var list []*Monitor
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}
	for _, m := range list {
		monitors[m.ID] = m
	}
	return nil
}

func saveMonitors(path string) error {
	list := make([]*Monitor, 0, len(monitors))
	for _, m := range monitors {
		list = append(list, m)
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// ── 对外视图 ──

type MonitorView struct {
	*Monitor
	Up        bool          `json:"up"`
	LatencyMs float64       `json:"latency_ms"`
	Uptime    float64       `json:"uptime"`   // 最近窗口可用率 %
	LastTS    int64         `json:"last_ts"`
	History   []ProbeResult `json:"history"`
}

func MonitorsList() []MonitorView {
	monitorsMu.RLock()
	defer monitorsMu.RUnlock()
	out := make([]MonitorView, 0, len(monitors))
	for id, m := range monitors {
		v := MonitorView{Monitor: m, History: []ProbeResult{}}
		if st := probeStates[id]; st != nil {
			v.Up = st.latest.Up
			v.LatencyMs = st.latest.LatencyMs
			v.LastTS = st.latest.TS
			v.History = append([]ProbeResult(nil), st.history...)
			if n := len(st.history); n > 0 {
				up := 0
				for _, r := range st.history {
					if r.Up {
						up++
					}
				}
				v.Uptime = round1(float64(up) / float64(n) * 100)
			}
		}
		out = append(out, v)
	}
	return out
}

func monitorsForAgent(agentID string) []Monitor {
	monitorsMu.RLock()
	defer monitorsMu.RUnlock()
	out := make([]Monitor, 0)
	for _, m := range monitors {
		if m.AgentID == agentID {
			out = append(out, *m)
		}
	}
	return out
}

// ── 探测结果入库 ──

func ingestProbeResult(agentID, data string) {
	var pr ProbeResult
	if json.Unmarshal([]byte(data), &pr) != nil || pr.MonitorID == "" {
		return
	}
	monitorsMu.Lock()
	defer monitorsMu.Unlock()
	m := monitors[pr.MonitorID]
	if m == nil || m.AgentID != agentID { // 只接受该监控归属节点上报的结果
		return
	}
	if pr.TS == 0 {
		pr.TS = time.Now().Unix()
	}
	st := probeStates[pr.MonitorID]
	if st == nil {
		st = &probeState{}
		probeStates[pr.MonitorID] = st
	}
	st.latest = pr
	st.history = append(st.history, pr)
	if len(st.history) > maxProbeHistory {
		st.history = append([]ProbeResult(nil), st.history[len(st.history)-maxProbeHistory:]...)
	}
}

// ── 向 agent 下发监控配置 ──

func pushMonitorConfig(agentID string) {
	agentMutex.RLock()
	sconn, ok := agentConns[agentID]
	agentMutex.RUnlock()
	if !ok {
		return
	}
	list := monitorsForAgent(agentID)
	payload, _ := json.Marshal(list)
	sconn.Write(mustJSON(Message{Type: "monitor_config", AgentID: agentID, Data: string(payload)}))
}

// ── HTTP API ──

// /api/monitors —— GET 公开（查看态）；POST/DELETE 需 operator。
func handleMonitors(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(MonitorsList())
	case http.MethodPost:
		if !operatorAuthorized(r) {
			http.Error(w, "未授权", http.StatusUnauthorized)
			return
		}
		var m Monitor
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if err := validateMonitor(&m); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		monitorsMu.Lock()
		var oldAgent string
		if m.ID == "" {
			m.ID = genSecret()[:8]
		} else if prev := monitors[m.ID]; prev != nil {
			oldAgent = prev.AgentID
		}
		monitors[m.ID] = &m
		_ = saveMonitors(monitorsFile)
		monitorsMu.Unlock()
		// 下发到（新/旧）执行节点
		pushMonitorConfig(m.AgentID)
		if oldAgent != "" && oldAgent != m.AgentID {
			pushMonitorConfig(oldAgent)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(m)
	case http.MethodDelete:
		if !operatorAuthorized(r) {
			http.Error(w, "未授权", http.StatusUnauthorized)
			return
		}
		var req struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		monitorsMu.Lock()
		agentID := ""
		if m := monitors[req.ID]; m != nil {
			agentID = m.AgentID
		}
		delete(monitors, req.ID)
		delete(probeStates, req.ID)
		_ = saveMonitors(monitorsFile)
		monitorsMu.Unlock()
		if agentID != "" {
			pushMonitorConfig(agentID)
		}
		w.WriteHeader(http.StatusOK)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func validateMonitor(m *Monitor) error {
	m.Type = strings.ToLower(strings.TrimSpace(m.Type))
	switch m.Type {
	case "tcp", "http", "icmp":
	default:
		return fmt.Errorf("type 必须是 tcp/http/icmp")
	}
	if strings.TrimSpace(m.Target) == "" {
		return fmt.Errorf("target 不能为空")
	}
	if strings.TrimSpace(m.AgentID) == "" {
		return fmt.Errorf("必须指定执行探测的节点 agent_id")
	}
	if m.Interval < 5 {
		m.Interval = 30
	}
	if m.Interval > 3600 {
		m.Interval = 3600
	}
	if m.Name == "" {
		m.Name = m.Target
	}
	return nil
}
