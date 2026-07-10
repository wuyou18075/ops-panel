package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

var db *sql.DB

const schemaSQL = `
CREATE TABLE IF NOT EXISTS agents        (agent_id TEXT PRIMARY KEY, secret TEXT NOT NULL, name TEXT, agent_ver TEXT, prefs TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS agent_groups  (name TEXT PRIMARY KEY, ord INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS monitors      (id TEXT PRIMARY KEY, data TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS alerts        (id INTEGER PRIMARY KEY CHECK(id=1), data TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS traffic_daily (agent_id TEXT NOT NULL, date TEXT NOT NULL, sent INTEGER NOT NULL DEFAULT 0, recv INTEGER NOT NULL DEFAULT 0, PRIMARY KEY(agent_id, date));
CREATE TABLE IF NOT EXISTS panel_login   (id INTEGER PRIMARY KEY AUTOINCREMENT, ts INTEGER NOT NULL, ip TEXT, location TEXT, device TEXT, username TEXT);
CREATE TABLE IF NOT EXISTS ssh_login     (id INTEGER PRIMARY KEY AUTOINCREMENT, agent_id TEXT NOT NULL, ts INTEGER NOT NULL, ip TEXT, location TEXT, username TEXT, method TEXT, success INTEGER NOT NULL);
CREATE INDEX IF NOT EXISTS idx_ssh_login_agent_ts ON ssh_login(agent_id, ts);
CREATE TABLE IF NOT EXISTS ssh_fail_reset (agent_id TEXT PRIMARY KEY, reset_at INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS system_settings (id INTEGER PRIMARY KEY CHECK(id=1), data TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS alert_events (id INTEGER PRIMARY KEY AUTOINCREMENT, ts INTEGER NOT NULL, agent_id TEXT, kind TEXT, title TEXT, detail TEXT);
`

// openDB 打开（或创建）SQLite 库并建表。
// 单连接 + WAL + busy_timeout，串行化写，避免并发 SQLITE_BUSY。
func openDB(path string) error {
	d, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return err
	}
	d.SetMaxOpenConns(1)
	if _, err := d.Exec(schemaSQL); err != nil {
		d.Close()
		return err
	}
	db = d
	return nil
}

// migrateFromJSON 幂等地把旧 JSON 配置导入 DB（仅当对应表为空且旧文件存在）。
// 导入成功后把 JSON 文件改名为 *.bak，实现平滑升级 + 数据迁移。
func migrateFromJSON(dir string) {
	migrateOne(dir, "agents.json", "agents", func(b []byte) error {
		var list []*AgentRecord
		if err := json.Unmarshal(b, &list); err != nil {
			return err
		}
		for _, a := range list {
			if a.LastStat == nil {
				a.LastStat = make(map[string]any)
			}
			agents[a.AgentID] = a
		}
		return saveAgents("")
	})
	migrateOne(dir, "groups.json", "agent_groups", func(b []byte) error {
		if err := json.Unmarshal(b, &groupsList); err != nil {
			return err
		}
		return saveGroups("")
	})
	migrateOne(dir, "monitors.json", "monitors", func(b []byte) error {
		var list []*Monitor
		if err := json.Unmarshal(b, &list); err != nil {
			return err
		}
		for _, m := range list {
			monitors[m.ID] = m
		}
		return saveMonitors("")
	})
	migrateOne(dir, "alerts.json", "alerts", func(b []byte) error {
		if err := json.Unmarshal(b, &alertConfig); err != nil {
			return err
		}
		return saveAlerts("")
	})
}

func migrateOne(dir, file, table string, load func([]byte) error) {
	var n int
	if err := db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil || n > 0 {
		return
	}
	path := filepath.Join(dir, file)
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	if err := load(b); err != nil {
		log.Printf("[迁移] %s 失败: %v", file, err)
		return
	}
	_ = os.Rename(path, path+".bak")
	log.Printf("[迁移] %s → SQLite 完成", file)
}
