# Ops Panel 增强设计规格

- 日期：2026-07-10
- 状态：已实现（dev 分支）
- 范围：监控面板一组增强 —— 流量展示/统计、告警、UI 微调、复制 bug、两个日志子系统、SQLite 统一持久化

---

## 1. 背景与目标

对现有 ops-panel（Go master + agent + Vue 前端）做一组增强，覆盖 12 个需求点，并借此把持久化统一迁移到 SQLite。

需求清单：

| # | 需求 | 类型 |
|---|------|------|
| 1 | 每台机器显示日流量（卡片 + 表格列） | 前端 |
| 2 | 月流量持久化 + 统一每月 1 日重置 | 后端 + 前端 |
| 3 | 真实告警（CPU/内存/磁盘/离线）+ 默认开启 | 后端 + 前端 |
| 4 | 添加节点「复制命令」bug 修复 | 前端 |
| 5 | 表格行拉长、不拥挤 | 前端 |
| 6 | 表格表头改中文 | 前端 |
| 7 | 主题切换圆点 → 下拉框 | 前端 |
| 8 | 「添加分组」按钮移到「添加节点」左侧相邻 | 前端 |
| 9 | 流量区分入站/出站 | 后端 + 前端 |
| 10 | 面板登录日志（IP/地点/设备，留 100，可清空） | 后端 + 前端 |
| 11 | 节点 SSH 登录日志（IP/地点/设备，每节点留 200，可清空） | agent + 后端 + 前端 |
| 12 | SSH 失败周告警（≥3 黄 / ≥5 红，显示周失败数，可重置） | agent + 后端 + 前端 |

## 2. 现状关键发现

- **流量**：`agent` 上报的是**速率**（B/s），master `RecordTraffic` 按区间积分成每日字节，存于**纯内存** `traffic` map（`main.go:484`），master 重启即丢、40 天清理。`TrafficDay` 已分别记 `Sent/Recv`，但 `TrafficStats` 对外把两者相加。日流量 `today` 后端已算、详情抽屉已显示，但卡片/表格未显示。
- **告警**：`alertConfig`（CPU/内存/磁盘/离线阈值 + `Enabled`）**从未被任何逻辑消费**——没有循环读实时指标比对阈值。唯一真实告警是流量超额（`checkTrafficQuota`），且不看 `Enabled`。当前告警页是装饰 UI。`AgentRecord.LastStat` 字段存在但运行时从不写入。
- **复制**：显示与复制都用同一个 `enrollCommand`，源码看似一致。面板走 HTTP（非安全上下文）→ `navigator.clipboard` 不可用 → 走 `execCommand` 兜底；enroll 表单在 Naive UI `NModal`（有焦点陷阱），`fallbackCopy` 的 textarea 挂在 `document.body`（模态框外），`ta.focus()` 被焦点陷阱夺回 → 选区失效 → 复制到错误/旧内容。
- **持久化**：现全为 JSON 文件（agents/groups/monitors/alerts + history 内存）。无数据库。
- **geoip**：`geoip.go` 用 ip-api.com（免费无 key，45 req/min），目前只取 countryCode，可扩展取 city。

## 3. 架构决策

### 3.1 SQLite 统一持久化（用户明确要求：能进库且不影响性能的都进库，统一管理 + 便于迁移）

- 引入 `modernc.org/sqlite`（**纯 Go，无 CGO**），新增 `master/db.go`，单库文件 `ops-panel.db`（权限 0600）。
- 现有 JSON 配置全部迁入 SQLite 表，但**保留内存缓存**（`agents` map / `groupsList` / `monitors` map / `alertConfig`）作为热读路径 → **读性能不变**；把原「写 JSON 文件」改为「写穿 SQLite」。
- **一次性迁移**：启动时若 DB 对应表为空且旧 JSON 存在 → 导入，之后把 JSON 重命名为 `*.bak`（幂等、平滑升级）。
- `loadX/saveX` 函数签名尽量不变，仅换实现，控制改动面。
- 遥测/日志类新数据（流量日累计、登录日志、SSH 日志）直接建表存 SQLite。

### 3.2 SSH 日志来源：解析 `/var/log/auth.log` | `/var/log/secure`

- agent 侧 tail 解析（首个存在且可读者），提取 SSH 成功/失败事件，上报 master。需 agent 有读权限（通常 root，install.sh 即 root 安装）。不可读时打印告警并跳过。

### 3.3 其它已定解释

