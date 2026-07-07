package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	tele "gopkg.in/telebot.v3"
)

//go:embed dist/*
var frontendFiles embed.FS

// Message 定义了各个端之间通信的标准数据包格式
type Message struct {
	Type    string `json:"type"`     // stat(状态), cmd(命令), log(日志)
	AgentID string `json:"agent_id"` // 来源或目标的 Agent 标识
	Data    string `json:"data"`     // 具体数据内容 (JSON字符串或纯文本)
}

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	// Agent 连接池 (AgentID -> WebSocket)
	agentConns = make(map[string]*websocket.Conn)
	agentMutex sync.RWMutex

	// Web 前端大屏连接池
	webConns = make(map[*websocket.Conn]bool)
	webMutex sync.RWMutex

	// TG Bot 实例
	bot *tele.Bot
)

func main() {
	// 1. 初始化 Telegram Bot (如果环境变量提供了 TG_TOKEN)
	tgToken := os.Getenv("TG_TOKEN")
	if tgToken != "" {
		initTGBot(tgToken)
	} else {
		fmt.Println("[警告] 未设置 TG_TOKEN 环境变量，Telegram Bot 功能将不启用。")
	}

	// 2. 注册路由
	http.HandleFunc("/ws/agent", handleAgentWS)
	http.HandleFunc("/ws/web", handleWebWS)
	registerFrontend()

	// 3. 启动服务
	fmt.Printf("[Master] 服务端已启动，Web 面板访问地址: http://%s:8080\n", publicIPv4())
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal("服务启动失败:", err)
	}
}

func registerFrontend() {
	distFS, err := fs.Sub(frontendFiles, "dist")
	if err != nil {
		log.Fatal("前端资源加载失败:", err)
	}

	http.Handle("/", http.FileServer(http.FS(distFS)))
}

func publicIPv4() string {
	if ip := fetchPublicIPv4(); ip != "" {
		return ip
	}
	if ip := localIPv4(); ip != "" {
		return ip
	}
	return "127.0.0.1"
}

func fetchPublicIPv4() string {
	client := http.Client{Timeout: 2 * time.Second}
	endpoints := []string{
		"https://api.ipify.org",
		"https://ifconfig.me/ip",
	}

	for _, endpoint := range endpoints {
		resp, err := client.Get(endpoint)
		if err != nil {
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil || resp.StatusCode != http.StatusOK {
			continue
		}

		ip := strings.TrimSpace(string(body))
		parsed := net.ParseIP(ip)
		if parsed != nil && parsed.To4() != nil {
			return ip
		}
	}

	return ""
}

func localIPv4() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}

	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP.IsLoopback() {
			continue
		}
		if ip := ipNet.IP.To4(); ip != nil {
			return ip.String()
		}
	}

	return ""
}

// 处理 Agent 端的 WebSocket 连接
func handleAgentWS(w http.ResponseWriter, r *http.Request) {
	agentID := r.URL.Query().Get("id")
	if agentID == "" {
		http.Error(w, "缺少 Agent ID", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// 注册 Agent
	agentMutex.Lock()
	agentConns[agentID] = conn
	agentMutex.Unlock()
	fmt.Printf("[Master] Agent 上线: %s\n", agentID)

	// 监听 Agent 发来的消息 (监控数据或命令执行日志)
	for {
		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var msg Message
		if err := json.Unmarshal(msgBytes, &msg); err == nil {
			if msg.Type == "stat" {
				// 收到状态数据，广播给所有打开了 Web 页面的前端大屏
				broadcastToWeb(msgBytes)
			} else if msg.Type == "log" {
				// 收到命令执行结果，回传给 TG 或 Web
				logStr := fmt.Sprintf("[%s] %s", msg.AgentID, msg.Data)
				fmt.Println(logStr)
				broadcastToWeb(msgBytes)
				// 如果配置了TG，则发送给TG控制者 (此处为广播给TG，实际生产可针对特定 ChatID 回复)
				// 注意：这里为了自测方便不强制校验 ChatID，请在生产环境增加权限验证
			}
		}
	}

	// Agent 离线清理
	agentMutex.Lock()
	delete(agentConns, agentID)
	agentMutex.Unlock()
	fmt.Printf("[Master] Agent 离线: %s\n", agentID)
}

// 处理 Web 前端的 WebSocket 连接
func handleWebWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	webMutex.Lock()
	webConns[conn] = true
	webMutex.Unlock()

	// 监听前端大屏发来的操作指令
	for {
		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			break
		}

		var msg Message
		if err := json.Unmarshal(msgBytes, &msg); err != nil {
			continue
		}
		if msg.Type == "cmd" {
			agentMutex.RLock()
			agentConn, exists := agentConns[msg.AgentID]
			agentMutex.RUnlock()
			if exists {
				agentConn.WriteMessage(websocket.TextMessage, msgBytes)
			}
		}
	}

	webMutex.Lock()
	delete(webConns, conn)
	webMutex.Unlock()
}

// 将监控数据广播给所有在线的 Web 页面
func broadcastToWeb(message []byte) {
	webMutex.RLock()
	defer webMutex.RUnlock()
	for conn := range webConns {
		conn.WriteMessage(websocket.TextMessage, message)
	}
}

// 初始化并启动 TG Bot
func initTGBot(token string) {
	var err error
	bot, err = tele.NewBot(tele.Settings{
		Token:  token,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	})
	if err != nil {
		log.Fatal("[TG Bot] 初始化失败:", err)
		return
	}

	// 监听指令: /cmd <agent_id> <shell_command>
	bot.Handle("/cmd", func(c tele.Context) error {
		args := c.Args()
		if len(args) < 2 {
			return c.Send("格式错误，请使用: /cmd <agent_id> <命令>")
		}

		targetAgent := args[0]
		cmdStr := strings.Join(args[1:], " ")

		agentMutex.RLock()
		agentConn, exists := agentConns[targetAgent]
		agentMutex.RUnlock()

		if !exists {
			return c.Send(fmt.Sprintf("❌ Agent [%s] 不在线", targetAgent))
		}

		// 封包并下发命令给 Agent
		req := Message{Type: "cmd", AgentID: targetAgent, Data: cmdStr}
		reqBytes, _ := json.Marshal(req)

		err := agentConn.WriteMessage(websocket.TextMessage, reqBytes)
		if err != nil {
			return c.Send("❌ 命令发送失败")
		}

		return c.Send(fmt.Sprintf("✅ 命令已下发至 [%s]: %s\n(执行结果将在控制台打印)", targetAgent, cmdStr))
	})

	go func() {
		fmt.Println("[TG Bot] 服务已启动，正在监听指令...")
		bot.Start()
	}()
}
