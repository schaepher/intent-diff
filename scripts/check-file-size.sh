#!/usr/bin/env bash
# 检查 Go 文件行数上限（默认 300 行，超限提示拆分）。
# 用法：
#   scripts/check-file-size.sh            # 检查全部 Go 文件
#   scripts/check-file-size.sh --staged   # 只检查暂存区新增/修改的 Go 文件
set -euo pipefail

LIMIT="${FILE_LINE_LIMIT:-300}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [ "${1:-}" = "--staged" ]; then
  files="$(git diff --cached --name-only --diff-filter=ACM -- '*.go' || true)"
else
  files="$(git ls-files '*.go' 2>/dev/null || find . -name '*.go' -not -path './.tmp/*' -not -path './.git/*')"
fi

fail=0
while IFS= read -r f; do
  [ -z "$f" ] && continue
  [ -f "$f" ] || continue
  n="$(wc -l < "$f" | tr -d ' ')"
  if [ "$n" -gt "$LIMIT" ]; then
    echo "文件过大: $f ($n 行 > $LIMIT) —— 请按职责拆分（dev-line-limit skill）"
    fail=1
  fi
done <<< "$files"

if [ "$fail" -ne 0 ]; then
  exit 1
fi
echo "check-file-size: ok (limit=$LIMIT)"
