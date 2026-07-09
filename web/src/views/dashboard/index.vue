<template>
  <div>
    <!-- 顶部右侧：UI 模式切换 + 主题色切换 -->
    <div class="fixed right-4 top-2 z-50 flex items-center gap-3">
      <!-- UI 模式切换 -->
      <div class="flex items-center gap-1 rounded-lg border px-1.5 py-1" :style="{borderColor:'var(--color-line)',background:'var(--bg-panel)'}">
        <button
          v-for="m in uiModes" :key="m.k"
          class="rounded-md px-2.5 py-1 text-xs font-medium transition-all"
          :class="uiMode===m.k ? 'text-white' : 'text-slate-400 hover:text-slate-200'"
          :style="uiMode===m.k ? {background:'var(--ca)',color:'#fff'} : {}"
          :title="m.l"
          @click="uiMode=m.k"
        >{{ m.l }}</button>
      </div>
      <!-- 主题色圆点 -->
      <div class="flex gap-1">
        <div v-for="t in themeList" :key="t.k" class="h-6 w-6 cursor-pointer rounded-full border-2 transition-all" :class="theme===t.k?'border-white scale-110':'border-transparent opacity-60 hover:opacity-100'" :style="{background:t.c}" :title="t.l" @click="setTheme(t.k)" />
      </div>
    </div>

    <NLayout class="min-h-screen" :style="themeStyle">
      <NLayoutSider bordered collapse-mode="width" :collapsed-width="72" :width="224" class="hidden bg-panel md:block">
        <div class="flex h-16 items-center gap-3 border-b border-line px-5">
          <div class="grid h-9 w-9 place-items-center rounded text-sm font-black" :style="{background:'var(--ca)'}">OP</div>
          <div><div class="text-sm font-semibold" :style="{color:'var(--ct)'}">Ops Panel</div><div class="text-xs text-slate-400">Soybean Admin</div></div>
        </div>
        <div class="space-y-1 p-3">
          <div v-for="(p,i) in pages" :key="i" class="rounded px-3 py-2 text-sm cursor-pointer" :class="activePage===p.k?'bg-accent/15 text-accent':'text-slate-400 hover:bg-white/5'" @click="activePage=p.k">{{ p.l }}</div>
        </div>
        <div class="border-t border-line p-3">
          <div class="mb-2 flex items-center justify-between"><span class="text-xs text-slate-400">分组</span><NButton size="tiny" tertiary @click="showAddGroup=true">+</NButton></div>
          <div v-for="g in groups" :key="g" class="mb-1 cursor-pointer rounded px-2 py-1 text-xs" :class="selectedGroup===g?'bg-accent/20 text-accent':'text-slate-400 hover:bg-white/5'" @click="selectedGroup=g">{{ g }}</div>
        </div>
      </NLayoutSider>

      <NLayout>
        <NLayoutHeader bordered class="sticky top-0 z-10 bg-panel/95 backdrop-blur">
          <div class="flex flex-col gap-3 px-4 py-4 lg:flex-row lg:items-center lg:justify-between lg:px-6">
            <div>
              <div class="flex items-center gap-3">
                <span class="h-2.5 w-2.5 rounded-full" :style="{background:'var(--ca)',boxShadow:'0 0 16px rgba(var(--ca-rgb),0.9)'}" />
                <h1 class="text-xl font-semibold" :style="{color:'var(--ct)'}">{{ pageTitle }}</h1>
              </div>
              <p class="mt-1 text-sm text-slate-400">{{ pages.find(p=>p.k===activePage)?.s||'' }}</p>
            </div>
            <NSpace>
              <NTag :type="wsConnected?'success':'error'" round>{{ wsConnected?"已连接":"重连中" }}</NTag>
              <NTag type="info" round>在线{{ onlineDisplay }}</NTag>
              <NButton size="small" tertiary @click="connectViewerWS(true)">刷新</NButton>
              <NButton v-if="operatorToken" size="small" type="success" ghost @click="showLogin=true">运维中</NButton>
              <NButton v-else size="small" type="primary" ghost @click="showLogin=true">登录</NButton>
            </NSpace>
          </div>
        </NLayoutHeader>

        <NLayoutContent class="p-4 lg:p-6">
          <template v-if="activePage==='dashboard'">
            <NGrid :cols="4" :x-gap="16" :y-gap="16" responsive="screen" item-responsive>
              <NGi span="4 s:2 l:1"><MC label="在线" :v="String(onlineCount)" tone="g" /></NGi>
              <NGi span="4 s:2 l:1"><MC label="CPU" :v="avgCpu.toFixed(1)+'%'" tone="b" /></NGi>
              <NGi span="4 s:2 l:1"><MC label="内存" :v="avgMem.toFixed(1)+'%'" tone="v" /></NGi>
              <NGi span="4 s:2 l:1"><MC label="告警" :v="String(alertCount)" tone="r" /></NGi>
            </NGrid>

            <!-- ── Watcher 毛玻璃卡片模式 ── -->
            <div v-if="uiMode==='watcher'" class="watcher-mode mt-5">
              <div class="w-tabs">
                <div class="w-tab" :class="selectedGroup==='全部'?'active':''" @click="selectedGroup='全部'">所有</div>
                <div v-for="g in groups" :key="g" class="w-tab" :class="selectedGroup===g?'active':''" @click="selectedGroup=g">{{ g }}</div>
              </div>
              <div class="w-grid">
                <div
                  v-for="n in filteredData" :key="n.id"
                  class="w-card"
                  :class="{'w-card-off':!n.online}"
                  @click="term(n.id)"
                >
                  <div class="w-card-head">
                    <div class="w-name">
                      <span class="w-flag" :style="{background:'var(--ca)'}"></span>
                      {{ agentNames[n.id as string]||n.id }}
                    </div>
                    <span class="w-dot" :class="n.online?'on':'off'"></span>
                  </div>
                  <div class="w-specs">
                    <span><i class="w-dot-sm"></i> {{ (n.cpuCount||0) }} Cores</span>
                    <span><i class="w-dot-sm"></i> {{ fmtMem(n.memTotal) }}</span>
                    <span><i class="w-dot-sm"></i> {{ fmtDisk(n.diskTotal) }}</span>
                  </div>
                  <div class="w-metric">
                    <div class="w-mrow"><span class="k">CPU</span><span class="v">{{ (n.cpu||0).toFixed(1) }}%</span></div>
                    <div class="w-bar"><i :class="barClass(n.cpu)" :style="{width:Math.min(n.cpu||0,100)+'%'}"></i></div>
                  </div>
                  <div class="w-metric">
                    <div class="w-mrow"><span class="k">内存</span><span class="v">{{ (n.mem||0).toFixed(1) }}%</span></div>
                    <div class="w-bar"><i :class="barClass(n.mem)" :style="{width:Math.min(n.mem||0,100)+'%'}"></i></div>
                  </div>
                  <div class="w-metric">
                    <div class="w-mrow"><span class="k">SWAP</span><span class="v">{{ n.swapPct!=null?(n.swapPct).toFixed(0)+'%':'OFF' }}</span></div>
                    <div class="w-bar"><i :class="barClass(n.swapPct||0)" :style="{width:Math.min(n.swapPct||0,100)+'%'}"></i></div>
                  </div>
                  <div class="w-metric">
                    <div class="w-mrow"><span class="k">硬盘</span><span class="v">{{ (n.disk||0).toFixed(1) }}%</span></div>
                    <div class="w-bar"><i :class="barClass(n.disk)" :style="{width:Math.min(n.disk||0,100)+'%'}"></i></div>
                  </div>
                  <div class="w-net">
                    <div class="w-net-row"><span>网络:</span><span>↑ {{ fmtB(n.net_sent||0) }}/s ↓ {{ fmtB(n.net_recv||0) }}/s</span></div>
                    <div class="w-net-row"><span>负载:</span><span class="b">{{ n.load1!=null?(n.load1.toFixed(2)+' | '+n.load5?.toFixed(2)+' | '+n.load15?.toFixed(2)):'-' }}</span></div>
                  </div>
                  <div class="w-foot">
                    <span>{{ n.uptime||'' }}</span>
                    <span :class="n.online?'w-online':'w-offline'">{{ n.online?'在线':'离线' }}</span>
                  </div>
                </div>
                <div v-if="filteredData.length===0" class="w-empty">暂无节点数据</div>
              </div>
            </div>

            <!-- ── Beszel 深色表格模式 ── -->
            <div v-else-if="uiMode==='beszel'" class="beszel-mode mt-5">
              <div class="b-header">
                <div>
                  <h1 :style="{color:'var(--ct)'}">All Systems</h1>
                  <p>实时更新 · 点击系统查看详情 · 共 {{ filteredData.length }} 个节点</p>
                </div>
                <div class="b-controls">
                  <input v-model="filterText" placeholder="Filter..." />
                </div>
              </div>
              <div class="b-table">
                <div class="b-row b-head">
                  <div>System</div>
                  <div>CPU</div>
                  <div>Memory</div>
                  <div>Disk</div>
                  <div>Net</div>
                  <div>状态</div>
                  <div></div>
                </div>
                <div
                  v-for="n in filteredData" :key="n.id"
                  class="b-row"
                  @click="term(n.id)"
                >
                  <div class="b-name">
                    <span class="b-dot" :class="n.online?'on':'off'"></span>
                    {{ agentNames[n.id as string]||n.id }}
                    <span class="b-link">↗</span>
                  </div>
                  <div class="b-cell">
                    <div class="b-meter"><i :class="barClass(n.cpu)" :style="{width:Math.min(n.cpu||0,100)+'%'}"></i></div>
                    <span class="b-pct">{{ (n.cpu||0).toFixed(1) }}%</span>
                  </div>
                  <div class="b-cell">
                    <div class="b-meter"><i :class="barClass(n.mem)" :style="{width:Math.min(n.mem||0,100)+'%'}"></i></div>
                    <span class="b-pct">{{ (n.mem||0).toFixed(1) }}%</span>
                  </div>
                  <div class="b-cell">
                    <div class="b-meter"><i :class="barClass(n.disk)" :style="{width:Math.min(n.disk||0,100)+'%'}"></i></div>
                    <span class="b-pct">{{ (n.disk||0).toFixed(1) }}%</span>
                  </div>
                  <div class="b-net">{{ fmtB(n.net_recv||0)+fmtB(n.net_sent||0) }}/s</div>
                  <div class="b-agent">
                    <span class="b-adot" :class="n.online?'on':'off'"></span>
                    {{ n.online?'0.8.0':'离线' }}
                  </div>
                  <div class="b-icons"><span>🔔</span><span @click.stop="term(n.id)">⌯</span></div>
                </div>
                <div v-if="filteredData.length===0" class="b-empty">暂无节点数据</div>
              </div>
            </div>

            <!-- ── 原始 Naive UI 表格/卡片模式（保留作为 fallback）── -->
            <div v-else class="mt-5">
              <div class="mb-3 flex items-center gap-3">
                <span class="text-lg font-semibold" :style="{color:'var(--ct)'}">节点</span>
                <NSwitch :value="viewMode==='cards'" @update:value="viewMode=$event?'cards':'table'" size="small" />
                <span class="text-xs text-slate-400">{{ viewMode==='table'?'表格':'卡片' }}</span>
              </div>
              <NCard v-if="viewMode==='table'" :bordered="false" class="border border-line bg-panel">
                <NDataTable :columns="cols" :data="mergedData" :bordered="false" :pagination="{pageSize:8}" size="small" :row-class="()=>'striped-row'" />
              </NCard>
              <div v-if="viewMode==='cards'" class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
                <div v-for="n in mergedData" :key="n.id" class="rounded-lg border border-line bg-panel p-4 cursor-pointer hover:border-accent/50 transition-all" @click="term(n.id)">
                  <div class="flex items-center justify-between mb-2">
                    <span class="text-sm font-medium text-white">{{ agentNames[n.id as string]||n.id }}</span>
                    <span class="text-xs">{{ n.online?'🟢':'🔴' }}</span>
                  </div>
                  <div class="grid grid-cols-2 gap-1 text-xs text-slate-400">
                    <div>CPU {{ n.cpu?.toFixed(1)||'0' }}% <NProgress :percentage="n.cpu||0" :height="3" :status="(n.cpu||0)>80?'error':'success'" :show-indicator="false" /></div>
                    <div>内存 {{ n.mem?.toFixed(1)||'0' }}% <NProgress :percentage="n.mem||0" :height="3" :status="(n.mem||0)>80?'error':'info'" :show-indicator="false" /></div>
                    <div>日流量 {{ fmtB((n.net_recv||0)+(n.net_sent||0)) }}</div>
                    <div>月流量 {{ fmtB((n.net_recv||0)+(n.net_sent||0)) }}</div>
                  </div>
                </div>
              </div>
            </div>
          </template>

          <template v-if="activePage==='nodes'">
            <div class="mb-4 flex items-center justify-between">
              <span class="text-lg font-semibold" :style="{color:'var(--ct)'}">全部节点（{{ mergedData.length }}）</span>
              <NButton type="primary" size="small" @click="showEnroll=true">+ 添加</NButton>
            </div>
            <NCard :bordered="false" class="border border-line bg-panel"><NDataTable :columns="nodeCols" :data="mergedData" :bordered="false" :pagination="{pageSize:15}" size="small" /></NCard>
          </template>

          <template v-if="activePage==='terminal'">
            <div class="mb-4 flex items-center justify-between">
              <div><span class="text-lg font-semibold" :style="{color:'var(--ct)'}">命令终端</span><p class="mt-1 text-sm text-slate-400">选择节点执行Shell命令</p></div>
              <NButton v-if="activeNodeId" size="small" tertiary @click="closeTerminal">关闭</NButton>
            </div>
            <NGrid :cols="4" :x-gap="16" responsive="screen" item-responsive>
              <NGi span="4 l:1">
                <NCard title="在线节点" :bordered="false" class="border border-line bg-panel">
                  <div v-for="n in mergedData" :key="n.id" class="mb-1 cursor-pointer rounded p-2 text-sm" :class="activeNodeId===n.id?'bg-accent/20 text-accent':'text-slate-400 hover:bg-white/5'" @click="activeNodeId=n.id as string;terminalLogs=[]">{{ agentNames[n.id as string]||n.id }}</div>
                </NCard>
              </NGi>
              <NGi span="4 l:3">
                <NCard :title="'终端 - '+(agentNames[activeNodeId]||activeNodeId||'未选择')" :bordered="false" class="border border-line bg-panel">
                  <template v-if="operatorToken && activeNodeId">
                    <div class="mb-4 grid gap-3">
                      <NInputGroup><NInput v-model:value="shellCommand" placeholder="输入Shell命令" @keyup.enter="sendShellCommand" /><NButton type="primary" :disabled="!canSend" @click="sendShellCommand">执行</NButton></NInputGroup>
                      <NSpace><NButton v-for="c in qc" :key="c" size="small" tertiary @click="shellCommand=c">{{ c }}</NButton></NSpace>
                    </div>
                    <div class="h-[400px] overflow-y-auto rounded bg-black p-4 font-mono text-xs leading-6 text-emerald-300"><div v-if="terminalLogs.length===0" class="text-slate-500">等待命令...</div><div v-for="(l,i) in terminalLogs" :key="i">{{ l }}</div></div>
                  </template>
                  <template v-else><NEmpty v-if="!operatorToken" description="请先登录" /><NEmpty v-else description="请选择节点" /></template>
                </NCard>
              </NGi>
            </NGrid>
          </template>

          <template v-if="activePage==='alerts'">
            <div class="mb-4"><span class="text-lg font-semibold" :style="{color:'var(--ct)'}">告警策略</span><p class="mt-1 text-sm text-slate-400">设置CPU/内存/磁盘/离线阈值</p></div>
            <NCard :bordered="false" class="border border-line bg-panel" style="max-width:480px">
              <NSpace vertical>
                <div class="flex items-center gap-4"><span class="w-32 text-sm text-slate-400">启用</span><NSwitch v-model:value="alertCfg.enabled" /></div>
                <div v-for="(item,i) in alertFields" :key="i" class="flex items-center gap-4">
                  <span class="w-32 text-sm text-slate-400">{{ item.l }}</span>
                  <NInputNumber v-model:value="alertCfg[item.k]" :min="1" :max="item.k==='offline_minutes'?60:100" class="w-24" />
                  <span class="text-xs text-slate-500">{{ item.u }}</span>
                </div>
                <NButton type="primary" :loading="savingAlert" @click="saveAlertConfig">保存</NButton>
              </NSpace>
            </NCard>
          </template>
        </NLayoutContent>
      </NLayout>

      <NModal v-model:show="showLogin" title="登录" :mask-closable="false" preset="card" style="width:400px">
        <NSpace vertical>
          <NInput v-model:value="loginU" placeholder="用户名" />
          <NInput v-model:value="loginP" type="password" placeholder="密码" />
          <NInput v-model:value="loginTOTP" placeholder="6位动态码（已开启时必填）" />
          <NButton type="primary" block :loading="loginLoading" @click="doLogin">登录</NButton>
        </NSpace>
      </NModal>

      <NModal v-model:show="showEnroll" title="添加节点" :mask-closable="false" preset="card" style="width:500px">
        <NSpace vertical>
          <NInput v-model:value="enrollName" placeholder="备注名称" />
          <NSelect v-model:value="enrollGroup" :options="groupOptions" placeholder="分组" />
          <NSpace align="center"><span class="text-sm text-slate-400">控制台</span><NSwitch v-model:value="enrollConsole" /></NSpace>
          <NSpace><NCheckbox v-model:checked="enrollTraffic">流量监控</NCheckbox><NCheckbox v-model:checked="enrollReport">日报</NCheckbox></NSpace>
          <div class="flex items-center gap-2"><span class="text-sm text-slate-400">刷新频率</span><NInputNumber v-model:value="enrollInterval" :min="1" :max="60" class="w-20" /><span class="text-xs text-slate-400">秒</span></div>
          <div v-if="enrollCommand" class="rounded bg-black p-3 font-mono text-xs text-emerald-300">
            <div class="mb-2 text-slate-400">在目标VPS上执行：</div>
            <div class="select-all break-all">{{ enrollCommand }}</div>
          </div>
          <NButton v-if="!enrollCommand" type="primary" block :loading="enrolling" @click="doEnroll">生成命令</NButton>
          <NButton v-else type="warning" block @click="resetEnroll">完成</NButton>
          <NButton v-if="enrollCommand" size="small" quaternary @click="copyCommand">复制</NButton>
        </NSpace>
      </NModal>

      <NModal v-model:show="showAddGroup" title="新建分组" :mask-closable="false" preset="card" style="width:350px">
        <NSpace vertical><NInput v-model:value="newGroupName" placeholder="名称" /><NButton type="primary" block :loading="addingGroup" @click="doAddGroup">创建</NButton></NSpace>
      </NModal>
    </NLayout>
  </div>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, onUnmounted, ref } from "vue";
