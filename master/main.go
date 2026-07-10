package main

import (
	"database/sql"
	"embed"
	"encoding/json"
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
	if masterPort == "" { masterPort = "8080" }
	masterPath = os.Getenv("MASTER_PATH")
	if masterPath != "" && !strings.HasPrefix(masterPath, "/") { masterPath = "/" + masterPath }
	masterPath = strings.TrimSuffix(masterPath, "/")
	operatorUser = os.Getenv("OPERATOR_USERNAME")
	if operatorUser == "" { operatorUser = "admin" }
	operatorPass = os.Getenv("OPERATOR_PASSWORD")
	if operatorPass == "" { operatorPass = genShortPassword() }
	if masterPath == "" { masterPath = "/" + genSecret()[:16] }

	if err := openDB("ops-panel.db"); err != nil {
		log.Fatal("打开数据库失败:", err)
	}
	migrateFromJSON(".")
	if err := loadTrafficFromDB(); err != nil {
		log.Println("[警告] 加载流量历史失败:", err)
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
	go trafficAlertLoop()
	go persistTrafficLoop()
	go alertLoop()

	totpEnabled := os.Getenv("OPERATOR_TOTP_SECRET") != ""
	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("       Ops Panel Master 已启动")
	fmt.Println("========================================")
	fmt.Printf("  端口:   %s\n", masterPort)
	fmt.Printf("  路径:   %s\n", masterPath)
	fmt.Printf("  用户名: %s\n", operatorUser)
	fmt.Printf("  密码:   %s\n", operatorPass)
	if totpEnabled { fmt.Println("  双因素: 已启用 (Google Authenticator)") }
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
	if err := loadMonitors(monitorsFile); err != nil {
		log.Println("[警告] 加载监控配置失败:", err)
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

	http.HandleFunc(masterPath+"/agent-install.sh", handleAgentInstall)

	http.HandleFunc(masterPath+"/api/enroll", handleEnroll)
	http.HandleFunc(masterPath+"/api/login", handleLogin)
	http.HandleFunc(masterPath+"/api/refresh", handleRefresh)
	http.HandleFunc(masterPath+"/api/groups", handleGroups)
	http.HandleFunc(masterPath+"/api/agents", handleAgents)
	http.HandleFunc(masterPath+"/api/preferences", handlePreferences)
	http.HandleFunc(masterPath+"/api/alerts", handleAlerts)
	http.HandleFunc(masterPath+"/api/traffic", handleTraffic)
	http.HandleFunc(masterPath+"/api/history", handleHistory)
	http.HandleFunc(masterPath+"/api/monitors", handleMonitors)
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
	if err != nil { return }
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

	// 按公网 IP 异步识别国家（仅当尚未设置）
	maybeResolveCountry(agentID, r.RemoteAddr, r.Header.Get("X-Forwarded-For"))

	sconn.Write(mustJSON(Message{Type: "config", AgentID: agentID, Data: fmt.Sprintf("%d", rec.Prefs.Interval)}))
	pushMonitorConfig(agentID) // 下发该节点的探测任务

	for {
		_, msgBytes, err := conn.ReadMessage()
		if err != nil { break }
		var msg Message
		if err := json.Unmarshal(msgBytes, &msg); err != nil { continue }
		switch msg.Type {
		case "stat":
			if !rateAllow(agentID, time.Duration(rec.Prefs.Interval)*time.Second/2) { continue }
			ingestStat(agentID, rec, msg.Data)
			broadcastToWeb(msgBytes)
		case "probe_result":
			ingestProbeResult(agentID, msg.Data)
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
	statMu.Lock(); defer statMu.Unlock()
	now := time.Now()
	if last, ok := lastStatTime[agentID]; ok && now.Sub(last) < minGap { return false }
	lastStatTime[agentID] = now; return true
}

// ingestStat 解析一次 stat 负载，捕获 agent 版本、记录流量与历史时序。
func ingestStat(agentID string, rec *AgentRecord, data string) {
	var s struct {
		CPU      float64 `json:"cpu"`
		Mem      float64 `json:"mem"`
		Disk     float64 `json:"disk"`
		NetSent  float64 `json:"net_sent"`
		NetRecv  float64 `json:"net_recv"`
		AgentVer string  `json:"agent_ver"`
	}
	if json.Unmarshal([]byte(data), &s) != nil { return }
	if s.AgentVer != "" && rec.AgentVer != s.AgentVer {
		agentsMu.Lock(); rec.AgentVer = s.AgentVer; agentsMu.Unlock()
	}
	if rec.Prefs.TrackTraffic { RecordTraffic(agentID, s.NetSent, s.NetRecv) }
	recordHistory(agentID, s.CPU, s.Mem, s.Disk, s.NetSent, s.NetRecv)
	metricMu.Lock()
	latestStat[agentID] = statSample{CPU: s.CPU, Mem: s.Mem, Disk: s.Disk}
	lastSeen[agentID] = time.Now()
	metricMu.Unlock()
}

// ============ Viewer ============

func handleViewerWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil { return }
	sconn := &SafeConn{Conn: conn, role: RoleViewer}
	defer sconn.Close()
	webMutex.Lock(); webConns[sconn] = true; webMutex.Unlock()
	for { if _, _, err := conn.ReadMessage(); err != nil { break } }
	webMutex.Lock(); delete(webConns, sconn); webMutex.Unlock()
}

// ============ Operator ============

func handleOperatorWS(w http.ResponseWriter, r *http.Request) {
	if !authenticateOperatorWS(r) {
		http.Error(w, "未授权：需提供有效 access_token", http.StatusUnauthorized)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil { return }
	sconn := &SafeConn{Conn: conn, role: RoleOperator, authOk: true}
	defer sconn.Close()
	operMutex.Lock(); operConns[sconn] = true; operMutex.Unlock()
	for {
		_, msgBytes, err := conn.ReadMessage()
		if err != nil { break }
		var msg Message
		if err := json.Unmarshal(msgBytes, &msg); err != nil { continue }
		if msg.Type == "cmd" { dispatchCommand(msg.AgentID, msg.Data) }
	}
	operMutex.Lock(); delete(operConns, sconn); operMutex.Unlock()
}

// ============ Command Dispatch ============

// SAFETY: SafeConn.Write 代替了旧的 agentConn.WriteMessage
func dispatchCommand(agentID, cmdStr string) {
	agentsMu.RLock()
	rec, ok := agents[agentID]
	sconn, online := agentConns[agentID]
	agentsMu.RUnlock()
	if !ok || !online { return }
	nonce := time.Now().Unix()
	sig := signCommand(rec.Secret, agentID, cmdStr, nonce)
	sconn.Write(mustJSON(Message{Type: "cmd", AgentID: agentID, Data: cmdStr, Nonce: nonce, Sig: sig}))
}

func broadcastToWeb(message []byte) {
	webMutex.RLock(); defer webMutex.RUnlock()
	for conn := range webConns { conn.Write(message) }
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
	masterAddr := fmt.Sprintf("http://%s:%s%s", publicIPv4(), masterPort, masterPath)
	rec, installCmd, err := enrollAgent(req, masterAddr)
	if err != nil {
		http.Error(w, "enroll failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"agent_id":    rec.AgentID,
		"secret":      rec.Secret,
		"install_cmd": installCmd,
	})
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
			http.Error(w, "invalid body", http.StatusBadRequest); return
		}
		if err := AddGroup(req.Name); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest); return
		}
		w.WriteHeader(http.StatusOK)
	case http.MethodPut:
		var req struct{ OldName string `json:"old_name"`; NewName string `json:"new_name"` }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest); return
		}
		if err := RenameGroup(req.OldName, req.NewName); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest); return
		}
		w.WriteHeader(http.StatusOK)
	case http.MethodDelete:
		var req struct{ Name string `json:"name"` }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest); return
		}
		RemoveGroup(req.Name); w.WriteHeader(http.StatusOK)
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
	case http.MethodPut:
		if !operatorAuthorized(r) {
			http.Error(w, "未授权", http.StatusUnauthorized); return
		}
		var req struct {
			AgentID string           `json:"agent_id"`
			Name    string           `json:"name"`
			Prefs   AgentPreferences `json:"prefs"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest); return
		}
		if err := UpdateAgentMeta(req.AgentID, req.Name, req.Prefs); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest); return
		}
		// 刷新频率可能变了，若在线则下发新 config
		agentMutex.RLock(); sconn, online := agentConns[req.AgentID]; agentMutex.RUnlock()
		if online { sconn.Write(mustJSON(Message{Type: "config", AgentID: req.AgentID, Data: fmt.Sprintf("%d", req.Prefs.Interval)})) }
		w.WriteHeader(http.StatusOK)
	case http.MethodDelete:
		if !operatorAuthorized(r) {
			http.Error(w, "未授权", http.StatusUnauthorized); return
		}
		var req struct{ AgentID string `json:"agent_id"` }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest); return
		}
		DeleteAgent(req.AgentID); w.WriteHeader(http.StatusOK)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// ============ Preferences API ============

func handlePreferences(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed); return
	}
	var req struct {
		AgentID string           `json:"agent_id"`
		Prefs   AgentPreferences `json:"prefs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest); return
	}
	if err := SetAgentPrefs(req.AgentID, req.Prefs); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest); return
	}
	w.WriteHeader(http.StatusOK)
}

