package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	netio "github.com/shirou/gopsutil/v3/net"
)

type Message struct {
	Type    string `json:"type"`
	AgentID string `json:"agent_id"`
	Data    string `json:"data"`
	Nonce   int64  `json:"nonce,omitempty"`
	Sig     string `json:"sig,omitempty"`
}

const defaultMasterURL = "127.0.0.1:8080"
const defaultInterval = 2 * time.Second

// AgentVersion 上报给 master，用于表格「Agent」列展示
const AgentVersion = "1.0.0"

var dangerousKeywords = []string{
	"rm -rf /", "mkfs", "dd if=", "> /dev/sda", ":(){:|:&};:",
}

type StatData struct {
	CPU         float64              `json:"cpu"`
	Mem         float64              `json:"mem"`
	Disk        float64              `json:"disk"`
	SwapPct     float64              `json:"swap_pct"`
	Load1       float64              `json:"load_1"`
	Load5       float64              `json:"load_5"`
	Load15      float64              `json:"load_15"`
	Uptime      uint64               `json:"uptime"`
	CPUCount    int                  `json:"cpu_count"`
	MemTotal    uint64               `json:"mem_total"`
	DiskTotal   uint64               `json:"disk_total"`
	NetSent     float64              `json:"net_sent"`
	NetRecv     float64              `json:"net_recv"`
	AgentVer    string               `json:"agent_ver"`
	TrafficDays []TrafficDaySnapshot `json:"traffic_days,omitempty"`
}

func main() {
	if err := connectAndServe(); err != nil {
		fmt.Println("[Agent] 连接失败:", err)
	}
	fmt.Println("[Agent] 进程退出")
}

