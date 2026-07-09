package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

type Role string

const (
	RoleAgent    Role = "agent"
	RoleViewer   Role = "viewer"
	RoleOperator Role = "operator"
)

type AgentPreferences struct {
	Group        string `json:"group"`
	TrackTraffic bool   `json:"track_traffic"`
	DailyReport  bool   `json:"daily_report"`
	Interval     int    `json:"interval"`
}

type AgentRecord struct {
	AgentID   string           `json:"agent_id"`
	Secret    string           `json:"secret"`
	Name      string           `json:"name"`
	Prefs     AgentPreferences `json:"prefs"`
	LastStat  map[string]any   `json:"-"`
	Connected bool             `json:"-"`
}

func NewAgentRecord() *AgentRecord {
	return &AgentRecord{
		AgentID: genAgentID(),
		Secret:  genSecret(),
		Prefs: AgentPreferences{
			Group:        "默认分组",
			TrackTraffic: true,
			DailyReport:  false,
			Interval:     5,
		},
		LastStat: make(map[string]any),
	}
}

const agentsFile = "agents.json"

var (
	agentsMu sync.RWMutex
	agents   = map[string]*AgentRecord{}
)

const (
	minInterval = 1
	maxInterval = 60
)

func genSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil { panic(err) }
	return hex.EncodeToString(b)
}

func genAgentID() string { return "node-" + genSecret()[:12] }

func genShortPassword() string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil { panic(err) }
	for i := range b { b[i] = chars[int(b[i])%len(chars)] }
	return string(b)
}

func loadAgents(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) { return nil }
		return err
	}
	var list []*AgentRecord
	if err := json.Unmarshal(data, &list); err != nil { return err }
	for _, a := range list {
		if a.Prefs.Interval < minInterval || a.Prefs.Interval > maxInterval {
			a.Prefs.Interval = 5
		}
		if a.LastStat == nil { a.LastStat = make(map[string]any) }
		agents[a.AgentID] = a
	}
	return nil
}

func saveAgents(path string) error {
	list := make([]*AgentRecord, 0, len(agents))
	tmp := make([]*AgentRecord, len(agents))
	idx := 0
	for _, a := range agents {
		cp := *a; cp.LastStat = nil; cp.Connected = false
		tmp[idx] = &cp; idx++
	}
	for _, a := range tmp { list = append(list, a) }
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil { return err }
	return os.WriteFile(path, data, 0o600)
}

type EnrollRequest struct {
	Name         string `json:"name"`
	Group        string `json:"group"`
	TrackTraffic bool   `json:"track_traffic"`
	DailyReport  bool   `json:"daily_report"`
}

// enrollAgent 生成新 agent 凭证，同时清理同名旧记录。
func enrollAgent(req EnrollRequest, masterAddr string) (*AgentRecord, string, error) {
	agentsMu.Lock()
	defer agentsMu.Unlock()

	// 清理同名旧 agent（防止重复注册堆积）
	if req.Name != "" {
		for id, a := range agents {
			if a.Name == req.Name {
				delete(agents, id)
				break
			}
		}
	}

	rec := NewAgentRecord()
	if req.Name != "" { rec.Name = req.Name }
	if req.Group != "" { rec.Prefs.Group = req.Group }
	rec.Prefs.TrackTraffic = req.TrackTraffic
	rec.Prefs.DailyReport = req.DailyReport

	agents[rec.AgentID] = rec
	if err := saveAgents(agentsFile); err != nil {
		delete(agents, rec.AgentID)
		return nil, "", err
	}
	return rec, buildInstallCmd(rec, masterAddr), nil
}

func buildInstallCmd(rec *AgentRecord, masterAddr string) string {
	host := masterAddr
	if host == "" { host = "wss://YOUR_HOST" }
	return "curl -fsSL " + host + "/agent-install.sh | " +
		"AGENT_ID=" + rec.AgentID + " " +
		"AGENT_SECRET=" + rec.Secret + " " +
		"MASTER=" + host + " sh"
}

func verifyAgentSecret(agentID, token string) (*AgentRecord, bool) {
	agentsMu.RLock()
	defer agentsMu.RUnlock()
	rec, ok := agents[agentID]
	if !ok { return nil, false }
	return rec, hmac.Equal([]byte(rec.Secret), []byte(token))
}