// ============ Alerts API ============

var (
	alertsMu    sync.RWMutex
	alertConfig = AlertConfig{CPUPercent: 80, MemPercent: 80, DiskPercent: 80, OfflineMinutes: 5, Enabled: true}
	alertsFile  = "alerts.json"
)

type AlertConfig struct {
	CPUPercent     int  `json:"cpu_percent"`
	MemPercent     int  `json:"mem_percent"`
	DiskPercent    int  `json:"disk_percent"`
	OfflineMinutes int  `json:"offline_minutes"`
	Enabled        bool `json:"enabled"`
}

func loadAlerts(_ string) error {
	var dj string
	err := db.QueryRow("SELECT data FROM alerts WHERE id=1").Scan(&dj)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(dj), &alertConfig)
}

func saveAlerts(_ string) error {
	dj, err := json.Marshal(alertConfig)
	if err != nil {
		return err
	}
	_, err = db.Exec("INSERT INTO alerts(id,data) VALUES(1,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data", string(dj))
	return err
}

func handleAlerts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		alertsMu.RLock(); defer alertsMu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(alertConfig)
	case http.MethodPost:
		alertsMu.Lock(); defer alertsMu.Unlock()
		var cfg AlertConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest); return
		}
		if cfg.CPUPercent < 1 || cfg.CPUPercent > 100 { cfg.CPUPercent = 80 }
		if cfg.MemPercent < 1 || cfg.MemPercent > 100 { cfg.MemPercent = 80 }
		if cfg.DiskPercent < 1 || cfg.DiskPercent > 100 { cfg.DiskPercent = 80 }
		if cfg.OfflineMinutes < 1 || cfg.OfflineMinutes > 60 { cfg.OfflineMinutes = 5 }
		alertConfig = cfg; _ = saveAlerts(alertsFile); w.WriteHeader(http.StatusOK)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// ============ Traffic API ============