import { NButton, NCard, NCheckbox, NDataTable, NEmpty, NGi, NGrid, NInput, NInputGroup, NInputNumber, NLayout, NLayoutContent, NLayoutHeader, NLayoutSider, NModal, NProgress, NSelect, NSpace, NSwitch, NTag, useMessage, type DataTableColumns, type SelectOption } from "naive-ui";

type T = "midnight"|"forest"|"ocean"|"sunset"|"aurora";
const themeList = [
  { k:"midnight", l:"极夜黑", c:"#1a1a2e" },
  { k:"forest",  l:"护眼绿", c:"#1a3a2a" },
  { k:"ocean",   l:"海洋蓝", c:"#0a2440" },
  { k:"sunset",  l:"落日橙", c:"#2a1a10" },
  { k:"aurora",  l:"极光紫", c:"#1a1030" },
];
const themeMap: Record<string,any> = {
  midnight: { body:"#0f1117", panel:"#171a22", line:"#2b3140", ca:"#18a058", car:"24,160,88", ct:"#e5e7eb" },
  forest:   { body:"#0d1f14", panel:"#152a1c", line:"#2a4535", ca:"#34d399", car:"52,211,153", ct:"#d1fae5" },
  ocean:    { body:"#0a1628", panel:"#0f2240", line:"#1a3a60", ca:"#38bdf8", car:"56,189,248", ct:"#e0f2fe" },
  sunset:   { body:"#1a0e08", panel:"#2a1a10", line:"#4a3020", ca:"#fb923c", car:"251,146,60", ct:"#fff7ed" },
  aurora:   { body:"#120820", panel:"#1c0e30", line:"#302050", ca:"#a78bfa", car:"167,139,250", ct:"#f5f3ff" },
};
const theme = ref("midnight");
const themeStyle = ref({});
// UI 渲染模式：watcher(毛玻璃卡片) | beszel(深色表格) | classic(原始 Naive UI)
type UiMode = "watcher"|"beszel"|"classic";
const uiModes: {k:UiMode,l:string}[] = [
  { k:"watcher", l:"卡片" },
  { k:"beszel",  l:"表格" },
  { k:"classic", l:"经典" },
];
const uiMode = ref<UiMode>("watcher");
const filterText = ref("");

