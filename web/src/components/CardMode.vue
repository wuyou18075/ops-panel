<template>
  <div>
    <!-- 分组 tabs -->
    <div class="tabs">
      <div class="tab" :class="{ active: selectedGroup === '全部' }" @click="selectedGroup = '全部'">所有</div>
      <div
        v-for="g in groups"
        :key="g"
        class="tab"
        :class="{ active: selectedGroup === g }"
        @click="selectedGroup = g"
      >
        {{ g }}
      </div>
    </div>

    <div class="grid">
      <div
        v-for="n in visibleNodes"
        :key="n.id"
        class="card"
        :class="{ off: !n.online }"
        @click="emit('open', n.id)"
      >
        <div class="card-head">
          <div class="name">
            <span class="flag">{{ countryFlag(n.prefs.country_code) }}</span>
            <span class="nm">{{ n.name || n.id }}</span>
            <span
              v-if="(n.sshFailWeek || 0) >= 3"
              class="sshbadge"
              :class="(n.sshFailWeek || 0) >= 5 ? 'red' : 'yellow'"
              :title="'本周 SSH 登录失败 ' + n.sshFailWeek + ' 次'"
            >SSH失败 {{ n.sshFailWeek }}</span>
          </div>
          <span
            class="star"
            :class="{ on: n.prefs.favorite }"
            :title="n.prefs.favorite ? '取消收藏' : '收藏'"
            @click.stop="emit('fav', n)"
          >{{ n.prefs.favorite ? "★" : "☆" }}</span>
        </div>

        <div class="price">
          <span v-if="priceLabel(n.prefs)" class="now" :class="{ free: n.prefs.billing_cycle === '免费' }">{{ priceLabel(n.prefs) }}</span>
          <span v-if="expiryLabel(n.prefs.expiry_date)" :class="{ warn: isExpiringSoon(n) }">{{ expiryLabel(n.prefs.expiry_date) }}</span>
          <span v-if="n.prefs.label" class="badge">{{ n.prefs.label }}</span>
        </div>

        <div class="specs">
          <span><i class="dot"></i>{{ n.cpu_count || 0 }} 核</span>
          <span><i class="dot"></i>{{ fmtCap(n.mem_total) }}</span>
          <span><i class="dot"></i>{{ fmtCap(n.disk_total) }}</span>
        </div>

        <div class="metric" v-for="m in metrics(n)" :key="m.k">
          <div class="row"><span class="k">{{ m.k }}</span><span class="v">{{ m.t }}</span></div>
          <div class="bar"><i :class="'fill-' + barClass(m.p)" :style="{ width: clampPct(m.p) + '%' }"></i></div>
        </div>

        <div class="net-info">
          <div class="line"><span>网络</span><span>↑ {{ fmtRate(n.net_sent) }} ↓ {{ fmtRate(n.net_recv) }}</span></div>
          <div class="line"><span>今日</span><span>出↑ {{ fmtBytes(n.todaySent || 0) }} · 入↓ {{ fmtBytes(n.todayRecv || 0) }}</span></div>
          <div class="line">
            <span>本月</span>
            <span>出↑ {{ fmtBytes(n.monthSent || 0) }} · 入↓ {{ fmtBytes(n.monthRecv || 0) }}<template v-if="n.quota"> / {{ fmtBytes(n.quota) }}</template></span>
          </div>
          <div v-if="n.quota" class="qbar"><i :class="'fill-' + barClass(quotaPct(n))" :style="{ width: clampPct(quotaPct(n)) + '%' }"></i></div>
          <div class="line"><span>负载</span><span class="b">{{ loadStr(n) }}</span></div>
        </div>

        <div class="card-foot">
          <span>{{ n.prefs.expiry_date ? "到期 " + n.prefs.expiry_date : "长期" }}</span>
          <span :class="n.online ? 'st-on' : 'st-off'">{{ n.online ? "在线 " + fmtUptime(n.uptime) : "离线" }}</span>
        </div>
      </div>

      <div v-if="visibleNodes.length === 0" class="empty">暂无节点数据</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { groups, selectedGroup, visibleNodes } from "../store";
import type { NodeView } from "../types";
import {
  barClass,
  clampPct,
  countryFlag,
  daysUntil,
  expiryLabel,
  fmtBytes,
  fmtCap,
  fmtRate,
  fmtUptime,
  priceLabel,
} from "../utils";

const emit = defineEmits<{ open: [string]; fav: [NodeView]; edit: [NodeView] }>();

function metrics(n: NodeView) {
  return [
    { k: "CPU", p: Number(n.cpu ?? 0), t: Number(n.cpu ?? 0).toFixed(1) + "%" },
    { k: "内存", p: Number(n.mem ?? 0), t: Number(n.mem ?? 0).toFixed(1) + "%" },
    { k: "SWAP", p: Number(n.swap_pct ?? 0), t: n.swap_pct != null ? Number(n.swap_pct).toFixed(0) + "%" : "OFF" },
    { k: "硬盘", p: Number(n.disk ?? 0), t: Number(n.disk ?? 0).toFixed(1) + "%" },
  ];
}
function loadStr(n: NodeView): string {
  if (n.load_1 == null) return "-";
  return `${n.load_1.toFixed(2)} | ${(n.load_5 ?? 0).toFixed(2)} | ${(n.load_15 ?? 0).toFixed(2)}`;
}
function quotaPct(n: NodeView): number {
  if (!n.quota) return 0;
  return ((n.cycleUsed || 0) / n.quota) * 100;
}
function isExpiringSoon(n: NodeView): boolean {
  const d = daysUntil(n.prefs.expiry_date);
  return d !== null && d <= 7;
}
</script>

