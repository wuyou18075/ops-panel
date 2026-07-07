import type { Config } from "tailwindcss";

export default {
  content: ["./index.html", "./src/**/*.{vue,ts}"],
  theme: {
    extend: {
      colors: {
        canvas: "#0f1117",
        panel: "#171a22",
        line: "#2b3140",
      },
    },
  },
  plugins: [],
} satisfies Config;
