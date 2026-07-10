package main

import (
	"encoding/json"
	"net/http"
	"strings"
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