// 按分组过滤后的节点列表（供 watcher/beszel 使用）
const filteredData = computed(() => {
  let list = mergedData.value;
  if (selectedGroup.value !== "全部") {
    list = list.filter((n: any) => n.group === selectedGroup.value);
  }
  if (filterText.value.trim()) {
    const q = filterText.value.toLowerCase();
    list = list.filter((n: any) => (agentNames.value[n.id as string] || n.id).toLowerCase().includes(q));
  }
  return list;
});

// 进度条颜色：<60% 绿, 60-80% 黄, >80% 红
function barClass(v: number | undefined): string {
  const p = v ?? 0;
  if (p > 80) return "br";
  if (p > 60) return "by";
  return "bg";
}
// 内存 / 磁盘容量格式化（节点上报的字节数）
function fmtMem(v: number | undefined): string {
  if (v == null) return "-";
  if (v < 1073741824) return (v / 1048576).toFixed(0) + " MB";
  return (v / 1073741824).toFixed(1) + " GB";
}
function fmtDisk(v: number | undefined): string {
  if (v == null) return "-";
  if (v < 1073741824) return (v / 1048576).toFixed(0) + " MB";
  return (v / 1073741824).toFixed(1) + " GB";
}

const pages = [{k:"dashboard",l:"监控概览",s:"实时监控VPS状态"},{k:"nodes",l:"节点列表",s:"管理全部节点"},{k:"terminal",l:"命令终端",s:"远程命令执行"},{k:"alerts",l:"告警策略",s:"设置触发条件"}];