- 登录日志「地点」= ip-api 升级到城市级（国家·城市）；「设备」= 解析 User-Agent（浏览器/系统）。
- SSH 日志无 UA，「设备」列显示 **用户名 + 认证方式**（如 `root · password`）。
- 面板登录日志记录**成功登录**（失败尝试列为后续增强，非本次目标）。
- SSH「每周」= **滚动 7 天**窗口；重置 = 记录该节点重置时间基线，计数仅统计基线之后。

## 4. 数据模型（SQLite schema）

```sql
-- 配置（迁移自 JSON；内存缓存 + 写穿）
CREATE TABLE agents   (agent_id TEXT PRIMARY KEY, secret TEXT NOT NULL, name TEXT,
                       agent_ver TEXT, prefs TEXT NOT NULL);         -- prefs = AgentPreferences JSON
CREATE TABLE groups   (name TEXT PRIMARY KEY, ord INTEGER NOT NULL); -- ord 保序
CREATE TABLE monitors (id TEXT PRIMARY KEY, data TEXT NOT NULL);     -- data = Monitor JSON
CREATE TABLE alerts   (id INTEGER PRIMARY KEY CHECK(id=1), data TEXT NOT NULL);

-- 遥测/日志
CREATE TABLE traffic_daily (agent_id TEXT NOT NULL, date TEXT NOT NULL,
                            sent INTEGER NOT NULL DEFAULT 0, recv INTEGER NOT NULL DEFAULT 0,
                            PRIMARY KEY(agent_id, date));
CREATE TABLE panel_login (id INTEGER PRIMARY KEY AUTOINCREMENT, ts INTEGER NOT NULL,
                          ip TEXT, location TEXT, device TEXT, username TEXT);
CREATE TABLE ssh_login (id INTEGER PRIMARY KEY AUTOINCREMENT, agent_id TEXT NOT NULL,
                        ts INTEGER NOT NULL, ip TEXT, location TEXT, username TEXT,
                        method TEXT, success INTEGER NOT NULL);
CREATE INDEX idx_ssh_login_agent_ts ON ssh_login(agent_id, ts);
CREATE TABLE ssh_fail_reset (agent_id TEXT PRIMARY KEY, reset_at INTEGER NOT NULL);
```

保留策略：`panel_login` 留最新 100；`ssh_login` 每 agent 留最新 200（插入后按 id 降序裁剪）；`traffic_daily` 清理 40 天前。

## 5. 分阶段详细设计

### 阶段 0 — SQLite 基础（前置）
- `master/db.go`：`openDB()`、建表、`migrateFromJSON()`。`main.go` 启动时打开 DB 并迁移。
- 改写 `auth.go`/`monitors.go`/`main.go` 里的 load/save：agents、groups、monitors、alerts → SQLite。
- 单元测试：每类配置 save→load 往返；迁移导入。

### 阶段 1 — 前端微调 + 复制修复（纯前端，本环境可构建验证）
- **#4 复制**：重写 `copyText/fallbackCopy` —— ①安全上下文优先 `navigator.clipboard.writeText`；②兜底把 textarea 挂到当前模态框 DOM（`.n-modal` 容器）内，绕过焦点陷阱，`select()+setSelectionRange()`，复制后恢复原选区；③去掉 `doEnroll` 里 `await` 之后的静默自动复制，保留「复制命令」按钮与点击命令框两条明确手势路径。
- **#5** 表格 `.row` 增大纵向 padding/行高、留白。
- **#6** 表头中文：系统 / CPU / 内存 / 磁盘 / 网络 / 客户端 / 今日流量。
- **#7** 主题：`.themes` 圆点 → `NSelect` 下拉框（选项 = THEMES）。
- **#8** 「添加分组」按钮从侧栏移到列表头 `.lhead`，紧挨「添加节点」左侧。

### 阶段 2 — 流量：入/出区分 + 日流量 + 持久化
- 后端：
  - `traffic` 改由 `traffic_daily` 支撑：启动 load 近 40 天入内存 map；`RecordTraffic` 仍写内存；新增 `trafficPersistLoop`（60s）UPSERT 落库。
  - `TrafficStats` 拆分字段：`today_sent/today_recv/month_sent/month_recv`（保留 `today/this_month` 合计以兼容）。
  - 移除自定义重置日：删 `cycleStartDate`、`AgentPreferences.TrafficResetDay` 及引用；cycle 一律自然月；`checkTrafficQuota` 周期键 `YYYY-MM`。
