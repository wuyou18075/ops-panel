<template>
  <NDrawer :show="show" @update:show="emit('update:show', $event)" :width="drawerWidth" placement="right">
    <NDrawerContent :native-scrollbar="false" closable>
      <template #header>
        <div class="dh" v-if="node">
          <span class="flag">{{ countryFlag(node.prefs.country_code) }}</span>
          <span class="nm">{{ node.name || node.id }}</span>
          <span class="badge" :class="node.online ? 'on' : 'off'">{{ node.online ? "在线" : "离线" }}</span>
          <div style="flex: 1"></div>
          <NButton v-if="!publicMode && isOperator && node.prefs.enable_console" size="small" type="primary" secondary @click="emit('console', node)">控制台</NButton>
          <NButton v-if="!publicMode && isOperator" size="small" tertiary @click="emit('edit', node)">编辑</NButton>
        </div>
      </template>

      <div v-if="node" class="body">
        <!-- 元数据 -->
        <div class="meta">
          <div class="mi"><span>分组</span><b>{{ node.prefs.group || "默认分组" }}</b></div>
          <div class="mi"><span>价格</span><b>{{ priceLabel(node.prefs) || "-" }}</b></div>
          <div class="mi"><span>到期</span><b>{{ node.prefs.expiry_date || "长期" }} {{ expiryLabel(node.prefs.expiry_date) }}</b></div>
          <div class="mi"><span>标签</span><b>{{ node.prefs.label || "-" }}</b></div>
          <div class="mi"><span>Agent</span><b>{{ node.agent_ver || "-" }}</b></div>
          <div class="mi"><span>运行</span><b>{{ fmtUptime(node.uptime) }}</b></div>
          <div class="mi"><span>规格</span><b>{{ node.cpu_count || 0 }}核 · {{ fmtCap(node.mem_total) }} · {{ fmtCap(node.disk_total) }}</b></div>
          <div class="mi"><span>负载</span><b>{{ loadStr }}</b></div>
        </div>

        <!-- 流量 -->
        <div class="section" v-if="node.quota || node.today != null">
          <div class="stitle">流量</div>
          <div class="qrow"><span>今日</span><span>出↑ {{ fmtBytes(node.todaySent || 0) }} · 入↓ {{ fmtBytes(node.todayRecv || 0) }}</span></div>
          <div class="qrow"><span>本月</span><span>出↑ {{ fmtBytes(node.monthSent || 0) }} · 入↓ {{ fmtBytes(node.monthRecv || 0) }}<template v-if="node.quota"> / {{ fmtBytes(node.quota) }}</template></span></div>
          <div v-if="node.quota" class="qbar"><i :class="'fill-' + barClass(quotaPct)" :style="{ width: clampPct(quotaPct) + '%' }"></i></div>
        </div>

        <!-- SSH 登录 -->
        <div class="section" v-if="isOperator">
          <div class="stitle sshhead">
            <span>SSH 登录<span class="muted" v-if="(node.sshFailWeek || 0) > 0"> · 本周失败 {{ node.sshFailWeek }} 次</span></span>
            <span class="sshacts">
              <button class="lnk" @click="showSSHDetail = true">查看详情</button>
            </span>
          </div>
          <div class="ssh-summary"><div><b class="ok">{{ sshSuccess }}</b><span>成功</span></div><div><b class="fail">{{ sshFailed }}</b><span>失败</span></div></div>
        </div>

        <!-- 当前指标 -->
        <div class="section">
          <div class="stitle">当前</div>
          <div class="metric" v-for="m in curMetrics" :key="m.k">
            <div class="mrow"><span>{{ m.k }}</span><b>{{ m.t }}</b></div>
            <div class="bar"><i :class="'fill-' + barClass(m.p)" :style="{ width: clampPct(m.p) + '%' }"></i></div>
          </div>
        </div>

        <!-- 历史曲线 -->
        <div class="section">
          <div class="stitle">近 24 小时 <span class="muted">（{{ hist.length }} 点）</span></div>
          <div class="chart"><div class="ch"><span>CPU</span><b>{{ last("cpu") }}%</b></div><Sparkline :values="series.cpu" :max="100" color="#22c55e" :height="46" /></div>
          <div class="chart"><div class="ch"><span>内存</span><b>{{ last("mem") }}%</b></div><Sparkline :values="series.mem" :max="100" color="#3b82f6" :height="46" /></div>
          <div class="chart"><div class="ch"><span>磁盘</span><b>{{ last("disk") }}%</b></div><Sparkline :values="series.disk" :max="100" color="#f59e0b" :height="46" /></div>
          <div class="chart"><div class="ch"><span>↑ 上行</span><b>{{ fmtRate(series.up[series.up.length - 1] || 0) }}</b></div><Sparkline :values="series.up" color="#8b5cf6" :height="40" /></div>
          <div class="chart"><div class="ch"><span>↓ 下行</span><b>{{ fmtRate(series.down[series.down.length - 1] || 0) }}</b></div><Sparkline :values="series.down" color="#06b6d4" :height="40" /></div>
        </div>
      </div>
      <NModal v-model:show="showSSHDetail" preset="card" title="SSH 登录详情" style="width:760px;max-width:94vw">
        <div class="sshfilters"><NSelect v-model:value="sshFilter" :options="sshFilterOptions" style="width:130px"/><NInput v-model:value="sshQuery" clearable placeholder="搜索 IP"/></div>
        <div class="sshdetail"><div v-for="(l,i) in filteredSSH" :key="i" class="sshrow"><span :class="l.success?'ok':'fail'">{{l.success?'成功':'失败'}}</span><span class="u">{{l.user}}</span><span>{{l.ip}}</span><span>{{l.location||'-'}}</span></div><div v-if="!filteredSSH.length" class="muted sshempty">没有匹配记录</div></div>
        <template #footer><div class="sshfooter"><NButton tertiary @click="resetFails">重置失败计数</NButton><NButton type="error" tertiary @click="clearSSH">清空记录</NButton><div style="flex:1"></div><NButton @click="showSSHDetail=false">关闭</NButton></div></template>
      </NModal>
    </NDrawerContent>
  </NDrawer>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { NButton, NDrawer, NDrawerContent, NInput, NModal, NSelect, useMessage } from "naive-ui";