const pageTitle = computed(() => pages.find(p=>p.k===activePage.value)?.l||"");
function setTheme(k: string) { theme.value=k; const m=themeMap[k]; themeStyle.value={"--bg-body":m.body,"--bg-panel":m.panel,"--color-line":m.line,"--ca":m.ca,"--ca-rgb":m.car,"--ct":m.ct}; document.documentElement.style.background=m.body; }

const MC = defineComponent({
  props:{label:String,v:String,tone:String},
  setup(p: any) {
    const tc:Record<string,string>={g:"from-emerald-500/20 text-emerald-300",b:"from-sky-500/20 text-sky-300",v:"from-violet-500/20 text-violet-300",r:"from-rose-500/20 text-rose-300"};
    return ()=>h("div",{class:`rounded border border-line bg-gradient-to-br ${tc[p.tone as string]||""} p-4`},[h("div",{class:"text-sm text-slate-400"},p.label),h("div",{class:"mt-3 text-3xl font-semibold text-white"},p.v)]);
  },
});

const basePath = (()=>{const p=window.location.pathname.split("/").filter(Boolean);return p.length?"/"+p[0]:"";})();
const api=(p:string)=>basePath+p;
const wsUrl=(p:string)=>(location.protocol==="https:"?"wss":"ws")+"://"+location.host+api(p);

