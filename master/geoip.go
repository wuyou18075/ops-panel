package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// geoip.go —— 按 agent 公网 IP 自动识别国家（ISO alpha-2）。
// 设计：
//   - 仅在 rec.Prefs.CountryCode 为空时解析（手动填写/已解析过就不覆盖）。
//   - 私网 / 回环 IP 直接跳过。
//   - 每个 agent 1 小时内最多尝试一次，避免抖动重连时反复请求外部 API。
//   - 外部服务 ip-api.com（免费、无需 key、限 45 req/min），失败则无旗帜。

var (
	geoMu      sync.Mutex
	geoAttempt = map[string]time.Time{} // agentID -> 上次尝试时间
)

// maybeResolveCountry 在 agent 连接时异步识别国家并写入 Prefs.CountryCode。
func maybeResolveCountry(agentID, remoteAddr, xff string) {
	rec := AgentByID(agentID)
	if rec == nil || rec.Prefs.CountryCode != "" {
		return
	}
	ip := clientIP(remoteAddr, xff)
	if ip == "" || isPrivateIP(ip) {
		return
	}

	geoMu.Lock()
	if t, ok := geoAttempt[agentID]; ok && time.Since(t) < time.Hour {
		geoMu.Unlock()
		return
	}
	geoAttempt[agentID] = time.Now()
	geoMu.Unlock()

	go func() {
		cc := lookupCountry(ip)
		if cc == "" {
			return
		}
		agentsMu.Lock()
		if r := agents[agentID]; r != nil && r.Prefs.CountryCode == "" {
			r.Prefs.CountryCode = cc
			_ = saveAgents(agentsFile)
		}
		agentsMu.Unlock()
		fmt.Printf("[GeoIP] %s -> %s (%s)\n", agentID, cc, ip)
	}()
}

// clientIP 从 X-Forwarded-For（反代场景）或 RemoteAddr 提取客户端 IP。
func clientIP(remoteAddr, xff string) string {
	if xff != "" {
		// 取第一跳（最初的客户端）
		first := strings.TrimSpace(strings.Split(xff, ",")[0])
		if ip := net.ParseIP(first); ip != nil {
			return ip.String()
		}
	}
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	if ip := net.ParseIP(strings.TrimSpace(host)); ip != nil {
		return ip.String()
	}
	return ""
}

// isPrivateIP 判断是否为私网 / 回环 / 链路本地地址。
func isPrivateIP(s string) bool {
	ip := net.ParseIP(s)
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
		return true
	}
	return false
}

// lookupCountry 调用 ip-api.com 返回国家码（如 "US"），失败返回空串。
func lookupCountry(ip string) string {
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://ip-api.com/json/" + ip + "?fields=status,countryCode")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<12))
	if err != nil {
		return ""
	}
	var v struct {
		Status      string `json:"status"`
		CountryCode string `json:"countryCode"`
	}
	if json.Unmarshal(body, &v) != nil || v.Status != "success" {
		return ""
	}
	return strings.ToUpper(strings.TrimSpace(v.CountryCode))
}
