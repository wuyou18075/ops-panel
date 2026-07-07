<template>
  <NConfigProvider :theme="darkTheme" :theme-overrides="themeOverrides">
    <NMessageProvider>
      <DashboardView />
    </NMessageProvider>
  </NConfigProvider>
</template>

<script setup lang="ts">
import { NConfigProvider, NMessageProvider, darkTheme, type GlobalThemeOverrides } from "naive-ui";
import DashboardView from "./views/dashboard/index.vue";

const themeOverrides: GlobalThemeOverrides = {
  common: {
    primaryColor: "#18a058",
    primaryColorHover: "#36ad6a",
    bodyColor: "#0f1117",
    cardColor: "#171a22",
    modalColor: "#171a22",
    borderColor: "#2b3140",
    textColorBase: "#e5e7eb",
  },
  Card: {
    borderRadius: "8px",
  },
};
</script>
