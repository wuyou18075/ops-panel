<template>
  <div class="app">
    <!-- ── 侧栏 ── -->
    <aside class="side" :class="{ open: sideOpen }">
      <div class="brand">
        <div class="logo">OP</div>
        <div class="bt">
          <div class="b1">Ops Panel</div>
          <div class="b2">VPS Monitor</div>
        </div>
      </div>
      <nav class="nav">
        <div
          v-for="p in navPages"
          :key="p.k"
          class="ni"
          :class="{ act: page === p.k }"
          @click="page = p.k; sideOpen = false"
        >
          <span class="nicon">{{ p.i }}</span>{{ p.l }}
        </div>
      </nav>
      <div class="groups">
        <div class="ghead">
          <span>分组</span>
        </div>
        <div class="gi" :class="{ act: selectedGroup === '全部' }" @click="selectedGroup = '全部'">全部节点</div>
        <div v-for="g in groups" :key="g" class="gi" :class="{ act: selectedGroup === g }" @click="selectedGroup = g">{{ g }}</div>
      </div>
    </aside>
    <div v-if="sideOpen" class="scrim" @click="sideOpen = false"></div>

    <!-- ── 主体 ── -->
    <main class="main">
      <header class="top">
        <button class="ham" @click="sideOpen = !sideOpen">☰</button>
        <div class="search">
          <span>🔍</span>
          <input v-model="filterText" placeholder="搜索节点 / 国家 / 标签…" />
        </div>
        <div class="spacer"></div>

        <!-- 视图切换（仅概览） -->
        <div v-if="page === 'dashboard'" class="seg">
          <button :class="{ on: viewMode === 'cards' }" @click="viewMode = 'cards'">卡片</button>
          <button :class="{ on: viewMode === 'table' }" @click="viewMode = 'table'">表格</button>
        </div>

        <!-- 主题下拉 -->
        <NSelect
          :value="themeKey"
          :options="THEMES.map((t) => ({ label: t.label, value: t.key }))"
          size="small"
          style="width: 120px"
          @update:value="applyTheme"
        />

        <NTag :type="wsConnected ? 'success' : 'error'" size="small" round>{{ wsConnected ? "已连接" : "重连中" }}</NTag>

        <template v-if="!publicMode">
          <NButton v-if="isOperator" size="small" type="success" ghost @click="doLogout">运维中 · 退出</NButton>
          <NButton v-else size="small" type="primary" @click="showLogin = true">登录</NButton>
        </template>
      </header>

      <div class="content">
        <!-- ══ 概览 ══ -->
        <template v-if="page === 'dashboard'">
          <SummaryBar />
          <div class="stats">
            <StatCard label="在线节点" :value="`${onlineCount}/${totalCount}`" tone="g" />
            <StatCard label="平均 CPU" :value="avgCpu.toFixed(1) + '%'" tone="b" />
            <StatCard label="平均内存" :value="avgMem.toFixed(1) + '%'" tone="v" />
            <StatCard label="告警节点" :value="String(alertCount)" tone="r" />
          </div>
          <div class="listwrap">
            <div class="lhead">
              <span class="ltitle">节点（{{ visibleNodes.length }}）</span>
              <div class="lhead-actions" v-if="!publicMode && isOperator">
                <NButton size="small" @click="showAddGroup = true">+ 添加分组</NButton>
                <NButton type="primary" size="small" @click="openEnroll">+ 添加节点</NButton>
              </div>
            </div>
            <CardMode v-if="viewMode === 'cards'" @open="openDetail" @fav="toggleFav" @edit="openEdit" />
            <TableMode v-else @open="openDetail" @edit="openEdit" />
          </div>
        </template>

        <!-- ══ 服务监控 ══ -->
        <MonitorsPage v-else-if="page === 'monitors'" />

        <!-- ══ 命令终端 ══ -->
        <template v-else-if="page === 'terminal' && !publicMode">
          <div class="termgrid">
            <div class="termnodes">
              <div class="tn-h">在线节点</div>
              <div
                v-for="n in consoleNodes"
                :key="n.id"
                class="tn"
                :class="{ act: activeNodeId === n.id }"
                @click="pickTermNode(n.id)"
              >
                <span class="dot" :class="n.online ? 'on' : 'off'"></span>{{ n.name || n.id }}
              </div>
            </div>
            <div class="termmain">
              <template v-if="isOperator && activeNodeId">
                <div class="cmdbar">
                  <NInputGroup>
                    <NInput v-model:value="shellCommand" placeholder="输入 Shell 命令" @keyup.enter="runCmd" />
                    <NButton type="primary" :disabled="!shellCommand.trim()" @click="runCmd">执行</NButton>
                  </NInputGroup>
                  <div class="qc">
                    <button v-for="c in quickCmds" :key="c" @click="shellCommand = c">{{ c }}</button>
                  </div>
                </div>
                <div class="termout">
                  <div v-if="terminalLogs.length === 0" class="tempty">等待命令…</div>
                  <div v-for="(l, i) in terminalLogs" :key="i">{{ l }}</div>
                </div>
              </template>
              <NEmpty v-else-if="!isOperator" description="请先登录" style="margin-top: 60px" />
              <NEmpty v-else description="请选择左侧节点" style="margin-top: 60px" />
            </div>
          </div>
        </template>

        <!-- ══ 告警策略 ══ -->
        <template v-else-if="page === 'alerts' && !publicMode">
          <div class="alertcard">
            <div class="ltitle" style="margin-bottom: 14px">告警策略</div>
            <div class="arow"><span>启用告警</span><NSwitch v-model:value="alertCfg.enabled" /></div>
            <div v-for="a in alertFields" :key="a.k" class="arow">
              <span>{{ a.l }}</span>
              <NInputNumber v-model:value="(alertCfg as any)[a.k]" :min="1" :max="a.k === 'offline_minutes' ? 60 : 100" style="width: 130px" />
              <span class="unit">{{ a.u }}</span>
            </div>
            <NButton type="primary" :loading="savingAlert" @click="saveAlerts" style="margin-top: 10px">保存</NButton>
          </div>
        </template>

        <!-- ══ 登录日志 ══ -->
        <LoginLogsPage v-else-if="page === 'loginlogs' && !publicMode" />
      </div>
    </main>

    <!-- ── 登录 ── -->
    <NModal v-model:show="showLogin" preset="card" title="运维登录" style="width: 380px; max-width: 94vw">
      <NSpace vertical>
        <NInput v-model:value="loginU" placeholder="用户名（默认 admin）" />
        <NInput v-model:value="loginP" type="password" placeholder="密码" @keyup.enter="doLogin" />
        <NInput v-model:value="loginCode" placeholder="6 位动态码（开启 2FA 时必填）" />
        <NButton type="primary" block :loading="loginLoading" @click="doLogin">登录</NButton>
      </NSpace>
    </NModal>

    <!-- ── 添加节点 ── -->
    <NModal v-model:show="showEnroll" preset="card" title="添加节点" style="width: 520px; max-width: 94vw">
      <NSpace vertical>
        <NInput v-model:value="enrollName" placeholder="备注名称" />
        <NSelect v-model:value="enrollGroup" :options="groupOptions" placeholder="分组" />
        <div class="erow">
          <NCheckbox v-model:checked="enrollConsole">控制台</NCheckbox>
          <NCheckbox v-model:checked="enrollTraffic">流量监控</NCheckbox>
          <NCheckbox v-model:checked="enrollReport">日报</NCheckbox>
        </div>
        <div class="erow"><span>刷新频率</span><NInputNumber v-model:value="enrollInterval" :min="1" :max="60" style="width: 100px" /><span class="unit">秒</span></div>
        <div class="erow">
          <span>月流量额度</span>
          <NInputNumber v-model:value="enrollQuota" :min="1" :precision="0" style="width: 150px" />
          <NSelect v-model:value="enrollQuotaUnit" :options="quotaUnitOptions" style="width: 92px" />
        </div>

        <div v-if="enrollCommand" class="cmdbox" @click="copyText(enrollCommand, $event)">
          <div class="cbh">在目标 VPS 上执行（点击复制）：</div>
          <div class="cbc">{{ enrollCommand }}</div>
        </div>
        <NButton v-if="!enrollCommand" type="primary" block :loading="enrolling" @click="doEnroll">生成安装命令</NButton>
        <NSpace v-else>
          <NButton type="primary" @click="copyText(enrollCommand, $event)">复制命令</NButton>
          <NButton @click="resetEnroll">完成</NButton>
        </NSpace>
      </NSpace>
    </NModal>

    <!-- ── 新建分组 ── -->
    <NModal v-model:show="showAddGroup" preset="card" title="新建分组" style="width: 340px; max-width: 94vw">
      <NSpace vertical>
        <NInput v-model:value="newGroup" placeholder="分组名称" @keyup.enter="doAddGroup" />
        <NButton type="primary" block :loading="addingGroup" @click="doAddGroup">创建</NButton>
      </NSpace>
    </NModal>

    <!-- ── 节点编辑 / 详情 ── -->
    <NodeEditModal v-model:show="showEdit" :node="editNode" @saved="onEdited" />
    <NodeDetailDrawer v-model:show="showDetail" :node-id="detailId" @edit="openEdit" @console="openConsole" />
  </div>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, ref } from "vue";
