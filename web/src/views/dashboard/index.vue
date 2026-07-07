<template>
  <NLayout class="min-h-screen bg-canvas text-slate-100" has-sider>
    <NLayoutSider bordered collapse-mode="width" :collapsed-width="72" :width="224" class="hidden bg-panel md:block">
      <div class="flex h-16 items-center gap-3 border-b border-line px-5">
        <div class="grid h-9 w-9 place-items-center rounded bg-emerald-500 text-sm font-black text-white">OP</div>
        <div>
          <div class="text-sm font-semibold text-white">Ops Panel</div>
          <div class="text-xs text-slate-400">Soybean Admin</div>
        </div>
      </div>
      <div class="space-y-1 p-3">
        <div class="rounded bg-emerald-500/15 px-3 py-2 text-sm font-medium text-emerald-300">监控概览</div>
        <div class="rounded px-3 py-2 text-sm text-slate-400">节点列表</div>
        <div class="rounded px-3 py-2 text-sm text-slate-400">命令终端</div>
        <div class="rounded px-3 py-2 text-sm text-slate-400">告警策略</div>
      </div>
    </NLayoutSider>

    <NLayout>
      <NLayoutHeader bordered class="sticky top-0 z-10 bg-panel/95 backdrop-blur">
        <div class="flex flex-col gap-3 px-4 py-4 lg:flex-row lg:items-center lg:justify-between lg:px-6">
          <div>
            <div class="flex items-center gap-3">
              <span class="h-2.5 w-2.5 rounded-full bg-emerald-400 shadow-[0_0_16px_rgba(52,211,153,0.9)]" />
              <h1 class="text-xl font-semibold text-white">哪吒集群监控控制台</h1>
            </div>
            <p class="mt-1 text-sm text-slate-400">实时掌控 VPS 节点在线状态、资源占用、流量与远程命令执行结果</p>
          </div>
          <NSpace>
            <NTag :type="wsConnected ? 'success' : 'error'" round>
              {{ wsConnected ? "主控已连接" : "主控重连中" }}
            </NTag>
            <NTag type="info" round>在线 {{ onlineCount }}</NTag>
            <NButton size="small" tertiary @click="connectWebSocket(true)">刷新连接</NButton>
          </NSpace>
        </div>
      </NLayoutHeader>

      <NLayoutContent class="p-4 lg:p-6">
        <NGrid :cols="4" :x-gap="16" :y-gap="16" responsive="screen" item-responsive>
          <NGi span="4 s:2 l:1">
            <MetricCard label="在线节点" :value="String(onlineCount)" hint="Agent 实时连接" tone="green" />
          </NGi>
          <NGi span="4 s:2 l:1">
            <MetricCard label="平均 CPU" :value="`${avgCpu.toFixed(1)}%`" hint="所有在线节点" tone="blue" />
          </NGi>
          <NGi span="4 s:2 l:1">
            <MetricCard label="平均内存" :value="`${avgMem.toFixed(1)}%`" hint="所有在线节点" tone="violet" />
          </NGi>
          <NGi span="4 s:2 l:1">
            <MetricCard label="告警节点" :value="String(alertCount)" hint="CPU/内存/磁盘 > 80%" tone="red" />
          </NGi>
        </NGrid>

        <div class="mt-5 grid gap-5 xl:grid-cols-[1fr_380px]">
          <NCard title="节点资源总览" :bordered="false" class="border border-line bg-panel">
            <NDataTable
              :columns="columns"
              :data="tableData"
              :bordered="false"
              :pagination="{ pageSize: 8 }"
              size="small"
            />
            <NEmpty v-if="tableData.length === 0" class="py-12" description="等待 Agent 上报监控数据" />
          </NCard>

          <NCard title="实时事件流" :bordered="false" class="border border-line bg-panel">
            <div class="h-[420px] space-y-3 overflow-y-auto pr-1">
              <div v-if="eventLogs.length === 0" class="rounded border border-dashed border-line p-5 text-center text-sm text-slate-500">
                暂无事件，连接 Agent 后会显示上线、状态和命令输出
              </div>
              <div v-for="(item, index) in eventLogs" :key="index" class="rounded border border-line bg-black/20 p-3">
                <div class="flex items-center justify-between text-xs text-slate-500">
                  <span>{{ item.agentId }}</span>
                  <span>{{ item.time }}</span>
                </div>
                <div class="mt-1 font-mono text-xs text-slate-200">{{ item.text }}</div>
              </div>
            </div>
          </NCard>
        </div>
      </NLayoutContent>
    </NLayout>

    <NDrawer v-model:show="showDrawer" :width="680" placement="right" resizable>
      <NDrawerContent :title="`节点控制台 - ${activeNodeId}`">
        <div class="mb-4 grid gap-3">
          <NInputGroup>
            <NInput v-model:value="shellCommand" placeholder="输入 Shell 命令，例如 df -h" @keyup.enter="sendShellCommand" />
            <NButton type="primary" :disabled="!canSendCommand" @click="sendShellCommand">分发执行</NButton>
          </NInputGroup>
          <NSpace>
            <NButton v-for="cmd in quickCommands" :key="cmd" size="small" tertiary @click="shellCommand = cmd">
              {{ cmd }}
            </NButton>
          </NSpace>
        </div>
        <div class="h-[calc(100vh-220px)] overflow-y-auto rounded bg-black p-4 font-mono text-xs leading-6 text-emerald-300">
          <div v-if="terminalLogs.length === 0" class="text-slate-500">等待分发命令输入...</div>
          <div v-for="(log, index) in terminalLogs" :key="index">{{ log }}</div>
        </div>
      </NDrawerContent>
    </NDrawer>
  </NLayout>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, onUnmounted, ref } from "vue";
