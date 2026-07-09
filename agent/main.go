package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
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

// 需与 Master 保持一致的数据结构
type Message struct {
	Type    string `json:"type"`
	AgentID string `json:"agent_id"`
	Data    string `json:"data"`
	Nonce   int64  `json:"nonce,omitempty"`
	Sig     string `json:"sig,omitempty"`
}

const defaultMasterURL = "127.0.0.1:8080"
const defaultInterval = 5 * time.Second

// 危险命令黑名单：即使 Master 已签名，本地仍拒绝执行破坏性指令。
// 纵深防御：防止 Master 被入侵后下发毁灭性命令。
var dangerousKeywords = []string{
	"rm -rf /", "mkfs", "dd if=", "> /dev/sda", ":(){:|:&};:",
}

type StatData struct {
	CPU     float64 `json:"cpu"`
	Mem     float64 `json:"mem"`
	Disk    float64 `json:"disk"`
	Load1   float64 `json:"load1"`
	Uptime  uint64  `json:"uptime"`
	NetSent float64 `json:"net_sent"`
	NetRecv float64 `json:"net_recv"`
}

func main() {
	for {
		err := connectAndServe()
		fmt.Println("[Agent] 连接断开，3秒后尝试重连...", err)
		time.Sleep(3 * time.Second)
	}
}

func connectAndServe() error {
	agentID := agentID()
	secret := os.Getenv("AGENT_SECRET")
	if secret == "" {
		return fmt.Errorf("未设置 AGENT_SECRET 环境变量，拒绝连接")
	}
	u := url.URL{
		Scheme:   masterScheme(),
		Host:     masterURL(),
		Path:     "/ws/agent",
		RawQuery: "id=" + url.QueryEscape(agentID) + "&token=" + url.QueryEscape(secret),
	}
	fmt.Printf("[Agent] 正在连接 Master: %s\n", u.String())

	dialer := websocket.DefaultDialer
	if masterScheme() == "wss" {
		// 允许自签证书（部署方应确保实际用可信证书）
		dialer = &websocket.Dialer{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	conn, _, err := dialer.Dial(u.String(), nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	fmt.Println("[Agent] 注册成功，开始上报系统状态。")

	var writeMutex sync.Mutex
	netSampler := newNetSampler()
	interval := defaultInterval

	// 1. 上报协程：按 Master 下发的频率上报
	go func() {
		for {
			statData := collectStats(netSampler)
			statBytes, _ := json.Marshal(statData)
			msg := Message{Type: "stat", AgentID: agentID, Data: string(statBytes)}
			writeJSON(conn, &writeMutex, msg)
			time.Sleep(interval)
		}
	}()

	// 2. 主循环：监听 Master 下发
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
		case "cmd":
			// 先验签再执行：无 secret 算不出合法 sig，伪造命令直接丢弃
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
		}
	}
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

func masterURL() string {
	if value := os.Getenv("MASTER_URL"); value != "" {
		return value
	}
	return defaultMasterURL
}

func masterScheme() string {
	if strings.HasPrefix(masterURL(), "wss") || os.Getenv("MASTER_WSS") == "1" {
		return "wss"
	}
	return "ws"
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
	loadVal := 0.0
	if loadInfo != nil {
		loadVal = loadInfo.Load1
	}
	uptimeVal := uint64(0)
	if hostInfo != nil {
		uptimeVal = hostInfo.Uptime
	}

	return StatData{
		CPU:     cpuVal,
		Mem:     memVal,
		Disk:    diskVal,
		Load1:   loadVal,
		Uptime:  uptimeVal,
		NetSent: netSent,
		NetRecv: netRecv,
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

// executeCommand 本地执行命令：30s 超时（防挂起）、输出截断（防内存爆）。
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
	scanner.Buffer(make([]byte, 0, 64*1024), 64*1024) // 单行最大 64KB
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
