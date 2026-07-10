package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// sshlog.go —— 解析 auth.log/secure 或 systemd journal 的 SSH 登录事件并上报 master。

type SSHEvent struct {
	TS      int64  `json:"ts"`
	IP      string `json:"ip"`
	User    string `json:"user"`
	Method  string `json:"method"`
	Success bool   `json:"success"`
}

// parseSSHLine 从一行 auth.log/secure 提取 SSH 登录事件；非登录行 ok=false。
// 支持：
//
//	Accepted password|publickey for USER from IP port N ...
//	Failed password for [invalid user] USER from IP port N ...
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

// sshPollInterval 是 tail 到达文件末尾后的轮询间隔（测试可调小）。
var sshPollInterval = 2 * time.Second

// startSSHCollector 优先 tail 传统日志；没有日志文件时回退到 systemd journal。
func startSSHCollector(conn *websocket.Conn, wm *sync.Mutex, agentID string, stop <-chan struct{}) {
	emit := func(ev SSHEvent) {
		b, _ := json.Marshal(ev)
		writeJSON(conn, wm, Message{Type: "ssh_event", AgentID: agentID, Data: string(b)})
	}
	path := firstReadable(sshLogPaths)
	if path != "" {
		tailSSHLog(path, stop, emit)
		return
	}
	if err := tailSSHJournal(stop, emit); err != nil {
		fmt.Printf("[Agent] SSH 登录日志采集不可用: %v\n", err)
	}
}

// tailSSHLog 从文件末尾 tail，解析 SSH 行并对每个事件调用 emit。
// 启动时 seek 到末尾仅采集新事件；处理日志轮转与分片行。stop 关闭后返回（nil=永久运行）。
func tailSSHLog(path string, stop <-chan struct{}, emit func(SSHEvent)) {
	f, err := openSSHLog(path, true)
	if err != nil {
		return
	}
	defer f.Close()
	reader := bufio.NewReader(f)
	var partial strings.Builder
	for {
		select {
		case <-stop:
			return
		default:
		}
		chunk, err := reader.ReadString('\n')
		if err != nil {
			partial.WriteString(chunk) // 累积不完整行，待下次补全
			// 轮转检测：同时识别 copytruncate 和 rename+create。
			if fi, e := os.Stat(path); e == nil {
				curInfo, _ := f.Stat()
				cur, _ := f.Seek(0, io.SeekCurrent)
				if curInfo == nil || !os.SameFile(curInfo, fi) {
					if nf, openErr := openSSHLog(path, false); openErr == nil {
						f.Close()
						f = nf
						reader.Reset(f)
						partial.Reset()
					}
				} else if fi.Size() < cur {
					_, _ = f.Seek(0, io.SeekStart)
					reader.Reset(f)
					partial.Reset()
				}
			}
			select {
			case <-stop:
				return
			case <-time.After(sshPollInterval):
			}
			continue
		}
		line := strings.TrimSpace(partial.String() + chunk)
		partial.Reset()
		ev, ok := parseSSHLine(line)
		if !ok {
			continue
		}
		ev.TS = time.Now().Unix()
		emit(ev)
	}
}

func openSSHLog(path string, seekEnd bool) (*os.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	if seekEnd {
		if _, err := f.Seek(0, io.SeekEnd); err != nil {
			f.Close()
			return nil, err
		}
	}
	return f, nil
}

// scanSSHLog 解析流中的每一行。journalctl 使用 -n 0，因此不会回放 agent 启动前的历史。
func scanSSHLog(r io.Reader, emit func(SSHEvent)) {
	s := bufio.NewScanner(r)
	for s.Scan() {
		ev, ok := parseSSHLine(strings.TrimSpace(s.Text()))
		if !ok {
			continue
		}
		ev.TS = time.Now().Unix()
		emit(ev)
	}
}

func tailSSHJournal(stop <-chan struct{}, emit func(SSHEvent)) error {
	if _, err := exec.LookPath("journalctl"); err != nil {
		return fmt.Errorf("未找到 auth.log、secure 或 journalctl")
	}
	cmd := exec.Command("journalctl", "--no-pager", "-n", "0", "-f", "-o", "cat", "-u", "ssh", "-u", "sshd")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan struct{})
	go func() {
		scanSSHLog(stdout, emit)
		close(done)
	}()
	select {
	case <-stop:
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil
	case <-done:
	}
	return cmd.Wait()
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
