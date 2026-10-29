#!/usr/bin/env bash
# 提交前验证：build + vet + gofmt + 全量单测。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

echo "== go build =="
go build ./...

echo "== go vet =="
go vet ./...

echo "== gofmt =="
out="$(gofmt -l cmd internal)"
if [ -n "$out" ]; then
  echo "以下文件需要 gofmt -w:"
  echo "$out"
  exit 1
fi

echo "== go test =="
go test ./...

echo "verify: ok"