<style scoped>
.tabs {
  display: flex;
  gap: 4px;
  margin-bottom: 14px;
  padding: 6px;
  border-radius: 12px;
  background: var(--glass);
  backdrop-filter: blur(12px);
  border: 1px solid var(--glass-border);
  width: fit-content;
  flex-wrap: wrap;
}
.tab {
  padding: 7px 18px;
  border-radius: 9px;
  font-size: 13px;
  cursor: pointer;
  color: var(--text-muted);
}
.tab:hover {
  color: var(--ct);
}
.tab.active {
  background: var(--bg-panel);
  color: var(--ct);
  font-weight: 600;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.14);
}

.grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 14px;
}
@media (max-width: 1100px) {
  .grid {
    grid-template-columns: repeat(2, 1fr);
  }
}
@media (max-width: 600px) {
  .grid {
    grid-template-columns: 1fr;
  }
}

.card {
  border-radius: 16px;
  padding: 16px;
  background: var(--glass);
  backdrop-filter: blur(18px);
  border: 1px solid var(--glass-border);
  box-shadow: var(--shadow);
  cursor: pointer;
  transition: transform 0.15s, box-shadow 0.15s, border-color 0.15s;
}
.card:hover {
  transform: translateY(-3px);
  box-shadow: 0 10px 30px rgba(0, 0, 0, 0.18);
  border-color: color-mix(in srgb, var(--ca) 45%, transparent);
}
.card.off {
  opacity: 0.55;
}

.card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 6px;
}
.name {
  font-weight: 700;
  font-size: 14px;
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--ct);
  min-width: 0;
}
.name .nm {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.flag {
  font-size: 16px;
  line-height: 1;
  flex-shrink: 0;
}
.star {
  font-size: 15px;
  cursor: pointer;
  color: var(--text-muted);
  flex-shrink: 0;
}
.sshbadge {
  flex-shrink: 0;
  padding: 1px 6px;
  border-radius: 8px;
  font-size: 10px;
  font-weight: 700;
  white-space: nowrap;
}
.sshbadge.yellow {
  background: #f59e0b;
  color: #111;
}
.sshbadge.red {
  background: #ef4444;
  color: #fff;
}
.star.on {
  color: #f1c40f;
}

.price {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 10px;
  font-size: 11px;
  color: var(--text-muted);
  flex-wrap: wrap;
}
.price .now {
  color: #27ae60;
  font-weight: 600;
  font-size: 13px;
}
.price .now.free {
  color: var(--ca);
}
.price .warn {
  color: #e74c3c;
  font-weight: 600;
}
.price .badge {
  padding: 2px 7px;
  border-radius: 10px;
  font-size: 10px;
  font-weight: 600;
  background: var(--ca);
  color: #fff;
}

.specs {
  display: flex;
  gap: 14px;
  font-size: 12px;
  color: var(--text-muted);
  margin-bottom: 12px;
  flex-wrap: wrap;
}
.specs span {
  display: flex;
  align-items: center;
  gap: 3px;
}
.specs .dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--ca);
  display: inline-block;
}

.metric {
  margin-bottom: 7px;
}
.metric .row {
  display: flex;
  justify-content: space-between;
  font-size: 12px;
  margin-bottom: 2px;
}
.metric .row .k {
  color: var(--text-muted);
}
.metric .row .v {
  font-weight: 600;
  color: var(--text);
}
.bar {
  height: 9px;
  border-radius: 5px;
  background: var(--bar-track);
  overflow: hidden;
}
.bar > i {
  display: block;
  height: 100%;
  border-radius: 5px;
}

.net-info {
  margin-top: 10px;
  padding-top: 10px;
  border-top: 1px dashed var(--color-line);
  font-size: 11px;
  color: var(--text-muted);
  line-height: 1.9;
}
.net-info .line {
  display: flex;
  justify-content: space-between;
}
.net-info .b {
  font-weight: 600;
  color: var(--text);
}
.qbar {
  height: 5px;
  border-radius: 3px;
  background: var(--bar-track);
  overflow: hidden;
  margin: 2px 0 4px;
}
.qbar > i {
  display: block;
  height: 100%;
}

.card-foot {
  margin-top: 10px;
  display: flex;
  justify-content: space-between;
  font-size: 11px;
  color: var(--text-muted);
}
.st-on {
  color: #27ae60;
  font-weight: 600;
}
.st-off {
  color: #e74c3c;
  font-weight: 600;
}

.empty {
  grid-column: 1 / -1;
  text-align: center;
  padding: 48px;
  color: var(--text-muted);
  font-size: 14px;
}
</style>