import {
  NButton,
  NCheckbox,
  NEmpty,
  NInput,
  NInputGroup,
  NInputNumber,
  NModal,
  NSelect,
  NSpace,
  NSwitch,
  NTag,
  useMessage,
} from "naive-ui";
import CardMode from "../../components/CardMode.vue";
import TableMode from "../../components/TableMode.vue";
import SummaryBar from "../../components/SummaryBar.vue";
import MonitorsPage from "../../components/MonitorsPage.vue";
import LoginLogsPage from "../../components/LoginLogsPage.vue";
import NodeEditModal from "../../components/NodeEditModal.vue";
import NodeDetailDrawer from "../../components/NodeDetailDrawer.vue";
import { Api } from "../../api";
import { copyToClipboard } from "../../clipboard";
import { applyTheme, THEMES, themeKey } from "../../theme";
import {
  activeNodeId,
  alertCfg,
  alertCount,
  avgCpu,
  avgMem,
  filterText,
  groups,
  isOperator,
  loadGroups,
  login,
  logout,
  nodeViews,
  onlineCount,
  publicMode,
  selectedGroup,
  sendCommand,
  startPolling,
  terminalLogs,
  totalCount,
  visibleNodes,
  wsConnected,
} from "../../store";
import type { NodeView, ViewMode } from "../../types";

