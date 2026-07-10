// 主题系统：5 套（默认黑 + 4 套亮色），CSS 变量驱动，localStorage 持久化
import { ref } from "vue";
import type { GlobalThemeOverrides } from "naive-ui";
import type { ThemeKey } from "./types";

export interface ThemeDef {
  key: ThemeKey;
  label: string;
  dot: string; // 主题选择圆点颜色
  light: boolean;
  vars: Record<string, string>;
}

export const THEMES: ThemeDef[] = [
  {
    key: "dark",
    label: "默认黑",
    dot: "#18a058",
    light: false,
    vars: {
      "--bg-body": "#0f1117",
      "--bg-grad": "linear-gradient(160deg,#0d0f15,#12151d 55%,#0f1420)",
      "--bg-panel": "#171a22",
      "--glass": "rgba(23,26,34,0.72)",
      "--glass-border": "rgba(255,255,255,0.07)",
      "--color-line": "#2b3140",
      "--ca": "#18a058",
      "--ca-rgb": "24,160,88",
      "--ct": "#e5e7eb",
      "--text": "#cbd2dc",
      "--text-muted": "#8b93a1",
      "--bar-track": "rgba(255,255,255,0.07)",
      "--shadow": "0 4px 22px rgba(0,0,0,0.38)",
    },
  },
  { key:"glass",label:"透明玻璃",dot:"#67e8f9",light:false,vars:{"--bg-body":"#07111f","--bg-grad":"radial-gradient(circle at 15% 10%,#164e63 0,transparent 35%),radial-gradient(circle at 85% 15%,#312e81 0,transparent 32%),#07111f","--bg-panel":"rgba(8,18,32,.72)","--glass":"rgba(255,255,255,.075)","--glass-border":"rgba(255,255,255,.16)","--color-line":"rgba(255,255,255,.12)","--ca":"#22d3ee","--ca-rgb":"34,211,238","--ct":"#f0f9ff","--text":"#d7eef7","--text-muted":"#87a9b8","--bar-track":"rgba(255,255,255,.09)","--shadow":"0 16px 45px rgba(0,0,0,.32)"}},
  {
    key: "green",
    label: "护眼绿",
    dot: "#2e9e5b",
    light: true,
    vars: {
      "--bg-body": "#c7edcc",
      "--bg-grad": "linear-gradient(160deg,#d6f1d9,#c3e8c8 55%,#b6e2be)",
      "--bg-panel": "#e8f6ea",
      "--glass": "rgba(255,255,255,0.60)",
      "--glass-border": "rgba(255,255,255,0.70)",
      "--color-line": "rgba(52,90,58,0.20)",
      "--ca": "#2e9e5b",
      "--ca-rgb": "46,158,91",
      "--ct": "#1f3a24",
      "--text": "#2c4a33",
      "--text-muted": "#5c7862",
      "--bar-track": "rgba(30,60,40,0.10)",
      "--shadow": "0 4px 20px rgba(40,80,50,0.13)",
    },
  },
  {
    key: "white",
    label: "纯净白",
    dot: "#3b82f6",
    light: true,
    vars: {
      "--bg-body": "#eceff3",
      "--bg-grad": "linear-gradient(160deg,#f3f5f8,#e7ebf0 55%,#dde3ea)",
      "--bg-panel": "#f8fafc",
      "--glass": "rgba(255,255,255,0.66)",
      "--glass-border": "rgba(255,255,255,0.78)",
      "--color-line": "rgba(30,41,59,0.12)",
      "--ca": "#3b82f6",
      "--ca-rgb": "59,130,246",
      "--ct": "#1e293b",
      "--text": "#334155",
      "--text-muted": "#64748b",
      "--bar-track": "rgba(20,30,50,0.08)",
      "--shadow": "0 4px 18px rgba(30,41,59,0.10)",
    },
  },
  {
    key: "sepia",
    label: "旧书黄",
    dot: "#b7791f",
    light: true,
    vars: {
      "--bg-body": "#f4ecd8",
      "--bg-grad": "linear-gradient(160deg,#f9f2df,#f1e6cc 55%,#ebdebe)",
      "--bg-panel": "#faf3e2",
      "--glass": "rgba(255,250,238,0.64)",
      "--glass-border": "rgba(255,255,255,0.62)",
      "--color-line": "rgba(120,90,50,0.20)",
      "--ca": "#b7791f",
      "--ca-rgb": "183,121,31",
      "--ct": "#4a3520",
      "--text": "#5b4630",
      "--text-muted": "#8a7355",
      "--bar-track": "rgba(90,60,20,0.11)",
      "--shadow": "0 4px 20px rgba(120,90,40,0.13)",
    },
  },
  {
    key: "purple",
    label: "淡紫色",
    dot: "#7c5cff",
    light: true,
    vars: {
      "--bg-body": "#ece9f7",
      "--bg-grad": "linear-gradient(160deg,#f2effb,#e7e2f6 55%,#dcd5f1)",
      "--bg-panel": "#f6f3fc",
      "--glass": "rgba(255,255,255,0.64)",
      "--glass-border": "rgba(255,255,255,0.74)",
      "--color-line": "rgba(90,70,140,0.16)",
      "--ca": "#7c5cff",
      "--ca-rgb": "124,92,255",
      "--ct": "#2e2350",
      "--text": "#3f3466",
      "--text-muted": "#6f6690",
      "--bar-track": "rgba(60,40,110,0.09)",
      "--shadow": "0 4px 20px rgba(90,70,160,0.15)",
    },
  },
  {key:"ocean",label:"深海蓝",dot:"#0ea5e9",light:false,vars:{"--bg-body":"#061827","--bg-grad":"linear-gradient(145deg,#04131f,#082f49 58%,#0c4a6e)","--bg-panel":"#082235","--glass":"rgba(8,47,73,.72)","--glass-border":"rgba(125,211,252,.14)","--color-line":"#164e63","--ca":"#38bdf8","--ca-rgb":"56,189,248","--ct":"#e0f2fe","--text":"#bae6fd","--text-muted":"#7dd3fc","--bar-track":"rgba(125,211,252,.1)","--shadow":"0 8px 30px rgba(2,132,199,.12)"}},
  {key:"rose",label:"雾粉玫瑰",dot:"#f472b6",light:true,vars:{"--bg-body":"#fff1f5","--bg-grad":"linear-gradient(145deg,#fff7fa,#fce7f3 55%,#fbcfe8)","--bg-panel":"#fff8fb","--glass":"rgba(255,255,255,.68)","--glass-border":"rgba(255,255,255,.85)","--color-line":"rgba(190,24,93,.12)","--ca":"#db2777","--ca-rgb":"219,39,119","--ct":"#831843","--text":"#9d174d","--text-muted":"#be5b83","--bar-track":"rgba(190,24,93,.08)","--shadow":"0 8px 26px rgba(190,24,93,.11)"}},
  {key:"slate",label:"石墨灰",dot:"#94a3b8",light:false,vars:{"--bg-body":"#111827","--bg-grad":"linear-gradient(150deg,#0f172a,#1e293b 60%,#111827)","--bg-panel":"#1e293b","--glass":"rgba(30,41,59,.76)","--glass-border":"rgba(148,163,184,.14)","--color-line":"#334155","--ca":"#94a3b8","--ca-rgb":"148,163,184","--ct":"#f8fafc","--text":"#e2e8f0","--text-muted":"#94a3b8","--bar-track":"rgba(148,163,184,.12)","--shadow":"0 8px 28px rgba(0,0,0,.3)"}},
  {key:"amber",label:"暖阳橙",dot:"#f59e0b",light:true,vars:{"--bg-body":"#fffbeb","--bg-grad":"linear-gradient(145deg,#fffbeb,#fef3c7 58%,#fde68a)","--bg-panel":"#fffdf5","--glass":"rgba(255,255,255,.66)","--glass-border":"rgba(255,255,255,.8)","--color-line":"rgba(180,83,9,.14)","--ca":"#d97706","--ca-rgb":"217,119,6","--ct":"#78350f","--text":"#92400e","--text-muted":"#b2763d","--bar-track":"rgba(180,83,9,.09)","--shadow":"0 8px 24px rgba(180,83,9,.11)"}},
  {key:"cyan",label:"薄荷青",dot:"#14b8a6",light:true,vars:{"--bg-body":"#ecfdf5","--bg-grad":"linear-gradient(145deg,#f0fdfa,#ccfbf1 58%,#99f6e4)","--bg-panel":"#f5fffc","--glass":"rgba(255,255,255,.66)","--glass-border":"rgba(255,255,255,.82)","--color-line":"rgba(13,148,136,.14)","--ca":"#0d9488","--ca-rgb":"13,148,136","--ct":"#134e4a","--text":"#115e59","--text-muted":"#4f8f89","--bar-track":"rgba(13,148,136,.09)","--shadow":"0 8px 24px rgba(13,148,136,.11)"}},
];

