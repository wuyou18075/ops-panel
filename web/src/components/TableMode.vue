<template>
  <div>
    <div class="header">
      <div>
        <h1>全部节点</h1>
        <p>实时更新 · 点击系统查看详情 · 共 {{ rows.length }} 个节点</p>
      </div>
      <div class="controls">
        <input v-model="filterText" placeholder="筛选…" />
        <NPopover trigger="click" placement="bottom-end">
          <template #trigger>
            <button class="colbtn">列 ▾</button>
          </template>
          <div class="colmenu">
            <label v-for="c in toggleable" :key="c.k">
              <input type="checkbox" :checked="cols[c.k]" @change="cols[c.k] = ($event.target as HTMLInputElement).checked" />
              {{ c.l }}
            </label>
          </div>
        </NPopover>
      </div>
    </div>

    <div class="table">
      <div class="row head" :style="gridStyle">
        <div @click="sortBy('name')">系统 <span class="sort">{{ arrow("name") }}</span></div>
        <div v-if="cols.cpu" @click="sortBy('cpu')">CPU <span class="sort">{{ arrow("cpu") }}</span></div>
        <div v-if="cols.mem" @click="sortBy('mem')">内存 <span class="sort">{{ arrow("mem") }}</span></div>
        <div v-if="cols.disk" @click="sortBy('disk')">磁盘 <span class="sort">{{ arrow("disk") }}</span></div>
        <div v-if="cols.net" @click="sortBy('net')">网络 <span class="sort">{{ arrow("net") }}</span></div>
        <div v-if="cols.today">今日流量</div>
        <div v-if="cols.agent">客户端</div>
        <div></div>
      </div>

      <div v-for="n in rows" :key="n.id" class="row" :style="gridStyle" @click="emit('open', n.id)">
        <div class="name-cell">
          <span class="dot" :class="n.online ? 'on' : 'off'"></span>
          <span class="flag">{{ countryFlag(n.prefs.country_code) }}</span>
          <span class="nm">{{ n.name || n.id }}</span>
          <span class="link">↗</span>
        </div>
        <div v-if="cols.cpu" class="meter-cell">
          <div class="meter"><i :class="'fill-' + barClass(n.cpu)" :style="{ width: clampPct(n.cpu) + '%' }"></i></div>
          <span class="pct">{{ Number(n.cpu ?? 0).toFixed(1) }}%</span>
        </div>
        <div v-if="cols.mem" class="meter-cell">
          <div class="meter"><i :class="'fill-' + barClass(n.mem)" :style="{ width: clampPct(n.mem) + '%' }"></i></div>
          <span class="pct">{{ Number(n.mem ?? 0).toFixed(1) }}%</span>
        </div>
        <div v-if="cols.disk" class="meter-cell">
          <div class="meter"><i :class="'fill-' + barClass(n.disk)" :style="{ width: clampPct(n.disk) + '%' }"></i></div>
          <span class="pct">{{ Number(n.disk ?? 0).toFixed(1) }}%</span>
        </div>
        <div v-if="cols.net" class="net-cell">↑{{ fmtRate(n.net_sent) }}<br />↓{{ fmtRate(n.net_recv) }}</div>
        <div v-if="cols.today" class="net-cell">出↑{{ fmtBytes(n.todaySent || 0) }}<br />入↓{{ fmtBytes(n.todayRecv || 0) }}</div>
        <div v-if="cols.agent" class="agent-cell">
          <span class="adot" :class="n.online ? 'on' : 'off'"></span>{{ n.online ? n.agent_ver || "—" : "离线" }}
        </div>
        <div class="icons">
          <span title="详情" @click.stop="emit('open', n.id)">📈</span>
          <span v-if="!publicMode && isOperator" title="编辑" @click.stop="emit('edit', n)">✎</span>
        </div>
      </div>

      <div v-if="rows.length === 0" class="empty">暂无节点数据</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from "vue";
import { NPopover } from "naive-ui";
import { filterText, isOperator, publicMode, visibleNodes } from "../store";
import type { NodeView } from "../types";
import { barClass, clampPct, countryFlag, fmtBytes, fmtRate } from "../utils";

const emit = defineEmits<{ open: [string]; edit: [NodeView] }>();

const cols = reactive({ cpu: true, mem: true, disk: true, net: true, today: true, agent: true });
const toggleable = [
  { k: "cpu", l: "CPU" },
  { k: "mem", l: "内存" },
  { k: "disk", l: "磁盘" },
  { k: "net", l: "网络" },
  { k: "today", l: "今日流量" },
  { k: "agent", l: "客户端" },
] as const;

const gridStyle = computed(() => {
  const parts = ["1.6fr"];
  if (cols.cpu) parts.push("1.1fr");
  if (cols.mem) parts.push("1.1fr");
  if (cols.disk) parts.push("1.1fr");
  if (cols.net) parts.push("0.9fr");
  if (cols.today) parts.push("1.2fr");
  if (cols.agent) parts.push("0.8fr");
  parts.push("auto");
  return { gridTemplateColumns: parts.join(" ") };
});

