<template>
  <div>
    <div class="fixed right-4 top-2 z-50 flex gap-1">
      <div v-for="t in themeList" :key="t.k" class="h-6 w-6 cursor-pointer rounded-full border-2 transition-all" :class="theme===t.k?'border-white scale-110':'border-transparent opacity-60 hover:opacity-100'" :style="{background:t.c}" :title="t.l" @click="setTheme(t.k)" />
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
            <div class="mt-5">
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
import { computed, h, onMounted, onUnmounted, ref } from "vue";
import { NButton, NCard, NCheckbox, NDataTable, NEmpty, NGi, NGrid, NInput, NInputGroup, NInputNumber, NLayout, NLayoutContent, NLayoutHeader, NLayoutSider, NModal, NProgress, NSelect, NSpace, NSwitch, NTag, useMessage, type DataTableColumns, type SelectOption } from "naive-ui";

type T = "midnight"|"forest"|"ocean"|"sunset"|"aurora";
const themeList = [
  { k:"midnight", l:"极夜黑", c:"#1a1a2e" },
  { k:"forest",  l:"护眼绿", c:"#1a3a2a" },
  { k:"ocean",   l:"海洋蓝", c:"#0a2440" },
  { k:"sunset",  l:"落日橙", c:"#2a1a10" },
  { k:"aurora",  l:"极光紫", c:"#1a1030" },
];
const themeMap: Record<T,any> = {
  midnight: { body:"#0f1117", panel:"#171a22", line:"#2b3140", ca:"#18a058", car:"24,160,88", ct:"#e5e7eb" },
  forest:   { body:"#0d1f14", panel:"#152a1c", line:"#2a4535", ca:"#34d399", car:"52,211,153", ct:"#d1fae5" },
  ocean:    { body:"#0a1628", panel:"#0f2240", line:"#1a3a60", ca:"#38bdf8", car:"56,189,248", ct:"#e0f2fe" },
  sunset:   { body:"#1a0e08", panel:"#2a1a10", line:"#4a3020", ca:"#fb923c", car:"251,146,60", ct:"#fff7ed" },
  aurora:   { body:"#120820", panel:"#1c0e30", line:"#302050", ca:"#a78bfa", car:"167,139,250", ct:"#f5f3ff" },
};
const theme = ref<T>("midnight");
const themeStyle = ref({});
const pages = [{k:"dashboard",l:"监控概览",s:"实时监控VPS状态"},{k:"nodes",l:"节点列表",s:"管理全部节点"},{k:"terminal",l:"命令终端",s:"远程命令执行"},{k:"alerts",l:"告警策略",s:"设置触发条件"}];
const pageTitle = computed(() => pages.find(p=>p.k===activePage.value)?.l||"");
function setTheme(k:T) { theme.value=k; const m=themeMap[k]; themeStyle.value={"--bg-body":m.body,"--bg-panel":m.panel,"--color-line":m.line,"--ca":m.ca,"--ca-rgb":m.car,"--ct":m.ct}; document.documentElement.style.background=m.body; }

