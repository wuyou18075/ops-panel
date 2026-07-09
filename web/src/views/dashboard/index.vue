<template>
  <NLayout class="min-h-screen bg-canvas text-slate-100" has-sider>
    <NLayoutSider bordered collapse-mode="width" :collapsed-width="72" :width="224" class="hidden bg-panel md:block">
      <div class="flex h-16 items-center gap-3 border-b border-line px-5">
        <div class="grid h-9 w-9 place-items-center rounded bg-emerald-500 text-sm font-black text-white">OP</div>
        <div>
          <div class="text-sm font-semibold text-white">Ops Panel</div>
          <div class="text-xs text-slate-400">Soybean Admin</div>
        </div>
      </div>
      <div class="space-y-1 p-3">
        <div class="rounded bg-emerald-500/15 px-3 py-2 text-sm font-medium text-emerald-300 cursor-pointer" @click="activePage = 'dashboard'">监控概览</div>
        <div class="rounded px-3 py-2 text-sm text-slate-400 cursor-pointer hover:bg-white/5" @click="activePage = 'nodes'">节点列表</div>
        <div class="rounded px-3 py-2 text-sm text-slate-400 cursor-pointer hover:bg-white/5" @click="activePage = 'terminal'">命令终端</div>
        <div class="rounded px-3 py-2 text-sm text-slate-400 cursor-pointer hover:bg-white/5" @click="activePage = 'alerts'">告警策略</div>
      </div>
      <!-- 分组切换 -->
      <div class="border-t border-line p-3">
        <div class="mb-2 flex items-center justify-between">
          <span class="text-xs text-slate-400">节点分组</span>
          <NButton size="tiny" tertiary @click="showAddGroup = true">+</NButton>
        </div>
        <div v-for="g in groups" :key="g" class="mb-1 cursor-pointer rounded px-2 py-1 text-xs"
          :class="selectedGroup === g ? 'bg-emerald-500/20 text-emerald-300' : 'text-slate-400 hover:bg-white/5'"
          @click="selectedGroup = g">
          {{ g }}
        </div>
      </div>
    </NLayoutSider>

    <NLayout>
      <NLayoutHeader bordered class="sticky top-0 z-10 bg-panel/95 backdrop-blur">
        <div class="flex flex-col gap-3 px-4 py-4 lg:flex-row lg:items-center lg:justify-between lg:px-6">
          <div>
            <div class="flex items-center gap-3">
              <span class="h-2.5 w-2.5 rounded-full bg-emerald-400 shadow-[0_0_16px_rgba(52,211,153,0.9)]" />
              <h1 class="text-xl font-semibold text-white">{{ pageTitle }}</h1>
            </div>
            <p class="mt-1 text-sm text-slate-400">{{ pageSubtitle }}</p>
          </div>
          <NSpace>
            <NTag :type="wsConnected ? 'success' : 'error'" round>
              {{ wsConnected ? "主控已连接" : "主控重连中" }}
            </NTag>
            <NTag type="info" round>在线 {{ onlineDisplay }}</NTag>
            <NButton size="small" tertiary @click="connectViewerWS(true)">刷新连接</NButton>
            <NButton v-if="operatorToken" size="small" type="success" ghost @click="showLogin = true">运维登录中</NButton>
            <NButton v-else size="small" type="primary" ghost @click="showLogin = true">登录</NButton>
          </NSpace>
        </div>
      </NLayoutHeader>

      <NLayoutContent class="p-4 lg:p-6">
        <!-- 监控概览页 -->
        <template v-if="activePage === 'dashboard'">
          <NGrid :cols="4" :x-gap="16" :y-gap="16" responsive="screen" item-responsive>
            <NGi span="4 s:2 l:1">
              <MetricCard label="在线节点" :value="String(onlineCount)" hint="过去 10 秒内有上报的 Agent" tone="green" />
            </NGi>
            <NGi span="4 s:2 l:1">
              <MetricCard label="平均 CPU" :value="`${avgCpu.toFixed(1)}%`" hint="所有在线节点" tone="blue" />
            </NGi>
            <NGi span="4 s:2 l:1">
              <MetricCard label="平均内存" :value="`${avgMem.toFixed(1)}%`" hint="所有在线节点" tone="violet" />
            </NGi>
            <NGi span="4 s:2 l:1">
              <MetricCard label="告警节点" :value="String(alertCount)" hint="CPU/内存/磁盘 > 80%" tone="red" />
            </NGi>
          </NGrid>
          <div class="mt-5 grid gap-5 xl:grid-cols-[1fr_380px]">
            <NCard title="节点资源总览" :bordered="false" class="border border-line bg-panel">
              <NDataTable :columns="columns" :data="filteredTableData" :bordered="false" :pagination="{ pageSize: 8 }" size="small" />
              <NEmpty v-if="filteredTableData.length === 0" class="py-12" description="等待 Agent 上报监控数据" />
            </NCard>
            <NCard title="实时事件流" :bordered="false" class="border border-line bg-panel">
              <div class="h-[420px] space-y-3 overflow-y-auto pr-1">
                <div v-if="eventLogs.length === 0" class="rounded border border-dashed border-line p-5 text-center text-sm text-slate-500">
                  暂无事件，连接 Agent 后会显示上线、状态和命令输出
                </div>
                <div v-for="(item, index) in eventLogs" :key="index" class="rounded border border-line bg-black/20 p-3">
                  <div class="flex items-center justify-between text-xs text-slate-500">
                    <span>{{ item.agentId }}</span>
                    <span>{{ item.time }}</span>
                  </div>
                  <div class="mt-1 font-mono text-xs text-slate-200">{{ item.text }}</div>
                </div>
              </div>
            </NCard>
          </div>
        </template>

        <!-- 节点列表页 -->
        <template v-if="activePage === 'nodes'">
          <div class="mb-4 flex items-center justify-between">
            <span class="text-lg font-semibold text-white">全部节点（{{ filteredTableData.length }}）</span>
            <NButton type="primary" size="small" @click="showEnroll = true">+ 添加节点</NButton>
          </div>
          <NCard :bordered="false" class="border border-line bg-panel">
            <NDataTable :columns="nodeColumns" :data="filteredTableData" :bordered="false" :pagination="{ pageSize: 15 }" size="small" />
          </NCard>
        </template>

        <!-- 命令终端页 -->
        <template v-if="activePage === 'terminal'">
          <div class="mb-4">
            <span class="text-lg font-semibold text-white">命令终端</span>
            <p class="mt-1 text-sm text-slate-400">选择一个节点，分发执行 Shell 命令</p>
          </div>
          <NGrid :cols="4" :x-gap="16" responsive="screen" item-responsive>
            <NGi span="4 l:1">
              <NCard title="在线节点" :bordered="false" class="border border-line bg-panel">
                <div v-for="node in filteredTableData" :key="node.id" class="mb-2 cursor-pointer rounded p-2 text-sm"
                  :class="activeNodeId === node.id ? 'bg-emerald-500/20 text-emerald-300' : 'text-slate-400 hover:bg-white/5'"
                  @click="selectTerminalNode(node.id as string)">
                  {{ node.id }}
                </div>
                <NEmpty v-if="filteredTableData.length === 0" description="暂无可用节点" />
              </NCard>
            </NGi>
            <NGi span="4 l:3">
              <NCard :title="`终端 - ${activeNodeId || '未选择'}`" :bordered="false" class="border border-line bg-panel">
                <template v-if="operatorToken && activeNodeId">
                  <div class="mb-4 grid gap-3">
                    <NInputGroup>
                      <NInput v-model:value="shellCommand" placeholder="输入 Shell 命令，例如 df -h" @keyup.enter="sendShellCommand" />
                      <NButton type="primary" :disabled="!canSendCommand" @click="sendShellCommand">执行</NButton>
                    </NInputGroup>
                    <NSpace>
                      <NButton v-for="cmd in quickCommands" :key="cmd" size="small" tertiary @click="shellCommand = cmd">{{ cmd }}</NButton>
                    </NSpace>
                  </div>
                  <div class="h-[400px] overflow-y-auto rounded bg-black p-4 font-mono text-xs leading-6 text-emerald-300">
                    <div v-if="terminalLogs.length === 0" class="text-slate-500">等待输入命令...</div>
                    <div v-for="(log, i) in terminalLogs" :key="i">{{ log }}</div>
                  </div>
                </template>
                <template v-else>
                  <NEmpty v-if="!operatorToken" description="请先点击右上角「登录」完成认证" />
                  <NEmpty v-else description="请从左侧选择一个节点" />
                </template>
              </NCard>
            </NGi>
          </NGrid>
        </template>

        <!-- 告警策略页 -->
        <template v-if="activePage === 'alerts'">
          <div class="mb-4">
            <span class="text-lg font-semibold text-white">告警策略</span>
            <p class="mt-1 text-sm text-slate-400">设置监控触发条件，超阈值时通知</p>
          </div>
          <NCard :bordered="false" class="border border-line bg-panel">
            <NSpace vertical>
              <div class="flex items-center gap-4">
                <span class="w-32 text-sm text-slate-400">启用告警</span>
                <NSwitch v-model:value="alertCfg.enabled" />
              </div>
              <div class="flex items-center gap-4">
                <span class="w-32 text-sm text-slate-400">CPU 阈值</span>
                <NInputNumber v-model:value="alertCfg.cpu_percent" :min="1" :max="100" class="w-24" />
                <span class="text-xs text-slate-500">%</span>
              </div>
              <div class="flex items-center gap-4">
                <span class="w-32 text-sm text-slate-400">内存阈值</span>
                <NInputNumber v-model:value="alertCfg.mem_percent" :min="1" :max="100" class="w-24" />
                <span class="text-xs text-slate-500">%</span>
              </div>
              <div class="flex items-center gap-4">
                <span class="w-32 text-sm text-slate-400">磁盘阈值</span>
                <NInputNumber v-model:value="alertCfg.disk_percent" :min="1" :max="100" class="w-24" />
                <span class="text-xs text-slate-500">%</span>
              </div>
              <div class="flex items-center gap-4">
                <span class="w-32 text-sm text-slate-400">离线告警（分钟）</span>
                <NInputNumber v-model:value="alertCfg.offline_minutes" :min="1" :max="60" class="w-24" />
              </div>
              <NButton type="primary" :loading="savingAlert" @click="saveAlertConfig">保存配置</NButton>
            </NSpace>
          </NCard>
        </template>
      </NLayoutContent>
    </NLayout>

    <!-- 登录弹窗 -->
    <NModal v-model:show="showLogin" title="运维指挥台登录" :mask-closable="false" preset="card" style="width: 400px">
      <NSpace vertical>
        <NInput v-model:value="loginUsername" placeholder="用户名" />
        <NInput v-model:value="loginPassword" type="password" placeholder="密码" />
        <NInput v-model:value="loginTOTP" placeholder="Google Authenticator 6位动态码（已开启时必填）" />
        <NButton type="primary" block :loading="loginLoading" @click="doLogin">验证并登录</NButton>
      </NSpace>
    </NModal>

    <!-- 注册节点弹窗 -->
    <NModal v-model:show="showEnroll" title="添加监控节点" :mask-closable="false" preset="card" style="width: 500px">
      <NSpace vertical>
        <NInput v-model:value="enrollName" placeholder="备注名称" />
        <NSelect v-model:value="enrollGroup" :options="groupOptions" placeholder="选择分组" />
        <NSpace>
          <NCheckbox v-model:checked="enrollTraffic">开启流量监控</NCheckbox>
          <NCheckbox v-model:checked="enrollReport">开启日报</NCheckbox>
        </NSpace>
        <div v-if="enrollCommand" class="rounded bg-black p-3 font-mono text-xs text-emerald-300">
          <div class="mb-2 text-slate-400">在目标 VPS 上执行以下命令：</div>
          <div class="select-all break-all">{{ enrollCommand }}</div>
          <div class="mt-2 text-slate-400">AgentID: {{ enrollResult?.agent_id }}</div>
        </div>
        <NButton v-if="!enrollCommand" type="primary" block :loading="enrolling" @click="doEnroll">生成注册命令</NButton>
        <NButton v-else type="warning" block @click="resetEnroll">完成，关闭</NButton>
        <NButton v-if="enrollCommand" size="small" quaternary @click="copyCommand">复制命令</NButton>
      </NSpace>
    </NModal>

    <!-- 新建分组弹窗 -->
    <NModal v-model:show="showAddGroup" title="新建分组" :mask-closable="false" preset="card" style="width: 350px">
      <NSpace vertical>
        <NInput v-model:value="newGroupName" placeholder="分组名称" />
        <NButton type="primary" block :loading="addingGroup" @click="doAddGroup">创建</NButton>
      </NSpace>
    </NModal>
  </NLayout>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, onUnmounted, ref } from "vue";
