package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type TrafficDaySnapshot struct {
	Date string `json:"date"`
	Sent int64  `json:"sent"`
	Recv int64  `json:"recv"`
}

type vnstatOutput struct {
	Interfaces []struct {
		Traffic struct {
			Day []struct {
				Date struct{ Year, Month, Day int } `json:"date"`
				RX   int64                          `json:"rx"`
				TX   int64                          `json:"tx"`
			} `json:"day"`
		} `json:"traffic"`
	} `json:"interfaces"`
}

func parseVnStatJSON(b []byte) ([]TrafficDaySnapshot, error) {
	var raw vnstatOutput
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	byDate := make(map[string]*TrafficDaySnapshot)
	for _, iface := range raw.Interfaces {
		for _, d := range iface.Traffic.Day {
			date := fmt.Sprintf("%04d-%02d-%02d", d.Date.Year, d.Date.Month, d.Date.Day)
			v := byDate[date]
			if v == nil {
				v = &TrafficDaySnapshot{Date: date}
				byDate[date] = v
			}
			v.Sent += d.TX
			v.Recv += d.RX
		}
	}
	cutoff := time.Now().AddDate(0, 0, -40).Format("2006-01-02")
	out := make([]TrafficDaySnapshot, 0, len(byDate))
	for date, d := range byDate {
		if date >= cutoff {
			out = append(out, *d)
		}
	}
	return out, nil
}

var (
	vnstatMu       sync.Mutex
	vnstatCached   []TrafficDaySnapshot
	vnstatCachedAt time.Time
)

func readVnStatTraffic() []TrafficDaySnapshot {
	vnstatMu.Lock()
	defer vnstatMu.Unlock()
	if time.Since(vnstatCachedAt) < 30*time.Second {
		return append([]TrafficDaySnapshot(nil), vnstatCached...)
	}
	args := []string{"--json", "d", "40"}
	if iface := defaultNetworkInterface(); iface != "" {
		args = append([]string{"-i", iface}, args...)
	}
	b, err := exec.Command("vnstat", args...).Output()
	if err != nil {
		return nil
	}
	days, err := parseVnStatJSON(b)
	if err != nil {
		return nil
	}
	vnstatCached, vnstatCachedAt = days, time.Now()
	return append([]TrafficDaySnapshot(nil), days...)
}

func defaultNetworkInterface() string {
	b, err := exec.Command("ip", "route", "show", "default").Output()
	if err != nil {
		return ""
	}
	f := strings.Fields(string(b))
	for i := 0; i+1 < len(f); i++ {
		if f[i] == "dev" {
			return f[i+1]
		}
	}
	return ""
}
