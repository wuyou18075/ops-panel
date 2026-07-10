# Ops Panel 增强 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法跟踪进度。

**目标：** 为 ops-panel 增加日流量/入出区分展示、月流量持久化、真实告警、复制修复、若干 UI 微调、面板登录日志与节点 SSH 日志两个子系统，并把持久化统一迁移到 SQLite。

**架构：** master 引入 `modernc.org/sqlite`（纯 Go）作为唯一持久层，现有 JSON 配置迁入库、保留内存缓存不损读性能、首启自动迁移；agent 解析 `/var/log/auth.log|secure` 上报 SSH 事件；Vue 前端展示。

**技术栈：** Go 1.24（`database/sql` + modernc.org/sqlite）、gorilla/websocket、Vue 3 + naive-ui + vite、Playwright（复制 E2E，若浏览器可用）。

**验证前提：** 本环境已装 Go 1.24.4，`go build/vet/test ./...` 可用；前端 `web/` 有 vite；浏览器可能缺失（复制项回退逻辑测试）。每个任务的验证命令均可实跑。

---

## 文件结构

**master/（后端）**
- 创建 `master/db.go` — 打开 DB、建表、schema 版本、JSON→SQLite 迁移、通用执行助手。
- 创建 `master/logs.go` — 面板登录日志 + SSH 日志的存取、周失败计数、UA/设备解析、相关 HTTP handler。
- 创建 `master/db_test.go`、`master/logs_test.go`、`master/traffic_test.go`、`master/alert_test.go`、`master/geoip_test.go`。
- 修改 `master/auth.go` — agents/groups 的 load/save 与增删改 → DB。
- 修改 `master/monitors.go` — monitors 的 load/save → DB。
- 修改 `master/main.go` — alerts→DB；traffic→`traffic_daily`（load + 60s 落库 + 入/出拆分）；`latestStat/lastSeen` + `alertLoop`；`main()` 初始化 DB + 迁移；注册新路由；登录日志埋点；`ssh_event` 处理；移除 `cycleStartDate`/重置日。
- 修改 `master/geoip.go` — 增 `lookupLocation`（国家·城市）。

**agent/**
- 创建 `agent/sshlog.go` — SSH 日志 tail + 纯解析函数 + 上报。
- 创建 `agent/sshlog_test.go` — 解析器样本测试。
- 修改 `agent/main.go` — 启动 SSH 采集协程。

**web/src/（前端）**
- 修改 `types.ts`、`api.ts`、`store.ts`。
- 修改 `components/CardMode.vue`、`TableMode.vue`、`NodeDetailDrawer.vue`、`NodeEditModal.vue`。
- 修改 `views/dashboard/index.vue`（复制修复、主题下拉、按钮移位、登录日志页挂载）。
- 创建 `components/LoginLogsPage.vue`。

**约定：** 每个 SQLite 表配 `save*/load*` 或 `insert*/list*/clear*` 助手，函数职责单一。commit message 用中文 Conventional Commits（`feat: …`/`fix: …`/`refactor: …`）。

---

## 阶段 0 — SQLite 基础与配置迁移

> 依赖：无。产出：master 用 SQLite 持久化 agents/groups/monitors/alerts，旧 JSON 自动迁移，所有现有行为不变。

### 任务 0.1：DB 打开与建表

**文件：**
- 创建：`master/db.go`
- 测试：`master/db_test.go`

- [ ] **步骤 1：写失败测试**

```go
// master/db_test.go
package main

import (
	"path/filepath"
	"testing"
)

func TestOpenDB_CreatesSchema(t *testing.T) {
	p := filepath.Join(t.TempDir(), "t.db")
	if err := openDB(p); err != nil {
		t.Fatalf("openDB: %v", err)
	}
	defer db.Close()
	// 建表后应能查询各表（空结果不报错）
	for _, tbl := range []string{"agents", "groups", "monitors", "alerts", "traffic_daily", "panel_login", "ssh_login", "ssh_fail_reset"} {
		if _, err := db.Exec("SELECT count(*) FROM " + tbl); err != nil {
			t.Errorf("表 %s 不可用: %v", tbl, err)
		}
	}
}
```

- [ ] **步骤 2：运行确认失败** — `cd master && go test ./ -run TestOpenDB -v`，预期 FAIL（`openDB` 未定义）。

- [ ] **步骤 3：实现 `master/db.go`**

```go
package main

import (
	"database/sql"

	_ "modernc.org/sqlite"
)

var db *sql.DB

const schemaSQL = `
CREATE TABLE IF NOT EXISTS agents   (agent_id TEXT PRIMARY KEY, secret TEXT NOT NULL, name TEXT, agent_ver TEXT, prefs TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS groups   (name TEXT PRIMARY KEY, ord INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS monitors (id TEXT PRIMARY KEY, data TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS alerts   (id INTEGER PRIMARY KEY CHECK(id=1), data TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS traffic_daily (agent_id TEXT NOT NULL, date TEXT NOT NULL, sent INTEGER NOT NULL DEFAULT 0, recv INTEGER NOT NULL DEFAULT 0, PRIMARY KEY(agent_id, date));
CREATE TABLE IF NOT EXISTS panel_login (id INTEGER PRIMARY KEY AUTOINCREMENT, ts INTEGER NOT NULL, ip TEXT, location TEXT, device TEXT, username TEXT);
CREATE TABLE IF NOT EXISTS ssh_login (id INTEGER PRIMARY KEY AUTOINCREMENT, agent_id TEXT NOT NULL, ts INTEGER NOT NULL, ip TEXT, location TEXT, username TEXT, method TEXT, success INTEGER NOT NULL);
CREATE INDEX IF NOT EXISTS idx_ssh_login_agent_ts ON ssh_login(agent_id, ts);
CREATE TABLE IF NOT EXISTS ssh_fail_reset (agent_id TEXT PRIMARY KEY, reset_at INTEGER NOT NULL);
`

// openDB 打开（或创建）SQLite 库并建表。单连接串行化写，避免 SQLITE_BUSY。
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
```

- [ ] **步骤 4：运行确认通过** — `cd master && go test ./ -run TestOpenDB -v`，预期 PASS。

- [ ] **步骤 5：Commit** — `git add master/db.go master/db_test.go && git commit -m "feat: 引入 SQLite 持久层与建表"`

### 任务 0.2：agents 迁移到 SQLite

**文件：** 修改 `master/auth.go`（`loadAgents`/`saveAgents`）；测试 `master/db_test.go`

- [ ] **步骤 1：写失败测试** —— agents 往返：

```go
func TestAgentsRoundTrip(t *testing.T) {
	if err := openDB(filepath.Join(t.TempDir(), "t.db")); err != nil { t.Fatal(err) }
	defer db.Close()
	agents = map[string]*AgentRecord{}
	rec := NewAgentRecord()
	rec.AgentID = "node-x"; rec.Name = "n1"; rec.Prefs.Group = "g1"
	agents[rec.AgentID] = rec
	if err := saveAgents(""); err != nil { t.Fatal(err) }
	agents = map[string]*AgentRecord{}
	if err := loadAgents(""); err != nil { t.Fatal(err) }
	got := agents["node-x"]
	if got == nil || got.Name != "n1" || got.Prefs.Group != "g1" {
		t.Fatalf("往返丢失: %+v", got)
	}
}
```

- [ ] **步骤 2：运行确认失败** — `go test ./ -run TestAgentsRoundTrip -v`（旧实现读文件，DB 空 → 失败）。

- [ ] **步骤 3：改写 `loadAgents`/`saveAgents`（保留签名，`path` 参数忽略）**

```go
// loadAgents 从 SQLite 读入内存 map。
func loadAgents(_ string) error {
	rows, err := db.Query("SELECT agent_id, secret, name, agent_ver, prefs FROM agents")
	if err != nil { return err }
	defer rows.Close()
	for rows.Next() {
		var a AgentRecord
		var prefsJSON string
		if err := rows.Scan(&a.AgentID, &a.Secret, &a.Name, &a.AgentVer, &prefsJSON); err != nil { return err }
		_ = json.Unmarshal([]byte(prefsJSON), &a.Prefs)
		if a.Prefs.Interval < minInterval || a.Prefs.Interval > maxInterval { a.Prefs.Interval = 5 }
		a.LastStat = make(map[string]any)
		agents[a.AgentID] = &a
	}
	return rows.Err()
}

