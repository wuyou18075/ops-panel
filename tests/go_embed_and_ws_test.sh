#!/bin/bash

set -euo pipefail

grep -q '"embed"' master/main.go
grep -q "//go:embed dist/*" master/main.go
grep -q "frontendFiles embed.FS" master/main.go
grep -q "http.FileServer(http.FS" master/main.go
grep -q 'if msg.Type == "cmd"' master/main.go
grep -q "agentConn.WriteMessage" master/main.go
grep -q "broadcastToWeb(msgBytes)" master/main.go
test -f master/dist/index.html