const nodes=ref<Record<string,any>>({});
const agentNames=ref<Record<string,string>>({});
const viewMode=ref<"table"|"cards">("table");
const wsConnected=ref(false);
const terminalLogs=ref<string[]>([]);
const activeNodeId=ref("");
const shellCommand=ref("df -h");
const qc=["df -h","free -m","uptime","docker ps","systemctl --failed"];
const showLogin=ref(false);const loginU=ref("");const loginP=ref("");const loginTOTP=ref("");const loginLoading=ref(false);
const operatorToken=ref("");const refreshToken=ref("");
const showEnroll=ref(false);const enrollName=ref("");const enrollGroup=ref("");const enrollConsole=ref(false);const enrollTraffic=ref(true);const enrollReport=ref(false);const enrollInterval=ref(5);
const enrollCommand=ref("");const enrollResult=ref<any>(null);const enrolling=ref(false);
const showAddGroup=ref(false);const newGroupName=ref("");const addingGroup=ref(false);
const groups=ref<string[]>(["默认分组"]);const selectedGroup=ref("全部");const activePage=ref("dashboard");
const savingAlert=ref(false);
const alertCfg=ref<any>({cpu_percent:80,mem_percent:80,disk_percent:80,offline_minutes:5,enabled:false});
const alertFields=[{k:"cpu_percent",l:"CPU阈值",u:"%"},{k:"mem_percent",l:"内存阈值",u:"%"},{k:"disk_percent",l:"磁盘阈值",u:"%"},{k:"offline_minutes",l:"离线告警",u:"分钟"}];
let viewerWS:WebSocket|null=null;let operatorWS:WebSocket|null=null;let reconnectTimer:number|undefined;
const nowMs=ref(Date.now());let clockTimer:number|undefined;const message=useMessage();
const groupOptions=computed<SelectOption[]>(()=>groups.value.map(g=>({label:g,value:g})));
const onlineCount=computed(()=>Object.values(nodes.value).filter((n:any)=>nowMs.value-(n.updatedAt||0)<10000).length);
const onlineDisplay=computed(()=>`${onlineCount.value}/${Object.keys(nodes.value).length}`);
const rawData=computed(()=>Object.entries(nodes.value).map(([id,s])=>({id,name:agentNames.value[id]||"",...s as any,online:nowMs.value-((s as any).updatedAt||0)<10000})));
const mergedData=computed(()=>rawData.value);
const avgCpu=computed(()=>{const v=rawData.value.map((n:any)=>n.cpu);return v.length?v.reduce((a,b)=>a+b)/v.length:0;});
const avgMem=computed(()=>{const v=rawData.value.map((n:any)=>n.mem);return v.length?v.reduce((a,b)=>a+b)/v.length:0;});
const alertCount=computed(()=>rawData.value.filter((n:any)=>n.cpu>80||n.mem>80||(n.disk??0)>80).length);
const canSend=computed(()=>!!operatorToken.value&&!!activeNodeId.value&&shellCommand.value.trim()!=="");
function closeTerminal(){activeNodeId.value="";terminalLogs.value=[];}
function term(id:string){activeNodeId.value=id;terminalLogs.value=[];activePage.value="terminal";}
function fmtB(v:number){if(v<1024)return `${v.toFixed(0)}B`;if(v<1048576)return `${(v/1024).toFixed(1)}KB`;return `${(v/1048576).toFixed(1)}MB`;}
const cols:DataTableColumns<any>=[
  {title:"节点",key:"name",render(r){return h("div",{class:"text-white font-medium"},agentNames.value[r.id]||r.id)}},
  {title:"状态",key:"online",render(_r,i){const n=rawData.value[i];return h(NTag,{type:n?.online?"success":"error",size:"small",round:true},{default:()=>n?.online?"在线":"离线"})}},
  pCol("CPU","cpu","success"),pCol("内存","mem","info"),pCol("磁盘","disk","warning"),
  {title:"日流量",key:"daily",render(r){return fmtB(Number(r.net_recv||0)+Number(r.net_sent||0))}},
  {title:"月流量",key:"monthly",render(r){return fmtB(Number(r.net_recv||0)+Number(r.net_sent||0))}},
  {title:"操作",key:"act",render(r){return h(NButton,{size:"small",type:"primary",secondary:true,onClick:()=>term(r.id)},{default:()=>"控制台"})}},
];
const nodeCols:DataTableColumns<any>=[
  {title:"ID",key:"id",render(r){return h("div",{class:"font-mono text-xs text-white"},r.id)}},
  {title:"名称",key:"name",render(r){return agentNames.value[r.id]||"-"}},
  {title:"状态",key:"online",render(_r,i){const n=rawData.value[i];return h(NTag,{type:n?.online?"success":"error",size:"small",round:true},{default:()=>n?.online?"在线":"离线"})}},
  pCol("CPU","cpu","success"),pCol("内存","mem","info"),pCol("磁盘","disk","warning"),
  {title:"日流量",key:"daily",render(r){return fmtB(Number(r.net_recv||0)+Number(r.net_sent||0))}},
  {title:"月流量",key:"monthly",render(r){return fmtB(Number(r.net_recv||0)+Number(r.net_sent||0))}},
];
function pCol(t:string,k:string,s:"success"|"info"|"warning"){return{title:t,key:k,render(r:any){const v=Number(r[k]??0);return h("div",{class:"min-w-[100px]"},[h("div",{class:"mb-1 text-xs text-slate-400"},`${v.toFixed(1)}%`),h(NProgress,{percentage:Number(v.toFixed(1)),status:v>80?"error":s,showIndicator:false,height:5})])}}}
async function loadNames(){try{const r=await fetch(api("/api/agents"));if(r.ok){const list=await r.json();list.forEach((a:any)=>agentNames.value[a.agent_id]=a.name||"")}}catch{}}
function connectViewerWS(force=false){if(force&&viewerWS)viewerWS.close();clearTimeout(reconnectTimer);viewerWS=new WebSocket(wsUrl("/ws/web"));viewerWS.onopen=()=>{wsConnected.value=true;};viewerWS.onmessage=(e)=>{let m;try{m=JSON.parse(e.data)}catch{return};if(m.type==="stat"){try{const p=JSON.parse(m.data);nodes.value[m.agent_id]={...nodes.value[m.agent_id],cpu:Number(p.cpu??0),mem:Number(p.mem??0),disk:Number(p.disk??0),net_recv:Number(p.net_recv??0),net_sent:Number(p.net_sent??0),swapPct:p.swap_pct!=null?Number(p.swap_pct):undefined,cpuCount:p.cpu_count!=null?Number(p.cpu_count):undefined,load1:p.load_1!=null?Number(p.load_1):undefined,load5:p.load_5!=null?Number(p.load_5):undefined,load15:p.load_15!=null?Number(p.load_15):undefined,memTotal:p.mem_total!=null?Number(p.mem_total):undefined,diskTotal:p.disk_total!=null?Number(p.disk_total):undefined,uptime:p.uptime!=null?String(p.uptime):undefined,updatedAt:Date.now()}}catch{}}};viewerWS.onclose=()=>{wsConnected.value=false;reconnectTimer=window.setTimeout(()=>connectViewerWS(),3000);};}
function connectOperatorWS(token:string){operatorWS?.close();operatorWS=new WebSocket(wsUrl("/ws/operator?token="+encodeURIComponent(token)));operatorWS.onclose=()=>{operatorToken.value="";operatorWS=null;};}
async function doLogin(){if(!loginP.value){message.error("请输入密码");return}loginLoading.value=true;try{const r=await fetch(api("/api/login"),{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({username:loginU.value||"admin",password:loginP.value,code:loginTOTP.value})});if(!r.ok){message.error(await r.text()||"登录失败");return}const d=await r.json();operatorToken.value=d.access_token;refreshToken.value=d.refresh_token;connectOperatorWS(d.access_token);message.success("登录成功");showLogin.value=false;loginP.value="";loginTOTP.value="";setTimeout(async()=>{try{const r=await fetch(api("/api/refresh"),{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({refresh_token:refreshToken.value})});if(r.ok){const d=await r.json();operatorToken.value=d.access_token;connectOperatorWS(d.access_token)}else{operatorToken.value="";refreshToken.value=""}}catch{}},13*60*1000)}catch{message.error("网络失败")}finally{loginLoading.value=false}}
async function doEnroll(){if(!enrollName.value){message.error("请输入名称");return}enrolling.value=true;try{const r=await fetch(api("/api/enroll"),{method:"POST",headers:{"Content-Type":"application/json","Authorization":"Bearer "+operatorToken.value},body:JSON.stringify({name:enrollName.value,group:enrollGroup.value||"默认分组",enable_console:enrollConsole.value,track_traffic:enrollTraffic.value,daily_report:enrollReport.value,interval:enrollInterval.value})});if(!r.ok){message.error(await r.text());return}const d=await r.json();enrollResult.value=d;enrollCommand.value=d.install_cmd;copyCommand()}catch{message.error("注册失败")}finally{enrolling.value=false}}
function copyCommand(){const txt=enrollCommand.value;if(!txt)return;(navigator.clipboard?.writeText(txt).catch(()=>fallbackCopy(txt))||fallbackCopy(txt))}
function fallbackCopy(txt: string){const ta=document.createElement("textarea");ta.value=txt;ta.style.position="fixed";ta.style.opacity="0";document.body.appendChild(ta);ta.select();document.execCommand("copy");document.body.removeChild(ta);message.success("已复制")}
function resetEnroll(){showEnroll.value=false;enrollCommand.value="";enrollResult.value=null;enrollName.value="";enrollGroup.value="";}
async function doAddGroup(){if(!newGroupName.value)return;addingGroup.value=true;try{const r=await fetch(api("/api/groups"),{method:"POST",headers:{"Content-Type":"application/json","Authorization":"Bearer "+operatorToken.value},body:JSON.stringify({name:newGroupName.value})});if(!r.ok){message.error(await r.text());return}groups.value=[...groups.value,newGroupName.value];showAddGroup.value=false;newGroupName.value="";message.success("已创建")}catch{}finally{addingGroup.value=false}}
async function saveAlertConfig(){savingAlert.value=true;try{const r=await fetch(api("/api/alerts"),{method:"POST",headers:{"Content-Type":"application/json","Authorization":"Bearer "+operatorToken.value},body:JSON.stringify(alertCfg.value)});r.ok?message.success("已保存"):message.error(await r.text())}catch{}finally{savingAlert.value=false}}
function sendShellCommand(){if(!canSend.value||!operatorWS||operatorWS.readyState!==WebSocket.OPEN){message.error("未就绪");return}const cmd=shellCommand.value.trim();terminalLogs.value.push(`$ ${cmd}`);operatorWS.send(JSON.stringify({type:"cmd",agent_id:activeNodeId.value,data:cmd}))}
async function loadGroups(){try{const r=await fetch(api("/api/groups"));if(r.ok)groups.value=await r.json()}catch{}}
async function loadAlertConfig(){try{const r=await fetch(api("/api/alerts"));if(r.ok)alertCfg.value=await r.json()}catch{}}
onMounted(()=>{setTheme("midnight");connectViewerWS();loadNames();loadGroups();loadAlertConfig();clockTimer=window.setInterval(()=>{nowMs.value=Date.now()},1000)});
onUnmounted(()=>{clearTimeout(reconnectTimer);clearInterval(clockTimer);viewerWS?.close();operatorWS?.close()});
</script>