import Sparkline from "./Sparkline.vue";
import { Api } from "../api";
import { isOperator, nodeById, publicMode } from "../store";
import type { HistPoint, NodeView, SSHLog } from "../types";
import { barClass, clampPct, countryFlag, expiryLabel, fmtBytes, fmtCap, fmtRate, fmtUptime, priceLabel } from "../utils";

const props = defineProps<{ show: boolean; nodeId: string }>();
const emit = defineEmits<{ "update:show": [boolean]; edit: [NodeView]; console: [NodeView] }>();

const hist = ref<HistPoint[]>([]);
const sshLogs = ref<SSHLog[]>([]);
const showSSHDetail = ref(false); const sshFilter=ref("all"); const sshQuery=ref("");
const sshFilterOptions=[{label:"全部",value:"all"},{label:"成功",value:"success"},{label:"失败",value:"failed"}];
const sshSuccess=computed(()=>sshLogs.value.filter(x=>x.success).length); const sshFailed=computed(()=>sshLogs.value.filter(x=>!x.success).length);
const filteredSSH=computed(()=>sshLogs.value.filter(x=>(sshFilter.value==="all"||(sshFilter.value==="success"?x.success:!x.success))&&x.ip.toLowerCase().includes(sshQuery.value.trim().toLowerCase())));
const message = useMessage();
let timer: number | undefined;

const drawerWidth = computed(() => Math.min(560, window.innerWidth - 20));
const node = computed<NodeView | undefined>(() => nodeById(props.nodeId));

const series = computed(() => ({
  cpu: hist.value.map((h) => h.cpu),
  mem: hist.value.map((h) => h.mem),
  disk: hist.value.map((h) => h.disk),
  up: hist.value.map((h) => h.up),
  down: hist.value.map((h) => h.down),
}));

const curMetrics = computed(() => {
  const n = node.value;
  if (!n) return [];
  return [
    { k: "CPU", p: Number(n.cpu ?? 0), t: Number(n.cpu ?? 0).toFixed(1) + "%" },
    { k: "内存", p: Number(n.mem ?? 0), t: Number(n.mem ?? 0).toFixed(1) + "%" },
    { k: "磁盘", p: Number(n.disk ?? 0), t: Number(n.disk ?? 0).toFixed(1) + "%" },
    { k: "SWAP", p: Number(n.swap_pct ?? 0), t: n.swap_pct != null ? Number(n.swap_pct).toFixed(0) + "%" : "OFF" },
  ];
});
const loadStr = computed(() => {
  const n = node.value;
  if (!n || n.load_1 == null) return "-";
  return `${n.load_1.toFixed(2)} | ${(n.load_5 ?? 0).toFixed(2)} | ${(n.load_15 ?? 0).toFixed(2)}`;
});
const quotaPct = computed(() => {
  const n = node.value;
  if (!n?.quota) return 0;
  return ((n.cycleUsed || 0) / n.quota) * 100;
});