import {
  NButton, NCard, NCheckbox, NDataTable, NEmpty, NGi, NGrid, NInput,
  NInputGroup, NInputNumber, NLayout, NLayoutContent, NLayoutHeader,
  NLayoutSider, NModal, NProgress, NSelect, NSpace, NSwitch, NTag,
  useMessage, type DataTableColumns, type SelectOption,
} from "naive-ui";

type AgentStats = { cpu: number; mem: number; disk?: number; load1?: number; uptime?: number; net_sent?: number; net_recv?: number; updatedAt: number };
type WireMessage = { type: string; agent_id: string; data: string };
type EventLog = { agentId: string; text: string; time: string };
type AgentRecord = { agent_id: string; name: string; prefs: { group: string; track_traffic: boolean; daily_report: boolean; interval: number } };

const MetricCard = defineComponent({
  props: { label: String, value: String, hint: String, tone: String },
  setup(props) {
    const tc: Record<string, string> = { green: "from-emerald-500/20 to-emerald-500/5 text-emerald-300", blue: "from-sky-500/20 to-sky-500/5 text-sky-300", violet: "from-violet-500/20 to-violet-500/5 text-violet-300", red: "from-rose-500/20 to-rose-500/5 text-rose-300" };
    return () => h("div", { class: `rounded border border-line bg-gradient-to-br ${tc[props.tone as string]} p-4` }, [
      h("div", { class: "text-sm text-slate-400" }, props.label),
      h("div", { class: "mt-3 text-3xl font-semibold text-white" }, props.value),
      h("div", { class: "mt-2 text-xs text-slate-500" }, props.hint),
    ]);
  },
});

