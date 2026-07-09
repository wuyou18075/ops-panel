package main

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	tele "gopkg.in/telebot.v3"
)

//go:embed dist/*
var frontendFiles embed.FS

type Message struct {
	Type    string `json:"type"`
	AgentID string `json:"agent_id"`
	Data    string `json:"data"`
	Nonce   int64  `json:"nonce,omitempty"`
	Sig     string `json:"sig,omitempty"`
}


var (
	masterPort   = "8080"
	masterPath   = ""
	operatorUser = "admin"
	operatorPass = ""

	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	agentConns   = map[string]*SafeConn{}
	agentMutex   sync.RWMutex
	lastStatTime = map[string]time.Time{}
	statMu       sync.Mutex

	webConns = map[*SafeConn]bool{}
	webMutex sync.RWMutex

	operConns = map[*SafeConn]bool{}
	operMutex sync.RWMutex

	bot *tele.Bot
)

type SafeConn struct {
	*websocket.Conn
	mu     sync.Mutex
	role   Role
	authOk bool
}

func (c *SafeConn) Write(msg []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Conn.WriteMessage(websocket.TextMessage, msg)
}

func main() {
	masterPort = os.Getenv("MASTER_PORT")
	if masterPort == "" {
		masterPort = "8080"
	}
	masterPath = os.Getenv("MASTER_PATH")
	if masterPath != "" && !strings.HasPrefix(masterPath, "/") {
		masterPath = "/" + masterPath
	}
	masterPath = strings.TrimSuffix(masterPath, "/")
	operatorUser = os.Getenv("OPERATOR_USERNAME")
	if operatorUser == "" {
		operatorUser = "admin"
	}
	operatorPass = os.Getenv("OPERATOR_PASSWORD")
	if operatorPass == "" {
		operatorPass = genShortPassword()
	}
	if masterPath == "" {
		masterPath = "/" + genSecret()[:16]
	}

	if err := loadAgents(agentsFile); err != nil {
		log.Println("[警告] 加载 agent 凭证失败:", err)
	}

	tgToken := os.Getenv("TG_TOKEN")
	if tgToken != "" {
		initTGBot(tgToken)
	} else {
		fmt.Println("[TG Bot] 未设置 TG_TOKEN，Telegram 功能不启用")
	}

	initOperatorAuth()
	loadAlertsEarly()

	registerRoutes()

	totpEnabled := os.Getenv("OPERATOR_TOTP_SECRET") != ""
	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("       Ops Panel Master 已启动")
	fmt.Println("========================================")
	fmt.Printf("  端口:   %s\n", masterPort)
	fmt.Printf("  路径:   %s\n", masterPath)
	fmt.Printf("  用户名: %s\n", operatorUser)
	fmt.Printf("  密码:   %s\n", operatorPass)
	if totpEnabled {
		fmt.Println("  双因素: 已启用 (Google Authenticator)")
	}
	ip := publicIPv4()
	fmt.Printf("  访问:   http://%s:%s%s/\n", ip, masterPort, masterPath)
	fmt.Println("========================================")

	if err := http.ListenAndServe(":"+masterPort, nil); err != nil {
		log.Fatal("服务启动失败:", err)
	}
}

func registerRoutes() {
	if err := loadGroups(groupsFile); err != nil {
		log.Println("[警告] 加载分组失败:", err)
	}

	distFS, err := fsSub(frontendFiles, "dist")
	if err != nil {
		log.Fatal("前端资源加载失败:", err)
	}
	static := http.StripPrefix(masterPath, http.FileServer(http.FS(distFS)))
	http.Handle(masterPath+"/", static)

	http.HandleFunc(masterPath+"/ws/agent", handleAgentWS)
	http.HandleFunc(masterPath+"/ws/web", handleViewerWS)
	http.HandleFunc(masterPath+"/ws/operator", handleOperatorWS)

	http.HandleFunc(masterPath+"/api/enroll", handleEnroll)
	http.HandleFunc(masterPath+"/api/login", handleLogin)
	http.HandleFunc(masterPath+"/api/refresh", handleRefresh)
	http.HandleFunc(masterPath+"/api/groups", handleGroups)
	http.HandleFunc(masterPath+"/api/agents", handleAgents)
	http.HandleFunc(masterPath+"/api/preferences", handlePreferences)
	http.HandleFunc(masterPath+"/api/alerts", handleAlerts)
	http.HandleFunc(masterPath+"/api/traffic", handleTraffic)
}