type TrafficDay struct{ Date string `json:"date"`; Sent int64 `json:"sent"`; Recv int64 `json:"recv"` }

type TrafficStats struct {
	AgentID   string `json:"agent_id"`
	Group     string `json:"group"`
	Name      string `json:"name"`
	Today     int64  `json:"today"`       // 今日合计字节
	TodaySent int64  `json:"today_sent"`  // 今日出站（上行）
	TodayRecv int64  `json:"today_recv"`  // 今日入站（下行）
	ThisMonth int64  `json:"this_month"`  // 本自然月合计字节
	MonthSent int64  `json:"month_sent"`  // 本月出站
	MonthRecv int64  `json:"month_recv"`  // 本月入站
	CycleUsed int64  `json:"cycle_used"`  // = 本自然月合计（保留字段名兼容前端）
	Quota     int64  `json:"quota"`       // 配额字节，0=不限
}

var (
	trafficMu     sync.RWMutex
	traffic       = make(map[string]*TrafficDay)
	lastTrafficAt = make(map[string]time.Time)
)

// RecordTraffic 对速率（字节/秒）按实际区间积分为字节量并按天累计。
func RecordTraffic(agentID string, sentRate, recvRate float64) {
	trafficMu.Lock(); defer trafficMu.Unlock()
	now := time.Now()
	last, ok := lastTrafficAt[agentID]
	lastTrafficAt[agentID] = now
	if !ok { return } // 首个样本没有区间，跳过
	elapsed := now.Sub(last).Seconds()
	if elapsed <= 0 || elapsed > 300 { return } // 断连/异常区间丢弃
	key := agentID + "|" + now.Format("2006-01-02")
	day := traffic[key]
	if day == nil { day = &TrafficDay{Date: now.Format("2006-01-02")}; traffic[key] = day }
	day.Sent += int64(sentRate * elapsed)
	day.Recv += int64(recvRate * elapsed)
	for k := range traffic {
		parts := strings.SplitN(k, "|", 2)
		if len(parts) != 2 { continue }
		d, err := time.Parse("2006-01-02", parts[1])
		if err == nil && time.Since(d) > 40*24*time.Hour { delete(traffic, k) }
	}
}

