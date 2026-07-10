// 纯工具函数：格式化、国旗、日期、进度条配色

// 字节量：1.23 GB
export function fmtBytes(v: number | undefined | null): string {
  const b = Number(v ?? 0);
  if (b < 1024) return `${b.toFixed(0)} B`;
  const units = ["KB", "MB", "GB", "TB", "PB"];
  let f = b / 1024;
  let i = 0;
  while (f >= 1024 && i < units.length - 1) {
    f /= 1024;
    i++;
  }
  return `${f.toFixed(f >= 100 ? 0 : f >= 10 ? 1 : 2)} ${units[i]}`;
}

// 速率：1.23 KB/s
export function fmtRate(v: number | undefined | null): string {
  return `${fmtBytes(v)}/s`;
}

// 容量简写（规格行）：5.8 GB / 721 MB
export function fmtCap(v: number | undefined | null): string {
  const b = Number(v ?? 0);
  if (b <= 0) return "-";
  if (b < 1073741824) return `${(b / 1048576).toFixed(0)} MB`;
  return `${(b / 1073741824).toFixed(b / 1073741824 >= 100 ? 0 : 1)} GB`;
}

// 运行时长：uptime 秒 → 31天16小时
export function fmtUptime(sec: number | undefined | null): string {
  const s = Number(sec ?? 0);
  if (s <= 0) return "-";
  const d = Math.floor(s / 86400);
  const h = Math.floor((s % 86400) / 3600);
  const m = Math.floor((s % 3600) / 60);
  if (d > 0) return `${d}天${h}小时`;
  if (h > 0) return `${h}小时${m}分`;
  return `${m}分`;
}

// ISO alpha-2 国家码 → 旗帜 emoji（区域指示符，纯 unicode）
export function countryFlag(cc: string | undefined | null): string {
  const c = (cc || "").trim().toUpperCase();
  if (c.length !== 2 || !/^[A-Z]{2}$/.test(c)) return "🏳️";
  const base = 0x1f1e6;
  return String.fromCodePoint(base + (c.charCodeAt(0) - 65), base + (c.charCodeAt(1) - 65));
}

// 剩余天数（到期日 YYYY-MM-DD）
export function daysUntil(date: string | undefined | null): number | null {
  if (!date) return null;
  const t = new Date(date + "T00:00:00").getTime();
  if (isNaN(t)) return null;
  return Math.ceil((t - Date.now()) / 86400000);
}

// 剩余天数展示：余 335 天 / 已过期
export function expiryLabel(date: string | undefined | null): string {
  const d = daysUntil(date);
  if (d === null) return "";
  if (d < 0) return "已过期";
  if (d === 0) return "今日到期";
  return `余 ${d} 天`;
}

// 价格展示：$28.10/月
export function priceLabel(p: {
  price_amount?: number;
  price_currency?: string;
  billing_cycle?: string;
}): string {
  if (p.billing_cycle === "免费") return "免费";
  if (!p.price_amount) return "";
  const cur = p.price_currency || "$";
  const cycle = p.billing_cycle ? `/${p.billing_cycle}` : "";
  return `${cur}${p.price_amount}${cycle}`;
}

// 进度条配色类：<60 绿, 60-80 黄, >80 红
export function barClass(v: number | undefined | null): "bg" | "by" | "br" {
  const p = Number(v ?? 0);
  if (p > 80) return "br";
  if (p > 60) return "by";
  return "bg";
}

// 延迟颜色：<80 好, <200 中, 否则差
export function latClass(ms: number): "bg" | "by" | "br" {
  if (ms <= 0) return "br";
  if (ms < 80) return "bg";
  if (ms < 200) return "by";
  return "br";
}

export function clampPct(v: number | undefined | null): number {
  return Math.max(0, Math.min(100, Number(v ?? 0)));
}