const basePath = (() => { const p = window.location.pathname.split("/").filter(Boolean); return p.length > 0 ? "/" + p[0] : ""; })();
function api(path: string) { return basePath + path; }
function wsUrl(path: string) { const p = window.location.protocol === "https:" ? "wss" : "ws"; return `${p}://${window.location.host}${api(path)}`; }

const nodes = ref<Record<string, AgentStats>>({});
const wsConnected = ref(false);
const eventLogs = ref<EventLog[]>([]);
const terminalLogs = ref<string[]>([]);
const activeNodeId = ref("");
const shellCommand = ref("df -h");
const quickCommands = ["df -h", "free -m", "uptime", "docker ps", "systemctl --failed"];

const showLogin = ref(false);
const loginUsername = ref("");
const loginPassword = ref("");
const loginTOTP = ref("");
const loginLoading = ref(false);
const operatorToken = ref("");
const refreshToken = ref("");

const showEnroll = ref(false);
const enrollName = ref("");
const enrollGroup = ref("");
const enrollTraffic = ref(true);
const enrollReport = ref(false);
const enrollCommand = ref("");
const enrollResult = ref<any>(null);
const enrolling = ref(false);

const showAddGroup = ref(false);
const newGroupName = ref("");
const addingGroup = ref(false);
const groups = ref<string[]>(["默认分组"]);
const selectedGroup = ref("全部");
const activePage = ref("dashboard");
const savingAlert = ref(false);
const alertCfg = ref({ cpu_percent: 80, mem_percent: 80, disk_percent: 80, offline_minutes: 5, enabled: false });

