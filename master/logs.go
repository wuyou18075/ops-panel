package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// logs.go —— 面板登录日志与节点 SSH 登录日志的存取（SQLite）。

// parseDevice 从 User-Agent 粗解析「浏览器 · 系统」。
func parseDevice(ua string) string {
	browser := "未知浏览器"
	switch {
	case strings.Contains(ua, "Edg/"):
		browser = "Edge"
	case strings.Contains(ua, "Chrome/"):
		browser = "Chrome"
	case strings.Contains(ua, "Firefox/"):
		browser = "Firefox"
	case strings.Contains(ua, "Safari/"):
		browser = "Safari"
	}
	os := "未知系统"
	switch {
	case strings.Contains(ua, "Windows"):
		os = "Windows"
	case strings.Contains(ua, "Mac OS X"), strings.Contains(ua, "Macintosh"):
		os = "macOS"
	case strings.Contains(ua, "Android"):
		os = "Android"
	case strings.Contains(ua, "iPhone"), strings.Contains(ua, "iPad"):
		os = "iOS"
	case strings.Contains(ua, "Linux"):
		os = "Linux"
	}
	return browser + " · " + os
}

// ── 面板登录日志 ──

type LoginLog struct {
	TS       int64  `json:"ts"`
	IP       string `json:"ip"`
	Location string `json:"location"`
	Device   string `json:"device"`
	Username string `json:"username"`
}

func insertPanelLogin(ts int64, ip, loc, device, user string) {
	db.Exec("INSERT INTO panel_login(ts,ip,location,device,username) VALUES(?,?,?,?,?)", ts, ip, loc, device, user)
	db.Exec("DELETE FROM panel_login WHERE id NOT IN (SELECT id FROM panel_login ORDER BY id DESC LIMIT 100)")
}

func listPanelLogin() []LoginLog {
	rows, err := db.Query("SELECT ts,ip,location,device,username FROM panel_login ORDER BY id DESC")
	if err != nil {
		return []LoginLog{}
	}
	defer rows.Close()
	out := []LoginLog{}
	for rows.Next() {
		var l LoginLog
		rows.Scan(&l.TS, &l.IP, &l.Location, &l.Device, &l.Username)
		out = append(out, l)
	}
	return out
}

func clearPanelLogin() { db.Exec("DELETE FROM panel_login") }

func handleLoginLogs(w http.ResponseWriter, r *http.Request) {
	if !operatorAuthorized(r) {
		http.Error(w, "未授权", http.StatusUnauthorized)
		return
	}
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(listPanelLogin())
	case http.MethodDelete:
		clearPanelLogin()
		w.WriteHeader(http.StatusOK)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// ── 节点 SSH 登录日志 ──

type SSHLog struct {
	TS       int64  `json:"ts"`
	IP       string `json:"ip"`
	Location string `json:"location"`
	User     string `json:"user"`
	Method   string `json:"method"`
	Success  bool   `json:"success"`
}

func insertSSHLogin(agentID string, ts int64, ip, loc, user, method string, success bool) {
	s := 0
	if success {
		s = 1
	}
	db.Exec("INSERT INTO ssh_login(agent_id,ts,ip,location,username,method,success) VALUES(?,?,?,?,?,?,?)", agentID, ts, ip, loc, user, method, s)
	db.Exec("DELETE FROM ssh_login WHERE agent_id=? AND id NOT IN (SELECT id FROM ssh_login WHERE agent_id=? ORDER BY id DESC LIMIT 200)", agentID, agentID)
}

func listSSHLogin(agentID string) []SSHLog {
	rows, err := db.Query("SELECT ts,ip,location,username,method,success FROM ssh_login WHERE agent_id=? ORDER BY id DESC", agentID)
	if err != nil {
		return []SSHLog{}
	}
	defer rows.Close()
	out := []SSHLog{}
	for rows.Next() {
		var l SSHLog
		var s int
		rows.Scan(&l.TS, &l.IP, &l.Location, &l.User, &l.Method, &s)
		l.Success = s == 1
		out = append(out, l)
	}
	return out
}

func clearSSHLogin(agentID string) { db.Exec("DELETE FROM ssh_login WHERE agent_id=?", agentID) }

// weeklySSHFails 统计滚动 7 天内失败登录数，扣除重置基线之前。
func weeklySSHFails(agentID string, now time.Time) int {
	from := now.AddDate(0, 0, -7).Unix()
	var reset int64
	db.QueryRow("SELECT reset_at FROM ssh_fail_reset WHERE agent_id=?", agentID).Scan(&reset)
	if reset > from {
		from = reset
	}
	var n int
	db.QueryRow("SELECT count(*) FROM ssh_login WHERE agent_id=? AND success=0 AND ts>=?", agentID, from).Scan(&n)
	return n
}

func resetSSHFails(agentID string, at int64) {
	db.Exec("INSERT INTO ssh_fail_reset(agent_id,reset_at) VALUES(?,?) ON CONFLICT(agent_id) DO UPDATE SET reset_at=excluded.reset_at", agentID, at)
}

type sshStat struct {
	AgentID   string `json:"agent_id"`
	WeekFails int    `json:"week_fails"`
}

func sshStatsSnapshot() []sshStat {
	agentsMu.RLock()
	ids := make([]string, 0, len(agents))
	for id := range agents {
		ids = append(ids, id)
	}
	agentsMu.RUnlock()
	now := time.Now()
	out := make([]sshStat, 0, len(ids))
	for _, id := range ids {
		out = append(out, sshStat{id, weeklySSHFails(id, now)})
	}
	return out
}

func handleSSHLogs(w http.ResponseWriter, r *http.Request) {
	if !operatorAuthorized(r) {
		http.Error(w, "未授权", http.StatusUnauthorized)
		return
	}
	id := sanitizeAgentID(r.URL.Query().Get("agent_id"))
	if id == "" {
		http.Error(w, "缺少 agent_id", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(listSSHLogin(id))
	case http.MethodDelete:
		clearSSHLogin(id)
		w.WriteHeader(http.StatusOK)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleSSHStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sshStatsSnapshot())
}

func handleSSHFailReset(w http.ResponseWriter, r *http.Request) {
	if !operatorAuthorized(r) {
		http.Error(w, "未授权", http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	resetSSHFails(req.AgentID, time.Now().Unix())
	w.WriteHeader(http.StatusOK)
}
