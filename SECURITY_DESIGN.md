# Ops-Panel 安全架构设计（防越权 / 防横向扩散）

> 设计目标：
> 1. 公开大屏可看机器状态，但**任何人 F12 都无法下发命令**。
> 2. 一台 VPS 被黑客控制，**不能借此控制其他 VPS**（限制爆炸半径）。
> 3. 每台机器可自定义上报频率，避免高频浪费流量 / 被误判为攻击。
> 4. 吸取教训：内网穿透被入侵、哪吒漏洞被入侵 → 控制面不裸奔、全链路认证、命令签名。

---

## 一、核心原则：三权分立 + 每机一密

把连接分为三种**身份（Role）**，服务端在 WebSocket 握手（upgrade）阶段就根据凭证判定角色，**不同角色能做的事严格隔离**：

| 角色 | 凭证 | 能力 | 对应端 |
|------|------|------|--------|
| `agent` | 自身 `agent_secret` | 上报状态、**仅接收发给自己的命令** | 被监控的 VPS |
| `viewer` | 无 / 仅 view 令牌 | **只能接收广播**，不能发任何消息 | 公开大屏页面 |
| `operator` | `operator_token` | 向指定 agent 下发命令 | 运维指挥台（必须登录） |

**关键防线**：`viewer` 连接即使构造 `{type:"cmd"}` 消息发给服务端，服务端直接**丢弃**——因为该连接的角色没有 command 权限。F12 破解在这层就失效，浏览器里根本没有 `operator_token`。

---

## 二、每机一密（防一台被黑拖全体）

### 注册 / 安装流程（服务端生成密钥）

```
运维在 Master 控制台点击「新增节点」
        │
        ▼
Master 生成：agent_id = uuid()
             agent_secret = 32字节随机  (服务端留存，用于验签+命令签名)
        │
        ▼
返回一行安装指令：
  curl -sSL https://host/install.sh | \
    AGENT_ID=uuid AGENT_SECRET=sec MASTER=wss://host sh
        │
        ▼
运维把指令粘贴到目标 VPS 执行 → Agent 带着自己的 secret 连接
```

Agent 首次连接：`/ws/agent?id=uuid&token=sec`
Master 用**常量时间比较**校验 token，通过才注册。

### 为什么能限制爆炸半径

- 每个 agent 的 `secret` 独立，泄露一个 = 只能动那一台。
- agent 在 map 中只能访问自己的连接，**无法枚举或给其他 agent 发命令**。
- 即使黑客完全控制某台 VPS，他手里的只有这台的 secret，动不了别的机器。

### 命令签名（防伪造 / MITM）

Master 下发命令时，用该 agent 的 `secret` 做 HMAC 签名：

```json
{
  "type": "cmd",
  "agent_id": "uuid",
  "data": "df -h",
  "nonce": "1712345678",
  "sig": "hmac_sha256(secret, data+nonce)"
}
```

Agent 端**先验签再执行**，签名不符直接丢弃。这样：
- 流量被截获也无法伪造命令（没有 secret 算不出 sig）；
- 即使 Master 被部分入侵，攻击者没有某 agent 的 secret 也伪造不了命令。

> 代价：Master 需保存各 agent 的 secret（用于签名）。这是为"限爆炸半径"做的取舍——防的是**agent 侧被黑**，不是 Master 被黑（Master 被黑无可避免，需靠审计+吊销兜底）。

---

## 三、刷新频率可配 + 服务端限速（防流量浪费 / 误判攻击）

### 每台机器独立频率

```go
// Master 侧每 agent 配置
type AgentConfig struct {
    ID       string
    Secret   string // 留存用于验签
    Interval time.Duration // 上报间隔，默认 5s，范围 1s~60s
}
```

- operator 在后台为每台机器设置 `Interval`，Master 在 agent 连接时下发。
- Agent 按 `Interval` 上报，**不信任客户端自报频率**。

### 服务端速率校验（防被控后高频刷）

