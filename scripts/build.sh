#!/usr/bin/env sh
set -eu
cd "$(dirname "$0")/.."
npm --prefix web ci
npm --prefix web run build
go build -o bin/loom ./cmd/loom
printf '%s\n' 'Built bin/loom. Run it to open the workspace at http://127.0.0.1:7331.'
