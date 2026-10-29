#!/usr/bin/env bash
# 端到端 demo：构造一个有 base/head 两提交的临时仓库，
# 依次运行 fill → validate → render，产物在 .tmp/demo/out/。
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
WORK="$ROOT/.tmp/demo"
REPO="$WORK/repo"
OUT="$WORK/out"
BIN="$WORK/intent-review"

rm -rf "$WORK"
mkdir -p "$REPO" "$OUT"

go build -o "$BIN" "$ROOT/cmd/intent-review"

cp -R "$HERE/files/base/." "$REPO/"
git -C "$REPO" init -q -b main
git -C "$REPO" -c user.name=demo -c user.email=demo@example.com add -A
git -C "$REPO" -c user.name=demo -c user.email=demo@example.com commit -q -m "base: 网关骨架"

cp -R "$HERE/files/head/." "$REPO/"
git -C "$REPO" -c user.name=demo -c user.email=demo@example.com add -A
git -C "$REPO" -c user.name=demo -c user.email=demo@example.com commit -q -m "feat: 限流 + 日志 + 审计"

echo "== fill =="
"$BIN" fill "$HERE/review.json" --repo "$REPO" -o "$OUT/review.filled.json"

echo "== validate =="
"$BIN" validate "$OUT/review.filled.json" --repo "$REPO" --json | tee "$OUT/validate.json"

echo "== render =="
"$BIN" render "$OUT/review.filled.json" -o "$OUT/review.html" --repo "$REPO"

echo
echo "打开： file://$OUT/review.html"