import {
  NButton,
  NCard,
  NDataTable,
  NDrawer,
  NDrawerContent,
  NEmpty,
  NGi,
  NGrid,
  NInput,
  NInputGroup,
  NLayout,
  NLayoutContent,
  NLayoutHeader,
  NLayoutSider,
  NProgress,
  NSpace,
  NTag,
  useMessage,
  type DataTableColumns,
} from "naive-ui";

type AgentStats = {
  cpu: number;
  mem: number;
  disk?: number;
  load1?: number;
  uptime?: number;
  net_sent?: number;
  net_recv?: number;
  updatedAt: number;
};

type WireMessage = {
  type: "stat" | "cmd" | "log";
  agent_id: string;
  data: string;
};

type EventLog = {
  agentId: string;
  text: string;
  time: string;
};

const MetricCard = defineComponent({
  props: {
    label: { type: String, required: true },
    value: { type: String, required: true },
    hint: { type: String, required: true },
    tone: { type: String, required: true },
  },
  setup(props) {
    const toneClass: Record<string, string> = {
      green: "from-emerald-500/20 to-emerald-500/5 text-emerald-300",
      blue: "from-sky-500/20 to-sky-500/5 text-sky-300",
      violet: "from-violet-500/20 to-violet-500/5 text-violet-300",
      red: "from-rose-500/20 to-rose-500/5 text-rose-300",
    };

    return () =>
      h(
        "div",
        { class: `rounded border border-line bg-gradient-to-br ${toneClass[props.tone]} p-4` },
        [
          h("div", { class: "text-sm text-slate-400" }, props.label),
          h("div", { class: "mt-3 text-3xl font-semibold text-white" }, props.value),
          h("div", { class: "mt-2 text-xs text-slate-500" }, props.hint),
        ],
      );
  },
});

const nodes = ref<Record<string, AgentStats>>({});
const wsConnected = ref(false);
const showDrawer = ref(false);
const activeNodeId = ref("");
const shellCommand = ref("df -h");
const terminalLogs = ref<string[]>([]);
const eventLogs = ref<EventLog[]>([]);
const message = useMessage();
const quickCommands = ["df -h", "free -m", "uptime", "docker ps", "systemctl --failed"];

let ws: WebSocket | null = null;
let reconnectTimer: number | undefined;

const onlineCount = computed(() => Object.keys(nodes.value).length);
const tableData = computed(() =>
  Object.entries(nodes.value).map(([id, stat]) => ({
    id,
    ...stat,
  })),
);
const avgCpu = computed(() => average(tableData.value.map((node) => node.cpu)));
const avgMem = computed(() => average(tableData.value.map((node) => node.mem)));
const alertCount = computed(
  () => tableData.value.filter((node) => node.cpu > 80 || node.mem > 80 || (node.disk ?? 0) > 80).length,
);
const canSendCommand = computed(
  () => wsConnected.value && activeNodeId.value !== "" && shellCommand.value.trim() !== "",
);