func revokeAgent(agentID string) bool {
	agentsMu.Lock()
	defer agentsMu.Unlock()
	if _, ok := agents[agentID]; !ok { return false }
	delete(agents, agentID)
	_ = saveAgents(agentsFile)
	return true
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
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return -1
		}
	}, id)
}

// ================== 分组管理 ==================

const groupsFile = "groups.json"

var (
	groupsMu   sync.RWMutex
	groupsList = []string{"默认分组"}
)

func loadGroups(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) { return nil }
		return err
	}
	var list []string
	if err := json.Unmarshal(data, &list); err != nil { return err }
	groupsList = list
	return nil
}

func saveGroups(path string) error {
	data, err := json.MarshalIndent(groupsList, "", "  ")
	if err != nil { return err }
	return os.WriteFile(path, data, 0o600)
}

func AddGroup(name string) error {
	groupsMu.Lock()
	defer groupsMu.Unlock()
	for _, g := range groupsList {
		if g == name { return fmt.Errorf("分组已存在: %s", name) }
	}
	groupsList = append(groupsList, name)
	return saveGroups(groupsFile)
}

func RemoveGroup(name string) error {
	if name == "" { return nil }
	groupsMu.Lock()
	defer groupsMu.Unlock()
	newList := make([]string, 0, len(groupsList))
	for _, g := range groupsList {
		if g == name { continue }
		newList = append(newList, g)
	}
	groupsList = newList
	_ = saveGroups(groupsFile)

	agentsMu.Lock()
	needSave := false
	for _, a := range agents {
		if a.Prefs.Group == name { a.Prefs.Group = "默认分组"; needSave = true }
	}
	if needSave { _ = saveAgents(agentsFile) }
	agentsMu.Unlock()
	return nil
}

func RenameGroup(oldName, newName string) error {
	groupsMu.Lock()
	defer groupsMu.Unlock()
	if oldName == newName { return nil }
	found := false
	for _, g := range groupsList {
		if g == newName { return fmt.Errorf("分组已存在: %s", newName) }
		if g == oldName { found = true }
	}
	if !found { return fmt.Errorf("分组不存在: %s", oldName) }
	newList := make([]string, 0, len(groupsList))
	for _, g := range groupsList {
		if g == oldName { newList = append(newList, newName) } else { newList = append(newList, g) }
	}
	groupsList = newList
	_ = saveGroups(groupsFile)

	agentsMu.Lock()
	needSave := false
	for _, a := range agents {
		if a.Prefs.Group == oldName { a.Prefs.Group = newName; needSave = true }
	}
	if needSave { _ = saveAgents(agentsFile) }
	agentsMu.Unlock()
	return nil
}

func GroupsList() []string {
	groupsMu.RLock()
	defer groupsMu.RUnlock()
	l := make([]string, len(groupsList))
	copy(l, groupsList)
	return l
}

func SetAgentPrefs(agentID string, prefs AgentPreferences) error {
	agentsMu.Lock()
	defer agentsMu.Unlock()
	rec, ok := agents[agentID]
	if !ok { return fmt.Errorf("agent 不存在: %s", agentID) }
	if prefs.Group != "" {
		groupsMu.RLock()
		exists := false
		for _, g := range groupsList { if g == prefs.Group { exists = true; break } }
		groupsMu.RUnlock()
		if !exists { return fmt.Errorf("分组不存在: %s", prefs.Group) }
	}
	rec.Prefs = prefs
	return saveAgents(agentsFile)
}

func AgentList() []*AgentRecord {
	agentsMu.RLock()
	defer agentsMu.RUnlock()
	l := make([]*AgentRecord, 0, len(agents))
	for _, a := range agents { l = append(l, a) }
	return l
}

func AgentByID(agentID string) *AgentRecord {
	agentsMu.RLock()
	defer agentsMu.RUnlock()
	return agents[agentID]
}

func DeleteAgent(agentID string) {
	agentsMu.Lock()
	defer agentsMu.Unlock()
	delete(agents, agentID)
	_ = saveAgents(agentsFile)
}
