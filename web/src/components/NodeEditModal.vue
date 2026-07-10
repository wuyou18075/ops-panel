<template>
  <NModal :show="show" @update:show="emit('update:show', $event)" preset="card" title="编辑节点" class="node-edit-modal" style="width: 680px; max-width: 94vw">
    <NScrollbar style="max-height: 66vh">
      <div class="form" v-if="node">
        <div class="hint">节点 ID：<code>{{ node.id }}</code></div>

        <div class="grp">基本</div>
        <Field label="备注名称"><NInput v-model:value="f.name" placeholder="名称" /></Field>
        <Field label="分组">
          <NSelect v-model:value="f.group" :options="groupOptions" />
        </Field>
        <Field label="收藏"><NSwitch v-model:value="f.favorite" /></Field>
        <Field label="国家代码" hint="ISO 两位，如 US/HK/JP（留空自动识别）">
          <NInput v-model:value="f.country_code" placeholder="自动" maxlength="2" style="width: 120px" />
          <span class="flag">{{ flag }}</span>
        </Field>

        <div class="grp">计费</div>
        <Field label="价格">
          <NInputNumber v-model:value="f.price_amount" :min="0" :precision="2" style="width: 130px" placeholder="金额" />
          <NSelect v-model:value="f.price_currency" :options="currencyOptions" style="width: 92px" />
          <NSelect v-model:value="f.billing_cycle" :options="cycleOptions" style="width: 110px" />
        </Field>
        <Field label="到期日" hint="格式 YYYY-MM-DD">
          <NInput v-model:value="f.expiry_date" placeholder="2026-07-15" style="width: 160px" />
          <span class="sub">{{ expiryLabel(f.expiry_date) }}</span>
        </Field>
        <Field label="计费标签" hint="如 主用/长租/玩具"><NInput v-model:value="f.label" placeholder="标签" style="width: 160px" /></Field>

        <div class="grp">流量</div>
        <Field label="月配额 (GB)" hint="0 = 不限">
          <NInputNumber v-model:value="quotaGB" :min="0" :precision="0" style="width: 130px" placeholder="0" />
        </Field>

        <div class="grp">采集</div>
        <Field label="刷新频率 (秒)"><NInputNumber v-model:value="f.interval" :min="1" :max="60" style="width: 110px" /></Field>
        <Field label="控制台"><NSwitch v-model:value="f.enable_console" /></Field>
        <Field label="流量监控"><NSwitch v-model:value="f.track_traffic" /></Field>
        <Field label="日报"><NSwitch v-model:value="f.daily_report" /></Field>

        <div class="grp">延迟探测</div>
        <div class="probe-list"><NCheckbox v-for="p in systemSettings.latency_templates" :key="p.id" :checked="f.latency_probe_ids?.includes(p.id)" @update:checked="toggleProbe(p.id,$event)"><b>{{p.name}}</b><small>{{p.target}}</small></NCheckbox></div>
      </div>
    </NScrollbar>
    <template #footer>
      <div class="footer">
        <NButton tertiary type="error" @click="doDelete">删除节点</NButton>
        <div style="flex: 1"></div>
        <NButton @click="emit('update:show', false)">取消</NButton>
        <NButton type="primary" :loading="saving" @click="save">保存</NButton>
      </div>
    </template>
  </NModal>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import {
  NButton,
	NCheckbox,
  NInput,
  NInputNumber,
  NModal,
  NScrollbar,
  NSelect,
  NSwitch,
  useDialog,
  useMessage,
} from "naive-ui";
import Field from "./Field.vue";
import { Api } from "../api";
import { groups, loadAgents, systemSettings } from "../store";
import type { AgentPreferences, NodeView } from "../types";
import { countryFlag, expiryLabel } from "../utils";

const props = defineProps<{ show: boolean; node: NodeView | null }>();
const emit = defineEmits<{ "update:show": [boolean]; saved: [] }>();
const message = useMessage();
const dialog = useDialog();

const GiB = 1073741824;
const f = reactive<Record<string, any>>({});
const quotaGB = ref<number | null>(0);
const saving = ref(false);

const groupOptions = computed(() => groups.value.map((g) => ({ label: g, value: g })));
const currencyOptions = ["$", "¥", "€", "£", "HK$", "₽"].map((v) => ({ label: v, value: v }));
const cycleOptions = ["月", "年", "季", "一次性", "免费"].map((v) => ({ label: v, value: v }));
const flag = computed(() => countryFlag(f.country_code));