func trafficStatsSnapshot() []*TrafficStats {
	agentsMu.RLock()
	recs := make([]*AgentRecord, 0, len(agents))
	for _, a := range agents { recs = append(recs, a) }
	agentsMu.RUnlock()

	now := time.Now()
	today := now.Format("2006-01-02")
	monthPrefix := now.Format("2006-01")

	trafficMu.RLock(); defer trafficMu.RUnlock()
	out := make([]*TrafficStats, 0, len(recs))
	for _, rec := range recs {
		id := rec.AgentID
		var ts, tr, ms, mr int64
		for k, d := range traffic {
			if !strings.HasPrefix(k, id+"|") { continue }
			if d.Date == today { ts += d.Sent; tr += d.Recv }
			if strings.HasPrefix(d.Date, monthPrefix) { ms += d.Sent; mr += d.Recv }
		}
		out = append(out, &TrafficStats{
			AgentID: id, Group: rec.Prefs.Group, Name: rec.Name,
			Today: ts + tr, TodaySent: ts, TodayRecv: tr,
			ThisMonth: ms + mr, MonthSent: ms, MonthRecv: mr,
			CycleUsed: ms + mr, Quota: rec.Prefs.TrafficQuota,
		})
	}
	return out
}

func handleTraffic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed); return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(trafficStatsSnapshot())
}