- 前端：
  - `TrafficStats`/`NodeView` 类型加 in/out 字段；卡片显示「今日 出↑ X / 入↓ Y」「本月 出/入」；表格加「今日流量」列（默认开，可切换）。
  - `NodeEditModal` 删除「重置日」字段与 `traffic_reset_day` 引用。

### 阶段 3 — 真实告警 + 默认开启
- 后端：
  - `ingestStat` 写入包级 `latestStat[agentID]={cpu,mem,disk}` 与 `lastSeen[agentID]=now`（`lastSeen` 断连不删，供离线判断）。
  - `alertLoop`（60s）：`alertConfig.Enabled` 时逐节点比对阈值与离线时长，越阈值 `sendTGAlert`；按 `agentID+指标` 去重（firing/恢复状态机，恢复后可再报）。
  - 默认 `alertConfig` 初始 `Enabled:true`。
- 前端：store 默认 `enabled:true`。
- 说明：投递沿用 TG（需 `TG_TOKEN`+`TG_ADMIN_IDS`），与现有流量告警一致；未配 TG 时循环照跑不投递。

### 阶段 4 — 面板登录日志
- 后端：`handleLogin` 成功后记录 `{ts, ip=clientIP, location=lookupLocation(ip), device=parseUA(ua), username}` → `panel_login`（裁剪 100）；`GET /api/login-logs`、`DELETE /api/login-logs`（operator 鉴权）。
- 前端：新增导航页「登录日志」（operator 可见），表格展示 + 清空按钮。

### 阶段 5 — 节点 SSH 日志 + 失败周告警
- agent：
  - 新增 SSH 日志采集：打开 `/var/log/auth.log`｜`/var/log/secure`（首个可读），启动 seek 到末尾，tail 新行；处理日志轮转（文件变小则重开）。
  - 解析：成功 `Accepted <method> for <user> from <ip>`；失败 `Failed password for [invalid user] <user> from <ip>` / `Failed publickey`。
  - 上报 `ssh_event`：`{ts, ip, user, method, success}`。
- 后端：
  - 处理 `ssh_event`：`location=lookupLocation(ip)` → `ssh_login`（每 agent 裁剪 200）。
  - 周失败数：`SELECT COUNT(*) FROM ssh_login WHERE agent_id=? AND success=0 AND ts>=max(now-7d, reset_at)`。
  - API：`GET /api/ssh-logs?agent_id=`、`DELETE /api/ssh-logs?agent_id=`、`GET /api/ssh-stats`（各节点周失败数）、`POST /api/ssh-fails/reset {agent_id}`（写 `ssh_fail_reset`）。
- 前端：
  - `NodeDetailDrawer` 加「SSH 登录」列表（近若干条）+ 清空按钮。
  - 卡片加每周 SSH 失败徽标：≥3 黄 / ≥5 红，显示次数；详情抽屉内提供「重置」按钮。
  - store 轮询 `/api/ssh-stats` 并入 `NodeView`。

## 6. 验证策略（含环境现实）

本环境**无 Go 工具链、无浏览器**。执行时：

1. **优先尝试安装 Go**（apt / 官方 tar）与 chromium（`npx playwright install`）。
2. 若 Go 可用：全后端 `go build ./...`、`go vet ./...`、`go test ./...`（DB 往返、SSH 解析器、周失败计数、迁移、告警阈值判定等纯逻辑单测）。
3. 若 Go 不可用：后端仅代码审查，**逐项标注"未编译验证"**，并在交付说明中给出你本地命令：`go mod tidy && go build ./... && go test ./...`。
4. 前端：`vite build` 必跑通；复制 bug 若有 chromium 则 Playwright 端到端断言剪贴板==命令，否则逻辑单测（mock clipboard）+ 审查。

> 遵循「验证先于完成」：任何未能实测的项，明确标注验证方式，不谎报通过。

## 7. 非目标（YAGNI）

- 不迁移 history 时序到 DB（保持内存 best-effort，量大且非本次需求）。
- 不实现面板内告警信息流/站内信（告警沿用 TG）。
- 不记录面板失败登录尝试（本次仅成功登录）。
- 不做 SSH 日志的全量历史回填（agent 启动后向前采集）。
- 不引入 ORM / 迁移框架（手写建表 + 版本常量足够）。

## 8. 风险

- **后端不可在此编译**：SQLite 迁移 + agent SSH 解析是最大不可实测面，依赖仔细审查与你本地构建。
- SQLite 依赖树较大（纯 Go），首次 `go mod tidy` 需联网拉取。
- auth.log 格式随发行版/sshd 版本略有差异；解析器需容错并覆盖常见格式，配单测样本。