const message = useMessage();
const page = ref("dashboard");
const viewMode = ref<ViewMode>("cards");
const sideOpen = ref(false);

const navPages = computed(() => {
  const base = [
    { k: "dashboard", l: "监控概览", i: "▤" },
    { k: "monitors", l: "服务监控", i: "◎" },
  ];
  if (!publicMode) {
    base.push({ k: "terminal", l: "命令终端", i: "⌘" });
    base.push({ k: "alerts", l: "告警策略", i: "!" });
    base.push({ k: "loginlogs", l: "登录日志", i: "≡" });
  }
  return base;
});

// 概览统计卡
const StatCard = defineComponent({
  props: { label: String, value: String, tone: String },
  setup(p) {
    const tone: Record<string, string> = { g: "#22c55e", b: "#3b82f6", v: "#8b5cf6", r: "#ef4444" };
    return () =>
      h("div", { class: "statcard" }, [
        h("div", { class: "sl" }, p.label),
        h("div", { class: "sv", style: { color: tone[p.tone || "g"] } }, p.value),
      ]);
  },
});

// ── 详情 / 编辑 ──
const showDetail = ref(false);
const detailId = ref("");
const showEdit = ref(false);
const editNode = ref<NodeView | null>(null);
function openDetail(id: string) {
  detailId.value = id;
  showDetail.value = true;
}
function openEdit(n: NodeView) {
  if (!isOperator.value) {
    message.warning("请先登录");
    return;
  }
  editNode.value = n;
  showEdit.value = true;
}
function onEdited() {
  /* store 已刷新 agents */
}
async function toggleFav(n: NodeView) {
  if (!isOperator.value) {
    message.warning("登录后可收藏");
    return;
  }
  try {
    await Api.updateAgent(n.id, n.name, { ...n.prefs, favorite: !n.prefs.favorite });
    n.prefs.favorite = !n.prefs.favorite; // 乐观更新
  } catch (e: any) {
    message.error(e?.message || "操作失败");
  }
}