// saveAgents 全量写穿（agent 数量小，UPSERT 全表；先清后插保证删除生效）。
func saveAgents(_ string) error {
	tx, err := db.Begin()
	if err != nil { return err }
	if _, err := tx.Exec("DELETE FROM agents"); err != nil { tx.Rollback(); return err }
	stmt, err := tx.Prepare("INSERT INTO agents(agent_id,secret,name,agent_ver,prefs) VALUES(?,?,?,?,?)")
	if err != nil { tx.Rollback(); return err }
	defer stmt.Close()
	for _, a := range agents {
		pj, _ := json.Marshal(a.Prefs)
		if _, err := stmt.Exec(a.AgentID, a.Secret, a.Name, a.AgentVer, string(pj)); err != nil { tx.Rollback(); return err }
	}
	return tx.Commit()
}
```
> 注：`auth.go` 顶部确保已 `import "encoding/json"`（已在用则无需改）。

- [ ] **步骤 4：运行确认通过** — `go test ./ -run TestAgentsRoundTrip -v`，预期 PASS。
- [ ] **步骤 5：Commit** — `git commit -am "refactor: agents 持久化迁移到 SQLite"`

### 任务 0.3：groups / monitors / alerts 迁移

**文件：** 修改 `master/auth.go`（groups）、`master/monitors.go`（monitors）、`master/main.go`（alerts）；测试 `master/db_test.go`

- [ ] **步骤 1：写失败测试**（三类各一往返，示例 groups）

```go
func TestGroupsRoundTrip(t *testing.T) {
	if err := openDB(filepath.Join(t.TempDir(), "t.db")); err != nil { t.Fatal(err) }
	defer db.Close()
	groupsList = []string{"默认分组", "香港"}
	if err := saveGroups(""); err != nil { t.Fatal(err) }
	groupsList = nil
	if err := loadGroups(""); err != nil { t.Fatal(err) }
	if len(groupsList) != 2 || groupsList[1] != "香港" { t.Fatalf("groups 往返错: %v", groupsList) }
}
```

- [ ] **步骤 2：确认失败** — `go test ./ -run 'TestGroupsRoundTrip|TestMonitorsRoundTrip|TestAlertsRoundTrip' -v`。

- [ ] **步骤 3：实现**

```go
// auth.go：groups 保序存表
func loadGroups(_ string) error {
	rows, err := db.Query("SELECT name FROM groups ORDER BY ord")
	if err != nil { return err }
	defer rows.Close()
	var out []string
	for rows.Next() { var n string; if err := rows.Scan(&n); err != nil { return err }; out = append(out, n) }
	if len(out) > 0 { groupsList = out }
	return rows.Err()
}
func saveGroups(_ string) error {
	tx, err := db.Begin()
	if err != nil { return err }
	if _, err := tx.Exec("DELETE FROM groups"); err != nil { tx.Rollback(); return err }
	stmt, _ := tx.Prepare("INSERT INTO groups(name,ord) VALUES(?,?)")
	defer stmt.Close()
	for i, n := range groupsList { if _, err := stmt.Exec(n, i); err != nil { tx.Rollback(); return err } }
	return tx.Commit()
}
```

```go
// monitors.go：monitors 存 JSON
func loadMonitors(_ string) error {
	rows, err := db.Query("SELECT data FROM monitors")
	if err != nil { return err }
	defer rows.Close()
	for rows.Next() {
		var dj string; if err := rows.Scan(&dj); err != nil { return err }
		var m Monitor; if json.Unmarshal([]byte(dj), &m) == nil { monitors[m.ID] = &m }
	}
	return rows.Err()
}
func saveMonitors(_ string) error {
	tx, err := db.Begin()
	if err != nil { return err }
	if _, err := tx.Exec("DELETE FROM monitors"); err != nil { tx.Rollback(); return err }
	stmt, _ := tx.Prepare("INSERT INTO monitors(id,data) VALUES(?,?)")
	defer stmt.Close()
	for _, m := range monitors { dj, _ := json.Marshal(m); if _, err := stmt.Exec(m.ID, string(dj)); err != nil { tx.Rollback(); return err } }
	return tx.Commit()
}
```

```go
// main.go：alerts 存单行 JSON
func loadAlerts(_ string) error {
	var dj string
	err := db.QueryRow("SELECT data FROM alerts WHERE id=1").Scan(&dj)
	if err == sql.ErrNoRows { return nil }
	if err != nil { return err }
	return json.Unmarshal([]byte(dj), &alertConfig)
}
func saveAlerts(_ string) error {
	dj, err := json.Marshal(alertConfig)
	if err != nil { return err }
	_, err = db.Exec("INSERT INTO alerts(id,data) VALUES(1,?) ON CONFLICT(id) DO UPDATE SET data=excluded.data", string(dj))
	return err
}
```
> `main.go` 需 `import "database/sql"`（用到 `sql.ErrNoRows`）。

- [ ] **步骤 4：确认通过** — 同步运行三测试，预期 PASS。
- [ ] **步骤 5：Commit** — `git commit -am "refactor: groups/monitors/alerts 迁移到 SQLite"`

### 任务 0.4：JSON→SQLite 首启迁移 + main() 接线

**文件：** 修改 `master/db.go`（`migrateFromJSON`）、`master/main.go`（`main()`）；测试 `master/db_test.go`

- [ ] **步骤 1：写失败测试** —— 存在旧 `agents.json` 时导入并改名 `.bak`：

```go
func TestMigrateFromJSON(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "agents.json"), []byte(`[{"agent_id":"n1","secret":"s","name":"old","prefs":{"group":"默认分组","interval":2}}]`), 0o600)
	if err := openDB(filepath.Join(dir, "t.db")); err != nil { t.Fatal(err) }
	defer db.Close()
	migrateFromJSON(dir)
	var n int
	db.QueryRow("SELECT count(*) FROM agents WHERE agent_id='n1'").Scan(&n)
	if n != 1 { t.Fatalf("未迁移 agents.json，count=%d", n) }
	if _, err := os.Stat(filepath.Join(dir, "agents.json.bak")); err != nil { t.Errorf("未改名 .bak: %v", err) }
}
```

- [ ] **步骤 2：确认失败** — `go test ./ -run TestMigrateFromJSON -v`。

- [ ] **步骤 3：实现 `migrateFromJSON(dir string)`**（仅当目标表空且旧文件存在时导入；导入后 `os.Rename` 为 `.bak`）

```go
// migrateFromJSON 幂等地把旧 JSON 配置导入 DB（仅当对应表为空）。
func migrateFromJSON(dir string) {
	migrateOne(dir, "agents.json", "agents", func(b []byte) error {
		var list []*AgentRecord
		if err := json.Unmarshal(b, &list); err != nil { return err }
		for _, a := range list { agents[a.AgentID] = a }
		return saveAgents("")
	})
	migrateOne(dir, "groups.json", "groups", func(b []byte) error {
		if err := json.Unmarshal(b, &groupsList); err != nil { return err }
		return saveGroups("")
	})
	migrateOne(dir, "monitors.json", "monitors", func(b []byte) error {
		var list []*Monitor
		if err := json.Unmarshal(b, &list); err != nil { return err }
		for _, m := range list { monitors[m.ID] = m }
		return saveMonitors("")
	})
	migrateOne(dir, "alerts.json", "alerts", func(b []byte) error {
		if err := json.Unmarshal(b, &alertConfig); err != nil { return err }
		return saveAlerts("")
	})
}