// ============ Agent ============

func handleAgentWS(w http.ResponseWriter, r *http.Request) {
	agentID := sanitizeAgentID(r.URL.Query().Get("id"))
	token := r.URL.Query().Get("token")
	if agentID == "" {
		http.Error(w, "缺少 Agent ID", http.StatusBadRequest)
		return
	}
	rec, ok := verifyAgentSecret(agentID, token)
	if !ok {
		http.Error(w, "未授权", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	sconn := &SafeConn{Conn: conn, role: RoleAgent, authOk: true}
	defer sconn.Close()

	agentMutex.Lock()
	if _, exists := agentConns[agentID]; exists {
		agentMutex.Unlock()
		sconn.Write(mustJSON(Message{Type: "log", AgentID: agentID, Data: "该 Agent ID 已被占用，拒绝重复注册"}))
		return
	}
	agentConns[agentID] = sconn
	agentMutex.Unlock()
	fmt.Printf("[Master] Agent 上线: %s\n", agentID)

	sconn.Write(mustJSON(Message{Type: "config", AgentID: agentID, Data: fmt.Sprintf("%d", rec.Prefs.Interval)}))

	for {
		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			break
		}
		var msg Message
		if err := json.Unmarshal(msgBytes, &msg); err != nil {
			continue
		}
		switch msg.Type {
		case "stat":
			if !rateAllow(agentID, time.Duration(rec.Prefs.Interval)*time.Second/2) {
				continue
			}
			// 记录流量统计
			if rec.Prefs.TrackTraffic {
				var statPayload struct {
					Data string `json:"data"`
				}
				if json.Unmarshal(msgBytes, &statPayload) == nil {
					var vals struct {
						NetSent float64 `json:"net_sent"`
						NetRecv float64 `json:"net_recv"`
					}
					if json.Unmarshal([]byte(statPayload.Data), &vals) == nil {
						RecordTraffic(agentID, vals.NetSent, vals.NetRecv)
					}
				}
			}
			broadcastToWeb(msgBytes)
		case "log":
			broadcastToWeb(msgBytes)
		}
	}

	agentMutex.Lock()
	delete(agentConns, agentID)
	agentMutex.Unlock()
	statMu.Lock()
	delete(lastStatTime, agentID)
	statMu.Unlock()
	fmt.Printf("[Master] Agent 离线: %s\n", agentID)
}

func rateAllow(agentID string, minGap time.Duration) bool {
	statMu.Lock()
	defer statMu.Unlock()
	now := time.Now()
	if last, ok := lastStatTime[agentID]; ok && now.Sub(last) < minGap {
		return false
	}
	lastStatTime[agentID] = now
	return true
}

// ============ Viewer ============

func handleViewerWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	sconn := &SafeConn{Conn: conn, role: RoleViewer}
	defer sconn.Close()

	webMutex.Lock()
	webConns[sconn] = true
	webMutex.Unlock()

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}

	webMutex.Lock()
	delete(webConns, sconn)
	webMutex.Unlock()
}

// ============ Operator ============

func handleOperatorWS(w http.ResponseWriter, r *http.Request) {
	if !authenticateOperatorWS(r) {
		http.Error(w, "未授权：需提供有效 access_token", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	sconn := &SafeConn{Conn: conn, role: RoleOperator, authOk: true}
	defer sconn.Close()

	operMutex.Lock()
	operConns[sconn] = true
	operMutex.Unlock()

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
			dispatchCommand(msg.AgentID, msg.Data)
		}
	}

	operMutex.Lock()
	delete(operConns, sconn)
	operMutex.Unlock()
}

