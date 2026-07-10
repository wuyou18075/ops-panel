// API 客户端：路径探测、token 状态、各端点封装
import { ref } from "vue";
import type {
  AgentRecord,
  AlertConfig,
  HistPoint,
  LoginLog,
  Monitor,
  MonitorView,
  SSHLog,
  TrafficStats,
	SystemSettings,
} from "./types";

// 面板挂在 MASTER_PATH 前缀下（如 /panel），取路径首段
export const basePath = (() => {
  const p = window.location.pathname.split("/").filter(Boolean);
  return p.length ? "/" + p[0] : "";
})();

export const api = (p: string) => basePath + p;
export const wsUrl = (p: string) =>
  (location.protocol === "https:" ? "wss" : "ws") + "://" + location.host + api(p);

// operator access_token，由 store 登录后写入
export const accessToken = ref("");

function authHeaders(): Record<string, string> {
  const h: Record<string, string> = { "Content-Type": "application/json" };
  if (accessToken.value) h["Authorization"] = "Bearer " + accessToken.value;
  return h;
}

async function parse(r: Response): Promise<any> {
  if (!r.ok) throw new Error((await r.text()) || `HTTP ${r.status}`);
  const ct = r.headers.get("content-type") || "";
  return ct.includes("json") ? r.json() : r.text();
}

export const Api = {
	settings(): Promise<SystemSettings> { return fetch(api("/api/settings"), { headers: authHeaders() }).then(parse); },
	saveSettings(body: SystemSettings) { return fetch(api("/api/settings"), { method:"POST", headers:authHeaders(), body:JSON.stringify(body) }).then(parse); },
  login(username: string, password: string, code: string) {
    return fetch(api("/api/login"), {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username, password, code }),
    }).then(parse);
  },
  refresh(refresh_token: string) {
    return fetch(api("/api/refresh"), {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ refresh_token }),
    }).then(parse);
  },
  enroll(body: Record<string, unknown>) {
    return fetch(api("/api/enroll"), {
      method: "POST",
      headers: authHeaders(),
      body: JSON.stringify(body),
    }).then(parse);
  },
  agents(): Promise<AgentRecord[]> {
    return fetch(api("/api/agents")).then(parse);
  },
  updateAgent(agent_id: string, name: string, prefs: AgentRecord["prefs"]) {
    return fetch(api("/api/agents"), {
      method: "PUT",
      headers: authHeaders(),
      body: JSON.stringify({ agent_id, name, prefs }),
    }).then(parse);
  },
  deleteAgent(agent_id: string) {
    return fetch(api("/api/agents"), {
      method: "DELETE",
      headers: authHeaders(),
      body: JSON.stringify({ agent_id }),
    }).then(parse);
  },
  groups(): Promise<string[]> {
    return fetch(api("/api/groups")).then(parse);
  },
  addGroup(name: string) {
    return fetch(api("/api/groups"), {
      method: "POST",
      headers: authHeaders(),
      body: JSON.stringify({ name }),
    }).then(parse);
  },
  renameGroup(old_name: string, new_name: string) {
    return fetch(api("/api/groups"), {
      method: "PUT",
      headers: authHeaders(),
      body: JSON.stringify({ old_name, new_name }),
    }).then(parse);
  },
  deleteGroup(name: string) {
    return fetch(api("/api/groups"), {
      method: "DELETE",
      headers: authHeaders(),
      body: JSON.stringify({ name }),
    }).then(parse);
  },
  alerts(): Promise<AlertConfig> {
    return fetch(api("/api/alerts")).then(parse);
  },
  saveAlerts(cfg: AlertConfig) {
    return fetch(api("/api/alerts"), {
      method: "POST",
      headers: authHeaders(),
      body: JSON.stringify(cfg),
    }).then(parse);
  },
  traffic(): Promise<TrafficStats[]> {
    return fetch(api("/api/traffic")).then(parse);
  },
  history(agent_id: string): Promise<HistPoint[]> {
    return fetch(api("/api/history?agent_id=" + encodeURIComponent(agent_id))).then(parse);
  },
  monitors(): Promise<MonitorView[]> {
    return fetch(api("/api/monitors")).then(parse);
  },
  saveMonitor(m: Partial<Monitor>) {
    return fetch(api("/api/monitors"), {
      method: "POST",
      headers: authHeaders(),
      body: JSON.stringify(m),
    }).then(parse);
  },
  deleteMonitor(id: string) {
    return fetch(api("/api/monitors"), {
      method: "DELETE",
      headers: authHeaders(),
      body: JSON.stringify({ id }),
    }).then(parse);
  },
  loginLogs(): Promise<LoginLog[]> {
    return fetch(api("/api/login-logs"), { headers: authHeaders() }).then(parse);
  },
  clearLoginLogs() {
    return fetch(api("/api/login-logs"), { method: "DELETE", headers: authHeaders() }).then(parse);
  },
  sshLogs(agent_id: string): Promise<SSHLog[]> {
    return fetch(api("/api/ssh-logs?agent_id=" + encodeURIComponent(agent_id)), { headers: authHeaders() }).then(parse);
  },
  clearSshLogs(agent_id: string) {
    return fetch(api("/api/ssh-logs?agent_id=" + encodeURIComponent(agent_id)), { method: "DELETE", headers: authHeaders() }).then(parse);
  },
  sshStats(): Promise<{ agent_id: string; week_fails: number }[]> {
    return fetch(api("/api/ssh-stats")).then(parse);
  },
  resetSshFails(agent_id: string) {
    return fetch(api("/api/ssh-fails/reset"), {
      method: "POST",
      headers: authHeaders(),
      body: JSON.stringify({ agent_id }),
    }).then(parse);
  },
};
