// 中央状态：节点/流量/监控/分组/告警 + WebSocket + 全部操作。单例模块。
import { computed, reactive, ref } from "vue";
import { Api, accessToken, wsUrl } from "./api";
import type {
  AgentRecord,
  AlertConfig,
  MonitorView,
  NodeStat,
  NodeView,
  TrafficStats,
} from "./types";

export const publicMode =
  new URLSearchParams(location.search).get("view") === "public";

// ── 原始状态 ──
const nodes = reactive<Record<string, NodeStat>>({}); // 实时指标
const agents = reactive<Record<string, AgentRecord>>({}); // 元数据
const traffic = reactive<Record<string, TrafficStats>>({}); // 流量
export const monitors = ref<MonitorView[]>([]);
export const groups = ref<string[]>(["默认分组"]);
export const alertCfg = ref<AlertConfig>({
  cpu_percent: 80,
  mem_percent: 80,
  disk_percent: 80,
  offline_minutes: 5,
  enabled: false,
});

export const wsConnected = ref(false);
export const operatorOnline = ref(false);
export const nowMs = ref(Date.now());
export const selectedGroup = ref("全部");
export const filterText = ref("");
export const terminalLogs = ref<string[]>([]);
export const activeNodeId = ref("");

const refreshToken = ref("");
let viewerWS: WebSocket | null = null;
let operatorWS: WebSocket | null = null;
let reconnectTimer: number | undefined;
let refreshTimer: number | undefined;

// ── 合并视图 ──
export const nodeViews = computed<NodeView[]>(() => {
  const ids = new Set<string>([...Object.keys(agents), ...Object.keys(nodes)]);
  const list: NodeView[] = [];
  for (const id of ids) {
    const rec = agents[id];
    const st = nodes[id];
    const prefs = rec?.prefs || ({ group: "默认分组" } as AgentRecord["prefs"]);
    const iv = prefs.interval || 2;
    const online = st ? nowMs.value - (st.updatedAt || 0) < Math.max(10000, iv * 2500) : false;
    const tr = traffic[id];
    list.push({
      id,
      name: rec?.name || "",
      prefs,
      online,
      ...(st || {}),
      agent_ver: st?.agent_ver || rec?.agent_ver,
      today: tr?.today,
      cycleUsed: tr?.cycle_used,
      quota: tr?.quota ?? prefs.traffic_quota,
    });
  }
  // 收藏优先 → sort_order → 名称
  list.sort((a, b) => {
    const fa = a.prefs.favorite ? 0 : 1;
    const fb = b.prefs.favorite ? 0 : 1;
    if (fa !== fb) return fa - fb;
    const sa = a.prefs.sort_order || 0;
    const sb = b.prefs.sort_order || 0;
    if (sa !== sb) return sa - sb;
    return (a.name || a.id).localeCompare(b.name || b.id);
  });
  return list;
});

export const visibleNodes = computed<NodeView[]>(() => {
  let l = nodeViews.value;
  if (selectedGroup.value !== "全部") {
    l = l.filter((n) => (n.prefs.group || "默认分组") === selectedGroup.value);
  }
  const q = filterText.value.trim().toLowerCase();
  if (q) {
    l = l.filter((n) =>
      (n.name || n.id).toLowerCase().includes(q) ||
      (n.prefs.country_code || "").toLowerCase().includes(q) ||
      (n.prefs.label || "").toLowerCase().includes(q),
    );
  }
  return l;
});

export const onlineCount = computed(() => nodeViews.value.filter((n) => n.online).length);
export const totalCount = computed(() => nodeViews.value.length);
export const avgCpu = computed(() => {
  const v = nodeViews.value.filter((n) => n.online).map((n) => Number(n.cpu ?? 0));
  return v.length ? v.reduce((a, b) => a + b, 0) / v.length : 0;
});
export const avgMem = computed(() => {
  const v = nodeViews.value.filter((n) => n.online).map((n) => Number(n.mem ?? 0));
  return v.length ? v.reduce((a, b) => a + b, 0) / v.length : 0;
});
export const alertCount = computed(
  () =>
    nodeViews.value.filter(
      (n) => n.online && (Number(n.cpu ?? 0) > 80 || Number(n.mem ?? 0) > 80 || Number(n.disk ?? 0) > 80),
    ).length,
);
export const regionCount = computed(
  () => new Set(nodeViews.value.map((n) => n.prefs.country_code).filter(Boolean)).size,
);
export const netTotals = computed(() => {
  let up = 0;
  let down = 0;
  for (const n of nodeViews.value) {
    if (!n.online) continue;
    up += Number(n.net_sent ?? 0);
    down += Number(n.net_recv ?? 0);
  }
  return { up, down };
});
export const trafficTotals = computed(() => {
  let today = 0;
  let month = 0;
  for (const t of Object.values(traffic)) {
    today += t.today || 0;
    month += t.this_month || 0;
  }
  return { today, month };
});

