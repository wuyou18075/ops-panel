# Ops-Panel 安全审计与优化方案

> 审查范围：`master/main.go`、`agent/main.go`、`web/src/views/dashboard/index.vue`、`master/static/index.html`、`install.sh`、`README.md`
> 审查结论：**当前版本不可直接暴露到公网**，存在命令执行未授权、WebSocket 并发写崩溃、节点永不离线等多处高危问题。

---

## 一、安全漏洞（按严重度排序）

### 🔴 1. 任意命令执行，零身份认证（Critical）

**现象**
- `master/main.go:32` `CheckOrigin` 直接返回 `true`，任意来源都能连 WebSocket。
- `handleWebWS`（`master/main.go:186`）对 `/ws/web` **不做任何 token 校验**，任何人连上即可向 Agent 下发 shell 命令。
- `agent/main.go:191` `executeCommand` 直接 `exec.Command("sh", "-c", command)` 执行任意命令。
- `initTGBot` 的 `/cmd` 处理器（`master/main.go:245`）不校验 ChatID，任何知道 bot 的人都能发命令。

**风险**：公网部署 = 把一台可任意执行 `rm -rf`、`curl|sh` 的服务器裸奔暴露。

**修复**：引入共享密钥（Token），Agent 注册与 Web 连接都必须携带。

```go
// master/main.go —— 在 upgrader 之后增加鉴权校验辅助函数
func authRequired(r *http.Request) (string, bool) {
    token := r.URL.Query().Get("token")
    if token == "" {
        // 兼容从 Header 取（Web 端浏览器 WS 不易带 query，可放 Header）
        token = r.Header.Get("X-Auth-Token")
    }
    return token, token == os.Getenv("PANEL_TOKEN")
}

func handleWebWS(w http.ResponseWriter, r *http.Request) {
    if _, ok := authRequired(r); !ok {
        http.Error(w, "未授权", http.StatusUnauthorized)
        return
    }
    // ... 原有逻辑
}
```

Agent 侧连接时也要带 token（`agent/main.go:51`）：

```go
u := url.URL{
    Scheme:   "ws",
    Host:     masterURL(),
    Path:     "/ws/agent",
    RawQuery: "id=" + agentID() + "&token=" + os.Getenv("PANEL_TOKEN"),
}
```

> `.env` / 启动脚本中通过 `PANEL_TOKEN` 注入，且**绝不进版本库**。

### 🔴 2. WebSocket 并发写导致 panic（Critical，也是 bug）

**现象**：`broadcastToWeb`（`master/main.go:224`）由多个 Agent 读循环并发调用，同时 `handleWebWS` 中 `agentConn.WriteMessage` 也可能并发写同一个连接。`gorilla/websocket` **不允许多 goroutine 同时写同一个连接**，必然触发 `concurrent write to websocket connection` panic。

**修复**：为每个连接封装带互斥锁的写方法，统一出口。

```go
type SafeConn struct {
    *websocket.Conn
    mu sync.Mutex
}

func (c *SafeConn) Write(msg []byte) error {
    c.mu.Lock()
    defer c.mu.Unlock()
    return c.Conn.WriteMessage(websocket.TextMessage, msg)
}
```

把 `agentConns` / `webConns` 的值类型从 `*websocket.Conn` 改为 `*SafeConn`，所有 `WriteMessage` 改为 `SafeConn.Write`。

### 🟠 3. 明文传输（High）

**现象**：Agent→Master、Web→Master 全部 `ws://`。命令与输出（可能含密钥、日志）在网络上明文。

**修复**：Master 前挂 HTTPS / 反代，`agent/main.go` 与前端 `wsUrl()` 改用 `wss://`（前端已根据 `location.protocol` 自动切换，Agent 需显式支持 `wss` + 跳过自签证书校验可选）。

### 🟠 4. 命令无白名单 / 无超时 / 输出无上限（High）

- Agent 对任何命令照单全收；
- `executeCommand`（`agent/main.go:191`）无 context 超时，挂起命令（如 `ping`、`cat`）永久占连接；
- `bufio.Scanner` 单行默认 64KB 限制，超长行被静默截断丢失。

**修复**：加超时 + 输出截断。

```go
func executeCommand(conn *websocket.Conn, wm *sync.Mutex, agentID, command string) {
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    cmd := exec.CommandContext(ctx, "sh", "-c", command)
    // ...
}
```

