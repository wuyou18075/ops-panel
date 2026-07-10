// 全局类型定义

export interface AgentPreferences {
  enable_console: boolean;
  group: string;
  track_traffic: boolean;
  daily_report: boolean;
  interval: number;
  // 手动元数据
  price_amount?: number;
  price_currency?: string;
  billing_cycle?: string; // 月/年/一次性/免费
  expiry_date?: string; // YYYY-MM-DD
  label?: string;
  traffic_quota?: number; // 字节
  country_code?: string; // ISO alpha-2
  favorite?: boolean;
  sort_order?: number;
	latency_probe_ids?: string[];
}

export interface AgentRecord {
  agent_id: string;
  secret?: string;
  name: string;
  agent_ver?: string;
  prefs: AgentPreferences;
}

// WebSocket 实时上报的原始指标（agent StatData）
export interface NodeStat {
  cpu: number;
  mem: number;
  disk: number;
  swap_pct?: number;
  load_1?: number;
  load_5?: number;
  load_15?: number;
  uptime?: number;
  cpu_count?: number;
  mem_total?: number;
  disk_total?: number;
  net_sent?: number; // 上行速率 B/s
  net_recv?: number; // 下行速率 B/s
  agent_ver?: string;
  updatedAt: number;
}

// 合并后的节点视图（记录 + 实时 + 流量 + 计算态）
export interface NodeView extends Partial<NodeStat> {
  id: string;
  name: string;
  prefs: AgentPreferences;
  agent_ver?: string;
  online: boolean;
  today?: number;
  todaySent?: number;
  todayRecv?: number;
  monthSent?: number;
  monthRecv?: number;
  cycleUsed?: number;
  quota?: number;
  sshFailWeek?: number;
	latencyMs?: number;
}

export interface TrafficStats {
  agent_id: string;
  group: string;
  name: string;
  today: number;
  today_sent: number;
  today_recv: number;
  this_month: number;
  month_sent: number;
  month_recv: number;
  cycle_used: number;
  quota: number;
}

export interface HistPoint {
  t: number;
  cpu: number;
  mem: number;
  disk: number;
  up: number;
  down: number;
}

export interface Monitor {
  id: string;
  name: string;
  type: string; // tcp | http | icmp
  target: string;
  interval: number;
  agent_id: string;
	template_id?: string;
}

export interface SystemSettings { probe_interval:number; probe_type:string; latency_templates:{id:string;name:string;target:string}[] }
export interface AlertEvent {id:number;ts:number;agent_id:string;kind:string;title:string;detail:string}

export interface ProbeResult {
  monitor_id: string;
  up: boolean;
  latency_ms: number;
  ts: number;
}

export interface MonitorView extends Monitor {
  up: boolean;
  latency_ms: number;
  uptime: number;
  last_ts: number;
  history: ProbeResult[];
}

export interface AlertConfig {
  cpu_percent: number;
  mem_percent: number;
  disk_percent: number;
  offline_minutes: number;
  enabled: boolean;
}

export interface LoginLog {
  ts: number;
  ip: string;
  location: string;
  device: string;
  username: string;
}

export interface SSHLog {
  ts: number;
  ip: string;
  location: string;
  user: string;
  method: string;
  success: boolean;
}

export type ThemeKey = "dark" | "glass" | "white" | "green" | "sepia" | "purple" | "ocean" | "rose" | "slate" | "amber" | "cyan";
export type ViewMode = "cards" | "table";
