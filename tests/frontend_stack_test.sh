#!/bin/bash

set -euo pipefail

test -f web/package.json
test -f web/vite.config.ts
test -f web/tailwind.config.ts
test -f web/src/views/dashboard/index.vue

grep -q '"vue"' web/package.json
grep -q '"vite"' web/package.json
grep -q '"naive-ui"' web/package.json
grep -q '"tailwindcss"' web/package.json
grep -q '"onlyBuiltDependencies"' web/package.json
grep -q '"esbuild"' web/package.json
grep -q "outDir: \"../master/dist\"" web/vite.config.ts
grep -q "NConfigProvider" web/src/App.vue
grep -q "NDataTable" web/src/views/dashboard/index.vue
grep -q "sendShellCommand" web/src/views/dashboard/index.vue