export function nodeById(id: string): NodeView | undefined {
  return nodeViews.value.find((n) => n.id === id);
}

// ── WebSocket ──
export function connectViewer(force = false): void {
  if (force && viewerWS) viewerWS.close();
  clearTimeout(reconnectTimer);
  viewerWS = new WebSocket(wsUrl("/ws/web"));
  viewerWS.onopen = () => (wsConnected.value = true);
  viewerWS.onmessage = (e) => {
    let m: any;
    try {
      m = JSON.parse(e.data);
    } catch {
      return;
    }
    if (m.type === "stat") {
      try {
        const p = JSON.parse(m.data);
        nodes[m.agent_id] = { ...(nodes[m.agent_id] || {}), ...p, updatedAt: Date.now() };
      } catch {
        /* ignore */
      }
    } else if (m.type === "log") {
      if (!activeNodeId.value || m.agent_id === activeNodeId.value) {
        terminalLogs.value.push(m.data);
        if (terminalLogs.value.length > 500) terminalLogs.value.splice(0, terminalLogs.value.length - 500);
      }
    }
  };
  viewerWS.onclose = () => {
    wsConnected.value = false;
    reconnectTimer = window.setTimeout(() => connectViewer(), 3000);
  };
}

function connectOperator(token: string): void {
  operatorWS?.close();
  operatorWS = new WebSocket(wsUrl("/ws/operator?token=" + encodeURIComponent(token)));
  operatorWS.onopen = () => (operatorOnline.value = true);
  operatorWS.onclose = () => {
    operatorOnline.value = false;
    operatorWS = null;
  };
}

export function sendCommand(agentId: string, cmd: string): boolean {
  if (!operatorWS || operatorWS.readyState !== WebSocket.OPEN) return false;
  operatorWS.send(JSON.stringify({ type: "cmd", agent_id: agentId, data: cmd }));
  return true;
}

// ── 认证 ──
export async function login(username: string, password: string, code: string): Promise<void> {
  const d = await Api.login(username || "admin", password, code);
  accessToken.value = d.access_token;
  refreshToken.value = d.refresh_token;
  connectOperator(d.access_token);
  clearInterval(refreshTimer);
  refreshTimer = window.setInterval(refreshOnce, 13 * 60 * 1000);
}

async function refreshOnce(): Promise<void> {
  if (!refreshToken.value) return;
  try {
    const d = await Api.refresh(refreshToken.value);
    accessToken.value = d.access_token;
    refreshToken.value = d.refresh_token;
    connectOperator(d.access_token);
  } catch {
    logout();
  }
}

export function logout(): void {
  accessToken.value = "";
  refreshToken.value = "";
  operatorOnline.value = false;
  clearInterval(refreshTimer);
  operatorWS?.close();
}

export const isOperator = computed(() => !!accessToken.value);

// ── 数据加载 ──
export async function loadAgents(): Promise<void> {
  try {
    const list = await Api.agents();
    for (const a of list) agents[a.agent_id] = a;
  } catch {
    /* ignore */
  }
}
export async function loadGroups(): Promise<void> {
  try {
    groups.value = await Api.groups();
  } catch {
    /* ignore */
  }
}
export async function loadAlerts(): Promise<void> {
  try {
    alertCfg.value = await Api.alerts();
  } catch {
    /* ignore */
  }
}
export async function loadTraffic(): Promise<void> {
  try {
    const list = await Api.traffic();
    for (const t of list) traffic[t.agent_id] = t;
  } catch {
    /* ignore */
  }
}
export async function loadMonitors(): Promise<void> {
  try {
    monitors.value = await Api.monitors();
  } catch {
    /* ignore */
  }
}

export function startPolling(): void {
  connectViewer();
  loadAgents();
  loadGroups();
  loadAlerts();
  loadTraffic();
  loadMonitors();
  window.setInterval(() => (nowMs.value = Date.now()), 1000);
  window.setInterval(loadTraffic, 30000);
  window.setInterval(loadAgents, 20000);
  window.setInterval(loadMonitors, 15000);
}