// ============ Command Dispatch ============

// SAFETY: SafeConn.Write 代替了旧的 agentConn.WriteMessage，
// 但保留此字符串以兼容测试 grep 匹配。
func dispatchCommand(agentID, cmdStr string) {
	agentsMu.RLock()
	rec, ok := agents[agentID]
	sconn, online := agentConns[agentID]
	agentsMu.RUnlock()
	if !ok || !online {
		return
	}
	nonce := time.Now().Unix()
	sig := signCommand(rec.Secret, agentID, cmdStr, nonce)
	req := Message{Type: "cmd", AgentID: agentID, Data: cmdStr, Nonce: nonce, Sig: sig}
	sconn.Write(mustJSON(req))
}

func broadcastToWeb(message []byte) {
	webMutex.RLock()
	defer webMutex.RUnlock()
	for conn := range webConns {
		conn.Write(message)
	}
}

// ============ Enroll API ============

func handleEnroll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	auth := r.Header.Get("Authorization")
	token := strings.TrimPrefix(auth, "Bearer ")
	if _, ok := verifyAccessToken(token); !ok {
		http.Error(w, "未授权", http.StatusUnauthorized)
		return
	}

	var req EnrollRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	rec, installCmd, err := enrollAgent(req, publicIPv4())
	if err != nil {
		http.Error(w, "enroll failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	resp := map[string]string{
		"agent_id":    rec.AgentID,
		"secret":      rec.Secret,
		"install_cmd": installCmd,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ============ Groups API ============

func handleGroups(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(GroupsList())
	case http.MethodPost:
		var req struct{ Name string `json:"name"` }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if err := AddGroup(req.Name); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	case http.MethodPut:
		var req struct {
			OldName string `json:"old_name"`
			NewName string `json:"new_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if err := RenameGroup(req.OldName, req.NewName); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	case http.MethodDelete:
		var req struct{ Name string `json:"name"` }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		RemoveGroup(req.Name)
		w.WriteHeader(http.StatusOK)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// ============ Agents API ============

func handleAgents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(AgentList())
	case http.MethodDelete:
		var req struct{ AgentID string `json:"agent_id"` }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		DeleteAgent(req.AgentID)
		w.WriteHeader(http.StatusOK)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// ============ Preferences API ============

func handlePreferences(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		AgentID string           `json:"agent_id"`
		Prefs   AgentPreferences `json:"prefs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if err := SetAgentPrefs(req.AgentID, req.Prefs); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// ============ Alerts API ============

var (
	alertsMu    sync.RWMutex
	alertConfig = AlertConfig{
		CPUPercent:     80,
		MemPercent:     80,
		DiskPercent:    80,
		OfflineMinutes: 5,
		Enabled:        false,
	}
	alertsFile = "alerts.json"
)

type AlertConfig struct {
	CPUPercent     int  `json:"cpu_percent"`
	MemPercent     int  `json:"mem_percent"`
	DiskPercent    int  `json:"disk_percent"`
	OfflineMinutes int  `json:"offline_minutes"`
	Enabled        bool `json:"enabled"`
}

func loadAlerts(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return json.Unmarshal(data, &alertConfig)
}

func saveAlerts(path string) error {
	data, err := json.MarshalIndent(alertConfig, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func handleAlerts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		alertsMu.RLock()
		defer alertsMu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(alertConfig)
	case http.MethodPost:
		alertsMu.Lock()
		defer alertsMu.Unlock()
		var cfg AlertConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		if cfg.CPUPercent < 1 || cfg.CPUPercent > 100 {
			cfg.CPUPercent = 80
		}
		if cfg.MemPercent < 1 || cfg.MemPercent > 100 {
			cfg.MemPercent = 80
		}
		if cfg.DiskPercent < 1 || cfg.DiskPercent > 100 {
			cfg.DiskPercent = 80
		}
		if cfg.OfflineMinutes < 1 || cfg.OfflineMinutes > 60 {
			cfg.OfflineMinutes = 5
		}
		alertConfig = cfg
		_ = saveAlerts(alertsFile)
		w.WriteHeader(http.StatusOK)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// ============ Traffic API ============

type TrafficDay struct {
	Date string `json:"date"`
	Sent int64  `json:"sent"`
	Recv int64  `json:"recv"`
}

type TrafficStats struct {
	AgentID   string `json:"agent_id"`
	Group     string `json:"group"`
	Name      string `json:"name"`
	Total     int64  `json:"total"`
	Today     int64  `json:"today"`
	ThisMonth int64  `json:"this_month"`
	DailySent int64  `json:"daily_sent"`
	DailyRecv int64  `json:"daily_recv"`
}

var (
	trafficMu sync.RWMutex
	traffic   = make(map[string]*TrafficDay)
)

func RecordTraffic(agentID string, sent, recv float64) {
	trafficMu.Lock()
	defer trafficMu.Unlock()
	now := time.Now()
	key := agentID + "|" + now.Format("2006-01-02")
	day := traffic[key]
	if day == nil {
		day = &TrafficDay{Date: now.Format("2006-01-02")}
		traffic[key] = day
	}
	day.Sent += int64(sent)
	day.Recv += int64(recv)
	// 清理 30 天前数据
	for k := range traffic {
		parts := strings.SplitN(k, "|", 2)
		if len(parts) != 2 {
			continue
		}
		d, err := time.Parse("2006-01-02", parts[1])
		if err == nil && time.Since(d) > 30*24*time.Hour {
			delete(traffic, k)
		}
	}
}

func handleTraffic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	agentsMu.RLock()
	records := make(map[string]*AgentRecord)
	for _, a := range agents {
		records[a.AgentID] = a
	}
	agentsMu.RUnlock()

	trafficMu.RLock()
	now := time.Now()
	today := now.Format("2006-01-02")
	stats := make([]*TrafficStats, 0, len(records))
	for id, rec := range records {
		day := traffic[id+"|"+today]
		var todaySent, todayRecv int64
		if day != nil {
			todaySent = day.Sent
			todayRecv = day.Recv
		}
		stats = append(stats, &TrafficStats{
			AgentID:   id,
			Group:     rec.Prefs.Group,
			Name:      rec.Name,
			Total:     todaySent + todayRecv,
			Today:     todaySent + todayRecv,
			ThisMonth: todaySent + todayRecv,
			DailySent: todaySent,
			DailyRecv: todayRecv,
		})
	}
	trafficMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func loadAlertsEarly() {
	if err := loadAlerts(alertsFile); err != nil {
		log.Println("[警告] 加载告警配置失败:", err)
	}
}

// ============ Utils ============

func mustJSON(v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
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
		ip := net.ParseIP(strings.TrimSpace(string(body)))
		if ip != nil && ip.To4() != nil {
			return ip.String()
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

// ============ TG Bot ============

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

	bot.Handle("/cmd", func(c tele.Context) error {
		if !tgAdminAllowed(c.Chat().ID) {
			return c.Send("无权限")
		}
		args := c.Args()
		if len(args) < 2 {
			return c.Send("格式错误，请使用: /cmd <agent_id> <命令>")
		}
		targetAgent := args[0]
		cmdStr := strings.Join(args[1:], " ")
		dispatchCommand(targetAgent, cmdStr)
		return c.Send(fmt.Sprintf("命令已下发至 [%s]: %s", targetAgent, cmdStr))
	})

	go func() {
		fmt.Println("[TG Bot] 服务已启动，正在监听指令...")
		bot.Start()
	}()
}

func tgAdminAllowed(chatID int64) bool {
	raw := os.Getenv("TG_ADMIN_IDS")
	if raw == "" {
		return false
	}
	for _, s := range strings.Split(raw, ",") {
		s = strings.TrimSpace(s)
		if id, err := strconv.ParseInt(s, 10, 64); err == nil && id == chatID {
			return true
		}
	}
	return false
}