func migrateOne(dir, file, table string, load func([]byte) error) {
	var n int
	if err := db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil || n > 0 { return }
	path := filepath.Join(dir, file)
	b, err := os.ReadFile(path)
	if err != nil { return }
	if err := load(b); err != nil { log.Printf("[迁移] %s 失败: %v", file, err); return }
	_ = os.Rename(path, path+".bak")
	log.Printf("[迁移] %s → SQLite 完成", file)
}
```
> `db.go` 需 `import ("encoding/json"; "log"; "os"; "path/filepath")`。

- [ ] **步骤 4：确认通过** — `go test ./ -run TestMigrateFromJSON -v`，预期 PASS。

- [ ] **步骤 5：接线 `main()`** —— 在 `loadAgents` 等调用之前初始化 DB。`master/main.go` 的 `main()` 内、`if err := loadAgents(agentsFile)` 之前插入：

```go
	if err := openDB("ops-panel.db"); err != nil {
		log.Fatal("打开数据库失败:", err)
	}
	migrateFromJSON(".")
```
并把 `registerRoutes()` 里 `loadGroups(groupsFile)`、`loadMonitors(monitorsFile)` 保留（现在读 DB）。

- [ ] **步骤 6：全量编译 + 测试 + 手启动**

运行：`cd master && go build ./... && go vet ./... && go test ./ -v`
预期：build/vet 通过；测试全绿。
运行冒烟：`cd master && (go run . &) ; sleep 2 ; ls -la ops-panel.db ; pkill -f 'go run'`（或 `go run .` 手动观察启动 banner 无报错、生成 `ops-panel.db`）。

- [ ] **步骤 7：Commit** — `git commit -am "feat: 首启 JSON→SQLite 自动迁移并接线 main()"`

---

## 阶段 1 — 前端微调 + 复制修复（纯前端，本环境可完整验证）

> 依赖：无（不涉及后端）。产出：复制可靠、表格中文/宽松、主题下拉、按钮移位。

### 任务 1.1：复制 bug 修复（#4）

**文件：** 修改 `web/src/views/dashboard/index.vue`（`copyText`/`fallbackCopy`/`doEnroll`）；测试 `web/src/utils.copy.test.ts`（若装了 vitest）或 Playwright。

- [ ] **步骤 1：抽出可测的纯复制模块** —— 创建 `web/src/clipboard.ts`：

```ts
// 复制到剪贴板：安全上下文优先异步 API，否则用 execCommand 兜底。
// 兜底时把 textarea 挂到给定容器（模态框）内以绕过焦点陷阱。
export async function copyToClipboard(text: string, container?: HTMLElement): Promise<boolean> {
  if (!text) return false;
  if (navigator.clipboard && window.isSecureContext) {
    try { await navigator.clipboard.writeText(text); return true; } catch { /* 落到兜底 */ }
  }
  return execCommandCopy(text, container);
}

export function execCommandCopy(text: string, container?: HTMLElement): boolean {
  const host = container || document.body;
  const ta = document.createElement("textarea");
  ta.value = text;
  ta.setAttribute("readonly", "");
  ta.style.position = "absolute";
  ta.style.left = "-9999px";
  ta.style.top = "0";
  host.appendChild(ta);
  const sel = document.getSelection();
  const prev = sel && sel.rangeCount > 0 ? sel.getRangeAt(0) : null;
  ta.focus();
  ta.select();
  ta.setSelectionRange(0, text.length);
  let ok = false;
  try { ok = document.execCommand("copy"); } catch { ok = false; }
  host.removeChild(ta);
  if (prev && sel) { sel.removeAllRanges(); sel.addRange(prev); }
  return ok;
}
```

- [ ] **步骤 2：写失败测试** —— 创建 `web/src/clipboard.test.ts`：

```ts
import { describe, it, expect, vi } from "vitest";
import { copyToClipboard } from "./clipboard";