const columns: DataTableColumns<Record<string, number | string | undefined>> = [
  {
    title: "节点",
    key: "id",
    render(row) {
      return h("div", { class: "font-medium text-white" }, String(row.id));
    },
  },
  {
    title: "状态",
    key: "status",
    render() {
      return h(NTag, { type: "success", size: "small", round: true }, { default: () => "在线" });
    },
  },
  progressColumn("CPU", "cpu", "success"),
  progressColumn("内存", "mem", "info"),
  progressColumn("磁盘", "disk", "warning"),
  {
    title: "负载",
    key: "load1",
    render(row) {
      return `${Number(row.load1 ?? 0).toFixed(2)}`;
    },
  },
  {
    title: "网络",
    key: "net",
    render(row) {
      return `${formatBytes(Number(row.net_recv ?? 0))}/s ↓  ${formatBytes(Number(row.net_sent ?? 0))}/s ↑`;
    },
  },
  {
    title: "操作",
    key: "actions",
    render(row) {
      return h(
        NButton,
        {
          size: "small",
          type: "primary",
          secondary: true,
          onClick: () => openTerminal(String(row.id)),
        },
        { default: () => "控制台" },
      );
    },
  },
];

function progressColumn(title: string, key: string, status: "success" | "info" | "warning") {
  return {
    title,
    key,
    render(row: Record<string, number | string | undefined>) {
      const value = Number(row[key] ?? 0);
      return h("div", { class: "min-w-[120px]" }, [
        h("div", { class: "mb-1 text-xs text-slate-400" }, `${value.toFixed(1)}%`),
        h(NProgress, {
          percentage: Number(value.toFixed(1)),
          status: value > 80 ? "error" : status,
          showIndicator: false,
          height: 8,
        }),
      ]);
    },
  };
}

function average(values: number[]) {
  if (values.length === 0) {
    return 0;
  }
  return values.reduce((sum, value) => sum + value, 0) / values.length;
}

function formatBytes(value: number) {
  if (value < 1024) {
    return `${value.toFixed(0)} B`;
  }
  if (value < 1024 * 1024) {
    return `${(value / 1024).toFixed(1)} KB`;
  }
  return `${(value / 1024 / 1024).toFixed(1)} MB`;
}

function wsUrl() {
  const protocol = window.location.protocol === "https:" ? "wss" : "ws";
  return `${protocol}://${window.location.host}/ws/web`;
}

function connectWebSocket(force = false) {
  if (force && ws) {
    ws.close();
  }
  window.clearTimeout(reconnectTimer);
  ws = new WebSocket(wsUrl());

  ws.onopen = () => {
    wsConnected.value = true;
    message.success("已连接主控实时监控流");
  };

  ws.onmessage = (event) => {
    const rawMsg = JSON.parse(event.data) as WireMessage;
    if (rawMsg.type === "stat") {
      const payload = JSON.parse(rawMsg.data) as Omit<AgentStats, "updatedAt">;
      nodes.value[rawMsg.agent_id] = {
        cpu: Number(payload.cpu ?? 0),
        mem: Number(payload.mem ?? 0),
        disk: Number(payload.disk ?? 0),
        load1: Number(payload.load1 ?? 0),
        uptime: Number(payload.uptime ?? 0),
        net_recv: Number(payload.net_recv ?? 0),
        net_sent: Number(payload.net_sent ?? 0),
        updatedAt: Date.now(),
      };
      pushEvent(rawMsg.agent_id, "状态数据已刷新");
      return;
    }

    if (rawMsg.type === "log") {
      pushEvent(rawMsg.agent_id, rawMsg.data);
      if (showDrawer.value && rawMsg.agent_id === activeNodeId.value) {
        terminalLogs.value.push(rawMsg.data);
      }
    }
  };

  ws.onclose = () => {
    wsConnected.value = false;
    reconnectTimer = window.setTimeout(() => connectWebSocket(), 3000);
  };
}

function openTerminal(agentId: string) {
  activeNodeId.value = agentId;
  terminalLogs.value = [];
  showDrawer.value = true;
}

function sendShellCommand() {
  if (!canSendCommand.value || !ws || ws.readyState !== WebSocket.OPEN) {
    message.error("主控连接已断开，无法下发指令");
    return;
  }

  const packet: WireMessage = {
    type: "cmd",
    agent_id: activeNodeId.value,
    data: shellCommand.value.trim(),
  };

  terminalLogs.value.push(`$ ${packet.data}`);
  ws.send(JSON.stringify(packet));
}

function pushEvent(agentId: string, text: string) {
  eventLogs.value.unshift({
    agentId,
    text,
    time: new Date().toLocaleTimeString(),
  });
  eventLogs.value = eventLogs.value.slice(0, 80);
}

onMounted(() => {
  connectWebSocket();
});

onUnmounted(() => {
  window.clearTimeout(reconnectTimer);
  ws?.close();
});
</script>
