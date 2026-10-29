# 端到端 demo

`run.sh` 会：

1. 在 `.tmp/demo/repo` 构造一个两提交的临时 git 仓库（base 提交 = 网关骨架，
   head 提交 = 限流 + 日志 + 审计）；
2. 用 `review.json`（骨架，含 patch）跑 `fill`；
3. 对填充结果跑 `validate --json`；
4. 生成 `review.html` 并打印 `file://` 地址。

```bash
bash examples/demo/run.sh
```

产物：

```
.tmp/demo/out/review.filled.json    # canonical：lines + 行号
.tmp/demo/out/validate.json         # {"ok": true, "errors": []}
.tmp/demo/out/review.html           # 单文件离线评审页
```

## review.json 说明

- `meta.base_commit` 用 `HEAD~1`、`head_ref` 用 `HEAD`：脚本固定构造两个提交，
  因此这份骨架对任意克隆都成立，不需要手填 sha。
- 两条链演示链间隔离：
  - `chain-rate-limit`：3 个意图（限流 → 日志 → 配置），演示同一文件多处修改、
    跨文件修改、上一意图结果成为下一意图 context；
  - `chain-audit-log`：新增文件的独立需求。
- 每个 change 恰好一个 hunk；`business_explanation` 写“为什么”。

修改 `files/base`、`files/head` 或 `review.json` 后重跑脚本即可观察校验反馈
（例如把某个 hunk 的 context 改成旧版本，会得到 `CONTEXT_MISMATCH` / `PATCH_CONTEXT_MISMATCH`）。
