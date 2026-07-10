package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

type LatencyTemplate struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Target string `json:"target"`
}
type SystemSettings struct {
	ProbeInterval    int               `json:"probe_interval"`
	ProbeType        string            `json:"probe_type"`
	LatencyTemplates []LatencyTemplate `json:"latency_templates"`
}

var systemSettings = SystemSettings{ProbeInterval: 30, ProbeType: "icmp", LatencyTemplates: []LatencyTemplate{
	{ID: "hefei-mobile", Name: "合肥移动", Target: "211.138.180.2"},
	{ID: "hefei-unicom", Name: "合肥联通", Target: "218.104.78.2"},
	{ID: "hefei-telecom", Name: "合肥电信", Target: "61.132.163.68"},
}}

func loadSystemSettings() error {
	var s string
	if db.QueryRow("SELECT data FROM system_settings WHERE id=1").Scan(&s) != nil {
		return nil
	}
	return json.Unmarshal([]byte(s), &systemSettings)
}
func saveSystemSettings() error {
	b, _ := json.Marshal(systemSettings)
	_, err := db.Exec("INSERT INTO system_settings(id,data) VALUES(1,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data", string(b))
	return err
}
func normalizeSystemSettings(s *SystemSettings) {
	if s.ProbeInterval < 5 {
		s.ProbeInterval = 30
	}
	if s.ProbeInterval > 3600 {
		s.ProbeInterval = 3600
	}
	s.ProbeType = strings.ToLower(s.ProbeType)
	if s.ProbeType != "tcp" && s.ProbeType != "http" && s.ProbeType != "icmp" {
		s.ProbeType = "icmp"
	}
}
func handleSystemSettings(w http.ResponseWriter, r *http.Request) {
	if !operatorAuthorized(r) {
		http.Error(w, "未授权", 401)
		return
	}
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(systemSettings)
	case http.MethodPost:
		var s SystemSettings
		if json.NewDecoder(r.Body).Decode(&s) != nil {
			http.Error(w, "invalid body", 400)
			return
		}
		normalizeSystemSettings(&s)
		systemSettings = s
		if err := saveSystemSettings(); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		syncAllLatencyMonitors()
		w.WriteHeader(200)
	default:
		http.Error(w, "method not allowed", 405)
	}
}
