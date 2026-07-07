package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os/exec"
	"time"

	"github.com/gorilla/websocket"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

// 需与 Master 保持一致的数据结构
type Message struct {
	Type    string `json:"type"`
	AgentID string `json:"agent_id"`
	Data    string `json:"data"`
}

const (
	AgentID   = "node-ubuntu-01" // 当前机器的唯一标识，可改为动态读取 hostname
	MasterURL = "127.0.0.1:8080" // 替换为你 Master 服务器的真实 IP
)

func main() {
	for {
		err := connectAndServe()
		fmt.Println("[Agent] 连接断开，3秒后尝试重连...", err)
		time.Sleep(3 * time.Second)
	}
}

func connectAndServe() error {
	u := url.URL{Scheme: "ws", Host: MasterURL, Path: "/ws/agent", RawQuery: "id=" + AgentID}
	fmt.Printf("[Agent] 正在连接 Master: %s\n", u.String())

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	fmt.Println("[Agent] 注册成功，开始上报系统状态。")

	// 1. 开启协程：每 2 秒上报一次 CPU 和内存状态
	go func() {
		for {
			cpuPercent, _ := cpu.Percent(0, false)
			memInfo, _ := mem.VirtualMemory()
			
			cpuVal := 0.0
			if len(cpuPercent) > 0 {
				cpuVal = cpuPercent[0]
			}

			statData := map[string]float64{
				"cpu": cpuVal,
				"mem": memInfo.UsedPercent,
			}
			statBytes, _ := json.Marshal(statData)

			msg := Message{Type: "stat", AgentID: AgentID, Data: string(statBytes)}
			conn.WriteJSON(msg)
			
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
			go executeCommand(conn, msg.Data)
		}
	}
}

// 异步执行命令并将输出流推回 Master
func executeCommand(conn *websocket.Conn, command string) {
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
		resp := Message{Type: "log", AgentID: AgentID, Data: text}
		conn.WriteJSON(resp)
	}

	cmd.Wait()
	conn.WriteJSON(Message{Type: "log", AgentID: AgentID, Data: "--- 执行结束 ---"})
}