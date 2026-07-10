package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

type Role string
const (RoleAgent Role = "agent"; RoleViewer Role = "viewer"; RoleOperator Role = "operator")

type AgentPreferences struct {
	EnableConsole bool   `json:"enable_console"`    // 开启控制台（默认关）

	Group        string `json:"group"`
	TrackTraffic bool   `json:"track_traffic"`
	DailyReport  bool   `json:"daily_report"`
	Interval     int    `json:"interval"`

	// ── 手动元数据（节点编辑表单录入）──
	PriceAmount     float64 `json:"price_amount,omitempty"`      // 价格金额
	PriceCurrency   string  `json:"price_currency,omitempty"`    // 货币：$/¥/€ 等
	BillingCycle    string  `json:"billing_cycle,omitempty"`     // 月/年/一次性/免费
	ExpiryDate      string  `json:"expiry_date,omitempty"`       // 到期日 YYYY-MM-DD（前端算剩余天数）
	Label           string  `json:"label,omitempty"`             // 计费标签（主用/长租/玩具…）
	TrafficQuota    int64   `json:"traffic_quota,omitempty"`     // 流量配额（字节，0=不限）
	TrafficResetDay int     `json:"traffic_reset_day,omitempty"` // 每月流量重置日 1-31
	CountryCode     string  `json:"country_code,omitempty"`      // ISO alpha-2（geo-IP 或手动覆盖）
	Favorite        bool    `json:"favorite,omitempty"`          // 收藏 ★
	SortOrder       int     `json:"sort_order,omitempty"`        // 手动排序
}

type AgentRecord struct {
	AgentID   string           `json:"agent_id"`
	Secret    string           `json:"secret"`
	Name      string           `json:"name"`
	AgentVer  string           `json:"agent_ver,omitempty"` // agent 上报的版本号
	Prefs     AgentPreferences `json:"prefs"`
	LastStat  map[string]any   `json:"-"`
	Connected bool             `json:"-"`
}

func NewAgentRecord() *AgentRecord {
	return &AgentRecord{
		AgentID: genAgentID(), Secret: genSecret(),
		Prefs: AgentPreferences{Group: "默认分组", TrackTraffic: true, DailyReport: false, Interval: 2},
		LastStat: make(map[string]any),
	}
}

const agentsFile = "agents.json"
var (agentsMu sync.RWMutex; agents = map[string]*AgentRecord{})
const (minInterval = 1; maxInterval = 60)

func genSecret() string { b := make([]byte, 32); rand.Read(b); return hex.EncodeToString(b) }
func genAgentID() string { return "node-" + genSecret()[:12] }
func genShortPassword() string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 8); rand.Read(b)
	for i := range b { b[i] = chars[int(b[i])%len(chars)] }; return string(b)
}

// loadAgents 从 SQLite 读入内存 map（path 参数保留以兼容调用点）。
func loadAgents(_ string) error {
	rows, err := db.Query("SELECT agent_id, secret, name, agent_ver, prefs FROM agents")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var a AgentRecord
		var prefsJSON string
		if err := rows.Scan(&a.AgentID, &a.Secret, &a.Name, &a.AgentVer, &prefsJSON); err != nil {
			return err
		}
		_ = json.Unmarshal([]byte(prefsJSON), &a.Prefs)
		if a.Prefs.Interval < minInterval || a.Prefs.Interval > maxInterval {
			a.Prefs.Interval = 5
		}
		a.LastStat = make(map[string]any)
		agents[a.AgentID] = &a
	}
	return rows.Err()
}

// saveAgents 全量写穿 SQLite（agent 数量小；先清后插以让删除生效）。
func saveAgents(_ string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM agents"); err != nil {
		tx.Rollback()
		return err
	}
	stmt, err := tx.Prepare("INSERT INTO agents(agent_id,secret,name,agent_ver,prefs) VALUES(?,?,?,?,?)")
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, a := range agents {
		pj, _ := json.Marshal(a.Prefs)
		if _, err := stmt.Exec(a.AgentID, a.Secret, a.Name, a.AgentVer, string(pj)); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

type EnrollRequest struct {
	Name string `json:"name"`; Group string `json:"group"`
	EnableConsole bool `json:"enable_console"`; TrackTraffic bool `json:"track_traffic"`; DailyReport bool `json:"daily_report"`
	Interval int `json:"interval"`
}

func enrollAgent(req EnrollRequest, masterAddr string) (*AgentRecord, string, error) {
	agentsMu.Lock(); defer agentsMu.Unlock()
	if req.Name != "" {
		for id, a := range agents { if a.Name == req.Name { delete(agents, id); break } }
	}
	rec := NewAgentRecord()
	if req.Name != "" { rec.Name = req.Name }; if req.Group != "" { rec.Prefs.Group = req.Group }
	rec.Prefs.EnableConsole = req.EnableConsole; rec.Prefs.TrackTraffic = req.TrackTraffic; rec.Prefs.DailyReport = req.DailyReport
	if req.Interval >= minInterval && req.Interval <= maxInterval { rec.Prefs.Interval = req.Interval }
	agents[rec.AgentID] = rec
	if err := saveAgents(agentsFile); err != nil { delete(agents, rec.AgentID); return nil, "", err }
	return rec, buildInstallCmd(rec, masterAddr), nil
}

func buildInstallCmd(rec *AgentRecord, masterAddr string) string {
	host := masterAddr; if host == "" { host = "wss://YOUR_HOST" }
	return "curl -fsSL " + host + "/agent-install.sh | " +
		"AGENT_ID=" + rec.AgentID + " " +
		"AGENT_SECRET=" + rec.Secret + " " +
		"MASTER_URL=" + host + " sh"
}

func verifyAgentSecret(agentID, token string) (*AgentRecord, bool) {
	agentsMu.RLock(); defer agentsMu.RUnlock()
	rec, ok := agents[agentID]
	if !ok { return nil, false }; return rec, hmac.Equal([]byte(rec.Secret), []byte(token))
}

func revokeAgent(agentID string) bool {
	agentsMu.Lock(); defer agentsMu.Unlock()
	if _, ok := agents[agentID]; !ok { return false }
	delete(agents, agentID); _ = saveAgents(agentsFile); return true
}

func signCommand(secret, agentID, data string, nonce int64) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(agentID)); mac.Write([]byte(data))
	mac.Write([]byte(time.Unix(nonce, 0).Format(time.RFC3339)))
	return hex.EncodeToString(mac.Sum(nil))
}