function last(k: "cpu" | "mem" | "disk"): string {
  const arr = series.value[k];
  return (arr[arr.length - 1] ?? 0).toFixed(1);
}

async function fetchHistory() {
  if (!props.nodeId) return;
  try {
    hist.value = await Api.history(props.nodeId);
  } catch {
    /* ignore */
  }
}

async function fetchSSH() {
  if (!props.nodeId || !isOperator.value) return;
  try {
    sshLogs.value = await Api.sshLogs(props.nodeId);
  } catch {
    /* ignore */
  }
}
async function clearSSH() {
  try {
    await Api.clearSshLogs(props.nodeId);
    sshLogs.value = [];
    message.success("已清空");
  } catch (e: any) {
    message.error(e?.message || "清空失败");
  }
}
async function resetFails() {
  try {
    await Api.resetSshFails(props.nodeId);
    message.success("已重置本周失败计数");
  } catch (e: any) {
    message.error(e?.message || "重置失败");
  }
}

watch(
  () => [props.show, props.nodeId],
  () => {
    clearInterval(timer);
    if (props.show && props.nodeId) {
      fetchHistory();
      fetchSSH();
      timer = window.setInterval(fetchHistory, 30000);
    }
  },
  { immediate: true },
);
</script>

<style scoped>
.dh {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
}
.dh .flag {
  font-size: 18px;
}
.dh .nm {
  font-weight: 700;
  color: var(--ct);
}
.badge {
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 10px;
  font-weight: 600;
}
.badge.on {
  background: rgba(34, 197, 94, 0.15);
  color: #22c55e;
}
.badge.off {
  background: rgba(239, 68, 68, 0.15);
  color: #ef4444;
}

.meta {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px 14px;
  margin-bottom: 16px;
}
.mi {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  font-size: 12px;
  border-bottom: 1px dashed var(--color-line);
  padding-bottom: 5px;
}
.mi span {
  color: var(--text-muted);
}
.mi b {
  color: var(--text);
  font-weight: 600;
  text-align: right;
}

.section {
  margin-bottom: 18px;
}
.stitle {
  font-size: 12px;
  font-weight: 700;
  color: var(--ca);
  margin-bottom: 8px;
  letter-spacing: 0.5px;
}
.muted {
  color: var(--text-muted);
  font-weight: 400;
}
.sshhead {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.sshacts {
  display: flex;
  gap: 10px;
}
.lnk {
  background: none;
  border: none;
  color: var(--ca);
  font-size: 11px;
  cursor: pointer;
  padding: 0;
}
.sshempty {
  font-size: 12px;
}
.sshrow {
  display: grid;
  grid-template-columns: 40px 1fr 1.2fr 1.4fr;
  gap: 8px;
  font-size: 11px;
  padding: 4px 0;
  border-bottom: 1px dashed var(--color-line);
  color: var(--text-muted);
  align-items: center;
}
.sshrow .ok {
  color: #22c55e;
}
.sshrow .fail {
  color: #ef4444;
  font-weight: 600;
}
.sshrow .u {
  color: var(--text);
}
.ssh-summary{display:grid;grid-template-columns:1fr 1fr;gap:12px}.ssh-summary>div{padding:14px;border-radius:12px;background:var(--bar-track);display:flex;align-items:baseline;gap:8px}.ssh-summary b{font-size:25px}.ssh-summary span{font-size:12px;color:var(--text-muted)}.sshfilters{display:flex;gap:10px;margin-bottom:12px}.sshdetail{max-height:55vh;overflow:auto}.sshfooter{display:flex;gap:8px;width:100%}

.metric {
  margin-bottom: 8px;
}
.mrow {
  display: flex;
  justify-content: space-between;
  font-size: 12px;
  margin-bottom: 3px;
  color: var(--text-muted);
}
.mrow b {
  color: var(--text);
}
.bar {
  height: 8px;
  border-radius: 4px;
  background: var(--bar-track);
  overflow: hidden;
}
.bar > i {
  display: block;
  height: 100%;
}

.qrow {
  display: flex;
  justify-content: space-between;
  font-size: 13px;
  margin-bottom: 6px;
}
.qbar {
  height: 8px;
  border-radius: 4px;
  background: var(--bar-track);
  overflow: hidden;
}
.qbar > i {
  display: block;
  height: 100%;
}

.chart {
  margin-bottom: 12px;
  padding: 8px 10px;
  border-radius: 10px;
  background: var(--glass);
  border: 1px solid var(--glass-border);
}
.ch {
  display: flex;
  justify-content: space-between;
  font-size: 12px;
  color: var(--text-muted);
  margin-bottom: 2px;
}
.ch b {
  color: var(--text);
}
</style>
