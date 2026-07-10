<template>
  <div class="field">
    <div class="flabel">
      {{ label }}<span v-if="hint" class="fhint">{{ hint }}</span>
    </div>
    <div class="fbody"><slot /></div>
  </div>
</template>

<script setup lang="ts">
defineProps<{ label: string; hint?: string }>();
</script>

<style scoped>
.field {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 10px;
}
.flabel {
  width: 110px;
  flex-shrink: 0;
  font-size: 13px;
  color: var(--text-muted);
  display: flex;
  flex-direction: column;
}
.fhint {
  font-size: 10px;
  opacity: 0.7;
}
.fbody {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 1;
  flex-wrap: wrap;
}
</style>