<style scoped>
/* ══════════════════════════════════════════════════════
   Watcher 毛玻璃卡片模式
   ══════════════════════════════════════════════════════ */
.watcher-mode { /* 容器 */ }

.w-tabs {
  display:flex;
  gap:4px;
  margin-bottom:14px;
  padding:6px;
  border-radius:12px;
  background: color-mix(in srgb, var(--bg-panel) 75%, transparent);
  backdrop-filter: blur(12px);
  width:fit-content;
}
.w-tab {
  padding:7px 18px;
  border-radius:9px;
  font-size:13px;
  cursor:pointer;
  color:#888;
  transition: all .15s;
}
.w-tab:hover { color:var(--ct); }
.w-tab.active {
  background:var(--bg-panel);
  color:var(--ct);
  font-weight:600;
  box-shadow:0 1px 3px rgba(0,0,0,.3);
}

.w-grid {
  display:grid;
  grid-template-columns:repeat(4,1fr);
  gap:14px;
}
@media(max-width:1100px){ .w-grid{grid-template-columns:repeat(2,1fr)} }
@media(max-width:600px){ .w-grid{grid-template-columns:1fr} }

.w-card {
  border-radius:16px;
  padding:16px;
  background: color-mix(in srgb, var(--bg-panel) 72%, transparent);
  backdrop-filter: blur(18px);
  border:1px solid color-mix(in srgb, var(--color-line) 60%, transparent);
  box-shadow:0 4px 20px rgba(0,0,0,.08);
  cursor:pointer;
  transition:transform .15s, box-shadow .15s, border-color .15s;
}
.w-card:hover {
  transform:translateY(-3px);
  box-shadow:0 8px 28px rgba(0,0,0,.16);
  border-color: color-mix(in srgb, var(--ca) 50%, transparent);
}
.w-card-off { opacity:.55; }