// ── 登录 ──
const showLogin = ref(false);
const loginU = ref("");
const loginP = ref("");
const loginCode = ref("");
const loginLoading = ref(false);
async function doLogin() {
  if (!loginP.value) {
    message.error("请输入密码");
    return;
  }
  loginLoading.value = true;
  try {
    await login(loginU.value, loginP.value, loginCode.value);
    message.success("登录成功");
    showLogin.value = false;
    loginP.value = "";
    loginCode.value = "";
  } catch (e: any) {
    message.error(e?.message || "登录失败");
  } finally {
    loginLoading.value = false;
  }
}
function doLogout() {
  logout();
  message.info("已退出运维");
}

// ── 添加节点 ──
const showEnroll = ref(false);
const enrollName = ref("");
const enrollGroup = ref("");
const enrollConsole = ref(false);
const enrollTraffic = ref(true);
const enrollReport = ref(false);
const enrollInterval = ref(3);
const enrollQuota = ref(1000);
const enrollQuotaUnit = ref("GB");
const quotaUnitOptions = ["MB", "GB", "T"].map((v) => ({ label: v, value: v }));
const enrollCommand = ref("");
const enrolling = ref(false);
const groupOptions = computed(() => groups.value.map((g) => ({ label: g, value: g })));
const consoleNodes = computed(() => nodeViews.value.filter((n) => n.prefs.enable_console));
function openEnroll() {
  if (!isOperator.value) {
    message.warning("请先登录");
    showLogin.value = true;
    return;
  }
  showEnroll.value = true;
}
async function doEnroll() {
  if (!enrollName.value) {
    message.error("请输入名称");
    return;
  }
  enrolling.value = true;
  try {
    const d = await Api.enroll({
      name: enrollName.value,
      group: enrollGroup.value || "默认分组",
      enable_console: enrollConsole.value,
      track_traffic: enrollTraffic.value,
      daily_report: enrollReport.value,
      interval: enrollInterval.value,
	  traffic_quota: Math.round(enrollQuota.value * ({ MB: 1024 ** 2, GB: 1024 ** 3, T: 1024 ** 4 }[enrollQuotaUnit.value] || 1024 ** 3)),
    });
    enrollCommand.value = d.install_cmd;
    message.success("安装命令已生成，点击下方复制");
  } catch (e: any) {
    message.error(e?.message || "生成失败");
  } finally {
    enrolling.value = false;
  }
}
function resetEnroll() {
  showEnroll.value = false;
  enrollCommand.value = "";
  enrollName.value = "";
  enrollGroup.value = "";
	enrollQuota.value = 1000;
	enrollQuotaUnit.value = "GB";
}

// ── 复制：统一走 clipboard.ts；兜底把 textarea 挂进当前模态框内绕过焦点陷阱 ──
async function copyText(txt: string, ev?: Event) {
  if (!txt) return;
  const trigger = ev?.currentTarget as HTMLElement | undefined;
  const host =
    (trigger?.closest(".n-modal") as HTMLElement) ||
    (document.querySelector(".n-modal") as HTMLElement) ||
    undefined;
  const ok = await copyToClipboard(txt, host);
  if (ok) message.success("已复制到剪贴板");
  else message.error("复制失败，请手动选择文本复制");
}

// ── 分组 ──
const showAddGroup = ref(false);
const newGroup = ref("");
const addingGroup = ref(false);
async function doAddGroup() {
  if (!newGroup.value) return;
  addingGroup.value = true;
  try {
    await Api.addGroup(newGroup.value);
    await loadGroups();
    showAddGroup.value = false;
    newGroup.value = "";
    message.success("已创建");
  } catch (e: any) {
    message.error(e?.message || "创建失败");
  } finally {
    addingGroup.value = false;
  }
}