// persistTrafficOnce 把内存 traffic map UPSERT 落库（关机不清空）。
func persistTrafficOnce() error {
	trafficMu.RLock()
	type row struct {
		id, date   string
		sent, recv int64
	}
	rows := make([]row, 0, len(traffic))
	for k, d := range traffic {
		parts := strings.SplitN(k, "|", 2)
		if len(parts) != 2 {
			continue
		}
		rows = append(rows, row{parts[0], d.Date, d.Sent, d.Recv})
	}
	trafficMu.RUnlock()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare("INSERT INTO traffic_daily(agent_id,date,sent,recv) VALUES(?,?,?,?) ON CONFLICT(agent_id,date) DO UPDATE SET sent=excluded.sent, recv=excluded.recv")
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, r := range rows {
		if _, err := stmt.Exec(r.id, r.date, r.sent, r.recv); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// loadTrafficFromDB 启动时把近 40 天流量读回内存 map。
func loadTrafficFromDB() error {
	cutoff := time.Now().AddDate(0, 0, -40).Format("2006-01-02")
	rows, err := db.Query("SELECT agent_id,date,sent,recv FROM traffic_daily WHERE date >= ?", cutoff)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, date string
		var sent, recv int64
		if err := rows.Scan(&id, &date, &sent, &recv); err != nil {
			return err
		}
		traffic[id+"|"+date] = &TrafficDay{Date: date, Sent: sent, Recv: recv}
	}
	return rows.Err()
}

func persistTrafficLoop() {
	for range time.Tick(60 * time.Second) {
		if err := persistTrafficOnce(); err != nil {
			log.Println("[流量落库]", err)
		}
		pruneOldTraffic()
	}
}

func pruneOldTraffic() {
	cutoff := time.Now().AddDate(0, 0, -40).Format("2006-01-02")
	_, _ = db.Exec("DELETE FROM traffic_daily WHERE date < ?", cutoff)
}

// ── 流量超额告警（每周期每节点仅告警一次）──

var (
	trafficAlertMu sync.Mutex
	trafficAlerted = make(map[string]string) // agentID -> 已告警的周期 key
)

func trafficAlertLoop() {
	for range time.Tick(5 * time.Minute) {
		checkTrafficQuota()
	}
}

func checkTrafficQuota() {
	now := time.Now()
	for _, s := range trafficStatsSnapshot() {
		if s.Quota <= 0 || s.CycleUsed < s.Quota { continue }
		cycleKey := now.Format("2006-01")
		trafficAlertMu.Lock()
		already := trafficAlerted[s.AgentID] == cycleKey
		if !already { trafficAlerted[s.AgentID] = cycleKey }
		trafficAlertMu.Unlock()
		if already { continue }
		name := s.Name; if name == "" { name = s.AgentID }
		sendTGAlert(fmt.Sprintf("⚠️ 流量超额：%s 本月已用 %s / 配额 %s", name, humanBytes(s.CycleUsed), humanBytes(s.Quota)))
	}
}

func sendTGAlert(text string) {
	if bot == nil { return }
	for _, s := range strings.Split(os.Getenv("TG_ADMIN_IDS"), ",") {
		s = strings.TrimSpace(s)
		if id, err := strconv.ParseInt(s, 10, 64); err == nil {
			_, _ = bot.Send(tele.ChatID(id), text)
		}
	}
}

func humanBytes(b int64) string {
	f := float64(b)
	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	i := 0
	for f >= 1024 && i < len(units)-1 { f /= 1024; i++ }
	return fmt.Sprintf("%.2f %s", f, units[i])
}

func loadAlertsEarly() {
	if err := loadAlerts(alertsFile); err != nil {
		log.Println("[警告] 加载告警配置失败:", err)
	}
}

// ============ Utils ============

func mustJSON(v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil { return []byte("{}") }; return b
}

func publicIPv4() string {
	if ip := fetchPublicIPv4(); ip != "" { return ip }
	if ip := localIPv4(); ip != "" { return ip }; return "127.0.0.1"
}

func fetchPublicIPv4() string {
	client := http.Client{Timeout: 2 * time.Second}
	for _, ep := range []string{"https://api.ipify.org", "https://ifconfig.me/ip"} {
		resp, err := client.Get(ep)
		if err != nil { continue }
		body, err := io.ReadAll(resp.Body); resp.Body.Close()
		if err != nil || resp.StatusCode != http.StatusOK { continue }
		ip := net.ParseIP(strings.TrimSpace(string(body)))
		if ip != nil && ip.To4() != nil { return ip.String() }
	}
	return ""
}

func localIPv4() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil { return "" }
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP.IsLoopback() { continue }
		if ip := ipNet.IP.To4(); ip != nil { return ip.String() }
	}
	return ""
}

// ============ TG Bot ============

func initTGBot(token string) {
	var err error
	bot, err = tele.NewBot(tele.Settings{Token: token, Poller: &tele.LongPoller{Timeout: 10 * time.Second}})
	if err != nil { log.Fatal("[TG Bot] 初始化失败:", err); return }

	bot.Handle("/cmd", func(c tele.Context) error {
		if !tgAdminAllowed(c.Chat().ID) { return c.Send("无权限") }
		args := c.Args()
		if len(args) < 2 { return c.Send("格式错误，请使用: /cmd <agent_id> <命令>") }
		dispatchCommand(args[0], strings.Join(args[1:], " "))
		return c.Send(fmt.Sprintf("命令已下发至 [%s]: %s", args[0], strings.Join(args[1:], " ")))
	})
	go func() { fmt.Println("[TG Bot] 服务已启动，正在监听指令..."); bot.Start() }()
}

func tgAdminAllowed(chatID int64) bool {
	raw := os.Getenv("TG_ADMIN_IDS")
	if raw == "" { return false }
	for _, s := range strings.Split(raw, ",") {
		s = strings.TrimSpace(s)
		if id, err := strconv.ParseInt(s, 10, 64); err == nil && id == chatID { return true }
	}
	return false
}