watch(
  () => [props.show, props.node],
  () => {
    if (props.show && props.node) {
      const p = props.node.prefs;
      Object.assign(f, {
        name: props.node.name || "",
        group: p.group || "默认分组",
        favorite: !!p.favorite,
        country_code: p.country_code || "",
        price_amount: p.price_amount || 0,
        price_currency: p.price_currency || "$",
        billing_cycle: p.billing_cycle || "月",
        expiry_date: p.expiry_date || "",
        label: p.label || "",
        interval: p.interval || 2,
        enable_console: !!p.enable_console,
        track_traffic: p.track_traffic !== false,
        daily_report: !!p.daily_report,
        sort_order: p.sort_order || 0,
		latency_probe_ids: [...(p.latency_probe_ids || [])],
      });
      quotaGB.value = p.traffic_quota ? Math.round(p.traffic_quota / GiB) : 0;
    }
  },
  { immediate: true },
);

async function save() {
  if (!props.node) return;
  saving.value = true;
  try {
    const prefs: AgentPreferences = {
      enable_console: f.enable_console,
      group: f.group || "默认分组",
      track_traffic: f.track_traffic,
      daily_report: f.daily_report,
      interval: f.interval || 2,
      price_amount: f.price_amount || 0,
      price_currency: f.price_currency,
      billing_cycle: f.billing_cycle,
      expiry_date: (f.expiry_date || "").trim(),
      label: (f.label || "").trim(),
      traffic_quota: Math.round((quotaGB.value || 0) * GiB),
      country_code: (f.country_code || "").trim().toUpperCase(),
      favorite: f.favorite,
      sort_order: f.sort_order || 0,
	  latency_probe_ids: [...(f.latency_probe_ids || [])],
    };
    await Api.updateAgent(props.node.id, (f.name || "").trim(), prefs);
    await loadAgents();
    message.success("已保存");
    emit("saved");
    emit("update:show", false);
  } catch (e: any) {
    message.error(e?.message || "保存失败");
  } finally {
    saving.value = false;
  }
}
function toggleProbe(id:string,on:boolean){const s=new Set<string>(f.latency_probe_ids||[]);on?s.add(id):s.delete(id);f.latency_probe_ids=[...s]}

function doDelete() {
  if (!props.node) return;
  const id = props.node.id;
  const nm = props.node.name || id;
  dialog.warning({
    title: "删除节点",
    content: `确定删除「${nm}」? 该操作不可撤销。`,
    positiveText: "删除",
    negativeText: "取消",
    onPositiveClick: async () => {
      try {
        await Api.deleteAgent(id);
        await loadAgents();
        message.success("已删除");
        emit("saved");
        emit("update:show", false);
      } catch (e: any) {
        message.error(e?.message || "删除失败");
      }
    },
  });
}
</script>

<style scoped>
.form {
  padding: 8px 14px;
}
.hint {
  font-size: 12px;
  color: var(--text-muted);
  margin-bottom: 10px;
}
.hint code {
  font-family: ui-monospace, monospace;
}
.grp {
  font-size: 12px;
  font-weight: 700;
  color: var(--ca);
  margin: 14px 0 8px;
  letter-spacing: 0.5px;
}
.probe-list{display:grid;grid-template-columns:repeat(3,1fr);gap:10px}.probe-list :deep(.n-checkbox){padding:12px;border:1px solid var(--color-line);border-radius:12px;background:var(--glass)}.probe-list b,.probe-list small{display:block}.probe-list small{font-size:10px;color:var(--text-muted);margin-top:2px}
:deep(.field) {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 10px;
}
:deep(.flabel) {
  width: 110px;
  flex-shrink: 0;
  font-size: 13px;
  color: var(--text-muted);
  display: flex;
  flex-direction: column;
}
:deep(.fhint) {
  font-size: 10px;
  opacity: 0.7;
}
:deep(.fbody) {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 1;
  flex-wrap: wrap;
}
.flag {
  font-size: 20px;
}
.sub {
  font-size: 12px;
  color: var(--text-muted);
}
.footer {
  display: flex;
  gap: 10px;
  align-items: center;
}
</style>
