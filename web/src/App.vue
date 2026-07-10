<template>
  <NConfigProvider :theme="naiveTheme" :theme-overrides="overrides">
    <NMessageProvider>
      <NDialogProvider>
        <DashboardView />
      </NDialogProvider>
    </NMessageProvider>
  </NConfigProvider>
</template>

<script setup lang="ts">
import { computed, onMounted } from "vue";
import { NConfigProvider, NDialogProvider, NMessageProvider, darkTheme, lightTheme } from "naive-ui";
import DashboardView from "./views/dashboard/index.vue";
import { applyTheme, isLight, naiveOverrides, themeKey } from "./theme";

const naiveTheme = computed(() => (isLight(themeKey.value) ? lightTheme : darkTheme));
const overrides = computed(() => naiveOverrides(themeKey.value));

onMounted(() => applyTheme(themeKey.value));
</script>
