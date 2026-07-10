<template>
  <div>
    <div class="lhead">
      <div>
        <h1>登录日志</h1>
        <p>最近 100 次面板登录 · 共 {{ logs.length }} 条</p>
      </div>
      <NPopconfirm @positive-click="clear">
        <template #trigger>
          <NButton size="small" type="error" ghost :disabled="logs.length === 0">清空</NButton>
        </template>
        确定清空全部登录日志？
      </NPopconfirm>
    </div>
    <div class="table">
      <div class="row head">
        <div>时间</div>
        <div>IP</div>
        <div>地点</div>
        <div>设备</div>
        <div>用户名</div>
      </div>
      <div v-for="(l, i) in logs" :key="i" class="row">
        <div>{{ fmtTime(l.ts) }}</div>
        <div>{{ l.ip || "-" }}</div>
        <div>{{ l.location || "-" }}</div>
        <div>{{ l.device || "-" }}</div>
        <div>{{ l.username }}</div>
      </div>
      <div v-if="logs.length === 0" class="empty">暂无登录记录</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from "vue";
import { NButton, NPopconfirm, useMessage } from "naive-ui";
import { Api } from "../api";
import type { LoginLog } from "../types";

const message = useMessage();
const logs = ref<LoginLog[]>([]);

async function load() {
  try {
    logs.value = await Api.loginLogs();
  } catch (e: any) {
    message.error(e?.message || "加载失败");
  }
}
async function clear() {
  try {
    await Api.clearLoginLogs();
    logs.value = [];
    message.success("已清空");
  } catch (e: any) {
    message.error(e?.message || "清空失败");
  }
}
function fmtTime(ts: number): string {
  const d = new Date(ts * 1000);
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
}
onMounted(load);
</script>

<style scoped>
.lhead {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  margin-bottom: 14px;
}
.lhead h1 {
  font-size: 20px;
  font-weight: 600;
  color: var(--ct);
  margin: 0;
}
.lhead p {
  font-size: 12px;
  color: var(--text-muted);
  margin-top: 4px;
}
.table {
  background: var(--glass);
  border: 1px solid var(--glass-border);
  border-radius: 14px;
  padding: 6px 14px;
}
.row {
  display: grid;
  grid-template-columns: 1.6fr 1.2fr 1.5fr 1.5fr 1fr;
  gap: 14px;
  padding: 13px 4px;
  font-size: 13px;
  color: var(--text);
  align-items: center;
}
.row + .row {
  border-top: 1px solid var(--color-line);
}
.row.head {
  font-size: 12px;
  color: var(--text-muted);
  letter-spacing: 0.5px;
}
.empty {
  text-align: center;
  padding: 40px;
  color: var(--text-muted);
}
</style>