.w-card-head {
  display:flex;
  align-items:center;
  justify-content:space-between;
  margin-bottom:6px;
}
.w-name {
  font-weight:700;
  font-size:14px;
  display:flex;
  align-items:center;
  gap:6px;
  color:var(--ct);
}
.w-flag {
  width:18px;
  height:13px;
  border-radius:2px;
  display:inline-block;
  flex-shrink:0;
}

.w-dot {
  width:8px;
  height:8px;
  border-radius:50%;
  display:inline-block;
}
.w-dot.on  { background:#22c55e; box-shadow:0 0 6px rgba(34,197,94,.6); }
.w-dot.off { background:#ef4444; }

.w-specs {
  display:flex;
  gap:14px;
  font-size:12px;
  color:#888;
  margin-bottom:12px;
}
.w-specs span { display:flex; align-items:center; gap:3px; }
.w-dot-sm {
  width:7px;
  height:7px;
  border-radius:50%;
  background:var(--ca);
  display:inline-block;
}

.w-metric { margin-bottom:7px; }
.w-mrow {
  display:flex;
  justify-content:space-between;
  font-size:12px;
  margin-bottom:2px;
}
.w-mrow .k { color:#888; }
.w-mrow .v { font-weight:600; color:#bbb; }

.w-bar {
  height:9px;
  border-radius:5px;
  background:rgba(255,255,255,.06);
  overflow:hidden;
  position:relative;
}
.w-bar > i {
  display:block;
  height:100%;
  border-radius:5px;
}
.w-bar > i.bg {
  background:linear-gradient(90deg, var(--ca), color-mix(in srgb, var(--ca) 70%, #000));
}
.w-bar > i.by {
  background:linear-gradient(90deg, #f1c40f, #e67e22);
}
.w-bar > i.br {
  background:linear-gradient(90deg, #ef4444, #b91c1c);
}

.w-net {
  margin-top:10px;
  padding-top:10px;
  border-top:1px dashed rgba(255,255,255,.08);
  font-size:11px;
  color:#777;
  line-height:1.7;
}
.w-net-row {
  display:flex;
  justify-content:space-between;
}
.w-net-row .b { font-weight:600; color:#aaa; }

.w-foot {
  margin-top:10px;
  display:flex;
  justify-content:space-between;
  font-size:11px;
  color:#777;
}
.w-online  { color:#22c55e; font-weight:600; }
.w-offline { color:#ef4444; font-weight:600; }

.w-empty {
  grid-column:1/-1;
  text-align:center;
  padding:40px;
  color:#666;
  font-size:14px;
}

/* ══════════════════════════════════════════════════════
   Beszel 深色表格模式
   ══════════════════════════════════════════════════════ */
.beszel-mode { }

.b-header {
  display:flex;
  align-items:flex-end;
  justify-content:space-between;
  padding:0 0 14px;
}
.b-header h1 {
  font-size:22px;
  font-weight:600;
  margin:0;
}
.b-header p {
  font-size:12px;
  color:#777;
  margin-top:4px;
}
.b-controls input {
  padding:6px 12px;
  border-radius:7px;
  background:var(--bg-panel);
  border:1px solid var(--color-line);
  font-size:13px;
  color:var(--ct);
  outline:none;
}
.b-controls input::placeholder { color:#555; }
.b-controls input:focus { border-color:var(--ca); }

.b-table { padding:0; }
.b-row {
  display:grid;
  grid-template-columns:1.6fr .9fr 1.1fr 1.1fr .7fr .9fr auto;
  align-items:center;
  gap:14px;
  padding:10px 14px;
  border-radius:8px;
  font-size:13px;
  color:#ccc;
}
.b-row + .b-row { border-top:1px solid color-mix(in srgb, var(--color-line) 40%, transparent); }
.b-row:not(.b-head) { cursor:pointer; }
.b-row:not(.b-head):hover { background:rgba(255,255,255,.04); }

.b-row.b-head {
  font-size:12px;
  color:#666;
  padding-top:14px;
  text-transform:uppercase;
  letter-spacing:.5px;
  cursor:default;
}
.b-row.b-head:hover { background:transparent; }

.b-name {
  display:flex;
  align-items:center;
  gap:8px;
  color:var(--ct);
  font-weight:500;
}
.b-dot {
  width:7px;
  height:7px;
  border-radius:50%;
  display:inline-block;
}
.b-dot.on  { background:#22c55e; box-shadow:0 0 6px rgba(34,197,94,.6); }
.b-dot.off { background:#ef4444; }
.b-link { color:#555; font-size:12px; margin-left:4px; }

.b-cell {
  display:flex;
  align-items:center;
  gap:10px;
}
.b-cell .b-pct {
  text-align:right;
  min-width:38px;
  color:#bbb;
}
.b-meter {
  height:8px;
  border-radius:4px;
  background:rgba(255,255,255,.04);
  overflow:hidden;
  position:relative;
  flex:1;
}
.b-meter > i {
  display:block;
  height:100%;
  border-radius:4px;
}
.b-meter > i.bg {
  background:linear-gradient(90deg, var(--ca), color-mix(in srgb, var(--ca) 70%, #000));
}
.b-meter > i.by { background:linear-gradient(90deg, #eab308, #f59e0b); }
.b-meter > i.br { background:linear-gradient(90deg, #ef4444, #b91c1c); }

.b-net { color:#888; font-variant-numeric:tabular-nums; }
.b-agent {
  display:flex;
  align-items:center;
  gap:6px;
  font-size:12px;
  color:#bbb;
}
.b-adot {
  width:6px;
  height:6px;
  border-radius:50%;
  display:inline-block;
}
.b-adot.on  { background:#22c55e; }
.b-adot.off { background:#ef4444; }

.b-icons {
  display:flex;
  align-items:center;
  gap:10px;
  color:#666;
}
.b-icons span {
  cursor:pointer;
  transition:color .15s;
}
.b-icons span:hover { color:#aaa; }

.b-empty {
  text-align:center;
  padding:40px;
  color:#666;
  font-size:14px;
}

@media(max-width:900px){
  .b-row {
    grid-template-columns:1.4fr .8fr 1fr 1fr .6fr .7fr auto;
    font-size:12px;
  }
  .b-header { flex-direction:column; align-items:flex-start; gap:10px; }
}
</style>