let viewerWS: WebSocket | null = null;
let operatorWS: WebSocket | null = null;
let reconnectTimer: number | undefined;
const nowMs = ref(Date.now());
let clockTimer: number | undefined;
const message = useMessage();

const groupOptions = computed<SelectOption[]>(() => groups.value.map(g => ({ label: g, value: g })));

const pageTitle = computed(() => {
  const m: Record<string, string> = { dashboard: "集群监控运维面板", nodes: "节点列表", terminal: "命令终端", alerts: "告警策略" };
  return m[activePage.value] || "集群监控运维面板";
});
const pageSubtitle = computed(() => {
  const m: Record<string, string> = { dashboard: "实时掌控 VPS 节点在线状态", nodes: "管理全部监控节点", terminal: "远程命令执行", alerts: "设置监控触发条件" };
  return m[activePage.value] || "";
});

const onlineCount = computed(() => Object.values(nodes.value).filter(n => nowMs.value - n.updatedAt < 10000).length);
const onlineDisplay = computed(() => `${onlineCount.value} / ${Object.keys(nodes.value).length}`);

const tableData = computed(() => Object.entries(nodes.value).map(([id, s]) => ({ id, ...s })));
const filteredTableData = computed(() => selectedGroup.value === "全部" ? tableData.value : tableData.value);
const avgCpu = computed(() => { const v = tableData.value.map(n => n.cpu); return v.length ? v.reduce((a, b) => a + b, 0) / v.length : 0; });
const avgMem = computed(() => { const v = tableData.value.map(n => n.mem); return v.length ? v.reduce((a, b) => a + b, 0) / v.length : 0; });
const alertCount = computed(() => tableData.value.filter(n => n.cpu > 80 || n.mem > 80 || (n.disk ?? 0) > 80).length);
const canSendCommand = computed(() => !!operatorToken.value && !!activeNodeId.value && shellCommand.value.trim() !== "");

