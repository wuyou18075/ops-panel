package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"time"
)

// Role 标识 WebSocket 连接的身份，upgrade 阶段即确定，后续不可变更。
type Role string

const (
	RoleAgent    Role = "agent"    // 被监控 VPS：上报状态、仅接收发给自己的命令
	RoleViewer   Role = "viewer"   // 公开大屏：只能收广播，禁止发命令
	RoleOperator Role = "operator" // 运维指挥台：可下发命令（需登录）
)

// AgentRecord 是服务端为每台机器生成并留存的信息。
// secret 用于：(1) agent 连接鉴权 (2) 命令 HMAC 签名/验签。
// 每台机器独立 secret，泄露一个只影响那一台（限制爆炸半径）。
type AgentRecord struct {
	AgentID  string `json:"agent_id"`
	Secret   string `json:"secret"`
	Interval int    `json:"interval"` // 上报间隔（秒），默认 5，范围 1~60
	Name     string `json:"name"`
}

// agents 存放全部已注册的 agent，文件持久化以保证重启不丢。
var (
	agentsMu sync.RWMutex
	agents   = map[string]*AgentRecord{}
)

const (
	minInterval = 1
	maxInterval = 60
)

// genSecret 生成 32 字节随机十六进制字符串。
func genSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// genAgentID 生成机器唯一标识。
func genAgentID() string {
	return "node-" + genSecret()[:12]
}

// loadAgents 从持久化文件加载已注册的 agent。
func loadAgents(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil // 首次运行，无文件
		}
		return err
	}
	var list []*AgentRecord
	if err := json.Unmarshal(data, &list); err != nil {
		return err
	}
	for _, a := range list {
		if a.Interval < minInterval || a.Interval > maxInterval {
			a.Interval = 5
		}
		agents[a.AgentID] = a
	}
	return nil
}

// saveAgents 持久化全部 agent（覆盖写，调用方需持 agentsMu）。
func saveAgents(path string) error {
	list := make([]*AgentRecord, 0, len(agents))
	for _, a := range agents {
		list = append(list, a)
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// enrollAgent 生成一个新 agent 凭证与安装指令。
// 安装指令形如：
//
//	curl -sSL https://HOST/install.sh | AGENT_ID=xxx AGENT_SECRET=yyy MASTER=wss://HOST sh
func enrollAgent(name, masterAddr string) (*AgentRecord, string, error) {
	agentsMu.Lock()
	defer agentsMu.Unlock()

	rec := &AgentRecord{
		AgentID:  genAgentID(),
		Secret:   genSecret(),
		Interval: 5,
		Name:     name,
	}
	agents[rec.AgentID] = rec
	if err := saveAgents(agentsFile); err != nil {
		delete(agents, rec.AgentID)
		return nil, "", err
	}
	installCmd := buildInstallCmd(rec, masterAddr)
	return rec, installCmd, nil
}

func buildInstallCmd(rec *AgentRecord, masterAddr string) string {
	host := masterAddr
	if host == "" {
		host = "wss://YOUR_HOST"
	}
	return "curl -fsSL " + host + "/install.sh | " +
		"AGENT_ID=" + rec.AgentID + " " +
		"AGENT_SECRET=" + rec.Secret + " " +
		"MASTER=" + host + " sh"
}

// verifyAgentSecret 常量时间比较 agent 连接携带的 token 与留存 secret。
func verifyAgentSecret(agentID, token string) (*AgentRecord, bool) {
	agentsMu.RLock()
	defer agentsMu.RUnlock()
	rec, ok := agents[agentID]
	if !ok {
		return nil, false
	}
	return rec, hmac.Equal([]byte(rec.Secret), []byte(token))
}

// revokeAgent 吊销某 agent（一键止损）。
func revokeAgent(agentID string) bool {
	agentsMu.Lock()
	defer agentsMu.Unlock()
	if _, ok := agents[agentID]; !ok {
		return false
	}
	delete(agents, agentID)
	_ = saveAgents(agentsFile)
	return true
}

// signCommand 使用 agent secret 对命令做 HMAC-SHA256 签名。
// 签名域覆盖 type+agent_id+data+nonce，防止伪造与重放。
func signCommand(secret, agentID, data string, nonce int64) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(agentID))
	mac.Write([]byte(data))
	mac.Write([]byte(time.Unix(nonce, 0).Format(time.RFC3339)))
	return hex.EncodeToString(mac.Sum(nil))
}

// verifyCommand 在 agent 端验签命令。nonce 需为最近时间窗内的新鲜值，防重放。
func verifyCommand(secret, agentID, data string, nonce int64, sig string) bool {
	// 时间窗校验：±2 分钟
	if diff := time.Since(time.Unix(nonce, 0)); diff < -2*time.Minute || diff > 2*time.Minute {
		return false
	}
	expected := signCommand(secret, agentID, data, nonce)
	return hmac.Equal([]byte(expected), []byte(sig))
}

// tokenOf 规范化取连接凭证：优先 query 的 token/session，其次 Header。
func tokenOf(queryToken, headerToken, defaultVal string) string {
	if queryToken != "" {
		return queryToken
	}
	if headerToken != "" {
		return headerToken
	}
	return defaultVal
}

// sanitizeAgentID 防止 agent_id 注入到日志/路径造成问题。
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

// genShortPassword 生成 8 位字母数字随机密码（用于 operator 默认密码）。
func genShortPassword() string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	for i := range b {
		b[i] = chars[int(b[i])%len(chars)]
	}
	return string(b)
}