func connectAndServe() error {
	agentID := agentID()
	secret := os.Getenv("AGENT_SECRET")
	if secret == "" {
		return fmt.Errorf("未设置 AGENT_SECRET 环境变量，拒绝连接")
	}

	// 从 MASTER_URL 构建 WebSocket 地址
	// MASTER_URL = http://IP:PORT/PATH → ws://IP:PORT/PATH/ws/agent?id=...
	masterAddr := os.Getenv("MASTER_URL")
	if masterAddr == "" {
		masterAddr = defaultMasterURL
	}
	wsAddr := strings.Replace(masterAddr, "http://", "ws://", 1)
	wsAddr = strings.Replace(wsAddr, "https://", "wss://", 1)
	if !strings.HasPrefix(wsAddr, "ws") {
		wsAddr = "ws://" + wsAddr
	}
	wsAddr += "/ws/agent?id=" + url.QueryEscape(agentID) + "&token=" + url.QueryEscape(secret)

	fmt.Printf("[Agent] 正在连接 Master: %s\n", wsAddr)

	dialer := websocket.DefaultDialer
	if strings.HasPrefix(wsAddr, "wss") {
		dialer = &websocket.Dialer{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	conn, _, err := dialer.Dial(wsAddr, nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	collectorStop := make(chan struct{})
	defer close(collectorStop)

	fmt.Println("[Agent] 注册成功，开始上报系统状态。")

	var writeMutex sync.Mutex
	netSampler := newNetSampler()
	interval := defaultInterval

	go func() {
		for {
			statData := collectStats(netSampler)
			statData.TrafficDays = readVnStatTraffic()
			statBytes, _ := json.Marshal(statData)
			msg := Message{Type: "stat", AgentID: agentID, Data: string(statBytes)}
			writeJSON(conn, &writeMutex, msg)
			select {
			case <-collectorStop:
				return
			case <-time.After(interval):
			}
		}
	}()

	go startSSHCollector(conn, &writeMutex, agentID, collectorStop)

	for {
		var msg Message
		err := conn.ReadJSON(&msg)
		if err != nil {
			return err
		}

		switch msg.Type {
		case "config":
			if secs, err := parseInterval(msg.Data); err == nil {
				interval = secs
				fmt.Printf("[Agent] 上报频率已更新为 %s\n", interval)
			}
		case "monitor_config":
			applyMonitorConfig(conn, &writeMutex, agentID, msg.Data)
		case "cmd":
			if !verifyCommand(secret, msg.AgentID, msg.Data, msg.Nonce, msg.Sig) {
				fmt.Println("[Agent] 命令签名校验失败，已丢弃")
				continue
			}
			if isDangerous(msg.Data) {
				fmt.Println("[Agent] 命中危险命令黑名单，已拒绝执行:", msg.Data)
				writeJSON(conn, &writeMutex, Message{
					Type: "log", AgentID: agentID, Data: "拒绝执行：命中危险命令黑名单",
				})
				continue
			}
			fmt.Printf("[Agent] 收到已签名的指令: %s\n", msg.Data)
			go executeCommand(conn, &writeMutex, agentID, msg.Data)
		case "process_snapshot":
			id, _ := strconv.ParseInt(msg.Data, 10, 64)
			go sendProcessSnapshot(conn, &writeMutex, agentID, id)
		}
	}
}

func sendProcessSnapshot(conn *websocket.Conn, wm *sync.Mutex, agentID string, recordID int64) {
	out, _ := exec.Command("ps", "-eo", "pid,comm,%cpu,%mem", "--no-headers").Output()
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	sort.Slice(lines, func(i, j int) bool {
		fi := strings.Fields(lines[i])
		fj := strings.Fields(lines[j])
		if len(fi) < 4 || len(fj) < 4 {
			return false
		}
		ai, _ := strconv.ParseFloat(fi[2], 64)
		am, _ := strconv.ParseFloat(fi[3], 64)
		bi, _ := strconv.ParseFloat(fj[2], 64)
		bm, _ := strconv.ParseFloat(fj[3], 64)
		return ai+am > bi+bm
	})
	if len(lines) > 5 {
		lines = lines[:5]
	}
	p := struct {
		RecordID int64  `json:"record_id"`
		Output   string `json:"output"`
	}{recordID, strings.Join(lines, "\n")}
	b, _ := json.Marshal(p)
	writeJSON(conn, wm, Message{Type: "process_snapshot", AgentID: agentID, Data: string(b)})
}

func agentID() string {
	if value := os.Getenv("AGENT_ID"); value != "" {
		return value
	}
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		return "node-unknown"
	}
	return hostname
}

func parseInterval(s string) (time.Duration, error) {
	n, err := time.ParseDuration(s + "s")
	if err != nil {
		return 0, err
	}
	if n < 1*time.Second {
		n = 1 * time.Second
	}
	if n > 60*time.Second {
		n = 60 * time.Second
	}
	return n, nil
}

func isDangerous(cmd string) bool {
	lower := strings.ToLower(cmd)
	for _, k := range dangerousKeywords {
		if strings.Contains(lower, k) {
			return true
		}
	}
	return false
}

func collectStats(netSampler *netSampler) StatData {
	cpuPercent, _ := cpu.Percent(0, false)
	memInfo, _ := mem.VirtualMemory()
	diskInfo, _ := disk.Usage("/")
	loadInfo, _ := load.Avg()
	hostInfo, _ := host.Info()
	netSent, netRecv := netSampler.rate()

	cpuVal := 0.0
	if len(cpuPercent) > 0 {
		cpuVal = cpuPercent[0]
	}
	memVal := 0.0
	if memInfo != nil {
		memVal = memInfo.UsedPercent
	}
	diskVal := 0.0
	if diskInfo != nil {
		diskVal = diskInfo.UsedPercent
	}
	load1 := 0.0
	load5 := 0.0
	load15 := 0.0
	if loadInfo != nil {
		load1 = loadInfo.Load1
		load5 = loadInfo.Load5
		load15 = loadInfo.Load15
	}
	swapVal := 0.0
	if memInfo != nil && memInfo.SwapTotal > 0 {
		swapVal = float64(memInfo.SwapTotal-memInfo.SwapFree) / float64(memInfo.SwapTotal) * 100
	}
	uptimeVal := uint64(0)
	if hostInfo != nil {
		uptimeVal = hostInfo.Uptime
	}
	cpuCount, _ := cpu.Counts(true)
	memTotal := uint64(0)
	if memInfo != nil {
		memTotal = memInfo.Total
	}
	diskTotal := uint64(0)
	if diskInfo != nil {
		diskTotal = diskInfo.Total
	}

	return StatData{
		CPU: cpuVal, Mem: memVal, Disk: diskVal,
		SwapPct: swapVal,
		Load1:   load1, Load5: load5, Load15: load15,
		Uptime:   uptimeVal,
		CPUCount: cpuCount, MemTotal: memTotal, DiskTotal: diskTotal,
		NetSent: netSent, NetRecv: netRecv,
		AgentVer: AgentVersion,
	}
}

type netSampler struct {
	lastSent uint64
	lastRecv uint64
	lastTime time.Time
}

func newNetSampler() *netSampler {
	return &netSampler{lastTime: time.Now()}
}

func (s *netSampler) rate() (float64, float64) {
	counters, err := netio.IOCounters(false)
	now := time.Now()
	if err != nil || len(counters) == 0 {
		return 0, 0
	}
	currentSent := counters[0].BytesSent
	currentRecv := counters[0].BytesRecv
	elapsed := now.Sub(s.lastTime).Seconds()
	if s.lastTime.IsZero() || elapsed <= 0 || (s.lastSent == 0 && s.lastRecv == 0) {
		s.lastSent = currentSent
		s.lastRecv = currentRecv
		s.lastTime = now
		return 0, 0
	}
	sentRate := float64(currentSent-s.lastSent) / elapsed
	recvRate := float64(currentRecv-s.lastRecv) / elapsed
	s.lastSent = currentSent
	s.lastRecv = currentRecv
	s.lastTime = now
	return sentRate, recvRate
}

func executeCommand(conn *websocket.Conn, writeMutex *sync.Mutex, agentID string, command string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Stderr = cmd.Stdout

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	if err := cmd.Start(); err != nil {
		return
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 64*1024)
	lines := 0
	const maxLines = 2000
	for scanner.Scan() && lines < maxLines {
		text := scanner.Text()
		writeJSON(conn, writeMutex, Message{Type: "log", AgentID: agentID, Data: text})
		lines++
	}
	if lines >= maxLines {
		writeJSON(conn, writeMutex, Message{Type: "log", AgentID: agentID, Data: "... 输出过长已截断 ..."})
	}

	if err := cmd.Wait(); err != nil {
		writeJSON(conn, writeMutex, Message{Type: "log", AgentID: agentID, Data: "执行错误: " + err.Error()})
	}
	writeJSON(conn, writeMutex, Message{Type: "log", AgentID: agentID, Data: "--- 执行结束 ---"})
}

func writeJSON(conn *websocket.Conn, writeMutex *sync.Mutex, message Message) {
	writeMutex.Lock()
	defer writeMutex.Unlock()
	conn.WriteJSON(message)
}

// ════════════════════════════════════════════════════════════
//  Ping/延迟监控探测
// ════════════════════════════════════════════════════════════

type Monitor struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"` // tcp | http | icmp
	Target   string `json:"target"`
	Interval int    `json:"interval"` // 秒
	AgentID  string `json:"agent_id"`
}

var (
	proberMu     sync.Mutex
	proberCancel context.CancelFunc
)

// applyMonitorConfig 收到 master 下发的监控列表后，重置全部探测器。
func applyMonitorConfig(conn *websocket.Conn, wm *sync.Mutex, agentID, data string) {
	var list []Monitor
	if json.Unmarshal([]byte(data), &list) != nil {
		return
	}
	proberMu.Lock()
	if proberCancel != nil {
		proberCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	proberCancel = cancel
	proberMu.Unlock()

	fmt.Printf("[Agent] 收到 %d 个探测任务\n", len(list))
	for _, m := range list {
		go runProber(ctx, conn, wm, agentID, m)
	}
}

func runProber(ctx context.Context, conn *websocket.Conn, wm *sync.Mutex, agentID string, m Monitor) {
	iv := time.Duration(m.Interval) * time.Second
	if iv < 5*time.Second {
		iv = 30 * time.Second
	}
	for {
		up, lat := probe(ctx, m)
		res := struct {
			MonitorID string  `json:"monitor_id"`
			Up        bool    `json:"up"`
			LatencyMs float64 `json:"latency_ms"`
			TS        int64   `json:"ts"`
		}{m.ID, up, lat, time.Now().Unix()}
		b, _ := json.Marshal(res)
		writeJSON(conn, wm, Message{Type: "probe_result", AgentID: agentID, Data: string(b)})
		select {
		case <-ctx.Done():
			return
		case <-time.After(iv):
		}
	}
}

// probe 执行一次探测，返回是否可达与延迟（毫秒）。
func probe(ctx context.Context, m Monitor) (bool, float64) {
	switch m.Type {
	case "tcp":
		return probeTCP(m.Target)
	case "http":
		return probeHTTP(ctx, m.Target)
	case "icmp":
		return probeICMP(ctx, m.Target)
	}
	return false, 0
}

func probeTCP(target string) (bool, float64) {
	if _, _, err := net.SplitHostPort(target); err != nil {
		target = net.JoinHostPort(strings.Trim(target, "[]"), "80")
	}
	start := time.Now()
	c, err := net.DialTimeout("tcp", target, 5*time.Second)
	if err != nil {
		return false, 0
	}
	c.Close()
	return true, float64(time.Since(start).Microseconds()) / 1000
}

func probeHTTP(ctx context.Context, target string) (bool, float64) {
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		target = "http://" + target
	}
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, target, nil)
	if err != nil {
		return false, 0
	}
	client := http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return false, 0
	}
	defer resp.Body.Close()
	lat := float64(time.Since(start).Microseconds()) / 1000
	return resp.StatusCode < 400, lat
}

// probeICMP 通过系统 ping 命令实现（无需 raw socket / root）。
func probeICMP(ctx context.Context, host string) (bool, float64) {
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, "ping", "-c", "1", "-W", "2", host).Output()
	if err != nil {
		return false, 0
	}
	// 解析 "time=12.3 ms"
	s := string(out)
	i := strings.Index(s, "time=")
	if i < 0 {
		return true, 0
	}
	rest := s[i+5:]
	j := strings.IndexByte(rest, ' ')
	if j < 0 {
		return true, 0
	}
	var ms float64
	if _, err := fmt.Sscanf(strings.TrimSpace(rest[:j]), "%f", &ms); err != nil {
		return true, 0
	}
	return true, ms
}
