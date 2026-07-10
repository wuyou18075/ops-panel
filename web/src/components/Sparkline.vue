<template>
  <svg
    class="spark"
    :viewBox="`0 0 100 ${height}`"
    preserveAspectRatio="none"
    :style="{ height: height + 'px', width: '100%' }"
  >
    <defs>
      <linearGradient :id="gid" x1="0" y1="0" x2="0" y2="1">
        <stop offset="0%" :stop-color="color" stop-opacity="0.35" />
        <stop offset="100%" :stop-color="color" stop-opacity="0" />
      </linearGradient>
    </defs>
    <path v-if="fill && area" :d="area" :fill="`url(#${gid})`" stroke="none" />
    <path v-if="line" :d="line" fill="none" :stroke="color" stroke-width="1.6" vector-effect="non-scaling-stroke" stroke-linejoin="round" />
    <text v-if="!values.length" x="50" :y="height / 2 + 3" text-anchor="middle" font-size="7" fill="var(--text-muted)">暂无数据</text>
  </svg>
</template>

<script setup lang="ts">
import { computed } from "vue";

const props = withDefaults(
  defineProps<{
    values: number[];
    color?: string;
    height?: number;
    max?: number;
    fill?: boolean;
  }>(),
  { color: "var(--ca)", height: 40, fill: true },
);

const gid = "spk" + Math.random().toString(36).slice(2, 8);

const pts = computed(() => {
  const v = props.values;
  if (!v.length) return [] as { x: number; y: number }[];
  const mx = props.max ?? Math.max(1, ...v);
  const h = props.height;
  const n = v.length;
  return v.map((val, i) => ({
    x: n === 1 ? 100 : (i / (n - 1)) * 100,
    y: h - Math.max(0, Math.min(1, val / mx)) * (h - 2) - 1,
  }));
});

const line = computed(() => {
  const p = pts.value;
  if (!p.length) return "";
  return p.map((q, i) => `${i === 0 ? "M" : "L"}${q.x.toFixed(2)} ${q.y.toFixed(2)}`).join(" ");
});

const area = computed(() => {
  const p = pts.value;
  if (!p.length) return "";
  const h = props.height;
  return (
    `M${p[0].x.toFixed(2)} ${h} ` +
    p.map((q) => `L${q.x.toFixed(2)} ${q.y.toFixed(2)}`).join(" ") +
    ` L${p[p.length - 1].x.toFixed(2)} ${h} Z`
  );
});
</script>

<style scoped>
.spark {
  display: block;
  overflow: visible;
}
</style>