const sortKey = ref("name");
const sortDir = ref<1 | -1>(1);
function sortBy(k: string) {
  if (sortKey.value === k) sortDir.value = (sortDir.value * -1) as 1 | -1;
  else {
    sortKey.value = k;
    sortDir.value = 1;
  }
}
function arrow(k: string) {
  if (sortKey.value !== k) return "⇅";
  return sortDir.value === 1 ? "↑" : "↓";
}

function metric(n: NodeView, k: string): number | string {
  if (k === "name") return (n.name || n.id).toLowerCase();
  if (k === "net") return Number(n.net_sent ?? 0) + Number(n.net_recv ?? 0);
  return Number((n as any)[k] ?? 0);
}
const rows = computed(() => {
  const l = [...visibleNodes.value];
  l.sort((a, b) => {
    const va = metric(a, sortKey.value);
    const vb = metric(b, sortKey.value);
    if (va < vb) return -1 * sortDir.value;
    if (va > vb) return 1 * sortDir.value;
    return 0;
  });
  return l;
});
</script>

<style scoped>
.header {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  padding: 0 0 14px;
}
.header h1 {
  font-size: 22px;
  font-weight: 600;
  color: var(--ct);
  margin: 0;
}
.header p {
  font-size: 12px;
  color: var(--text-muted);
  margin-top: 4px;
}
.controls {
  display: flex;
  gap: 8px;
  align-items: center;
}
.controls input,
.colbtn {
  padding: 6px 12px;
  border-radius: 7px;
  background: var(--glass);
  border: 1px solid var(--color-line);
  font-size: 13px;
  color: var(--text);
  outline: none;
  cursor: pointer;
}
.controls input {
  cursor: text;
}
.controls input:focus {
  border-color: var(--ca);
}
.colmenu {
  display: flex;
  flex-direction: column;
  gap: 8px;
  font-size: 13px;
  padding: 2px;
}
.colmenu label {
  display: flex;
  gap: 8px;
  align-items: center;
  cursor: pointer;
}

.table {
  padding: 0;
}
.row {
  display: grid;
  align-items: center;
  gap: 18px;
  padding: 16px 18px;
  border-radius: 8px;
  font-size: 13px;
  color: var(--text);
}
.row + .row {
  border-top: 1px solid var(--color-line);
}
.row:not(.head) {
  cursor: pointer;
}
.row:not(.head):hover {
  background: color-mix(in srgb, var(--text) 6%, transparent);
}
.row.head {
  font-size: 12px;
  color: var(--text-muted);
  padding-top: 18px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  cursor: default;
}
.row.head > div {
  display: flex;
  align-items: center;
  gap: 5px;
  cursor: pointer;
  user-select: none;
}
.row.head .sort {
  color: var(--text-muted);
  opacity: 0.6;
}

.name-cell {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--ct);
  font-weight: 500;
  min-width: 0;
}
.name-cell .nm {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.name-cell .flag {
  font-size: 14px;
}
.dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  display: inline-block;
  flex-shrink: 0;
}
.dot.on {
  background: #22c55e;
  box-shadow: 0 0 6px rgba(34, 197, 94, 0.6);
}
.dot.off {
  background: #ef4444;
}
.link {
  color: var(--text-muted);
  font-size: 12px;
  opacity: 0.5;
}

.meter-cell {
  display: flex;
  align-items: center;
  gap: 10px;
  color: var(--text);
}
.meter-cell .pct {
  text-align: right;
  min-width: 40px;
  font-variant-numeric: tabular-nums;
}
.meter {
  height: 8px;
  border-radius: 4px;
  background: var(--bar-track);
  overflow: hidden;
  flex: 1;
}
.meter > i {
  display: block;
  height: 100%;
  border-radius: 4px;
}

.net-cell {
  color: var(--text-muted);
  font-variant-numeric: tabular-nums;
  font-size: 11px;
  line-height: 1.4;
}
.agent-cell {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--text-muted);
}
.adot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  display: inline-block;
}
.adot.on {
  background: #22c55e;
}
.adot.off {
  background: #ef4444;
}

.icons {
  display: flex;
  align-items: center;
  gap: 10px;
  color: var(--text-muted);
  justify-content: flex-end;
}
.icons span {
  cursor: pointer;
  transition: opacity 0.15s;
  opacity: 0.7;
}
.icons span:hover {
  opacity: 1;
}

.empty {
  text-align: center;
  padding: 48px;
  color: var(--text-muted);
  font-size: 14px;
}

@media (max-width: 900px) {
  .row {
    font-size: 12px;
  }
  .header {
    flex-direction: column;
    align-items: flex-start;
    gap: 10px;
  }
}
</style>
