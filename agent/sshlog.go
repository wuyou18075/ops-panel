package main

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// sshlog.go —— 解析 /var/log/auth.log|secure 的 SSH 登录事件并上报 master。

type SSHEvent struct {
	TS      int64  `json:"ts"`
	IP      string `json:"ip"`
	User    string `json:"user"`
	Method  string `json:"method"`
	Success bool   `json:"success"`
}

// parseSSHLine 从一行 auth.log/secure 提取 SSH 登录事件；非登录行 ok=false。
// 支持：
//   Accepted password|publickey for USER from IP port N ...
//   Failed password for [invalid user] USER from IP port N ...
func parseSSHLine(line string) (SSHEvent, bool) {
	var ev SSHEvent
	idx := -1
	if i := strings.Index(line, "Accepted "); i >= 0 {
		ev.Success = true
		idx = i + len("Accepted ")
	} else if i := strings.Index(line, "Failed "); i >= 0 {
		ev.Success = false
		idx = i + len("Failed ")
	} else {
		return ev, false
	}
	rest := strings.Fields(line[idx:]) // [method for (invalid user)? USER from IP port N ...]
	if len(rest) < 5 {
		return ev, false
	}
	ev.Method = rest[0] // password / publickey
	fi := indexOf(rest, "for")
	if fi < 0 || fi+1 >= len(rest) {
		return ev, false
	}
	ui := fi + 1
	if rest[ui] == "invalid" && ui+2 < len(rest) && rest[ui+1] == "user" {
		ui += 2 // 跳过 "invalid user"
	}
	ev.User = rest[ui]
	fmIdx := indexOf(rest, "from")
	if fmIdx < 0 || fmIdx+1 >= len(rest) {
		return ev, false
	}
	ev.IP = rest[fmIdx+1]
	return ev, true
}

func indexOf(ss []string, s string) int {
	for i, v := range ss {
		if v == s {
			return i
		}
	}
	return -1
}

var sshLogPaths = []string{"/var/log/auth.log", "/var/log/secure"}

// startSSHCollector tail 第一个可读日志，解析新行并上报。
// 启动时 seek 到末尾，仅采集启动后新事件；处理日志轮转与分片行。
func startSSHCollector(conn *websocket.Conn, wm *sync.Mutex, agentID string) {
	path := firstReadable(sshLogPaths)
	if path == "" {
		return // 无可读日志（权限不足/文件不存在）则静默退出
	}
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	f.Seek(0, io.SeekEnd)
	reader := bufio.NewReader(f)
	var partial strings.Builder
	for {
		chunk, err := reader.ReadString('\n')
		if err != nil {
			partial.WriteString(chunk) // 累积不完整行，待下次补全
			// 轮转检测：文件被截断/替换（当前偏移 > 文件大小）则回到开头
			if fi, e := os.Stat(path); e == nil {
				if cur, _ := f.Seek(0, io.SeekCurrent); fi.Size() < cur {
					f.Seek(0, io.SeekStart)
					reader.Reset(f)
					partial.Reset()
				}
			}
			time.Sleep(2 * time.Second)
			continue
		}
		line := strings.TrimSpace(partial.String() + chunk)
		partial.Reset()
		ev, ok := parseSSHLine(line)
		if !ok {
			continue
		}
		ev.TS = time.Now().Unix()
		b, _ := json.Marshal(ev)
		writeJSON(conn, wm, Message{Type: "ssh_event", AgentID: agentID, Data: string(b)})
	}
}

func firstReadable(paths []string) string {
	for _, p := range paths {
		if f, err := os.Open(p); err == nil {
			f.Close()
			return p
		}
	}
	return ""
}