```go
// 收到 stat 时检查
minGap := cfg.Interval / 2
if time.Since(lastStatTime[agentID]) < minGap {
    return // 丢弃，防被控机器狂刷流量 / 伪装成攻击
}
```

### 避免被外部平台误判

- 统一 `User-Agent`，控制并发连接数；
- `Interval` 设下限（如 ≥1s），避免被当成扫描/攻击；
- 可选：agent→master 走 `wss://` + 证书指纹固定（pinned fingerprint），防 MITM。

---

## 四、传输与部署安全

- **强制 `wss://`**：Agent 与 Web 全部 TLS。前端 `wsUrl()` 已按 `location.protocol` 自动切 `wss`，Agent 需显式支持 `wss`。
- **Master 不裸奔**：`ListenAndServe` 不直连公网，必须前置反向代理（Nginx/Caddy）+ 防火墙，仅暴露 443。
- **控制面与数据面分离**：`/ws/agent`、`/ws/web(viewer)`、`/ws/operator` 三个端点，权限各不同。

---

## 五、命令执行的纵深防御

| 控制点 | 措施 |
|--------|------|
| 通道 | 仅 `operator` 角色可发命令，`viewer` 消息直接丢弃 |
| 身份 | `operator_token` + 指挥台登录（公开看板不登录，但指挥台必须登录） |
| 签名 | 命令 HMAC 签名，agent 验签后才执行 |
| 白名单 | agent 端可设命令 allowlist / 只读模式，危险命令（`rm -rf`、`curl|sh`）默认拦截或二次确认 |
| 超时 | `exec.CommandContext` + 30s 超时，防挂起占连接 |
| 输出 | 截断上限（如 64KB/行、总量上限），防内存爆 |
| 审计 | 记录「谁、何时、对哪台、发了什么、结果」 |
| 吊销 | operator 后台一键吊销某 agent secret，不影响其他 |

---

## 六、被入侵后的收敛

- **一键吊销**：某 agent secret 泄露 → 后台吊销，该 agent 立即失去注册资格，其他 unaffected。
- **审计溯源**：命令日志可追溯哪台最先异常、谁发的命令。
- **最小权限**：公开看板对 viewer **脱敏**（隐藏内网 IP、hostname 等敏感字段），只看资源水位。

---

## 七、实施优先级

| 优先级 | 项 |
|--------|----|
| P0 | 三角色 WS（agent/viewer/operator）+ upgrade 阶段鉴权；viewer 的 cmd 丢弃 |
| P0 | 每机一密注册流程（enroll API + 安装指令生成） |
| P0 | 命令 HMAC 签名 + agent 验签 |
| P1 | 指挥台登录（operator_token 不进公开页面） |
| P1 | 每机 `Interval` 可配 + 服务端速率校验 |
| P2 | 命令 allowlist / 只读模式 / 超时 / 输出上限 |
| P2 | 审计日志 + 一键吊销 |
| P3 | 证书指纹固定、viewer 脱敏 |

---

## 八、与现有代码的映射（改造点）

| 现有代码 | 改造 |
|----------|------|
| `master/main.go:32` `CheckOrigin:true` | 改为按角色鉴权 + `CheckOrigin` 校验同源 |
| `master/main.go:136` `handleAgentWS` | 增加 `token` 校验（常量时间）、`SafeConn` 并发写、速率校验 |
| `master/main.go:186` `handleWebWS` | 拆成 viewer/operator 两路；viewer 丢弃 cmd |
| `agent/main.go:51` 连接 | 带 `agent_secret`；支持 `wss`；按下发 `Interval` 上报 |
| `agent/main.go:191` `executeCommand` | 验签 + 超时 + 输出截断 + allowlist |
| `web/index.vue` | 看板用 viewer WS（无 token）；指挥台单独 operator 连接（登录后） |
| `install.sh` | 改为接收 `AGENT_ID/AGENT_SECRET/MASTER` 参数 |
