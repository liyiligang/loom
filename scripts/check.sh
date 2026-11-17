#!/usr/bin/env sh
set -eu
cd "$(dirname "$0")/.."
install=false
case "${1-}" in
  '') ;;
  --install) install=true ;;
  *) printf '%s\n' 'Usage: sh scripts/check.sh [--install]' >&2; exit 1 ;;
esac
if [ "$#" -gt 1 ]; then
  printf '%s\n' 'Usage: sh scripts/check.sh [--install]' >&2
  exit 1
fi
if [ "$install" = true ] || ! node scripts/dependencies.mjs web; then npm --prefix web ci; fi
if [ "$install" = true ] || ! node scripts/dependencies.mjs bridge/claude; then npm --prefix bridge/claude ci; fi
node scripts/skills.mjs check
node --test scripts/tests/*.test.mjs
npm --prefix web test
npm --prefix web run build
npm --prefix bridge/claude test
npm --prefix bridge/claude run build
node scripts/go-format.mjs
go vet ./...
go test ./...
printf '%s\n' 'Repository checks passed.'