// ── 终端 ──
const shellCommand = ref("df -h");
const quickCmds = ["df -h", "free -m", "uptime", "docker ps", "systemctl --failed"];
function pickTermNode(id: string) {
  activeNodeId.value = id;
  terminalLogs.value = [];
}
function openConsole(n: NodeView) {
	showDetail.value = false;
	page.value = "terminal";
	pickTermNode(n.id);
}
function runCmd() {
  const cmd = shellCommand.value.trim();
  if (!cmd || !activeNodeId.value) return;
  if (!sendCommand(activeNodeId.value, cmd)) {
    message.error("运维通道未就绪");
    return;
  }
  terminalLogs.value.push(`$ ${cmd}`);
}

// ── 告警 ──
const savingAlert = ref(false);
const alertFields = [
  { k: "cpu_percent", l: "CPU 阈值", u: "%" },
  { k: "mem_percent", l: "内存阈值", u: "%" },
  { k: "disk_percent", l: "磁盘阈值", u: "%" },
  { k: "offline_minutes", l: "离线告警", u: "分钟" },
];
async function saveAlerts() {
  savingAlert.value = true;
  try {
    await Api.saveAlerts(alertCfg.value);
    message.success("已保存");
  } catch (e: any) {
    message.error(e?.message || "保存失败");
  } finally {
    savingAlert.value = false;
  }
}

onMounted(() => {
  applyTheme(themeKey.value);
  startPolling();
});
</script>

<style scoped>
.app {
  display: flex;
  min-height: 100vh;
}

