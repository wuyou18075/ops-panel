<template><div><div class="head"><div><h1>告警记录</h1><p>记录掉线、流量、CPU 与内存异常发生的时刻</p></div><NSelect v-model:value="filter" :options="options" style="width:150px"/></div><div class="events"><div v-for="e in shown" :key="e.id" class="event"><i :class="e.kind"></i><div class="emain"><div class="et"><b>{{e.title}}</b><span>{{nodeName(e.agent_id)}}</span><time>{{fmtTime(e.ts)}}</time></div><pre>{{e.detail}}</pre></div></div><div v-if="!shown.length" class="empty">暂无告警记录</div></div></div></template>
<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { NSelect } from "naive-ui";
import { Api } from "../api";
import { nodeViews } from "../store";
import type { AlertEvent } from "../types";
const list = ref<AlertEvent[]>([]);
const filter = ref("all");
const options = [{label:"全部告警",value:"all"},{label:"掉线",value:"offline"},{label:"流量",value:"traffic"},{label:"CPU",value:"cpu"},{label:"内存",value:"memory"}];
const shown = computed(() => filter.value === "all" ? list.value : list.value.filter(e => e.kind === filter.value));
const nodeName = (id:string) => nodeViews.value.find(n => n.id === id)?.name || id;
const fmtTime = (t:number) => new Date(t * 1000).toLocaleString();
onMounted(async () => { try { list.value = await Api.alertEvents(); } catch { /* ignore */ } });
</script>
<style scoped>.head{display:flex;justify-content:space-between;align-items:end;margin-bottom:16px}.head h1{margin:0;color:var(--ct);font-size:22px}.head p{color:var(--text-muted);font-size:12px}.events{display:flex;flex-direction:column;gap:10px}.event{display:flex;gap:14px;padding:16px;border-radius:15px;background:var(--glass);border:1px solid var(--glass-border);box-shadow:var(--shadow)}.event>i{width:9px;border-radius:8px;background:#ef4444}.event>i.traffic{background:#f59e0b}.event>i.cpu{background:#8b5cf6}.event>i.memory{background:#3b82f6}.emain{flex:1}.et{display:flex;gap:12px;align-items:center}.et b{color:var(--ct)}.et span{font-size:12px;color:var(--ca)}.et time{margin-left:auto;font-size:11px;color:var(--text-muted)}pre{white-space:pre-wrap;margin:9px 0 0;color:var(--text-muted);font:12px/1.6 ui-monospace,monospace}.empty{text-align:center;padding:60px;color:var(--text-muted)}</style>