function selectTerminalNode(id: string) { activeNodeId.value = id; terminalLogs.value = []; }

const columns: DataTableColumns<Record<string, number | string | undefined>> = [
  { title: "节点", key: "id", render(r) { return h("div", { class: "font-medium text-white" }, String(r.id)); } },
  { title: "状态", key: "status", render(_r, i) { const n = tableData.value[i]; return h(NTag, { type: n && nowMs.value - (n.updatedAt as number) < 10000 ? "success" : "error", size: "small", round: true }, { default: () => n && nowMs.value - (n.updatedAt as number) < 10000 ? "在线" : "离线" }); } },
  progressColumn("CPU", "cpu", "success"),
  progressColumn("内存", "mem", "info"),
  progressColumn("磁盘", "disk", "warning"),
  { title: "负载", key: "load1", render(r) { return `${Number(r.load1 ?? 0).toFixed(2)}`; } },
  { title: "网络", key: "net", render(r) { return `${formatBytes(Number(r.net_recv ?? 0))}/s ↓  ${formatBytes(Number(r.net_sent ?? 0))}/s ↑`; } },
  { title: "操作", key: "actions", render(r) { return h(NButton, { size: "small", type: "primary", secondary: true, onClick: () => { activePage.value = "terminal"; activeNodeId.value = String(r.id); } }, { default: () => "控制台" }); } },
];