/* 侧栏 */
.side {
  width: 220px;
  flex-shrink: 0;
  background: var(--bg-panel);
  border-right: 1px solid var(--color-line);
  display: flex;
  flex-direction: column;
  position: sticky;
  top: 0;
  height: 100vh;
}
.brand {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 18px 20px;
  border-bottom: 1px solid var(--color-line);
}
.logo {
  width: 38px;
  height: 38px;
  border-radius: 9px;
  display: grid;
  place-items: center;
  font-weight: 800;
  color: #fff;
  background: linear-gradient(135deg, var(--ca), color-mix(in srgb, var(--ca) 55%, #000));
}
.b1 {
  font-weight: 700;
  color: var(--ct);
  font-size: 15px;
}
.b2 {
  font-size: 11px;
  color: var(--text-muted);
}
.nav {
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.ni {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 12px;
  border-radius: 9px;
  font-size: 14px;
  cursor: pointer;
  color: var(--text-muted);
}
.ni:hover {
  background: color-mix(in srgb, var(--text) 6%, transparent);
  color: var(--text);
}
.ni.act {
  background: color-mix(in srgb, var(--ca) 15%, transparent);
  color: var(--ca);
  font-weight: 600;
}
.nicon {
  width: 18px;
  text-align: center;
}
.groups {
  padding: 12px;
  border-top: 1px solid var(--color-line);
  margin-top: auto;
}
.ghead {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 12px;
  color: var(--text-muted);
  margin-bottom: 8px;
}
.gi {
  padding: 6px 10px;
  border-radius: 7px;
  font-size: 13px;
  cursor: pointer;
  color: var(--text-muted);
  margin-bottom: 2px;
}
.gi:hover {
  color: var(--text);
}
.gi.act {
  background: color-mix(in srgb, var(--ca) 15%, transparent);
  color: var(--ca);
}

/* 主体 */
.main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}
.top {
  position: sticky;
  top: 0;
  z-index: 20;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 20px;
  background: color-mix(in srgb, var(--bg-panel) 90%, transparent);
  backdrop-filter: blur(10px);
  border-bottom: 1px solid var(--color-line);
}
.ham {
  display: none;
  border: none;
  background: transparent;
  color: var(--text);
  font-size: 20px;
  cursor: pointer;
}
.search {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 7px 12px;
  border-radius: 9px;
  background: var(--bar-track);
  flex: 1;
  max-width: 420px;
}
.search input {
  background: transparent;
  border: none;
  outline: none;
  color: var(--text);
  font-size: 13px;
  width: 100%;
}
.spacer {
  flex: 1;
}
.seg {
  display: flex;
  gap: 2px;
  padding: 3px;
  border-radius: 9px;
  background: var(--bar-track);
}
.seg button {
  border: none;
  background: transparent;
  color: var(--text-muted);
  padding: 5px 12px;
  border-radius: 7px;
  font-size: 13px;
  cursor: pointer;
}
.seg button.on {
  background: var(--ca);
  color: #fff;
  font-weight: 600;
}
.content {
  padding: 20px;
  flex: 1;
}

/* 概览统计卡 */
.stats {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 14px;
  margin-bottom: 16px;
}
:deep(.statcard) {
  padding: 16px 18px;
  border-radius: 14px;
  background: var(--glass);
  border: 1px solid var(--glass-border);
  box-shadow: var(--shadow);
  backdrop-filter: blur(14px);
}
:deep(.statcard .sl) {
  font-size: 13px;
  color: var(--text-muted);
}
:deep(.statcard .sv) {
  font-size: 28px;
  font-weight: 700;
  margin-top: 8px;
  font-variant-numeric: tabular-nums;
}
@media (max-width: 700px) {
  .stats {
    grid-template-columns: repeat(2, 1fr);
  }
}

.listwrap {
  margin-top: 4px;
}
.lhead {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}
.lhead-actions {
  display: flex;
  gap: 8px;
}
.ltitle {
  font-size: 17px;
  font-weight: 700;
  color: var(--ct);
}

/* 终端 */
.termgrid {
  display: grid;
  grid-template-columns: 240px 1fr;
  gap: 14px;
}
.termnodes,
.termmain {
  background: var(--glass);
  border: 1px solid var(--glass-border);
  border-radius: 14px;
  padding: 12px;
}
.tn-h {
  font-size: 12px;
  color: var(--text-muted);
  margin-bottom: 8px;
}
.tn {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  border-radius: 8px;
  font-size: 13px;
  cursor: pointer;
  color: var(--text-muted);
}
.tn:hover {
  background: color-mix(in srgb, var(--text) 6%, transparent);
}
.tn.act {
  background: color-mix(in srgb, var(--ca) 16%, transparent);
  color: var(--ca);
}
.dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
}
.dot.on {
  background: #22c55e;
}
.dot.off {
  background: #ef4444;
}
.cmdbar {
  margin-bottom: 12px;
}
.qc {
  display: flex;
  gap: 6px;
  margin-top: 8px;
  flex-wrap: wrap;
}
.qc button {
  border: 1px solid var(--color-line);
  background: transparent;
  color: var(--text-muted);
  padding: 3px 10px;
  border-radius: 7px;
  font-size: 12px;
  cursor: pointer;
}
.qc button:hover {
  color: var(--text);
  border-color: var(--ca);
}
.termout {
  height: 440px;
  overflow-y: auto;
  background: #0a0d12;
  border-radius: 10px;
  padding: 14px;
  font-family: ui-monospace, Menlo, monospace;
  font-size: 12px;
  line-height: 1.7;
  color: #6ee7a8;
}
.tempty {
  color: #556;
}

/* 告警 */
.alertcard {
  max-width: 460px;
  background: var(--glass);
  border: 1px solid var(--glass-border);
  border-radius: 14px;
  padding: 20px;
}
.arow {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}
.arow > span:first-child {
  width: 90px;
  color: var(--text-muted);
  font-size: 14px;
}
.unit {
  color: var(--text-muted);
  font-size: 12px;
}

/* 添加节点命令框 */
.erow {
  display: flex;
  align-items: center;
  gap: 12px;
}
.cmdbox {
  background: #0a0d12;
  border-radius: 10px;
  padding: 12px;
  cursor: pointer;
  border: 1px solid var(--color-line);
}
.cbh {
  font-size: 12px;
  color: var(--text-muted);
  margin-bottom: 6px;
}
.cbc {
  font-family: ui-monospace, monospace;
  font-size: 12px;
  color: #6ee7a8;
  word-break: break-all;
  line-height: 1.6;
}

.scrim {
  display: none;
}

/* 响应式 */
@media (max-width: 900px) {
  .side {
    position: fixed;
    left: 0;
    top: 0;
    z-index: 50;
    transform: translateX(-100%);
    transition: transform 0.2s;
  }
  .side.open {
    transform: translateX(0);
  }
  .scrim {
    display: block;
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.4);
    z-index: 40;
  }
  .ham {
    display: block;
  }
  .termgrid {
    grid-template-columns: 1fr;
  }
}
</style>
