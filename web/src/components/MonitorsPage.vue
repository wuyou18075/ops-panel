<template>
  <div>
    <div class="topline">
      <div>
        <span class="title">服务监控</span>
        <p class="sub">TCP / HTTP / ICMP 探测 · 延迟与可用率 · 共 {{ monitors.length }} 项</p>
      </div>
      <NButton v-if="!publicMode && isOperator" type="primary" size="small" @click="openAdd">+ 添加监控</NButton>
    </div>

    <div class="grid">
      <div v-for="m in monitors" :key="m.id" class="mcard">
        <div class="mhead">
          <span class="mdot" :class="m.up ? 'on' : 'off'"></span>
          <span class="mname">{{ m.name }}</span>
          <span class="mtype">{{ m.type.toUpperCase() }}</span>
          <div style="flex: 1"></div>
          <NButton v-if="!publicMode && isOperator" text size="tiny" @click="openEdit(m)">编辑</NButton>
        </div>
        <div class="mtarget">{{ m.target }}</div>
        <div class="mstats">
          <div><span>延迟</span><b :class="'t-' + latClass(m.latency_ms)">{{ m.up ? m.latency_ms.toFixed(0) + " ms" : "超时" }}</b></div>
          <div><span>可用率</span><b>{{ m.uptime.toFixed(1) }}%</b></div>
          <div><span>探测节点</span><b>{{ agentName(m.agent_id) }}</b></div>
        </div>
        <Sparkline :values="m.history.map((h) => h.latency_ms)" color="var(--ca)" :height="38" />
      </div>
      <div v-if="monitors.length === 0" class="empty">暂无监控项{{ isOperator ? "，点击右上角添加" : "" }}</div>
    </div>

    <NModal v-model:show="showForm" preset="card" :title="editing.id ? '编辑监控' : '添加监控'" style="width: 460px; max-width: 94vw">
      <NSpace vertical>
        <NInput v-model:value="editing.name" placeholder="名称" />
        <NSelect v-model:value="editing.type" :options="typeOptions" />
        <NInput v-model:value="editing.target" :placeholder="targetHint" />
        <NSelect v-model:value="editing.agent_id" :options="agentOptions" placeholder="选择执行探测的节点" />
        <div class="frow"><span>探测间隔（秒）</span><NInputNumber v-model:value="editing.interval" :min="5" :max="3600" style="width: 120px" /></div>
        <div class="fbtns">
          <NButton v-if="editing.id" tertiary type="error" @click="doDelete">删除</NButton>
          <div style="flex: 1"></div>
          <NButton @click="showForm = false">取消</NButton>
          <NButton type="primary" :loading="saving" @click="save">保存</NButton>
        </div>
      </NSpace>
    </NModal>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from "vue";
import { NButton, NInput, NInputNumber, NModal, NSelect, NSpace, useMessage } from "naive-ui";
import Sparkline from "./Sparkline.vue";
import { Api } from "../api";
import { isOperator, loadMonitors, monitors, nodeViews, publicMode } from "../store";
import type { Monitor, MonitorView } from "../types";
import { latClass } from "../utils";

const message = useMessage();
const showForm = ref(false);
const saving = ref(false);
const editing = reactive<Partial<Monitor>>({});

const typeOptions = [
  { label: "TCP 连接", value: "tcp" },
  { label: "HTTP 请求", value: "http" },
  { label: "ICMP Ping", value: "icmp" },
];
const targetHint = computed(() => {
  if (editing.type === "tcp") return "host:port，如 1.1.1.1:443";
  if (editing.type === "http") return "URL，如 https://example.com";
  return "主机，如 8.8.8.8";
});
const agentOptions = computed(() =>
  nodeViews.value.map((n) => ({ label: (n.name || n.id) + (n.online ? "" : "（离线）"), value: n.id })),
);
function agentName(id: string): string {
  const n = nodeViews.value.find((x) => x.id === id);
  return n ? n.name || n.id : id;
}

function openAdd() {
  Object.assign(editing, { id: "", name: "", type: "tcp", target: "", agent_id: "", interval: 30 });
  showForm.value = true;
}
function openEdit(m: MonitorView) {
  Object.assign(editing, { id: m.id, name: m.name, type: m.type, target: m.target, agent_id: m.agent_id, interval: m.interval });
  showForm.value = true;
}

async function save() {
  if (!editing.target || !editing.agent_id) {
    message.error("请填写探测目标并选择节点");
    return;
  }
  saving.value = true;
  try {
    await Api.saveMonitor(editing);
    await loadMonitors();
    message.success("已保存");
    showForm.value = false;
  } catch (e: any) {
    message.error(e?.message || "保存失败");
  } finally {
    saving.value = false;
  }
}
async function doDelete() {
  if (!editing.id) return;
  try {
    await Api.deleteMonitor(editing.id);
    await loadMonitors();
    message.success("已删除");
    showForm.value = false;
  } catch (e: any) {
    message.error(e?.message || "删除失败");
  }
}
</script>

<style scoped>
.topline {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  margin-bottom: 14px;
}
.title {
  font-size: 18px;
  font-weight: 700;
  color: var(--ct);
}
.sub {
  font-size: 12px;
  color: var(--text-muted);
  margin-top: 4px;
}
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 14px;
}
.mcard {
  border-radius: 14px;
  padding: 14px;
  background: var(--glass);
  border: 1px solid var(--glass-border);
  box-shadow: var(--shadow);
  backdrop-filter: blur(14px);
}
.mhead {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 4px;
}
.mdot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
}
.mdot.on {
  background: #22c55e;
  box-shadow: 0 0 6px rgba(34, 197, 94, 0.6);
}
.mdot.off {
  background: #ef4444;
}
.mname {
  font-weight: 700;
  color: var(--ct);
}
.mtype {
  font-size: 10px;
  padding: 1px 6px;
  border-radius: 6px;
  background: var(--bar-track);
  color: var(--text-muted);
}
.mtarget {
  font-size: 12px;
  color: var(--text-muted);
  font-family: ui-monospace, monospace;
  margin-bottom: 10px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.mstats {
  display: flex;
  justify-content: space-between;
  margin-bottom: 8px;
}
.mstats div {
  display: flex;
  flex-direction: column;
  font-size: 11px;
  color: var(--text-muted);
}
.mstats b {
  font-size: 14px;
  color: var(--text);
  margin-top: 2px;
}
.t-bg {
  color: #22c55e !important;
}
.t-by {
  color: #eab308 !important;
}
.t-br {
  color: #ef4444 !important;
}
.empty {
  grid-column: 1 / -1;
  text-align: center;
  padding: 40px;
  color: var(--text-muted);
}
.frow {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 13px;
  color: var(--text-muted);
}
.fbtns {
  display: flex;
  gap: 8px;
  align-items: center;
}
</style>
