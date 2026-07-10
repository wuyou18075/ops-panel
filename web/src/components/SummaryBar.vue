<template>
  <div class="summary">
    <div class="item">
      <div class="label">当前时间</div>
      <div class="value">{{ clock }}</div>
    </div>
    <div class="item">
      <div class="label">当前在线</div>
      <div class="value">{{ onlineCount }}<small>/ {{ totalCount }}</small></div>
    </div>
    <div class="item">
      <div class="label">点亮地区</div>
      <div class="value">{{ regionCount }}</div>
    </div>
    <div class="item">
      <div class="label">本月流量</div>
      <div class="value">{{ fmtBytes(trafficTotals.month) }} <small>今日 {{ fmtBytes(trafficTotals.today) }}</small></div>
    </div>
    <div class="item">
      <div class="label">网络速率</div>
      <div class="value rate">
        <span class="up">↑ {{ fmtRate(netTotals.up) }}</span>
        <span class="down">↓ {{ fmtRate(netTotals.down) }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { fmtBytes, fmtRate } from "../utils";
import { nowMs, onlineCount, totalCount, regionCount, trafficTotals, netTotals } from "../store";

const clock = computed(() => {
  const d = new Date(nowMs.value);
  const p = (n: number) => String(n).padStart(2, "0");
  return `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
});
</script>

<style scoped>
.summary {
  display: grid;
  grid-template-columns: repeat(5, 1fr);
  gap: 0;
  padding: 16px 24px;
  border-radius: 14px;
  background: var(--glass);
  backdrop-filter: blur(16px);
  border: 1px solid var(--glass-border);
  box-shadow: var(--shadow);
  margin-bottom: 14px;
}
.item {
  text-align: center;
  border-right: 1px solid var(--color-line);
}
.item:last-child {
  border-right: none;
}
.label {
  font-size: 12px;
  color: var(--text-muted);
  margin-bottom: 4px;
}
.value {
  font-size: 22px;
  font-weight: 700;
  color: var(--ct);
  font-variant-numeric: tabular-nums;
}
.value small {
  font-size: 13px;
  color: var(--text-muted);
  margin-left: 6px;
  font-weight: 500;
}
.value.rate {
  font-size: 14px;
}
.value .up {
  color: #27ae60;
}
.value .down {
  color: #e74c3c;
}
@media (max-width: 700px) {
  .summary {
    grid-template-columns: repeat(3, 1fr);
    gap: 12px 0;
  }
  .item:nth-child(3) {
    border-right: none;
  }
}
</style>