describe("copyToClipboard", () => {
  it("安全上下文用异步 API 复制原文", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    (globalThis as any).navigator = { clipboard: { writeText } };
    (globalThis as any).window = { isSecureContext: true };
    const ok = await copyToClipboard("curl xyz | sh");
    expect(ok).toBe(true);
    expect(writeText).toHaveBeenCalledWith("curl xyz | sh"); // 复制的正是传入命令
  });
  it("空串返回 false", async () => {
    expect(await copyToClipboard("")).toBe(false);
  });
});
```

- [ ] **步骤 3：确认失败/通过** — `cd web && npx vitest run src/clipboard.test.ts`（若无 vitest 依赖，改为 `pnpm/npm i -D vitest` 或跳过，见验证节）。预期该模块测试 PASS。

- [ ] **步骤 4：在 dashboard 接入** —— `views/dashboard/index.vue`：
  - `import { copyToClipboard } from "../../clipboard";`
  - 用 `copyText` 包装：从事件目标向上找最近 `.n-modal` 作为容器：
```ts
async function copyText(txt: string) {
  if (!txt) return;
  const host = (document.querySelector(".n-modal") as HTMLElement) || undefined;
  const ok = await copyToClipboard(txt, host);
  if (ok) message.success("已复制到剪贴板");
  else message.error("复制失败，请手动选择文本复制");
}
```
  - 删除旧 `fallbackCopy`。
  - `doEnroll` 中移除 `await copyText(d.install_cmd)` 这一行（去掉 await 后失去手势的静默自动复制）；仅设 `enrollCommand.value = d.install_cmd` 并 `message.success("安装命令已生成，点击下方复制")`。

- [ ] **步骤 5：构建验证** — `cd web && npx vite build`，预期成功产出 `../master/dist/assets/*`。
- [ ] **步骤 6：Commit** — `git add web/src/clipboard.ts web/src/clipboard.test.ts web/src/views/dashboard/index.vue && git commit -m "fix: 修复添加节点复制命令复制到错误内容的问题"`

### 任务 1.2：表格中文表头 + 行拉长（#5 #6）

**文件：** 修改 `web/src/components/TableMode.vue`

- [ ] **步骤 1：中文表头** —— 表头 `<div>` 文案改中文，并把 `toggleable` 标签中文化：
  - `System→系统`、`CPU→CPU`、`Memory→内存`、`Disk→磁盘`、`Net→网络`、`Agent→客户端`。
  - `h1` 文案 `All Systems → 全部节点`；filter 占位 `Filter...→筛选…`；列按钮 `列 ▾` 保留。
- [ ] **步骤 2：行拉长** —— `.row` 的 `padding: 10px 14px` → `padding: 16px 16px`；`gap: 14px` → `18px`；`.row.head` `padding-top: 14px` → `18px`；`.table` 增加行分隔留白（`.row + .row` 已有分隔线，保留）。
- [ ] **步骤 3：构建验证** — `cd web && npx vite build`，预期成功。
- [ ] **步骤 4：Commit** — `git commit -am "feat: 表格表头中文化并放宽行距"`

### 任务 1.3：主题下拉框（#7）

**文件：** 修改 `web/src/views/dashboard/index.vue`

- [ ] **步骤 1：替换圆点为下拉** —— 把 `.themes` 那段 `v-for` 圆点替换为：
```vue
<NSelect
  :value="themeKey"
  :options="THEMES.map(t => ({ label: t.label, value: t.key }))"
  size="small"
  style="width: 130px"
  @update:value="applyTheme"
/>
```
  - 确保 `NSelect` 已在 `import { ... } from 'naive-ui'`（已导入 NSelect）。
- [ ] **步骤 2：清理无用样式** —— 删除 `.themes/.tdot` 相关 CSS（可保留不碍事，优先删）。
- [ ] **步骤 3：构建验证** — `npx vite build`，预期成功。
- [ ] **步骤 4：Commit** — `git commit -am "feat: 主题切换改为下拉框"`

### 任务 1.4：添加分组按钮移位（#8）

**文件：** 修改 `web/src/views/dashboard/index.vue`

- [ ] **步骤 1：移动按钮** —— 侧栏 `.ghead` 中的 `+`（`showAddGroup`）保留或删除；在列表头 `.lhead` 里、`+ 添加节点`按钮**左侧**新增：
```vue
<div class="lhead">
  <span class="ltitle">节点（{{ visibleNodes.length }}）</span>
  <div style="display:flex; gap:8px">
    <NButton v-if="!publicMode && isOperator" size="small" @click="showAddGroup = true">+ 添加分组</NButton>
    <NButton v-if="!publicMode && isOperator" type="primary" size="small" @click="openEnroll">+ 添加节点</NButton>
  </div>
</div>
```
- [ ] **步骤 2：构建验证** — `npx vite build`，预期成功。
- [ ] **步骤 3：Commit** — `git commit -am "feat: 添加分组按钮移至添加节点左侧"`

---

## 阶段 2 — 流量：入/出区分 + 日流量展示 + 持久化（#1 #2 #9）

> 依赖：阶段 0（traffic_daily 表）。产出：流量按入/出统计、卡片与表格显示今日/本月入出、关机不清空、统一每月1日。

### 任务 2.1：TrafficStats 入/出拆分 + 移除自定义重置日（后端）

**文件：** 修改 `master/main.go`（`TrafficStats`、`trafficStatsSnapshot`、`checkTrafficQuota`，删 `cycleStartDate`）、`master/auth.go`（删 `AgentPreferences.TrafficResetDay`）；测试 `master/traffic_test.go`

- [ ] **步骤 1：写失败测试** —— 快照按今日/本月分别汇总 sent/recv：

```go
func TestTrafficSnapshot_SplitInOut(t *testing.T) {
	if err := openDB(filepath.Join(t.TempDir(), "t.db")); err != nil { t.Fatal(err) }
	defer db.Close()
	agents = map[string]*AgentRecord{"n1": {AgentID: "n1", Name: "n1", Prefs: AgentPreferences{TrackTraffic: true}}}
	traffic = map[string]*TrafficDay{}
	today := time.Now().Format("2006-01-02")
	traffic["n1|"+today] = &TrafficDay{Date: today, Sent: 100, Recv: 300}
	out := trafficStatsSnapshot()
	if len(out) != 1 { t.Fatalf("want 1, got %d", len(out)) }
	s := out[0]
	if s.TodaySent != 100 || s.TodayRecv != 300 { t.Errorf("今日入出错: %+v", s) }
	if s.MonthSent != 100 || s.MonthRecv != 300 { t.Errorf("本月入出错: %+v", s) }
}
```

- [ ] **步骤 2：确认失败** — `go test ./ -run TestTrafficSnapshot_SplitInOut -v`。

- [ ] **步骤 3：实现** —— `TrafficStats` 增字段并改 `trafficStatsSnapshot`：

```go
type TrafficStats struct {
	AgentID   string `json:"agent_id"`
	Group     string `json:"group"`
	Name      string `json:"name"`
	Today     int64  `json:"today"`
	TodaySent int64  `json:"today_sent"`
	TodayRecv int64  `json:"today_recv"`
	ThisMonth int64  `json:"this_month"`
	MonthSent int64  `json:"month_sent"`
	MonthRecv int64  `json:"month_recv"`
	CycleUsed int64  `json:"cycle_used"` // = 本自然月合计（保留字段名兼容前端）
	Quota     int64  `json:"quota"`
}
```
```go
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
```
  - 删除 `cycleStartDate` 函数。
  - `checkTrafficQuota`：周期键改 `cycleKey := now.Format("2006-01")`，去掉 `if s.ResetDay > 0 {...}` 分支；比较 `s.CycleUsed >= s.Quota`（`CycleUsed` 现为自然月）。
  - `auth.go`：删除 `AgentPreferences.TrafficResetDay` 字段（及注释）。

- [ ] **步骤 4：确认通过 + 全量** — `go test ./ -run TestTrafficSnapshot_SplitInOut -v` PASS；`go build ./... && go vet ./...` 通过。
- [ ] **步骤 5：Commit** — `git commit -am "feat: 流量统计区分入站/出站并统一自然月，移除自定义重置日"`

### 任务 2.2：traffic 持久化到 traffic_daily（后端）

**文件：** 修改 `master/main.go`（新增 `loadTrafficFromDB`/`persistTrafficLoop`，`main()` 接线）；测试 `master/traffic_test.go`

- [ ] **步骤 1：写失败测试** —— 落库 + 重载往返：

```go
func TestTrafficPersistRoundTrip(t *testing.T) {
	if err := openDB(filepath.Join(t.TempDir(), "t.db")); err != nil { t.Fatal(err) }
	defer db.Close()
	traffic = map[string]*TrafficDay{}
	d := "2026-07-10"
	traffic["n1|"+d] = &TrafficDay{Date: d, Sent: 5, Recv: 7}
	if err := persistTrafficOnce(); err != nil { t.Fatal(err) }
	traffic = map[string]*TrafficDay{}
	if err := loadTrafficFromDB(); err != nil { t.Fatal(err) }
	got := traffic["n1|"+d]
	if got == nil || got.Sent != 5 || got.Recv != 7 { t.Fatalf("往返错: %+v", got) }
}
```

- [ ] **步骤 2：确认失败** — `go test ./ -run TestTrafficPersistRoundTrip -v`。

- [ ] **步骤 3：实现**

```go
// persistTrafficOnce 把内存 traffic map UPSERT 落库。
func persistTrafficOnce() error {
	trafficMu.RLock()
	type row struct{ id, date string; sent, recv int64 }
	rows := make([]row, 0, len(traffic))
	for k, d := range traffic {
		parts := strings.SplitN(k, "|", 2)
		if len(parts) != 2 { continue }
		rows = append(rows, row{parts[0], d.Date, d.Sent, d.Recv})
	}
	trafficMu.RUnlock()
	tx, err := db.Begin()
	if err != nil { return err }
	stmt, err := tx.Prepare("INSERT INTO traffic_daily(agent_id,date,sent,recv) VALUES(?,?,?,?) ON CONFLICT(agent_id,date) DO UPDATE SET sent=excluded.sent, recv=excluded.recv")
	if err != nil { tx.Rollback(); return err }
	defer stmt.Close()
	for _, r := range rows { if _, err := stmt.Exec(r.id, r.date, r.sent, r.recv); err != nil { tx.Rollback(); return err } }
	return tx.Commit()
}

// loadTrafficFromDB 启动时把近 40 天流量读回内存 map。
func loadTrafficFromDB() error {
	cutoff := time.Now().AddDate(0, 0, -40).Format("2006-01-02")
	rows, err := db.Query("SELECT agent_id,date,sent,recv FROM traffic_daily WHERE date >= ?", cutoff)
	if err != nil { return err }
	defer rows.Close()
	for rows.Next() {
		var id, date string; var sent, recv int64
		if err := rows.Scan(&id, &date, &sent, &recv); err != nil { return err }
		traffic[id+"|"+date] = &TrafficDay{Date: date, Sent: sent, Recv: recv}
	}
	return rows.Err()
}

func persistTrafficLoop() {
	for range time.Tick(60 * time.Second) {
		if err := persistTrafficOnce(); err != nil { log.Println("[流量落库] ", err) }
		pruneOldTraffic()
	}
}

func pruneOldTraffic() {
	cutoff := time.Now().AddDate(0, 0, -40).Format("2006-01-02")
	_, _ = db.Exec("DELETE FROM traffic_daily WHERE date < ?", cutoff)
}
```
  - `main()` 接线：在 `openDB`+迁移之后加 `loadTrafficFromDB()`；在 `go trafficAlertLoop()` 旁加 `go persistTrafficLoop()`。

- [ ] **步骤 4：确认通过 + 冒烟** — `go test ./ -run TestTrafficPersistRoundTrip -v` PASS；`go build ./...` 通过。
- [ ] **步骤 5：Commit** — `git commit -am "feat: 月流量持久化到 SQLite（关机不清空）"`

### 任务 2.3：前端展示今日/本月入出 + 表格今日列 + 删重置日字段（#1 #9 前端）

**文件：** 修改 `web/src/types.ts`、`store.ts`、`components/CardMode.vue`、`TableMode.vue`、`NodeDetailDrawer.vue`、`NodeEditModal.vue`

- [ ] **步骤 1：类型** —— `TrafficStats` 加 `today_sent/today_recv/month_sent/month_recv`；`NodeView` 加 `todaySent?/todayRecv?/monthSent?/monthRecv?`；`AgentPreferences` 删 `traffic_reset_day`。
- [ ] **步骤 2：store 合并** —— `nodeViews` 里从 `traffic[id]` 映射新字段：`todaySent: tr?.today_sent` 等。
- [ ] **步骤 3：CardMode** —— `.net-info` 中「流量」行改为两行：
```vue
<div class="line"><span>今日</span><span>出↑ {{ fmtBytes(n.todaySent||0) }} · 入↓ {{ fmtBytes(n.todayRecv||0) }}</span></div>
<div class="line"><span>本月</span><span>出↑ {{ fmtBytes(n.monthSent||0) }} · 入↓ {{ fmtBytes(n.monthRecv||0) }}<template v-if="n.quota"> / {{ fmtBytes(n.quota) }}</template></span></div>
```
- [ ] **步骤 4：TableMode** —— `cols` 增 `today: true`；`toggleable` 增 `{k:"today", l:"今日流量"}`；表头增 `<div v-if="cols.today">今日流量</div>`；行增 `<div v-if="cols.today" class="net-cell">出↑{{fmtBytes(n.todaySent||0)}}<br/>入↓{{fmtBytes(n.todayRecv||0)}}</div>`；`gridStyle` 对应加一列 `1.1fr`。需 `import { fmtBytes } from "../utils"`.
- [ ] **步骤 5：NodeDetailDrawer** —— 「流量」段的 `qrow` 改为显示今日/本月出入：
```vue
<div class="qrow"><span>今日 出↑{{fmtBytes(node.todaySent||0)}} 入↓{{fmtBytes(node.todayRecv||0)}}</span></div>
<div class="qrow"><span>本月 出↑{{fmtBytes(node.monthSent||0)}} 入↓{{fmtBytes(node.monthRecv||0)}}<template v-if="node.quota"> / {{fmtBytes(node.quota)}}</template></span></div>
```
- [ ] **步骤 6：NodeEditModal** —— 删除「重置日」`Field`（模板 34-36 行）与 `save()` 里的 `traffic_reset_day`、watch 里的 `traffic_reset_day` 赋值。
- [ ] **步骤 7：构建验证** — `cd web && npx vite build`，预期成功。
- [ ] **步骤 8：Commit** — `git commit -am "feat: 卡片/表格/详情展示今日与本月入出流量并移除重置日字段"`

---

## 阶段 3 — 真实告警 + 默认开启（#3）

> 依赖：阶段 0。产出：CPU/内存/磁盘/离线阈值真实触发 TG 告警，默认开启。

### 任务 3.1：记录每节点最新指标与最后在线时间（后端）

**文件：** 修改 `master/main.go`（`ingestStat`、新增 `latestStat`/`lastSeen`）；测试 `master/alert_test.go`

- [ ] **步骤 1：写失败测试** —— `ingestStat` 后可取到最新指标：

```go
func TestIngestStat_TracksLatest(t *testing.T) {
	if err := openDB(filepath.Join(t.TempDir(), "t.db")); err != nil { t.Fatal(err) }
	defer db.Close()
	latestStat = map[string]statSample{}; lastSeen = map[string]time.Time{}
	rec := &AgentRecord{AgentID: "n1", Prefs: AgentPreferences{Interval: 2}}
	ingestStat("n1", rec, `{"cpu":91.5,"mem":40,"disk":20,"net_sent":1,"net_recv":2}`)
	s, ok := latestStat["n1"]
	if !ok || s.CPU != 91.5 { t.Fatalf("最新指标未记录: %+v", s) }
	if lastSeen["n1"].IsZero() { t.Errorf("lastSeen 未更新") }
}
```

- [ ] **步骤 2：确认失败** — `go test ./ -run TestIngestStat_TracksLatest -v`。

- [ ] **步骤 3：实现** —— 新增类型与全局，`ingestStat` 末尾写入：

```go
type statSample struct{ CPU, Mem, Disk float64 }

var (
	metricMu   sync.Mutex
	latestStat = map[string]statSample{}
	lastSeen   = map[string]time.Time{}
)
```
在 `ingestStat` 解析出 `s` 后追加：
```go
	metricMu.Lock()
	latestStat[agentID] = statSample{CPU: s.CPU, Mem: s.Mem, Disk: s.Disk}
	lastSeen[agentID] = time.Now()
	metricMu.Unlock()
```
> 注意：`ingestStat` 的匿名 struct 已含 cpu/mem/disk 字段，无需改解析。`lastSeen` 不在断连时删除（`handleAgentWS` 退出处不要清 `lastSeen`）。

- [ ] **步骤 4：确认通过** — `go test ./ -run TestIngestStat_TracksLatest -v` PASS。
- [ ] **步骤 5：Commit** — `git commit -am "feat: 记录每节点最新指标与最后在线时间"`

### 任务 3.2：告警判定 + 循环 + 默认开启（后端）

**文件：** 修改 `master/main.go`（`evalAlerts` 纯函数、`alertLoop`、默认值、`main()` 接线）；测试 `master/alert_test.go`

- [ ] **步骤 1：写失败测试** —— 纯判定函数（易测、不发 TG）：

```go
func TestEvalAlerts(t *testing.T) {
	cfg := AlertConfig{CPUPercent: 80, MemPercent: 80, DiskPercent: 90, OfflineMinutes: 5, Enabled: true}
	now := time.Now()
	latest := map[string]statSample{"n1": {CPU: 95, Mem: 10, Disk: 10}}
	seen := map[string]time.Time{"n1": now, "n2": now.Add(-10 * time.Minute)}
	ids := []string{"n1", "n2"}
	msgs := evalAlerts(cfg, ids, latest, seen, now)
	joined := strings.Join(msgs, "\n")
	if !strings.Contains(joined, "n1") || !strings.Contains(joined, "CPU") { t.Errorf("应含 n1 CPU 告警: %v", msgs) }
	if !strings.Contains(joined, "n2") || !strings.Contains(joined, "离线") { t.Errorf("应含 n2 离线告警: %v", msgs) }
}

func TestEvalAlerts_DisabledSilent(t *testing.T) {
	cfg := AlertConfig{CPUPercent: 80, Enabled: false}
	msgs := evalAlerts(cfg, []string{"n1"}, map[string]statSample{"n1": {CPU: 99}}, map[string]time.Time{"n1": time.Now()}, time.Now())
	if len(msgs) != 0 { t.Errorf("关闭时不应告警: %v", msgs) }
}
```

- [ ] **步骤 2：确认失败** — `go test ./ -run TestEvalAlerts -v`。

- [ ] **步骤 3：实现纯判定 + 循环 + 去重**

```go
// evalAlerts 返回本轮应发送的告警文案（纯函数，便于测试）。
func evalAlerts(cfg AlertConfig, ids []string, latest map[string]statSample, seen map[string]time.Time, now time.Time) []string {
	if !cfg.Enabled { return nil }
	var out []string
	for _, id := range ids {
		name := alertName(id)
		if t, ok := seen[id]; ok && now.Sub(t) > time.Duration(cfg.OfflineMinutes)*time.Minute {
			out = append(out, fmt.Sprintf("🔴 %s 离线超过 %d 分钟", name, cfg.OfflineMinutes))
			continue // 离线则不再报资源阈值
		}
		s, ok := latest[id]
		if !ok { continue }
		if s.CPU >= float64(cfg.CPUPercent) { out = append(out, fmt.Sprintf("⚠️ %s CPU %.0f%% 超阈值 %d%%", name, s.CPU, cfg.CPUPercent)) }
		if s.Mem >= float64(cfg.MemPercent) { out = append(out, fmt.Sprintf("⚠️ %s 内存 %.0f%% 超阈值 %d%%", name, s.Mem, cfg.MemPercent)) }
		if s.Disk >= float64(cfg.DiskPercent) { out = append(out, fmt.Sprintf("⚠️ %s 磁盘 %.0f%% 超阈值 %d%%", name, s.Disk, cfg.DiskPercent)) }
	}
	return out
}

func alertName(id string) string {
	if r := AgentByID(id); r != nil && r.Name != "" { return r.Name }
	return id
}

var alertFireMu sync.Mutex
var alertFired = map[string]bool{} // 文案 -> 已发，去重防刷屏；本轮不在集合内才发

func alertLoop() {
	for range time.Tick(60 * time.Second) {
		alertsMu.RLock(); cfg := alertConfig; alertsMu.RUnlock()
		metricMu.Lock()
		ls := make(map[string]statSample, len(latestStat)); for k, v := range latestStat { ls[k] = v }
		sn := make(map[string]time.Time, len(lastSeen)); for k, v := range lastSeen { sn[k] = v }
		metricMu.Unlock()
		agentsMu.RLock(); ids := make([]string, 0, len(agents)); for id := range agents { ids = append(ids, id) }; agentsMu.RUnlock()
		msgs := evalAlerts(cfg, ids, ls, sn, time.Now())
		cur := map[string]bool{}
		for _, m := range msgs { cur[m] = true }
		alertFireMu.Lock()
		for _, m := range msgs { if !alertFired[m] { sendTGAlert(m) } }
		alertFired = cur // 未再出现的告警清除，恢复后可再报
		alertFireMu.Unlock()
	}
}
```
  - 默认开启：`alertConfig = AlertConfig{CPUPercent: 80, MemPercent: 80, DiskPercent: 80, OfflineMinutes: 5, Enabled: true}`。
  - `main()`：`go trafficAlertLoop()` 旁加 `go alertLoop()`。

- [ ] **步骤 4：确认通过 + 全量** — `go test ./ -run TestEvalAlerts -v` PASS；`go build ./... && go vet ./...` 通过。
- [ ] **步骤 5：前端默认** —— `store.ts` 的 `alertCfg` 默认 `enabled: true`。
- [ ] **步骤 6：构建验证** — `cd web && npx vite build`，预期成功。
- [ ] **步骤 7：Commit** — `git commit -am "feat: 实现 CPU/内存/磁盘/离线真实告警并默认开启"`

---

## 阶段 4 — 面板登录日志（#10）

> 依赖：阶段 0。产出：成功登录记录 IP/地点/设备/用户名，留 100，可查可清空。

### 任务 4.1：地点与设备解析

**文件：** 修改 `master/geoip.go`（`lookupLocation`）、创建 `master/logs.go`（`parseDevice`）；测试 `master/geoip_test.go`、`master/logs_test.go`

- [ ] **步骤 1：写失败测试**（`parseDevice` 纯函数）

```go
// master/logs_test.go
func TestParseDevice(t *testing.T) {
	ua := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120 Safari/537.36"
	got := parseDevice(ua)
	if !strings.Contains(got, "Chrome") || !strings.Contains(got, "macOS") { t.Errorf("解析设备错: %s", got) }
}
```

- [ ] **步骤 2：确认失败** — `go test ./ -run TestParseDevice -v`。

- [ ] **步骤 3：实现 `parseDevice`（logs.go）与 `lookupLocation`（geoip.go）**

```go
// logs.go
func parseDevice(ua string) string {
	browser := "未知浏览器"
	switch {
	case strings.Contains(ua, "Edg/"): browser = "Edge"
	case strings.Contains(ua, "Chrome/"): browser = "Chrome"
	case strings.Contains(ua, "Firefox/"): browser = "Firefox"
	case strings.Contains(ua, "Safari/"): browser = "Safari"
	}
	os := "未知系统"
	switch {
	case strings.Contains(ua, "Windows"): os = "Windows"
	case strings.Contains(ua, "Mac OS X"), strings.Contains(ua, "Macintosh"): os = "macOS"
	case strings.Contains(ua, "Android"): os = "Android"
	case strings.Contains(ua, "iPhone"), strings.Contains(ua, "iPad"): os = "iOS"
	case strings.Contains(ua, "Linux"): os = "Linux"
	}
	return browser + " · " + os
}
```
```go
// geoip.go：返回 "国家·城市"，失败返回空。带 1 小时 IP 缓存。
var (locMu sync.Mutex; locCache = map[string]string{})
func lookupLocation(ip string) string {
	if ip == "" || isPrivateIP(ip) { return "" }
	locMu.Lock(); if v, ok := locCache[ip]; ok { locMu.Unlock(); return v }; locMu.Unlock()
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://ip-api.com/json/" + ip + "?fields=status,country,city")
	if err != nil { return "" }
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<12))
	var v struct{ Status, Country, City string }
	if json.Unmarshal(body, &v) != nil || v.Status != "success" { return "" }
	loc := strings.TrimSpace(v.Country + "·" + v.City)
	locMu.Lock(); locCache[ip] = loc; locMu.Unlock()
	return loc
}
```

- [ ] **步骤 4：确认通过** — `go test ./ -run TestParseDevice -v` PASS；`go build ./...` 通过。
- [ ] **步骤 5：Commit** — `git commit -am "feat: 新增设备(UA)与城市级地点解析"`

### 任务 4.2：登录日志存取 + 埋点 + API

**文件：** 修改 `master/logs.go`（存取 + handler）、`master/operator.go`（`handleLogin` 埋点）、`master/main.go`（注册路由）；测试 `master/logs_test.go`

- [ ] **步骤 1：写失败测试** —— 插入裁剪到 100 + 列表 + 清空：

```go
func TestPanelLogin_InsertCapClear(t *testing.T) {
	if err := openDB(filepath.Join(t.TempDir(), "t.db")); err != nil { t.Fatal(err) }
	defer db.Close()
	for i := 0; i < 120; i++ { insertPanelLogin(int64(i), "1.1.1.1", "中国·北京", "Chrome · macOS", "admin") }
	list := listPanelLogin()
	if len(list) != 100 { t.Fatalf("应裁剪到 100，got %d", len(list)) }
	if list[0].TS < list[len(list)-1].TS { t.Errorf("应按时间倒序") }
	clearPanelLogin()
	if len(listPanelLogin()) != 0 { t.Errorf("清空失败") }
}
```

- [ ] **步骤 2：确认失败** — `go test ./ -run TestPanelLogin -v`。

- [ ] **步骤 3：实现存取 + handler**

```go
// logs.go
type LoginLog struct {
	TS int64 `json:"ts"`; IP string `json:"ip"`; Location string `json:"location"`; Device string `json:"device"`; Username string `json:"username"`
}
func insertPanelLogin(ts int64, ip, loc, device, user string) {
	db.Exec("INSERT INTO panel_login(ts,ip,location,device,username) VALUES(?,?,?,?,?)", ts, ip, loc, device, user)
	db.Exec("DELETE FROM panel_login WHERE id NOT IN (SELECT id FROM panel_login ORDER BY id DESC LIMIT 100)")
}
func listPanelLogin() []LoginLog {
	rows, err := db.Query("SELECT ts,ip,location,device,username FROM panel_login ORDER BY id DESC")
	if err != nil { return []LoginLog{} }
	defer rows.Close()
	out := []LoginLog{}
	for rows.Next() { var l LoginLog; rows.Scan(&l.TS, &l.IP, &l.Location, &l.Device, &l.Username); out = append(out, l) }
	return out
}
func clearPanelLogin() { db.Exec("DELETE FROM panel_login") }

func handleLoginLogs(w http.ResponseWriter, r *http.Request) {
	if !operatorAuthorized(r) { http.Error(w, "未授权", http.StatusUnauthorized); return }
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json"); json.NewEncoder(w).Encode(listPanelLogin())
	case http.MethodDelete:
		clearPanelLogin(); w.WriteHeader(http.StatusOK)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
```
  - `operator.go` 的 `handleLogin` 成功签发 token 前后（`body.Username`/`body.Password` 校验通过后）加埋点：
```go
	ip := clientIP(r.RemoteAddr, r.Header.Get("X-Forwarded-For"))
	go insertPanelLogin(time.Now().Unix(), ip, lookupLocation(ip), parseDevice(r.UserAgent()), body.Username)
```
  （`operator.go` 需 `import "time"`。）
  - `main.go` 注册：`http.HandleFunc(masterPath+"/api/login-logs", handleLoginLogs)`。

- [ ] **步骤 4：确认通过 + 全量** — `go test ./ -run TestPanelLogin -v` PASS；`go build ./... && go vet ./...` 通过。
- [ ] **步骤 5：Commit** — `git commit -am "feat: 面板登录日志记录与查询/清空 API"`

### 任务 4.3：登录日志前端页

**文件：** 创建 `web/src/components/LoginLogsPage.vue`；修改 `web/src/api.ts`、`web/src/views/dashboard/index.vue`

- [ ] **步骤 1：api.ts** —— 加 `loginLogs()`（GET）、`clearLoginLogs()`（DELETE）。
- [ ] **步骤 2：LoginLogsPage.vue** —— 表格列：时间 / IP / 地点 / 设备 / 用户名 + 顶部「清空」按钮（`NPopconfirm`）。加载调 `Api.loginLogs()`。
- [ ] **步骤 3：挂载** —— `index.vue` 的 `navPages`（非 public）加 `{ k: "loginlogs", l: "登录日志", i: "≡" }`；`content` 区加 `<LoginLogsPage v-else-if="page === 'loginlogs' && !publicMode" />` 并 import。
- [ ] **步骤 4：构建验证** — `cd web && npx vite build`，预期成功。
- [ ] **步骤 5：Commit** — `git commit -am "feat: 登录日志前端页与清空"`

---

## 阶段 5 — 节点 SSH 日志 + 失败周告警（#11 #12）

> 依赖：阶段 0、4（复用 lookupLocation）。产出：agent 采集 SSH 登录、master 存储与周失败统计、前端查看/清空/徽标/重置。

### 任务 5.1：agent SSH 日志解析器（纯函数）

**文件：** 创建 `agent/sshlog.go`、`agent/sshlog_test.go`

- [ ] **步骤 1：写失败测试** —— 覆盖成功/失败/invalid user：

```go
// agent/sshlog_test.go
package main
import "testing"
func TestParseSSHLine(t *testing.T) {
	cases := []struct{ line string; ok bool; success bool; user, ip, method string }{
		{"Jul 10 12:00:00 h sshd[1]: Accepted password for root from 1.2.3.4 port 22 ssh2", true, true, "root", "1.2.3.4", "password"},
		{"Jul 10 12:00:00 h sshd[1]: Accepted publickey for alice from 5.6.7.8 port 22 ssh2", true, true, "alice", "5.6.7.8", "publickey"},
		{"Jul 10 12:00:00 h sshd[1]: Failed password for root from 9.9.9.9 port 22 ssh2", true, false, "root", "9.9.9.9", "password"},
		{"Jul 10 12:00:00 h sshd[1]: Failed password for invalid user bob from 9.9.9.9 port 22 ssh2", true, false, "bob", "9.9.9.9", "password"},
		{"Jul 10 12:00:00 h sshd[1]: Server listening on 0.0.0.0 port 22", false, false, "", "", ""},
	}
	for _, c := range cases {
		ev, ok := parseSSHLine(c.line)
		if ok != c.ok { t.Fatalf("ok mismatch for %q: %v", c.line, ok) }
		if !ok { continue }
		if ev.Success != c.success || ev.User != c.user || ev.IP != c.ip || ev.Method != c.method {
			t.Errorf("解析错 %q -> %+v", c.line, ev)
		}
	}
}
```

- [ ] **步骤 2：确认失败** — `cd agent && go test ./ -run TestParseSSHLine -v`。

- [ ] **步骤 3：实现 `parseSSHLine`**

```go
// agent/sshlog.go
package main

import (
	"strings"
)

type SSHEvent struct {
	TS      int64  `json:"ts"`
	IP      string `json:"ip"`
	User    string `json:"user"`
	Method  string `json:"method"`
	Success bool   `json:"success"`
}

// parseSSHLine 从一行 auth.log/secure 提取 SSH 登录事件；非登录行 ok=false。
func parseSSHLine(line string) (SSHEvent, bool) {
	var ev SSHEvent
	idx := -1
	if i := strings.Index(line, "Accepted "); i >= 0 { ev.Success = true; idx = i + len("Accepted ") } else if i := strings.Index(line, "Failed "); i >= 0 { ev.Success = false; idx = i + len("Failed ") } else { return ev, false }
	rest := strings.Fields(line[idx:]) // [method for (invalid user)? USER from IP port N ...]
	if len(rest) < 5 { return ev, false }
	ev.Method = rest[0] // password/publickey
	// 找 "for" 后的用户名，处理 "invalid user X"
	fi := indexOf(rest, "for")
	if fi < 0 || fi+1 >= len(rest) { return ev, false }
	ui := fi + 1
	if rest[ui] == "invalid" && ui+2 < len(rest) && rest[ui+1] == "user" { ui += 2 }
	ev.User = rest[ui]
	// 找 "from" 后的 IP
	fmIdx := indexOf(rest, "from")
	if fmIdx < 0 || fmIdx+1 >= len(rest) { return ev, false }
	ev.IP = rest[fmIdx+1]
	return ev, true
}

func indexOf(ss []string, s string) int { for i, v := range ss { if v == s { return i } }; return -1 }
```

- [ ] **步骤 4：确认通过** — `cd agent && go test ./ -run TestParseSSHLine -v` PASS。
- [ ] **步骤 5：Commit** — `git commit -am "feat(agent): SSH 日志行解析器"`

### 任务 5.2：agent tail 采集 + 上报

**文件：** 修改 `agent/sshlog.go`（tail 循环）、`agent/main.go`（启动协程、`ssh_event` 消息）

- [ ] **步骤 1：实现 tail**（无自动化测试，逻辑审查 + 手测）

```go
// agent/sshlog.go 追加
import (
	"bufio"
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var sshLogPaths = []string{"/var/log/auth.log", "/var/log/secure"}

// startSSHCollector tail 第一个可读日志，解析新行并上报。
func startSSHCollector(conn *websocket.Conn, wm *sync.Mutex, agentID string) {
	path := firstReadable(sshLogPaths)
	if path == "" { return } // 无可读日志则静默退出
	f, err := os.Open(path)
	if err != nil { return }
	f.Seek(0, 2) // 从末尾开始，仅采集启动后新事件
	reader := bufio.NewReader(f)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			// 处理轮转：文件被截断则重开
			if fi, e := os.Stat(path); e == nil { if cur, _ := f.Seek(0, 1); fi.Size() < cur { f.Seek(0, 0) } }
			time.Sleep(2 * time.Second)
			continue
		}
		ev, ok := parseSSHLine(strings.TrimSpace(line))
		if !ok { continue }
		ev.TS = time.Now().Unix()
		b, _ := json.Marshal(ev)
		writeJSON(conn, wm, Message{Type: "ssh_event", AgentID: agentID, Data: string(b)})
	}
}

func firstReadable(paths []string) string {
	for _, p := range paths { if f, err := os.Open(p); err == nil { f.Close(); return p } }
	return ""
}
```
  - `agent/main.go`：在 stat 上报协程旁启动 `go startSSHCollector(conn, &writeMutex, agentID)`。

- [ ] **步骤 2：编译验证** — `cd agent && go build ./... && go vet ./...`，预期通过。
- [ ] **步骤 3：Commit** — `git commit -am "feat(agent): tail auth.log 采集 SSH 登录并上报"`

### 任务 5.3：master 存储 SSH 日志 + 周失败统计 + API

**文件：** 修改 `master/logs.go`（存取/统计/handler）、`master/main.go`（`handleAgentWS` 处理 `ssh_event`、注册路由）；测试 `master/logs_test.go`

- [ ] **步骤 1：写失败测试** —— 插入裁剪 200 + 周失败计数扣除重置基线：

```go
func TestSSHLogin_CapAndWeeklyFails(t *testing.T) {
	if err := openDB(filepath.Join(t.TempDir(), "t.db")); err != nil { t.Fatal(err) }
	defer db.Close()
	now := time.Now().Unix()
	for i := 0; i < 210; i++ { insertSSHLogin("n1", now, "1.1.1.1", "", "root", "password", false) }
	if got := len(listSSHLogin("n1")); got != 200 { t.Fatalf("应裁剪 200，got %d", got) }
	if wf := weeklySSHFails("n1", time.Now()); wf < 200 { t.Errorf("周失败数应≥200，got %d", wf) }
	resetSSHFails("n1", time.Now().Unix()+1) // 基线设为将来 → 计数归零
	if wf := weeklySSHFails("n1", time.Now()); wf != 0 { t.Errorf("重置后应为 0，got %d", wf) }
}
```

- [ ] **步骤 2：确认失败** — `go test ./ -run TestSSHLogin -v`。

- [ ] **步骤 3：实现**

```go
// logs.go
type SSHLog struct {
	TS int64 `json:"ts"`; IP string `json:"ip"`; Location string `json:"location"`; User string `json:"user"`; Method string `json:"method"`; Success bool `json:"success"`
}
func insertSSHLogin(agentID string, ts int64, ip, loc, user, method string, success bool) {
	s := 0; if success { s = 1 }
	db.Exec("INSERT INTO ssh_login(agent_id,ts,ip,location,username,method,success) VALUES(?,?,?,?,?,?,?)", agentID, ts, ip, loc, user, method, s)
	db.Exec("DELETE FROM ssh_login WHERE agent_id=? AND id NOT IN (SELECT id FROM ssh_login WHERE agent_id=? ORDER BY id DESC LIMIT 200)", agentID, agentID)
}
func listSSHLogin(agentID string) []SSHLog {
	rows, err := db.Query("SELECT ts,ip,location,username,method,success FROM ssh_login WHERE agent_id=? ORDER BY id DESC", agentID)
	if err != nil { return []SSHLog{} }
	defer rows.Close()
	out := []SSHLog{}
	for rows.Next() { var l SSHLog; var s int; rows.Scan(&l.TS, &l.IP, &l.Location, &l.User, &l.Method, &s); l.Success = s == 1; out = append(out, l) }
	return out
}
func clearSSHLogin(agentID string) { db.Exec("DELETE FROM ssh_login WHERE agent_id=?", agentID) }

func weeklySSHFails(agentID string, now time.Time) int {
	from := now.AddDate(0, 0, -7).Unix()
	var reset int64
	db.QueryRow("SELECT reset_at FROM ssh_fail_reset WHERE agent_id=?", agentID).Scan(&reset)
	if reset > from { from = reset }
	var n int
	db.QueryRow("SELECT count(*) FROM ssh_login WHERE agent_id=? AND success=0 AND ts>=?", agentID, from).Scan(&n)
	return n
}
func resetSSHFails(agentID string, at int64) {
	db.Exec("INSERT INTO ssh_fail_reset(agent_id,reset_at) VALUES(?,?) ON CONFLICT(agent_id) DO UPDATE SET reset_at=excluded.reset_at", agentID, at)
}
```
  - handler：`handleSSHLogs`（GET `?agent_id=` 列表 / DELETE 清空，operator）、`handleSSHStats`（GET：`[{agent_id, week_fails}]`，公开或 operator 均可，供卡片徽标）、`handleSSHFailReset`（POST `{agent_id}`，operator，调 `resetSSHFails(id, now)`）。
  - `main.go` `handleAgentWS` 的 `switch msg.Type` 加：
```go
		case "ssh_event":
			var ev struct{ TS int64 `json:"ts"`; IP, User, Method string; Success bool }
			if json.Unmarshal([]byte(msg.Data), &ev) == nil {
				insertSSHLogin(agentID, ev.TS, ev.IP, lookupLocation(ev.IP), ev.User, ev.Method, ev.Success)
			}
```
  - 注册路由：`/api/ssh-logs`、`/api/ssh-stats`、`/api/ssh-fails/reset`。

- [ ] **步骤 4：确认通过 + 全量** — `go test ./ -run TestSSHLogin -v` PASS；`go build ./... && go vet ./...` 通过。
- [ ] **步骤 5：Commit** — `git commit -am "feat: master 存储 SSH 登录日志、周失败统计与 API"`

### 任务 5.4：前端 SSH 日志 + 失败徽标 + 重置

**文件：** 修改 `web/src/types.ts`、`api.ts`、`store.ts`、`components/NodeDetailDrawer.vue`、`CardMode.vue`

- [ ] **步骤 1：类型 + api** —— `SSHLog` 类型；`NodeView` 加 `sshFailWeek?: number`；`api.ts` 加 `sshLogs(id)`、`clearSshLogs(id)`、`sshStats()`、`resetSshFails(id)`。
- [ ] **步骤 2：store** —— 新增 `loadSshStats()` 轮询 `/api/ssh-stats`（30s），把 `week_fails` 并入 `nodeViews`（类似 traffic）。
- [ ] **步骤 3：CardMode 徽标** —— 卡片头部 `card-head` 内、名字旁加：
```vue
<span v-if="(n.sshFailWeek||0) >= 3" class="sshbadge" :class="(n.sshFailWeek||0) >= 5 ? 'red' : 'yellow'">SSH失败 {{ n.sshFailWeek }}</span>
```
  样式：`.sshbadge.yellow{background:#f59e0b;color:#111} .sshbadge.red{background:#ef4444;color:#fff}`（小圆角徽标）。
- [ ] **步骤 4：NodeDetailDrawer SSH 段** —— 新增「SSH 登录」段：加载 `Api.sshLogs(nodeId)`，列表显示 时间/结果/用户/IP/地点；顶部「清空」按钮（`clearSshLogs`）与「重置周失败」按钮（`resetSshFails` 后刷新）。
- [ ] **步骤 5：构建验证** — `cd web && npx vite build`，预期成功。
- [ ] **步骤 6：Commit** — `git commit -am "feat: 前端 SSH 登录日志、失败徽标与重置"`

---

## 最终验证与收尾

- [ ] **后端全量** — `cd master && go build ./... && go vet ./... && go test ./ -v`；`cd agent && go build ./... && go vet ./... && go test ./ -v`。全部通过。
- [ ] **前端全量** — `cd web && npx vite build`，产出 `master/dist/assets/*`。
- [ ] **复制 E2E（若浏览器可用）** — 尝试 `cd web && npx playwright install chromium`；成功则写 `web/e2e/copy.spec.ts` 驱动 enroll 流程断言剪贴板==命令；失败则以 `clipboard.test.ts` 逻辑测试 + 审查交付，并标注。
- [ ] **端到端冒烟** — `cd master && go run .`，浏览器访问 banner 中 URL：验证 添加节点复制、主题下拉、表格中文、今日/本月入出、告警页默认开、登录日志页。（此项需你在有浏览器的环境执行，我给出清单。）
- [ ] **更新 spec 状态** — 规格首部状态改「已实现」。
- [ ] **合并** — 用 finishing-a-development-branch 技能决定 dev→main 的合并/PR 方式。

---

## 规格覆盖度自检

| 规格需求 | 覆盖任务 |
|---|---|
| #1 日流量展示 | 2.3 |
| #2 月流量持久化+每月1日 | 2.1, 2.2 |
| #3 真实告警+默认开 | 3.1, 3.2 |
| #4 复制 bug | 1.1 |
| #5 表格拉长 | 1.2 |
| #6 表头中文 | 1.2 |
| #7 主题下拉 | 1.3 |
| #8 分组按钮移位 | 1.4 |
| #9 入/出区分 | 2.1, 2.3 |
| #10 登录日志 | 4.1, 4.2, 4.3 |
| #11 SSH 日志 | 5.1, 5.2, 5.3, 5.4 |
| #12 SSH 失败周告警 | 5.3, 5.4 |
| SQLite 统一持久化+迁移 | 0.1–0.4 |
