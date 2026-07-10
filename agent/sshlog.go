package main

import "strings"

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