const nodeColumns: DataTableColumns<Record<string, number | string | undefined>> = [
  { title: "ID", key: "id", render(r) { return h("div", { class: "font-mono text-xs text-white" }, String(r.id)); } },
  { title: "名称", key: "name", render(r) { return (r as any).name || "-"; } },
  { title: "状态", key: "status", render(_r, i) { const n = tableData.value[i]; return h(NTag, { type: n && nowMs.value - (n.updatedAt as number) < 10000 ? "success" : "error", size: "small", round: true }, { default: () => n && nowMs.value - (n.updatedAt as number) < 10000 ? "在线" : "离线" }); } },
  progressColumn("CPU", "cpu", "success"),
  progressColumn("内存", "mem", "info"),
  progressColumn("磁盘", "disk", "warning"),
  { title: "网络", key: "net", render(r) { return `${formatBytes(Number(r.net_recv ?? 0))}/s ↓  ${formatBytes(Number(r.net_sent ?? 0))}/s ↑`; } },
];

function progressColumn(title: string, key: string, status: "success" | "info" | "warning") {
  return { title, key, render(r: Record<string, number | string | undefined>) { const v = Number(r[key] ?? 0); return h("div", { class: "min-w-[120px]" }, [h("div", { class: "mb-1 text-xs text-slate-400" }, `${v.toFixed(1)}%`), h(NProgress, { percentage: Number(v.toFixed(1)), status: v > 80 ? "error" : status, showIndicator: false, height: 8 })]); } };
}
function average(v: number[]) { return v.length ? v.reduce((a, b) => a + b, 0) / v.length : 0; }
function formatBytes(v: number) { if (v < 1024) return `${v.toFixed(0)} B`; if (v < 1024 * 1024) return `${(v / 1024).toFixed(1)} KB`; return `${(v / 1024 / 1024).toFixed(1)} MB`; }

function connectViewerWS(force = false) {
  if (force && viewerWS) viewerWS.close();
  window.clearTimeout(reconnectTimer);
  viewerWS = new WebSocket(wsUrl("/ws/web"));
  viewerWS.onopen = () => { wsConnected.value = true; };
  viewerWS.onmessage = (e) => {
    let m: WireMessage; try { m = JSON.parse(e.data); } catch { return; }
    if (m.type === "stat") {
      try {
        const p = JSON.parse(m.data);
        nodes.value[m.agent_id] = { cpu: Number(p.cpu ?? 0), mem: Number(p.mem ?? 0), disk: Number(p.disk ?? 0), load1: Number(p.load1 ?? 0), uptime: Number(p.uptime ?? 0), net_recv: Number(p.net_recv ?? 0), net_sent: Number(p.net_sent ?? 0), updatedAt: Date.now() };
      } catch {}
      pushEvent(m.agent_id, "状态数据已刷新"); return;
    }
    if (m.type === "log") {
      pushEvent(m.agent_id, m.data);
      if (activePage.value === "terminal" && m.agent_id === activeNodeId.value) terminalLogs.value.push(m.data);
    }
  };
  viewerWS.onclose = () => { wsConnected.value = false; reconnectTimer = window.setTimeout(() => connectViewerWS(), 3000); };
}

function connectOperatorWS(token: string) {
  operatorWS?.close();
  operatorWS = new WebSocket(wsUrl(`/ws/operator?token=${encodeURIComponent(token)}`));
  operatorWS.onmessage = (e) => { try { const m = JSON.parse(e.data); if (m.type === "log" && activePage.value === "terminal" && m.agent_id === activeNodeId.value) terminalLogs.value.push(m.data); } catch {} };
  operatorWS.onclose = () => { message.error("指挥台连接已断开"); operatorToken.value = ""; operatorWS = null; };
}

async function doLogin() {
  if (!loginUsername.value || !loginPassword.value) { message.error("请输入用户名和密码"); return; }
  loginLoading.value = true;
  try {
    const r = await fetch(api("/api/login"), { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ username: loginUsername.value, password: loginPassword.value, code: loginTOTP.value }) });
    if (!r.ok) { message.error((await r.text()) || "登录失败"); return; }
    const d = await r.json();
    operatorToken.value = d.access_token; refreshToken.value = d.refresh_token;
    connectOperatorWS(d.access_token);
    message.success("登录成功"); showLogin.value = false;
    loginPassword.value = ""; loginTOTP.value = "";
    setTimeout(async () => { try { const r = await fetch(api("/api/refresh"), { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ refresh_token: refreshToken.value }) }); if (r.ok) { const d = await r.json(); operatorToken.value = d.access_token; connectOperatorWS(d.access_token); } else { message.error("Token 刷新失败，请重新登录"); operatorToken.value = ""; refreshToken.value = ""; } } catch {} }, 13 * 60 * 1000);
  } catch { message.error("网络请求失败"); } finally { loginLoading.value = false; }
}