const MC = defineComponent({
  props:{label:String,v:String,tone:String},
  setup(p) {
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
function connectViewerWS(force=false){if(force&&viewerWS)viewerWS.close();clearTimeout(reconnectTimer);viewerWS=new WebSocket(wsUrl("/ws/web"));viewerWS.onopen=()=>{wsConnected.value=true;};viewerWS.onmessage=(e)=>{let m;try{m=JSON.parse(e.data)}catch{return};if(m.type==="stat"){try{const p=JSON.parse(m.data);nodes.value[m.agent_id]={...nodes.value[m.agent_id],cpu:Number(p.cpu??0),mem:Number(p.mem??0),disk:Number(p.disk??0),net_recv:Number(p.net_recv??0),net_sent:Number(p.net_sent??0),updatedAt:Date.now()}}catch{}}};viewerWS.onclose=()=>{wsConnected.value=false;reconnectTimer=window.setTimeout(()=>connectViewerWS(),3000);};}
function connectOperatorWS(token:string){operatorWS?.close();operatorWS=new WebSocket(wsUrl("/ws/operator?token="+encodeURIComponent(token)));operatorWS.onclose=()=>{operatorToken.value="";operatorWS=null;};}
async function doLogin(){if(!loginP.value){message.error("请输入密码");return}loginLoading.value=true;try{const r=await fetch(api("/api/login"),{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({username:loginU.value||"admin",password:loginP.value,code:loginTOTP.value})});if(!r.ok){message.error(await r.text()||"登录失败");return}const d=await r.json();operatorToken.value=d.access_token;refreshToken.value=d.refresh_token;connectOperatorWS(d.access_token);message.success("登录成功");showLogin.value=false;loginP.value="";loginTOTP.value="";setTimeout(async()=>{try{const r=await fetch(api("/api/refresh"),{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({refresh_token:refreshToken.value})});if(r.ok){const d=await r.json();operatorToken.value=d.access_token;connectOperatorWS(d.access_token)}else{operatorToken.value="";refreshToken.value=""}}catch{}},13*60*1000)}catch{message.error("网络失败")}finally{loginLoading.value=false}}
async function doEnroll(){if(!enrollName.value){message.error("请输入名称");return}enrolling.value=true;try{const r=await fetch(api("/api/enroll"),{method:"POST",headers:{"Content-Type":"application/json","Authorization":"Bearer "+operatorToken.value},body:JSON.stringify({name:enrollName.value,group:enrollGroup.value||"默认分组",enable_console:enrollConsole.value,track_traffic:enrollTraffic.value,daily_report:enrollReport.value,interval:enrollInterval.value})});if(!r.ok){message.error(await r.text());return}const d=await r.json();enrollResult.value=d;enrollCommand.value=d.install_cmd;copyCommand()}catch{message.error("注册失败")}finally{enrolling.value=false}}
function copyCommand(){const txt=enrollCommand.value;if(!txt)return;(navigator.clipboard?.writeText(txt).catch(()=>fallbackCopy(txt))||fallbackCopy(txt))}
function fallbackCopy(txt){const ta=document.createElement("textarea");ta.value=txt;ta.style.position="fixed";ta.style.opacity="0";document.body.appendChild(ta);ta.select();document.execCommand("copy");document.body.removeChild(ta);message.success("已复制")}
function resetEnroll(){showEnroll.value=false;enrollCommand.value="";enrollResult.value=null;enrollName.value="";enrollGroup.value="";}
async function doAddGroup(){if(!newGroupName.value)return;addingGroup.value=true;try{const r=await fetch(api("/api/groups"),{method:"POST",headers:{"Content-Type":"application/json","Authorization":"Bearer "+operatorToken.value},body:JSON.stringify({name:newGroupName.value})});if(!r.ok){message.error(await r.text());return}groups.value=[...groups.value,newGroupName.value];showAddGroup.value=false;newGroupName.value="";message.success("已创建")}catch{}finally{addingGroup.value=false}}
async function saveAlertConfig(){savingAlert.value=true;try{const r=await fetch(api("/api/alerts"),{method:"POST",headers:{"Content-Type":"application/json","Authorization":"Bearer "+operatorToken.value},body:JSON.stringify(alertCfg.value)});r.ok?message.success("已保存"):message.error(await r.text())}catch{}finally{savingAlert.value=false}}
function sendShellCommand(){if(!canSend.value||!operatorWS||operatorWS.readyState!==WebSocket.OPEN){message.error("未就绪");return}const cmd=shellCommand.value.trim();terminalLogs.value.push(`$ ${cmd}`);operatorWS.send(JSON.stringify({type:"cmd",agent_id:activeNodeId.value,data:cmd}))}
async function loadGroups(){try{const r=await fetch(api("/api/groups"));if(r.ok)groups.value=await r.json()}catch{}}
async function loadAlertConfig(){try{const r=await fetch(api("/api/alerts"));if(r.ok)alertCfg.value=await r.json()}catch{}}
onMounted(()=>{setTheme("midnight");connectViewerWS();loadNames();loadGroups();loadAlertConfig();clockTimer=window.setInterval(()=>{nowMs.value=Date.now()},1000)});
onUnmounted(()=>{clearTimeout(reconnectTimer);clearInterval(clockTimer);viewerWS?.close();operatorWS?.close()});
</script>
