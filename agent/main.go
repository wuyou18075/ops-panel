package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
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
}

const defaultMasterURL = "127.0.0.1:8080"

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
	u := url.URL{Scheme: "ws", Host: masterURL(), Path: "/ws/agent", RawQuery: "id=" + agentID}
	fmt.Printf("[Agent] 正在连接 Master: %s\n", u.String())

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	fmt.Println("[Agent] 注册成功，开始上报系统状态。")

	var writeMutex sync.Mutex
	netSampler := newNetSampler()

	// 1. 开启协程：每 2 秒上报一次 CPU 和内存状态
	go func() {
		for {
			statData := collectStats(netSampler)
			statBytes, _ := json.Marshal(statData)

			msg := Message{Type: "stat", AgentID: agentID, Data: string(statBytes)}
			writeJSON(conn, &writeMutex, msg)

			time.Sleep(2 * time.Second)
		}
	}()

	// 2. 主循环：监听 Master 下发的命令
	for {
		var msg Message
		err := conn.ReadJSON(&msg)
		if err != nil {
			return err
		}

		if msg.Type == "cmd" {
			fmt.Printf("[Agent] 收到指令: %s\n", msg.Data)
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
	if s.lastTime.IsZero() || elapsed <= 0 || s.lastSent == 0 && s.lastRecv == 0 {
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

// 异步执行命令并将输出流推回 Master
func executeCommand(conn *websocket.Conn, writeMutex *sync.Mutex, agentID string, command string) {
	cmd := exec.Command("sh", "-c", command)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		return
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		text := scanner.Text()
		resp := Message{Type: "log", AgentID: agentID, Data: text}
		writeJSON(conn, writeMutex, resp)
	}

	cmd.Wait()
	writeJSON(conn, writeMutex, Message{Type: "log", AgentID: agentID, Data: "--- 执行结束 ---"})
}

func writeJSON(conn *websocket.Conn, writeMutex *sync.Mutex, message Message) {
	writeMutex.Lock()
	defer writeMutex.Unlock()
	conn.WriteJSON(message)
}