async function doEnroll() {
  if (!enrollName.value) { message.error("请输入备注名称"); return; }
  enrolling.value = true;
  try {
    const r = await fetch(api("/api/enroll"), { method: "POST", headers: { "Content-Type": "application/json", "Authorization": "Bearer " + operatorToken.value }, body: JSON.stringify({ name: enrollName.value, group: enrollGroup.value || "默认分组", track_traffic: enrollTraffic.value, daily_report: enrollReport.value }) });
    if (!r.ok) { message.error(await r.text()); return; }
    const d = await r.json();
    enrollResult.value = d; enrollCommand.value = d.install_cmd;
    message.success("注册成功，请执行命令");
  } catch { message.error("注册失败"); } finally { enrolling.value = false; }
}
function copyCommand() { navigator.clipboard.writeText(enrollCommand.value).then(() => message.success("命令已复制")); }
function resetEnroll() { showEnroll.value = false; enrollCommand.value = ""; enrollResult.value = null; enrollName.value = ""; enrollGroup.value = ""; }

async function doAddGroup() {
  if (!newGroupName.value) { message.error("请输入分组名称"); return; }
  addingGroup.value = true;
  try {
    const r = await fetch(api("/api/groups"), { method: "POST", headers: { "Content-Type": "application/json", "Authorization": "Bearer " + operatorToken.value }, body: JSON.stringify({ name: newGroupName.value }) });
    if (!r.ok) { message.error(await r.text()); return; }
    groups.value = [...groups.value, newGroupName.value];
    showAddGroup.value = false; newGroupName.value = "";
    message.success("分组已创建");
  } catch { message.error("创建失败"); } finally { addingGroup.value = false; }
}

async function saveAlertConfig() {
  savingAlert.value = true;
  try {
    const r = await fetch(api("/api/alerts"), { method: "POST", headers: { "Content-Type": "application/json", "Authorization": "Bearer " + operatorToken.value }, body: JSON.stringify(alertCfg.value) });
    if (r.ok) message.success("告警配置已保存"); else message.error(await r.text());
  } catch { message.error("保存失败"); } finally { savingAlert.value = false; }
}

function sendShellCommand() {
  if (!canSendCommand.value || !operatorWS || operatorWS.readyState !== WebSocket.OPEN) { message.error("指挥台未就绪"); return; }
  const cmd = shellCommand.value.trim();
  terminalLogs.value.push(`$ ${cmd}`);
  operatorWS.send(JSON.stringify({ type: "cmd", agent_id: activeNodeId.value, data: cmd }));
}

function pushEvent(agentId: string, text: string) {
  eventLogs.value.unshift({ agentId, text, time: new Date().toLocaleTimeString() });
  eventLogs.value = eventLogs.value.slice(0, 80);
}

async function loadGroups() {
  try {
    const r = await fetch(api("/api/groups"));
    if (r.ok) groups.value = await r.json();
  } catch {}
}

async function loadAlertConfig() {
  try {
    const r = await fetch(api("/api/alerts"));
    if (r.ok) alertCfg.value = await r.json();
  } catch {}
}

onMounted(() => {
  connectViewerWS();
  loadGroups();
  loadAlertConfig();
  clockTimer = window.setInterval(() => { nowMs.value = Date.now(); }, 1000);
});
onUnmounted(() => {
  window.clearTimeout(reconnectTimer);
  window.clearInterval(clockTimer);
  viewerWS?.close();
  operatorWS?.close();
});
</script>