### 🟡 5. `master/static/index.html` 存在 XSS（Medium）

**现象**：`card.innerHTML = ... ${agentId}` 直接拼接不可信输入（Agent 自报 ID）。

**修复**：该文件已被 Vue 版 `dist` 取代，建议删除 `master/static/` 或改用 `textContent`；Vue 版用 `{{ }}` 渲染，默认转义，不受影响。

### 🟡 6. TG Bot 无 ChatID 白名单（Medium）

**修复**：`/cmd` 处理器中校验 `c.Chat().ID` 是否在管理员列表（环境变量 `TG_ADMIN_IDS`）内。

---

## 二、功能性 Bug

### 🔴 1. 同 ID Agent 互相覆盖，数据静默丢失

`handleAgentWS`（`master/main.go:150`）用 `agentConns[agentID] = conn` 直接覆盖；前者断开时 `delete` 会误删后者的连接。

**修复**：注册时若 ID 已存在，拒绝或追加后缀：

```go
agentMutex.Lock()
if _, exists := agentConns[agentID]; exists {
    agentMutex.Unlock()
    conn.Close()
    return // 或返回 409
}
agentConns[agentID] = &SafeConn{Conn: conn}
agentMutex.Unlock()
```

### 🔴 2. 前端节点永不标记离线（显著体验问题）

Agent 断开后 Master 仅从 map 删除，前端 `nodes` 字典保留旧数据，`onlineCount` 一直虚高，无心跳超时。

**修复**：前端增加最后更新时间阈值，超过 N 秒未刷新则标记离线。

```ts
// index.vue
const ONLINE_TIMEOUT = 10_000;
const onlineCount = computed(() =>
  Object.values(nodes.value).filter(
    (n) => Date.now() - n.updatedAt < ONLINE_TIMEOUT,
  ).length,
);
```

### 🟠 3. 前端 `JSON.parse` 无异常保护

`ws.onmessage`（`index.vue:315`）解析失败会抛异常，中断整个消息处理。

**修复**：`try { ... } catch { message.error("数据解析失败") }`。

### 🟠 4. `terminalLogs` 无上限，内存泄漏

`eventLogs` 有 `slice(0,80)`，但 `terminalLogs` 无限增长。

**修复**：`terminalLogs.value = terminalLogs.value.slice(-500)`。

### 🟡 5. `netSampler.rate()` 运算符优先级陷阱

`agent/main.go:175`：
```go
if s.lastTime.IsZero() || elapsed <= 0 || s.lastSent == 0 && s.lastRecv == 0 {
```
`&&` 优先级高于 `||`，逻辑虽大多正确，但建议显式加括号避免歧义。

### 🟡 6. Master 监听 `:8080` 无说明

`http.ListenAndServe(":8080", nil)` 绑定所有网卡，配合无认证极其危险。文档应明确：生产环境必须置于防火墙 / 反代之后，且启用 Token。

### 🟡 7. `install.sh` 以 root 运行敏感操作

脚本含 `rm -rf /usr/local/go`、`apt-get`、全局 `npm install -g`，需 root 且无可回滚。建议脚本开头声明需求，并对 `rm -rf` 目标加存在性校验。

---

## 三、建议实施优先级

| 优先级 | 项 | 类型 |
|--------|----|------|
| P0 | WS 认证（Token） | 安全 |
| P0 | SafeConn 并发写封装 | 崩溃修复 |
| P1 | 命令执行超时 + 输出限制 | 安全/稳定 |
| P1 | 前端离线检测 + JSON 异常保护 | 功能 |
| P2 | 同 ID Agent 冲突处理 | 稳定 |
| P2 | TG ChatID 白名单 | 安全 |
| P3 | 删除 `master/static` XSS 旧页面 | 安全 |
| P3 | `install.sh` 加固 | 运维 |

---

## 四、落地方式建议

1. **P0 两项**必须一起做（认证 + 并发写），否则暴露即崩溃或裸奔。
2. Agent 与 Master 的 `Message` 结构、Token 传递需同步修改，建议放在同一个 commit。
3. 前端需配合在 `connectWebSocket` 时附带 `?token=`，否则连不上新的 Master。

> 注：本文档仅作审计与方案说明，未改动任何源码。如需我直接落地 P0/P1 修复，请告知，我将按上面代码片段逐文件修改并通过测试。
