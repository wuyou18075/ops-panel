#!/bin/bash

set -euo pipefail

test -f web/package.json
test -f web/vite.config.ts
test -f web/tailwind.config.ts
test -f web/pnpm-workspace.yaml
test -f web/src/views/dashboard/index.vue

grep -q '"vue"' web/package.json
grep -q '"vite"' web/package.json
grep -q '"naive-ui"' web/package.json
grep -q '"tailwindcss"' web/package.json
grep -q "allowBuilds:" web/pnpm-workspace.yaml
grep -q "esbuild: true" web/pnpm-workspace.yaml
if grep -q '"onlyBuiltDependencies"' web/package.json; then
  echo "pnpm 11 ignores package.json pnpm.onlyBuiltDependencies; use pnpm-workspace.yaml allowBuilds" >&2
  exit 1
fi
grep -q "outDir: \"../master/dist\"" web/vite.config.ts
grep -q "NConfigProvider" web/src/App.vue
grep -q "NDataTable" web/src/views/dashboard/index.vue
grep -q "sendShellCommand" web/src/views/dashboard/index.vue
