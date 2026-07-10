package main

import "testing"

func TestParseSSHLine(t *testing.T) {
	cases := []struct {
		line                    string
		ok, success             bool
		user, ip, method        string
	}{
		{"Jul 10 12:00:00 h sshd[1]: Accepted password for root from 1.2.3.4 port 22 ssh2", true, true, "root", "1.2.3.4", "password"},
		{"Jul 10 12:00:00 h sshd[1]: Accepted publickey for alice from 5.6.7.8 port 22 ssh2", true, true, "alice", "5.6.7.8", "publickey"},
		{"Jul 10 12:00:00 h sshd[1]: Failed password for root from 9.9.9.9 port 22 ssh2", true, false, "root", "9.9.9.9", "password"},
		{"Jul 10 12:00:00 h sshd[1]: Failed password for invalid user bob from 9.9.9.9 port 22 ssh2", true, false, "bob", "9.9.9.9", "password"},
		{"Jul 10 12:00:00 h sshd[1]: Server listening on 0.0.0.0 port 22", false, false, "", "", ""},
	}
	for _, c := range cases {
		ev, ok := parseSSHLine(c.line)
		if ok != c.ok {
			t.Fatalf("ok mismatch for %q: %v", c.line, ok)
		}
		if !ok {
			continue
		}
		if ev.Success != c.success || ev.User != c.user || ev.IP != c.ip || ev.Method != c.method {
			t.Errorf("解析错 %q -> %+v", c.line, ev)
		}
	}
}