export const themeKey = ref<ThemeKey>(
  (localStorage.getItem("ops-theme") as ThemeKey) || "dark",
);

export function themeDef(key: ThemeKey): ThemeDef {
  return THEMES.find((t) => t.key === key) || THEMES[0];
}

export function applyTheme(key: ThemeKey): void {
  const def = themeDef(key);
  themeKey.value = def.key;
  localStorage.setItem("ops-theme", def.key);
  const root = document.documentElement;
  for (const [k, v] of Object.entries(def.vars)) root.style.setProperty(k, v);
  root.style.setProperty("--is-light", def.light ? "1" : "0");
  root.dataset.theme = def.key;
  document.body.style.background = def.vars["--bg-grad"];
  document.body.style.color = def.vars["--text"];
}

export function isLight(key: ThemeKey): boolean {
  return themeDef(key).light;
}

export function naiveOverrides(key: ThemeKey): GlobalThemeOverrides {
  const d = themeDef(key).vars;
  return {
    common: {
      primaryColor: d["--ca"],
      primaryColorHover: d["--ca"],
      primaryColorPressed: d["--ca"],
      primaryColorSuppl: d["--ca"],
      bodyColor: d["--bg-panel"],
      cardColor: d["--bg-panel"],
      modalColor: d["--bg-panel"],
      popoverColor: d["--bg-panel"],
      borderColor: d["--color-line"],
      textColorBase: d["--text"],
    },
    Card: { borderRadius: "12px" },
  };
}