func verifyCommand(secret, agentID, data string, nonce int64, sig string) bool {
	diff := time.Since(time.Unix(nonce, 0))
	if diff < -2*time.Minute || diff > 2*time.Minute { return false }
	return hmac.Equal([]byte(signCommand(secret, agentID, data, nonce)), []byte(sig))
}

func sanitizeAgentID(id string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' { return r }; return -1
	}, id)
}

const groupsFile = "groups.json"
var (groupsMu sync.RWMutex; groupsList = []string{"默认分组"})

func loadGroups(_ string) error {
	rows, err := db.Query("SELECT name FROM agent_groups ORDER BY ord")
	if err != nil {
		return err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return err
		}
		out = append(out, n)
	}
	if len(out) > 0 {
		groupsList = out
	}
	return rows.Err()
}

func saveGroups(_ string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM agent_groups"); err != nil {
		tx.Rollback()
		return err
	}
	stmt, err := tx.Prepare("INSERT INTO agent_groups(name,ord) VALUES(?,?)")
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()
	for i, n := range groupsList {
		if _, err := stmt.Exec(n, i); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func AddGroup(name string) error {
	groupsMu.Lock(); defer groupsMu.Unlock()
	for _, g := range groupsList { if g == name { return fmt.Errorf("分组已存在: %s", name) } }
	groupsList = append(groupsList, name); return saveGroups(groupsFile)
}

func RemoveGroup(name string) error {
	if name == "" { return nil }
	groupsMu.Lock(); defer groupsMu.Unlock()
	newList := make([]string, 0, len(groupsList))
	for _, g := range groupsList { if g != name { newList = append(newList, g) } }
	groupsList = newList; _ = saveGroups(groupsFile)
	agentsMu.Lock()
	for _, a := range agents { if a.Prefs.Group == name { a.Prefs.Group = "默认分组" } }
	_ = saveAgents(agentsFile); agentsMu.Unlock(); return nil
}

func RenameGroup(oldName, newName string) error {
	groupsMu.Lock(); defer groupsMu.Unlock()
	if oldName == newName { return nil }; found := false
	for _, g := range groupsList {
		if g == newName { return fmt.Errorf("分组已存在: %s", newName) }; if g == oldName { found = true }
	}
	if !found { return fmt.Errorf("分组不存在: %s", oldName) }
	newList := make([]string, 0, len(groupsList))
	for _, g := range groupsList { if g == oldName { newList = append(newList, newName) } else { newList = append(newList, g) } }
	groupsList = newList; _ = saveGroups(groupsFile)
	agentsMu.Lock()
	for _, a := range agents { if a.Prefs.Group == oldName { a.Prefs.Group = newName } }
	_ = saveAgents(agentsFile); agentsMu.Unlock(); return nil
}

func GroupsList() []string { groupsMu.RLock(); defer groupsMu.RUnlock(); l := make([]string, len(groupsList)); copy(l, groupsList); return l }

func SetAgentPrefs(agentID string, prefs AgentPreferences) error {
	agentsMu.Lock(); defer agentsMu.Unlock()
	rec, ok := agents[agentID]; if !ok { return fmt.Errorf("agent 不存在: %s", agentID) }
	rec.Prefs = prefs; return saveAgents(agentsFile)
}

// UpdateAgentMeta 更新节点的名称与全部偏好/元数据（节点编辑表单）。
func UpdateAgentMeta(agentID, name string, prefs AgentPreferences) error {
	agentsMu.Lock(); defer agentsMu.Unlock()
	rec, ok := agents[agentID]; if !ok { return fmt.Errorf("agent 不存在: %s", agentID) }
	if prefs.Interval < minInterval || prefs.Interval > maxInterval { prefs.Interval = rec.Prefs.Interval }
	if prefs.CountryCode != "" { prefs.CountryCode = strings.ToUpper(prefs.CountryCode) }
	rec.Name = name; rec.Prefs = prefs
	return saveAgents(agentsFile)
}

func AgentList() []*AgentRecord { agentsMu.RLock(); defer agentsMu.RUnlock(); l := make([]*AgentRecord, 0, len(agents)); for _, a := range agents { l = append(l, a) }; return l }
func AgentByID(agentID string) *AgentRecord { agentsMu.RLock(); defer agentsMu.RUnlock(); return agents[agentID] }
func DeleteAgent(agentID string) { agentsMu.Lock(); defer agentsMu.Unlock(); delete(agents, agentID); _ = saveAgents(agentsFile) }
